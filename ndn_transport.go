package raft

import (
	"fmt"
	"net"
	"time"

	"github.com/go-ndn/mux"
	"github.com/go-ndn/ndn"
	"github.com/go-ndn/tlv"
)

type ndnLogEntry struct {
	Term  uint64 `tlv:"128"`
	Value []byte `tlv:"129"`
}

type ndnAppendRequest struct {
	Name string `tlv:"131"`

	Term         uint64 `tlv:"128"`
	PrevLogTerm  uint64 `tlv:"132"`
	PrevLogIndex uint64 `tlv:"133"`
	CommitIndex  uint64 `tlv:"134"`

	LogCount uint64 `tlv:"135"`
}

type ndnAppendResponse struct {
	Term    uint64 `tlv:"128"`
	Success bool   `tlv:"136"`
}

type ndnVoteRequest struct {
	Name string `tlv:"131"`

	Term         uint64 `tlv:"128"`
	LastLogTerm  uint64 `tlv:"137"`
	LastLogIndex uint64 `tlv:"138"`
}

type ndnVoteResponse struct {
	Term    uint64 `tlv:"128"`
	Success bool   `tlv:"136"`
}

type ndnRedirectRequest struct {
	Input [][]byte `tlv:"139"`
}

type ndnRedirectResponse struct {
	Leader  string `tlv:"140"`
	Success bool   `tlv:"136"`
}

type ndnTransport struct {
	Name string
	*mux.Publisher
	*mux.Fetcher
	ndn.Face

	append   chan *AppendRequest
	vote     chan *VoteRequest
	redirect chan *RedirectRequest
}

func now() uint64 {
	return uint64(time.Now().UTC().UnixNano() / 1000000)
}

// NewNDNTransport creates a new transport that uses NDN packets.
func NewNDNTransport(name string, conn net.Conn, key ndn.Key) Transport {
	name = "/" + name

	cache := ndn.NewCache(65536)

	recv := make(chan *ndn.Interest)
	t := &ndnTransport{
		Name:      name,
		Face:      ndn.NewFace(conn, recv),
		Publisher: mux.NewPublisher(cache),
		Fetcher:   mux.NewFetcher(),

		append:   make(chan *AppendRequest),
		vote:     make(chan *VoteRequest),
		redirect: make(chan *RedirectRequest),
	}

	// verify checksum
	t.Fetcher.Use(mux.ChecksumVerifier)
	// assemble chunks
	t.Fetcher.Use(mux.Assembler)

	// segment
	t.Publisher.Use(mux.Segmentor(4096))

	m := mux.New()
	m.Use(mux.RawCacher(cache, false))

	// served from cache
	m.HandleFunc(name+"/log", func(w ndn.Sender, i *ndn.Interest) {})
	m.HandleFunc(name+"/append", func(w ndn.Sender, i *ndn.Interest) {})
	m.HandleFunc(name+"/vote", func(w ndn.Sender, i *ndn.Interest) {})
	m.HandleFunc(name+"/redirect", func(w ndn.Sender, i *ndn.Interest) {})

	m.Handle(mux.Listener(name+"/listen/append", func(locator string, w ndn.Sender, i *ndn.Interest) {
		var req ndnAppendRequest
		err := tlv.Unmarshal(
			t.Fetch(w, &ndn.Interest{
				Name: ndn.NewName(locator),
			}),
			&req, 101,
		)
		if err != nil {
			return
		}

		// fetch leader log entries
		log := make([]LogEntry, req.LogCount)
		for i := req.PrevLogIndex + 1; i <= req.PrevLogIndex+req.LogCount; i++ {
			var entry ndnLogEntry
			err := tlv.Unmarshal(
				t.Fetch(w, &ndn.Interest{
					Name: ndn.NewName(fmt.Sprintf("/%s/log/%d", req.Name, i)),
				}),
				&entry, 101,
			)
			if err != nil {
				return
			}
			log[i-req.PrevLogIndex-1] = LogEntry{
				Term:  entry.Term,
				Value: entry.Value,
			}
		}

		ch := make(chan *AppendResponse)
		t.append <- &AppendRequest{
			Name: req.Name,

			Term:         req.Term,
			PrevLogTerm:  req.PrevLogTerm,
			PrevLogIndex: req.PrevLogIndex,
			CommitIndex:  req.CommitIndex,
			Log:          log,

			Response: ch,
		}

		resp := <-ch
		b, err := tlv.Marshal(&ndnAppendResponse{
			Term:    resp.Term,
			Success: resp.Success,
		}, 101)
		if err != nil {
			return
		}
		w.SendData(&ndn.Data{
			Name:    i.Name,
			Content: b,
		})
	}))

	m.Handle(mux.Listener(name+"/listen/vote", func(locator string, w ndn.Sender, i *ndn.Interest) {
		var req ndnVoteRequest
		err := tlv.Unmarshal(
			t.Fetch(w, &ndn.Interest{
				Name: ndn.NewName(locator),
			}),
			&req, 101,
		)
		if err != nil {
			return
		}

		ch := make(chan *VoteResponse)
		t.vote <- &VoteRequest{
			Name: req.Name,

			Term:         req.Term,
			LastLogTerm:  req.LastLogTerm,
			LastLogIndex: req.LastLogIndex,

			Response: ch,
		}

		resp := <-ch
		b, err := tlv.Marshal(&ndnVoteResponse{
			Term:    resp.Term,
			Success: resp.Success,
		}, 101)
		if err != nil {
			return
		}
		w.SendData(&ndn.Data{
			Name:    i.Name,
			Content: b,
		})
	}))

	m.Handle(mux.Listener(name+"/listen/redirect", func(locator string, w ndn.Sender, i *ndn.Interest) {
		var req ndnRedirectRequest
		err := tlv.Unmarshal(
			t.Fetch(w, &ndn.Interest{
				Name: ndn.NewName(locator),
			}),
			&req, 101,
		)
		if err != nil {
			return
		}

		ch := make(chan *RedirectResponse)
		t.redirect <- &RedirectRequest{
			Input: req.Input,

			Response: ch,
		}

		resp := <-ch
		b, err := tlv.Marshal(&ndnRedirectResponse{
			Leader:  resp.Leader,
			Success: resp.Success,
		}, 101)
		if err != nil {
			return
		}
		w.SendData(&ndn.Data{
			Name:    i.Name,
			Content: b,
		})
	}))

	m.Register(t, key)

	go func() {
		for i := range recv {
			m.ServeNDN(t, i)
		}
	}()

	return t
}

