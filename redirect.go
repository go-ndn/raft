package raft

type RedirectRequest struct {
	Input [][]byte

	Response chan<- *RedirectResponse
}

type RedirectResponse struct {
	Leader  string
	Success bool
}

func (s *Server) RedirectRPC(req *RedirectRequest) (resp *RedirectResponse) {
	resp = &RedirectResponse{}
	if s.State != Leader {
		resp.Leader = s.Leader
		return
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
