package raftndn

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/go-ndn/ndn"
	"github.com/go-ndn/packet"
)

func newRaftNode(ctx context.Context, id uint64, peers []raft.Peer) (raft.Node, error) {
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

	go func() {
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
	}()
	return n, nil
}

func TestElection(t *testing.T) {
	ctx := context.Background()

	doneCtx, cancel := context.WithCancel(ctx)

	const clusterSize = 3

	var peers []raft.Peer
	for i := 0; i < clusterSize; i++ {
		peers = append(peers, raft.Peer{ID: uint64(i + 1)})
	}
	var nodes []raft.Node
	for i := 0; i < clusterSize; i++ {
		n, err := newRaftNode(doneCtx, uint64(i+1), peers)
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
	}

	time.Sleep(time.Second)

	var hasLeader bool
	for _, n := range nodes {
		if n.Status().SoftState.RaftState == raft.StateLeader {
			hasLeader = true
		}
	}

	cancel()
	if !hasLeader {
		t.Fatalf("no raft leader")
	}
}
