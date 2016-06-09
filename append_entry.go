package raft

// AppendRequest is the command used to append entries to replicated log.
type AppendRequest struct {
	Leader string

	Term         uint64
	PrevLogTerm  uint64
	PrevLogIndex uint64
	CommitIndex  uint64

	Log []LogEntry

	Response chan<- *AppendResponse
}

// AppendResponse is the response of AppendRequest.
type AppendResponse struct {
	Term    uint64
	Success bool
}

func (s *Server) appendEntryRPC(req *AppendRequest) (resp *AppendResponse) {
	resp = &AppendResponse{
		Term: s.Term,
	}
	if req.Term < s.Term {
		return
	}
	if s.updateTermIfNewer(req.Term) {
		resp.Term = s.Term
	}

	if req.PrevLogIndex != 0 {
		if uint64(len(s.Log)) < req.PrevLogIndex {
			return
		}
		if s.Log[req.PrevLogIndex-1].Term != req.PrevLogTerm {
			return
		}
	}

	s.Log = append(s.Log[:req.PrevLogIndex], req.Log...)

	commitIndex := req.CommitIndex
	if commitIndex > uint64(len(s.Log)) {
		commitIndex = uint64(len(s.Log))
	}

	if commitIndex > s.CommitIndex {
		err := s.CommitLog(s.Log[s.CommitIndex:commitIndex])
		if err != nil {
			return
		}

		s.CommitIndex = commitIndex
	}
	resp.Success = true

	if s.State == Candidate || s.State == Leader {
		s.State = Follower
	}
	s.Leader = req.Leader

	return
}
