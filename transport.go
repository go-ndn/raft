package raft

type Transport interface {
	AppendTransport
	VoteTransport
	RedirectTransport
}

type AppendTransport interface {
	AcceptAppend() <-chan *AppendRequest
	RequestAppend(peer string, req *AppendRequest) *AppendResponse
}

type VoteTransport interface {
	AcceptVote() <-chan *VoteRequest
	RequestVote(peer string, req *VoteRequest) *VoteResponse
}

type RedirectTransport interface {
	AcceptRedirect() <-chan *RedirectRequest
	RequestRedirect(leader string, req *RedirectRequest) *RedirectResponse
}
