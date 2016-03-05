package raft

// Store represents persist storage.
type Store interface {
	TermStore
	LogStore
}

// TermStore stores term and the voted candidate for that term persistently.
//
// votedFor will be empty if term progresses.
type TermStore interface {
	GetTerm() (term uint64, votedFor string, err error)
	SetTerm(term uint64, votedFor string) error
}

// LogStore stores log entries persistently.
//
// If log entries are commited, FSM can progress to the next state with them.
type LogStore interface {
	GetLog() ([]LogEntry, error)
	CommitLog([]LogEntry) error
}
