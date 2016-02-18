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
	Name  string   `tlv:"131"`
	Term  uint64   `tlv:"128"`
	Input [][]byte `tlv:"139"`
}

type ndnRedirectResponse struct {
	Term    uint64 `tlv:"128"`
	Success bool   `tlv:"136"`
}

type ndnTransport struct {
	Name string
	ndn.Face
	Store ndn.Cache

	append   chan *AppendRequest
	vote     chan *VoteRequest
	redirect chan *RedirectRequest
}

func now() uint64 {
	return uint64(time.Now().UTC().UnixNano() / 1000000)
}

func NewNDNTransport(name string, conn net.Conn, key ndn.Key) Transport {
	name = "/" + name

	recv := make(chan *ndn.Interest)
	face := ndn.NewFace(conn, recv)

	tr := &ndnTransport{
		Name:  name,
		Face:  face,
		Store: ndn.NewCache(65536),

		append:   make(chan *AppendRequest),
		vote:     make(chan *VoteRequest),
		redirect: make(chan *RedirectRequest),
	}

	fetcher := mux.NewFetcher()
	m := mux.New()
	m.Use(mux.RawCacher(tr.Store, false))

	m.HandleFunc(name+"/log", func(w ndn.Sender, i *ndn.Interest) {})
	m.HandleFunc(name+"/append", func(w ndn.Sender, i *ndn.Interest) {})
	m.HandleFunc(name+"/vote", func(w ndn.Sender, i *ndn.Interest) {})
	m.HandleFunc(name+"/redirect", func(w ndn.Sender, i *ndn.Interest) {})

	m.Handle(mux.Listener(name+"/listen/append", func(locator string, w ndn.Sender, i *ndn.Interest) {
		var req ndnAppendRequest
		err := tlv.Unmarshal(
			fetcher.Fetch(w, &ndn.Interest{
				Name: ndn.NewName(locator),
			}),
			&req,
			101,
		)
		if err != nil {
			return
		}

		var log []LogEntry
		for i := req.PrevLogIndex + 1; i <= req.PrevLogIndex+req.LogCount; i++ {
			var entry ndnLogEntry
			err := tlv.Unmarshal(
				fetcher.Fetch(face, &ndn.Interest{
					Name: ndn.NewName(fmt.Sprintf("/%s/log/%d", req.Name, i)),
				}),
				&entry,
				101,
			)
			if err != nil {
				return
			}
			log = append(log, LogEntry{
				Term:  entry.Term,
				Value: entry.Value,
			})
		}

		tr.append <- &AppendRequest{
			Name: req.Name,

			Term:         req.Term,
			PrevLogTerm:  req.PrevLogTerm,
			PrevLogIndex: req.PrevLogIndex,
			CommitIndex:  req.CommitIndex,
			Log:          log,

			Respond: func(resp *AppendResponse) {
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
			},
		}
	}))

	m.Handle(mux.Listener(name+"/listen/vote", func(locator string, w ndn.Sender, i *ndn.Interest) {
		var req ndnVoteRequest
		err := tlv.Unmarshal(
			fetcher.Fetch(w, &ndn.Interest{
				Name: ndn.NewName(locator),
			}),
			&req,
			101,
		)
		if err != nil {
			return
		}

		tr.vote <- &VoteRequest{
			Name: req.Name,

			Term:         req.Term,
			LastLogTerm:  req.LastLogTerm,
			LastLogIndex: req.LastLogIndex,

			Respond: func(resp *VoteResponse) {
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
			},
		}
	}))

	m.Handle(mux.Listener(name+"/listen/redirect", func(locator string, w ndn.Sender, i *ndn.Interest) {
		var req ndnRedirectRequest
		err := tlv.Unmarshal(
			fetcher.Fetch(w, &ndn.Interest{
				Name: ndn.NewName(locator),
			}),
			&req,
			101,
		)
		if err != nil {
			return
		}

		tr.redirect <- &RedirectRequest{
			Name: req.Name,

			Term:  req.Term,
			Input: req.Input,

			Respond: func(resp *RedirectResponse) {
				b, err := tlv.Marshal(&ndnRedirectResponse{
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
			},
		}
	}))

	m.Register(face, key)

	go func() {
		for i := range recv {
			m.ServeNDN(face, i)
		}
	}()

	return tr
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
	for i, entry := range req.Log {
		entryName := fmt.Sprintf("/%s/log/%d", t.Name, req.PrevLogIndex+uint64(i)+1)
		b, err := tlv.Marshal(&ndnLogEntry{
			Term:  entry.Term,
			Value: entry.Value,
		}, 101)
		if err != nil {
			return &AppendResponse{}
		}
		t.Store.Add(&ndn.Data{
			Name:    ndn.NewName(entryName),
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
	t.Store.Add(&ndn.Data{
		Name:    ndn.NewName(reqName),
		Content: b,
	})

	d, ok := <-mux.Notify(t, fmt.Sprintf("/%s/listen/append", peer), reqName, 4*time.Second)
	if !ok {
		return &AppendResponse{}
	}
	var resp ndnAppendResponse
	err = tlv.Unmarshal(d.Content, &resp, 101)
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
	t.Store.Add(&ndn.Data{
		Name:    ndn.NewName(reqName),
		Content: b,
	})

	d, ok := <-mux.Notify(t, fmt.Sprintf("/%s/listen/vote", peer), reqName, 4*time.Second)
	if !ok {
		return &VoteResponse{}
	}
	var resp ndnVoteResponse
	err = tlv.Unmarshal(d.Content, &resp, 101)
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
		Name:  req.Name,
		Term:  req.Term,
		Input: req.Input,
	}, 101)

	if err != nil {
		return &RedirectResponse{}
	}

	reqName := fmt.Sprintf("%s/redirect/%d", t.Name, now())
	t.Store.Add(&ndn.Data{
		Name:    ndn.NewName(reqName),
		Content: b,
	})

	d, ok := <-mux.Notify(t, fmt.Sprintf("/%s/listen/redirect", peer), reqName, 4*time.Second)
	if !ok {
		return &RedirectResponse{}
	}
	var resp ndnRedirectResponse
	err = tlv.Unmarshal(d.Content, &resp, 101)
	if err != nil {
		return &RedirectResponse{}
	}
	return &RedirectResponse{
		Term:    resp.Term,
		Success: resp.Success,
	}
}
