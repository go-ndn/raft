// Package raft implements the Raft distributed consensus protocol.
package raft

import (
	"math/rand"
	"sync"
	"time"
)

// ServerState is one of Follower, Leader, and Candidate.
type ServerState uint8

func (s ServerState) String() string {
	switch s {
	case Leader:
		return "leader"
	case Candidate:
		return "candidate"
	default:
		return "follower"
	}
}

const (
	// Follower accepts AppendRequest from Leader
	Follower ServerState = iota
	// Leader periodically sends AppendRequest
	Leader
	// Candidate sends VoteRequest to become next Leader
	Candidate
)

// Peer is a member of the Raft cluster.
type Peer struct {
	Name  string
	Index uint64
}

// LogEntry is replicated to all members of the Raft cluster, and forms the heart of the replicated state machine.
type LogEntry struct {
	Term  uint64
	Value []byte
}

// Server implements a Raft node.
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

	stop chan struct{}
}

// Option provides any necessary configuration to the Raft server.
type Option struct {
	Name string
	Peer []string
	Store
	Transport
}

// NewServer creates a new Raft server.
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

		stop: make(chan struct{}),
	}, nil
}

const (
	// HeartbeatTimeout is the time that a follower loses its leader, and should become a candidate.
	// The actual timeout is randomized between 1x to 2x of this value.
	// Some RPCs might delay it.
	HeartbeatTimeout = 300 * time.Millisecond
	// ElectionTimeout is the time that a candidate should request votes from its peers.
	// The actual timeout is randomized between 1x to 2x of this value.
	// Some RPCs might delay it.
	ElectionTimeout = 400 * time.Millisecond
	// HeartbeatIntv is the time that a leader should append new log entries to its peers.
	// Some RPCs might delay it.
	HeartbeatIntv = 100 * time.Millisecond
)

func approx(d time.Duration) time.Duration {
	return time.Duration((1 + rand.Float64()) * float64(d))
}

// Stop halts the server.
func (s *Server) Stop() {
	s.stop <- struct{}{}
}

// Start restarts the server.
func (s *Server) Start() {
	heartbeatTicker := time.NewTicker(HeartbeatIntv)
	defer heartbeatTicker.Stop()

	for {
		switch s.State {
		case Follower:
			select {
			case <-s.stop:
				return
			case req := <-s.AcceptRedirect():
				req.Response <- s.redirectRPC(req)
			case req := <-s.AcceptAppend():
				req.Response <- s.appendEntryRPC(req)
			case req := <-s.AcceptVote():
				req.Response <- s.voteRPC(req)
			case <-time.After(approx(HeartbeatTimeout)):
				if s.VotedFor == "" {
					s.State = Candidate
				}
			}
		case Leader:
			select {
			case <-s.stop:
				return
			case req := <-s.AcceptRedirect():
				req.Response <- s.redirectRPC(req)
			case <-heartbeatTicker.C:
				count := 1
				for i, resp := range s.requestAppendFromPeers() {
					if resp.Success {
						count++
						s.Peer[i].Index = uint64(len(s.Log))
					} else {
						if s.Peer[i].Index > 0 {
							s.Peer[i].Index--
						}
					}
					s.updateTermIfNewer(resp.Term)
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
			case <-s.stop:
				return
			case req := <-s.AcceptAppend():
				req.Response <- s.appendEntryRPC(req)
			case req := <-s.AcceptVote():
				req.Response <- s.voteRPC(req)
			case <-time.After(approx(ElectionTimeout)):
				s.updateTermIfNewer(s.Term + 1)

				count := 1
				for _, resp := range s.requestVoteFromPeers() {
					if resp.Success {
						count++
					}
					s.updateTermIfNewer(resp.Term)
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

func (s *Server) updateTermIfNewer(term uint64) bool {
	if s.Term >= term {
		return false
	}
	s.Term = term
	s.VotedFor = ""
	s.Leader = ""
	s.SetTerm(s.Term, s.VotedFor)
	return true
}

func (s *Server) requestAppendFromPeers() []*AppendResponse {
	result := make([]*AppendResponse, len(s.Peer))
	var wg sync.WaitGroup
	wg.Add(len(s.Peer))
	for i, p := range s.Peer {
		go func(i int, p Peer) {
			defer wg.Done()
			req := &AppendRequest{
				Leader:      s.Name,
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

func (s *Server) requestVoteFromPeers() []*VoteResponse {
	result := make([]*VoteResponse, len(s.Peer))
	var wg sync.WaitGroup
	wg.Add(len(s.Peer))
	for i, p := range s.Peer {
		go func(i int, p Peer) {
			defer wg.Done()
			req := &VoteRequest{
				Candidate: s.Name,
				Term:      s.Term,
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
