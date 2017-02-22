<p align="center"><img src="logo.png"/></p>

# NDN Raft

Existing NDN synchronization protocols like "iSync" and "ChronoSync" are __eventual consistent__; if no update happens, eventually all node states will converge. However it does not satisfy distributed applications that must have strong consistency.

Although different applications have different consistency requirements, __strong consistency__ and __partition tolerance__ (see [CAP](https://en.wikipedia.org/wiki/CAP_theorem)) are often needed, because network partition does occur, and any observed inconsistency is undesirable.

The [Raft](https://ramcloud.stanford.edu/raft.pdf) distributed consensus protocol is a protocol by which a cluster of nodes can maintain a replicated state machine. The state machine is kept in sync through the use of a replicated log. NDN enables a cluster of nodes to synchronize data efficiently by replicating a log of NDN names.

In 2017, the core raft implementation is replaced with [etcd/raft](https://github.com/coreos/etcd/tree/master/raft) from core os, and this package only offers network transport.

> This Raft library is stable and feature complete. As of 2016, it is the most widely used Raft library in production, serving tens of thousands clusters each day. It powers distributed systems such as etcd, Kubernetes, Docker Swarm, Cloud Foundry Diego, CockroachDB, TiDB, Project Calico, Flannel, and more.

[![GoDoc](https://godoc.org/github.com/go-ndn/raft?status.svg)](https://godoc.org/github.com/go-ndn/raft)

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

## Example

```go
// connect to nfd
conn, err := packet.Dial("tcp", ":6363")
if err != nil {
  return nil, err
}

// read producer key
pem, err := os.Open("key/default.pri")
if err != nil {
  return nil, err
}
defer pem.Close()
key, err := ndn.DecodePrivateKey(pem)
if err != nil {
  return nil, err
}

// create NDN raft transport
tr := New(&Config{
  Prefix:    "/raft",
  NodeID:    id,
  Conn:      conn,
  Key:       key,
  CacheSize: 64,
})

// start one raft node
storage := raft.NewMemoryStorage()
logger := &raft.DefaultLogger{Logger: log.New(os.Stderr, "", log.LstdFlags)}
n := raft.StartNode(
  &raft.Config{
    ID:              id,
    ElectionTick:    10,
    HeartbeatTick:   1,
    Storage:         storage,
    MaxSizePerMsg:   4096,
    MaxInflightMsgs: 256,
    Logger:          logger,
  },
  peers,
)

ticker := time.NewTicker(50 * time.Millisecond)
defer ticker.Stop()

defer tr.Close()
defer n.Stop()
for {
  select {
  case <-ticker.C:
    // Call Node.Tick() at regular intervals.
    n.Tick()
  case msg := <-tr.RecvMessage:
    // When you receive a message from another node, pass it to Node.Step.
    n.Step(ctx, msg)
  case rd := <-n.Ready():
    // Write HardState, Entries, and Snapshot to persistent storage if they are not empty.
    storage.Append(rd.Entries)
    if !raft.IsEmptyHardState(rd.HardState) {
      storage.SetHardState(rd.HardState)
    }
    if !raft.IsEmptySnap(rd.Snapshot) {
      storage.ApplySnapshot(rd.Snapshot)
    }

    // Send all Messages to the nodes named in the To field.
    // If any Message has type MsgSnap, call Node.ReportSnapshot() after it has been sent.
    for _, msg := range rd.Messages {
      err := tr.Send(msg)
      switch msg.Type {
      case raftpb.MsgSnap:
        s := raft.SnapshotFinish
        if err != nil {
          s = raft.SnapshotFailure
        }
        n.ReportSnapshot(msg.To, s)
      }
    }

    // Apply Snapshot (if any) and CommittedEntries to the state machine.
    // If any committed Entry has Type EntryConfChange, call Node.ApplyConfChange() to apply it to the node.
    for _, entry := range rd.CommittedEntries {
      switch entry.Type {
      case raftpb.EntryConfChange:
        var cc raftpb.ConfChange
        cc.Unmarshal(entry.Data)
        n.ApplyConfChange(cc)
      }
    }

    // Call Node.Advance() to signal readiness for the next batch of updates.
    n.Advance()
  case <-ctx.Done():
    return
  }
}
```
