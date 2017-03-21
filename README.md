<p align="center"><img src="logo.png"/></p>

# NDN Raft

Existing NDN synchronization protocols like "iSync" and "ChronoSync" are __eventual consistent__; if no update happens, eventually all node states will converge. However it does not satisfy distributed applications that must have strong consistency.

Although different applications have different consistency requirements, __strong consistency__ and __partition tolerance__ (see [CAP](https://en.wikipedia.org/wiki/CAP_theorem)) are often needed, because network partition does occur, and any observed inconsistency is undesirable.

The [Raft](https://ramcloud.stanford.edu/raft.pdf) distributed consensus protocol is a protocol by which a cluster of nodes can maintain a replicated state machine. The state machine is kept in sync through the use of a replicated log. NDN enables a cluster of nodes to synchronize data efficiently by replicating a log of NDN names.

In 2017, the core raft implementation is replaced with [etcd/raft](https://github.com/coreos/etcd/tree/master/raft) from core os, and this package only offers network transport.

> This Raft library is stable and feature complete. As of 2016, it is the most widely used Raft library in production, serving tens of thousands clusters each day. It powers distributed systems such as etcd, Kubernetes, Docker Swarm, Cloud Foundry Diego, CockroachDB, TiDB, Project Calico, Flannel, and more.

[![GoDoc](https://godoc.org/github.com/go-ndn/raft?status.svg)](https://godoc.org/github.com/go-ndn/raft)

See [transport_test.go](https://github.com/go-ndn/raft/blob/master/transport_test.go) for example.

```
[[::1]:63817] 2017/02/21 18:46:47 face created
[fib] 2017/02/21 18:46:47 add /raft/1/message
[fib] 2017/02/21 18:46:47 add /raft/1/listen/ACK
[[::1]:63818] 2017/02/21 18:46:47 face created
[fib] 2017/02/21 18:46:47 add /raft/2/message
[fib] 2017/02/21 18:46:47 add /raft/2/listen/ACK
[[::1]:63819] 2017/02/21 18:46:47 face created
[fib] 2017/02/21 18:46:47 add /raft/3/message
[fib] 2017/02/21 18:46:47 add /raft/3/listen/ACK
[[::1]:63817] 2017/02/21 18:46:48 forward /raft/1/listen/ACK/raft/2/message/1487731608223/36d9ff45
[[::1]:63818] 2017/02/21 18:46:48 forward /raft/2/message/1487731608223/36d9ff45
[[::1]:63818] 2017/02/21 18:46:48 receive /raft/2/message/1487731608223/36d9ff45/%00%00
[[::1]:63817] 2017/02/21 18:46:48 receive /raft/1/listen/ACK/raft/2/message/1487731608223/36d9ff45
```
