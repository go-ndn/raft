package raft

import "sync"

type appendNetwork struct {
	nexthop map[string]chan<- *AppendRequest
	sync.RWMutex
}

func (f *appendNetwork) SendRequest(name string, req *AppendRequest, ch chan<- *AppendResponse) {
	f.RLock()
	h, ok := f.nexthop[name]
	f.RUnlock()
	if ok {
		req.Response = ch
		h <- req
	}
}

func (f *appendNetwork) Register(name string, ch chan<- *AppendRequest) {
	f.Lock()
	f.nexthop[name] = ch
	f.Unlock()
}

type voteNetwork struct {
	nexthop map[string]chan<- *VoteRequest
	sync.RWMutex
}

func (f *voteNetwork) SendRequest(name string, req *VoteRequest, ch chan<- *VoteResponse) {
	f.RLock()
	h, ok := f.nexthop[name]
	f.RUnlock()
	if ok {
		req.Response = ch
		h <- req
	}
}

func (f *voteNetwork) Register(name string, ch chan<- *VoteRequest) {
	f.Lock()
	f.nexthop[name] = ch
	f.Unlock()
}

type redirectNetwork struct {
	nexthop map[string]chan<- *RedirectRequest
	sync.RWMutex
}

func (f *redirectNetwork) SendRequest(name string, req *RedirectRequest, ch chan<- *RedirectResponse) {
	f.RLock()
	h, ok := f.nexthop[name]
	f.RUnlock()
	if ok {
		req.Response = ch
		h <- req
	}
}

func (f *redirectNetwork) Register(name string, ch chan<- *RedirectRequest) {
	f.Lock()
	f.nexthop[name] = ch
	f.Unlock()
}

var (
	defaultAppendNetwork = appendNetwork{
		nexthop: make(map[string]chan<- *AppendRequest),
	}
	defaultVoteNetwork = voteNetwork{
		nexthop: make(map[string]chan<- *VoteRequest),
	}
	defaultRedirectNetwork = redirectNetwork{
		nexthop: make(map[string]chan<- *RedirectRequest),
	}
)

type inmemTransport struct {
	append   chan *AppendRequest
	vote     chan *VoteRequest
	redirect chan *RedirectRequest
}

// NewInmemTransport creates a fake transport for testing.
func NewInmemTransport(name string) Transport {
	i := &inmemTransport{
		append:   make(chan *AppendRequest),
		vote:     make(chan *VoteRequest),
		redirect: make(chan *RedirectRequest),
	}

	defaultAppendNetwork.Register(name, i.append)
	defaultVoteNetwork.Register(name, i.vote)
	defaultRedirectNetwork.Register(name, i.redirect)
	return i
}

func (t *inmemTransport) AcceptAppend() <-chan *AppendRequest {
	return t.append
}

func (t *inmemTransport) AcceptVote() <-chan *VoteRequest {
	return t.vote
}

func (t *inmemTransport) AcceptRedirect() <-chan *RedirectRequest {
	return t.redirect
}

func (t *inmemTransport) RequestAppend(peer string, req *AppendRequest) *AppendResponse {
	ch := make(chan *AppendResponse)
	defaultAppendNetwork.SendRequest(peer, req, ch)
	return <-ch
}

func (t *inmemTransport) RequestVote(peer string, req *VoteRequest) *VoteResponse {
	ch := make(chan *VoteResponse)
	defaultVoteNetwork.SendRequest(peer, req, ch)
	return <-ch
}

func (t *inmemTransport) RequestRedirect(peer string, req *RedirectRequest) *RedirectResponse {
	ch := make(chan *RedirectResponse)
	defaultRedirectNetwork.SendRequest(peer, req, ch)
	return <-ch
}
