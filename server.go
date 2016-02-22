package raft

import (
	"math/rand"
	"sync"
	"time"
)

type ServerState uint8

const (
	Follower ServerState = iota
	Leader
	Candidate
)

type Peer struct {
	Name  string
	Index uint64
}

type LogEntry struct {
	Term  uint64
	Value []byte
}

type Server struct {
	State ServerState

	Name string
	Peer []Peer

	Transport
	Store

	Log         []LogEntry
	CommitIndex uint64

	Term     uint64
	VotedFor string
	Leader   string

	Input    <-chan []byte
	Shutdown chan struct{}
}

type Option struct {
	Name string
	Peer []string
	Store
	Transport

	Input <-chan []byte
}

func NewServer(opt *Option) (*Server, error) {
	log, err := opt.Store.GetLog()
	if err != nil {
		return nil, err
	}
	term, votedFor, err := opt.Store.GetTerm()
	if err != nil {
		return nil, err
	}

	var peer []Peer
	for _, name := range opt.Peer {
		peer = append(peer, Peer{
			Name: name,
		})
	}
	return &Server{
		Name: opt.Name,
		Peer: peer,

		Transport: opt.Transport,
		Store:     opt.Store,

		Log:         log,
		CommitIndex: uint64(len(log)),

		Term:     term,
		VotedFor: votedFor,

		Input:    opt.Input,
		Shutdown: make(chan struct{}),
	}, nil
}

const (
	HeartbeatTimeout = 40 * HeartbeatIntv
	ElectionTimeout  = 30 * HeartbeatIntv

	HeartbeatIntv = 100 * time.Millisecond
)

func timeAfter(d time.Duration) <-chan time.Time {
	return time.After(time.Duration((1 + rand.Float64()) * float64(d)))
}

func (s *Server) Stop() {
	s.Shutdown <- struct{}{}
}

func (s *Server) Start() {
	for {
		switch s.State {
		case Follower:
			var input <-chan []byte
			if s.Leader != "" {
				input = s.Input
			}
			select {
			case <-s.Shutdown:
				return
			case b := <-input:
				bs := [][]byte{b}
			INPUT:
				for {
					select {
					case b = <-input:
						bs = append(bs, b)
					default:
						break INPUT
					}
				}
				resp := s.RequestRedirect(s.Leader, &RedirectRequest{
					Term:  s.Term,
					Input: bs,
				})
				s.UpdateTermIfNewer(resp.Term)
			case req := <-s.AcceptAppend():
				req.Response <- s.AppendEntryRPC(req)
			case req := <-s.AcceptVote():
				req.Response <- s.VoteRPC(req)
			case <-timeAfter(HeartbeatTimeout):
				if s.VotedFor == "" {
					s.State = Candidate
				}
			}
		case Leader:
			select {
			case <-s.Shutdown:
				return
			case req := <-s.AcceptRedirect():
				req.Response <- s.RedirectRPC(req)
			case b := <-s.Input:
				s.Log = append(s.Log, LogEntry{
					Term:  s.Term,
					Value: b,
				})
			case <-time.After(HeartbeatIntv):
				count := 1
				for i, resp := range s.RequestAppendFromPeers() {
					if resp.Success {
						count++
						s.Peer[i].Index = uint64(len(s.Log))
					} else {
						if s.Peer[i].Index > 0 {
							s.Peer[i].Index--
						}
					}
					s.UpdateTermIfNewer(resp.Term)
				}
				if count >= (len(s.Peer)+1)/2+1 {
					err := s.CommitLog(s.Log[s.CommitIndex:])
					if err != nil {
						continue
					}
					s.CommitIndex = uint64(len(s.Log))
				}
			}
		case Candidate:
			select {
			case <-s.Shutdown:
				return
			case req := <-s.AcceptAppend():
				req.Response <- s.AppendEntryRPC(req)
			case req := <-s.AcceptVote():
				req.Response <- s.VoteRPC(req)
			case <-timeAfter(ElectionTimeout):
				s.UpdateTermIfNewer(s.Term + 1)

				count := 1
				for _, resp := range s.RequestVoteFromPeers() {
					if resp.Success {
						count++
					}
					s.UpdateTermIfNewer(resp.Term)
				}
				if count >= (len(s.Peer)+1)/2+1 {
					s.State = Leader

					for i := range s.Peer {
						s.Peer[i].Index = uint64(len(s.Log))
					}
				} else {
					s.State = Follower
				}
			}
		}
	}
}

func (s *Server) UpdateTermIfNewer(term uint64) bool {
	if s.Term >= term {
		return false
	}
	s.Term = term
	s.VotedFor = ""
	s.Leader = ""
	s.SetTerm(s.Term, s.VotedFor)
	return true
}

func (s *Server) RequestAppendFromPeers() []*AppendResponse {
	result := make([]*AppendResponse, len(s.Peer))
	var wg sync.WaitGroup
	wg.Add(len(s.Peer))
	for i, p := range s.Peer {
		go func(i int, p Peer) {
			defer wg.Done()
			req := &AppendRequest{
				Name:        s.Name,
				Term:        s.Term,
				CommitIndex: s.CommitIndex,
				Log:         s.Log[p.Index:],
			}
			if p.Index > 0 {
				req.PrevLogIndex = p.Index
				req.PrevLogTerm = s.Log[p.Index-1].Term
			}
			result[i] = s.RequestAppend(p.Name, req)
		}(i, p)
	}
	wg.Wait()
	return result
}

func (s *Server) RequestVoteFromPeers() []*VoteResponse {
	result := make([]*VoteResponse, len(s.Peer))
	var wg sync.WaitGroup
	wg.Add(len(s.Peer))
	for i, p := range s.Peer {
		go func(i int, p Peer) {
			defer wg.Done()
			req := &VoteRequest{
				Name: s.Name,
				Term: s.Term,
			}
			if len(s.Log) > 0 {
				req.LastLogIndex = uint64(len(s.Log))
				req.LastLogTerm = s.Log[len(s.Log)-1].Term
			}
			result[i] = s.RequestVote(p.Name, req)
		}(i, p)
	}
	wg.Wait()
	return result
}
