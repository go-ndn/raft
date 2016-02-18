package raft

type Store interface {
	GetTerm() (uint64, string, error)
	SetTerm(uint64, string) error

	GetLog() ([]LogEntry, error)
	CommitLog([]LogEntry) error
}
