package raft

import "bytes"

// RedirectRequest is the command used to apply new log entries to the leader.
type RedirectRequest struct {
	Input [][]byte
	Index uint64

	Response chan<- *RedirectResponse
}

// RedirectResponse is the response of RedirectRequest.
//
// If the server is not the leader, the request will fail, and the actual leader name will be returned.
type RedirectResponse struct {
	Leader  string
	Index   uint64
	Success bool
}

func (s *Server) redirectRPC(req *RedirectRequest) (resp *RedirectResponse) {
	resp = &RedirectResponse{}
	if s.State != Leader {
		resp.Leader = s.Leader
		return
	}

	if req.Index == 0 {
		for _, b := range req.Input {
			s.Log = append(s.Log, LogEntry{
				Term:  s.Term,
				Value: b,
			})
		}
		resp.Index = uint64(len(s.Log))
	} else {
		if req.Index < uint64(len(req.Input)) {
			return
		}
		if s.CommitIndex < req.Index {
			return
		}
		for i, b := range req.Input {
			if !bytes.Equal(s.Log[int(req.Index)-len(req.Input)+i].Value, b) {
				return
			}
		}
	}

	resp.Success = true

	return
}
