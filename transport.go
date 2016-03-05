package raft

// Transport sends and accepts various RPCs.
type Transport interface {
	AppendTransport
	VoteTransport
	RedirectTransport
}

// AppendTransport sends and accepts AppendRequest.
type AppendTransport interface {
	AcceptAppend() <-chan *AppendRequest
	RequestAppend(peer string, req *AppendRequest) *AppendResponse
}

// VoteTransport sends and accepts VoteRequest.
type VoteTransport interface {
	AcceptVote() <-chan *VoteRequest
	RequestVote(peer string, req *VoteRequest) *VoteResponse
}

// RedirectTransport sends and accepts RedirectRequest.
type RedirectTransport interface {
	AcceptRedirect() <-chan *RedirectRequest
	RequestRedirect(leader string, req *RedirectRequest) *RedirectResponse
}
