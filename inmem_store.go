package raft

import "sync"

type inmemStore struct {
	sync.RWMutex

	Term     uint64
	VotedFor string
	Log      []LogEntry
}

func NewInmemStore() Store {
	return &inmemStore{}
}

func (s *inmemStore) GetTerm() (uint64, string, error) {
	s.RLock()
	defer s.RUnlock()
	return s.Term, s.VotedFor, nil
}

func (s *inmemStore) SetTerm(term uint64, votedFor string) error {
	s.Lock()
	s.Term = term
	s.VotedFor = votedFor
	s.Unlock()
	return nil
}

func (s *inmemStore) GetLog() ([]LogEntry, error) {
	s.RLock()
	defer s.RUnlock()
	return s.Log, nil
}

func (s *inmemStore) CommitLog(log []LogEntry) error {
	s.Lock()
	s.Log = append(s.Log, log...)
	s.Unlock()
	return nil
}
