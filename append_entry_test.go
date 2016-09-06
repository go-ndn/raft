package raft

import (
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/go-ndn/ndn"
	"github.com/go-ndn/packet"
)

func TestAppendEntryInmem(t *testing.T) {
	testAppendEntry(t,
		func(opt *Option) Transport {
			return NewInmemTransport(opt.Name)
		},
		func(opt *Option) Store {
			return NewInmemStore()
		},
	)
}

func TestAppendEntryNDN(t *testing.T) {
	testAppendEntry(t,
		func(opt *Option) Transport {
			// connect to nfd
			conn, err := packet.Dial("tcp", ":6363")
			if err != nil {
				t.Fatal(err)
			}
			// read producer key
			pem, err := os.Open("key/default.pri")
			if err != nil {
				t.Fatal(err)
			}
			defer pem.Close()
			key, _ := ndn.DecodePrivateKey(pem)

			return NewNDNTransport(opt.Name, conn, key)
		},
		func(opt *Option) Store {
			return NewInmemStore()
		},
	)
}

func testAppendEntry(t *testing.T, transport func(*Option) Transport, store func(*Option) Store) {
	leaderLog := []LogEntry{
		{Term: 1},
		{Term: 1},
		{Term: 1},
		{Term: 4},
		{Term: 4},
		{Term: 5},
		{Term: 5},
		{Term: 6},
		{Term: 6},
		{Term: 6},
	}

	for _, test := range []struct {
		followerLog []LogEntry
	}{
		{
			followerLog: []LogEntry{
				{Term: 1},
				{Term: 1},
				{Term: 1},
				{Term: 4},
				{Term: 4},
				{Term: 5},
				{Term: 5},
				{Term: 6},
				{Term: 6},
			},
		},
		{
			followerLog: []LogEntry{
				{Term: 1},
				{Term: 1},
				{Term: 1},
				{Term: 4},
			},
		},
		{
			followerLog: []LogEntry{
				{Term: 1},
				{Term: 1},
				{Term: 1},
				{Term: 4},
				{Term: 4},
				{Term: 5},
				{Term: 5},
				{Term: 6},
				{Term: 6},
				{Term: 6},
				{Term: 6},
			},
		},
		{
			followerLog: []LogEntry{
				{Term: 1},
				{Term: 1},
				{Term: 1},
				{Term: 4},
				{Term: 4},
				{Term: 5},
				{Term: 5},
				{Term: 6},
				{Term: 6},
				{Term: 6},
				{Term: 7},
				{Term: 7},
			},
		},
		{
			followerLog: []LogEntry{
				{Term: 1},
				{Term: 1},
				{Term: 1},
				{Term: 4},
				{Term: 4},
				{Term: 4},
				{Term: 4},
			},
		},
		{
			followerLog: []LogEntry{
				{Term: 1},
				{Term: 1},
				{Term: 1},
				{Term: 2},
				{Term: 2},
				{Term: 2},
				{Term: 3},
				{Term: 3},
				{Term: 3},
				{Term: 3},
				{Term: 3},
			},
		},
	} {
		leaderOpt := &Option{
			Name: "leader",
			Peer: []string{"follower"},
		}

		leaderOpt.Transport = transport(leaderOpt)
		leaderOpt.Store = store(leaderOpt)

		leader, err := NewServer(leaderOpt)
		if err != nil {
			t.Fatal(err)
		}
		leader.State = Leader
		leader.Log = leaderLog
		leader.CommitIndex = 3
		for i := range leader.Peer {
			leader.Peer[i].Index = uint64(len(leaderLog))
		}
		leader.Term = 8

		followerOpt := &Option{
			Name: "follower",
			Peer: []string{"leader"},
		}

		followerOpt.Transport = transport(followerOpt)
		followerOpt.Store = store(followerOpt)

		follower, err := NewServer(followerOpt)
		if err != nil {
			t.Fatal(err)
		}
		follower.Log = test.followerLog
		follower.Term = 8
		follower.CommitIndex = 3

		go leader.Start()
		go follower.Start()

		// It takes len(leaderLog[3:]) * HeartbeatIntv to rollback, then
		// it takes 2 * HeartbeatIntv and some extra time to fully commit log entries.
		time.Sleep(time.Duration(len(leaderLog[3:])+3) * HeartbeatIntv)

		leaderCommitted, err := leader.GetLog()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(leaderLog[3:], leaderCommitted) {
			t.Fatalf("expect %v, got %v", leaderLog[3:], leaderCommitted)
		}

		followerCommitted, err := follower.GetLog()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(leaderLog[3:], followerCommitted) {
			t.Fatalf("expect %v, got %v", leaderLog[3:], followerCommitted)
		}

		leader.Stop()
		follower.Stop()

		if closer, ok := leaderOpt.Transport.(io.Closer); ok {
			closer.Close()
		}
		if closer, ok := leaderOpt.Store.(io.Closer); ok {
			closer.Close()
		}

		if closer, ok := followerOpt.Transport.(io.Closer); ok {
			closer.Close()
		}
		if closer, ok := followerOpt.Store.(io.Closer); ok {
			closer.Close()
		}
	}
}
