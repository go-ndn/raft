package raft

// RedirectRequest is the command used to apply new log entries to the leader.
type RedirectRequest struct {
	Input [][]byte

	Response chan<- *RedirectResponse
}

// RedirectResponse is the response of RedirectRequest.
//
// If the server is not the leader, the request will fail, and the actual leader name will be returned.
type RedirectResponse struct {
	Leader  string
	Success bool
}

func (s *Server) redirectRPC(req *RedirectRequest) (resp *RedirectResponse) {
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