func (t *ndnTransport) AcceptAppend() <-chan *AppendRequest {
	return t.append
}

func (t *ndnTransport) AcceptVote() <-chan *VoteRequest {
	return t.vote
}

func (t *ndnTransport) AcceptRedirect() <-chan *RedirectRequest {
	return t.redirect
}

func (t *ndnTransport) RequestAppend(peer string, req *AppendRequest) *AppendResponse {
	// publish leader log entry
	for i, entry := range req.Log {
		entryName := ndn.NewName(fmt.Sprintf("/%s/log/%d", t.Name, req.PrevLogIndex+uint64(i)+1))
		if t.Publisher.Get(&ndn.Interest{Name: entryName}) != nil {
			// already published
			continue
		}

		b, err := tlv.Marshal(&ndnLogEntry{
			Term:  entry.Term,
			Value: entry.Value,
		}, 101)
		if err != nil {
			return &AppendResponse{}
		}
		t.Publish(&ndn.Data{
			Name:    entryName,
			Content: b,
		})
	}

	b, err := tlv.Marshal(&ndnAppendRequest{
		Name:         req.Name,
		Term:         req.Term,
		PrevLogTerm:  req.PrevLogTerm,
		PrevLogIndex: req.PrevLogIndex,
		CommitIndex:  req.CommitIndex,
		LogCount:     uint64(len(req.Log)),
	}, 101)
	if err != nil {
		return &AppendResponse{}
	}

	reqName := fmt.Sprintf("%s/append/%d", t.Name, now())
	t.Publish(&ndn.Data{
		Name:    ndn.NewName(reqName),
		Content: b,
	})

	var resp ndnAppendResponse
	err = tlv.Unmarshal(t.Fetch(t, &ndn.Interest{
		Name: mux.Notify(fmt.Sprintf("/%s/listen/append", peer), reqName),
	}), &resp, 101)
	if err != nil {
		return &AppendResponse{}
	}
	return &AppendResponse{
		Term:    resp.Term,
		Success: resp.Success,
	}
}

func (t *ndnTransport) RequestVote(peer string, req *VoteRequest) *VoteResponse {
	b, err := tlv.Marshal(&ndnVoteRequest{
		Name:         req.Name,
		Term:         req.Term,
		LastLogTerm:  req.LastLogTerm,
		LastLogIndex: req.LastLogIndex,
	}, 101)
	if err != nil {
		return &VoteResponse{}
	}

	reqName := fmt.Sprintf("%s/vote/%d", t.Name, now())
	t.Publish(&ndn.Data{
		Name:    ndn.NewName(reqName),
		Content: b,
	})

	var resp ndnVoteResponse
	err = tlv.Unmarshal(t.Fetch(t, &ndn.Interest{
		Name: mux.Notify(fmt.Sprintf("/%s/listen/vote", peer), reqName),
	}), &resp, 101)
	if err != nil {
		return &VoteResponse{}
	}
	return &VoteResponse{
		Term:    resp.Term,
		Success: resp.Success,
	}
}

func (t *ndnTransport) RequestRedirect(peer string, req *RedirectRequest) *RedirectResponse {
	b, err := tlv.Marshal(&ndnRedirectRequest{
		Input: req.Input,
	}, 101)

	if err != nil {
		return &RedirectResponse{}
	}

	reqName := fmt.Sprintf("%s/redirect/%d", t.Name, now())
	t.Publish(&ndn.Data{
		Name:    ndn.NewName(reqName),
		Content: b,
	})

	var resp ndnRedirectResponse
	err = tlv.Unmarshal(t.Fetch(t, &ndn.Interest{
		Name: mux.Notify(fmt.Sprintf("/%s/listen/redirect", peer), reqName),
	}), &resp, 101)
	if err != nil {
		return &RedirectResponse{}
	}
	return &RedirectResponse{
		Leader:  resp.Leader,
		Success: resp.Success,
	}
}
