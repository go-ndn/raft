package raft

type RedirectRequest struct {
	Name  string
	Term  uint64
	Input [][]byte

	Respond func(*RedirectResponse)
}

type RedirectResponse struct {
	Term    uint64
	Success bool
}

func (s *Server) RedirectRPC(req *RedirectRequest) (resp *RedirectResponse) {
	resp = &RedirectResponse{
		Term: s.Term,
	}
	if req.Term < s.Term {
		return
	}

	if s.UpdateTermIfNewer(req.Term) {
		resp.Term = s.Term
	}

	for _, b := range req.Input {
		s.Log = append(s.Log, LogEntry{
			Term:  s.Term,
			Value: b,
		})
	}

	resp.Success = true

	return
}
