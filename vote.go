package raft

type VoteRequest struct {
	Name string

	Term         uint64
	LastLogTerm  uint64
	LastLogIndex uint64

	Response chan<- *VoteResponse
}

type VoteResponse struct {
	Term    uint64
	Success bool
}

func (s *Server) VoteRPC(req *VoteRequest) (resp *VoteResponse) {
	resp = &VoteResponse{
		Term: s.Term,
	}
	if req.Term < s.Term {
		return
	}
	if s.UpdateTermIfNewer(req.Term) {
		resp.Term = s.Term
	}

	if s.VotedFor != "" && s.VotedFor != req.Name {
		return
	}

	if len(s.Log) > 0 {
		lastLog := s.Log[len(s.Log)-1]
		if lastLog.Term > req.LastLogTerm ||
			lastLog.Term == req.LastLogTerm && uint64(len(s.Log)) > req.LastLogIndex {
			return
		}
	}

	s.VotedFor = req.Name
	s.SetTerm(s.Term, s.VotedFor)
	resp.Success = true

	if s.State == Candidate || s.State == Leader {
		s.State = Follower
	}
	return
}
