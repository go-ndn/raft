package raft

import (
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/go-ndn/log"
	"github.com/go-ndn/ndn"
	"github.com/go-ndn/packet"
)

func TestRedirectInmem(t *testing.T) {
	testRedirect(t,
		func(opt *Option) Transport {
			return NewInmemTransport(opt.Name)
		},
		func(opt *Option) Store {
			return NewInmemStore()
		},
	)
}

func TestRedirectNDN(t *testing.T) {
	testRedirect(t,
		func(opt *Option) Transport {
			// connect to nfd
			conn, err := packet.Dial("tcp", ":6363")
			if err != nil {
				log.Fatalln(err)
			}
			// read producer key
			pem, err := os.Open("key/default.pri")
			if err != nil {
				log.Fatalln(err)
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

func testRedirect(t *testing.T, transport func(*Option) Transport, store func(*Option) Store) {
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

	expect := []LogEntry{
		{Term: 0, Value: []byte("hello")},
	}

	go leader.Start()
	go follower.Start()

	resp := leader.RequestRedirect(followerOpt.Name, &RedirectRequest{
		Input: [][]byte{expect[0].Value},
	})

	if resp.Success {
		t.Fatalf("follower should not accept redirect")
	}

	resp = follower.RequestRedirect(leaderOpt.Name, &RedirectRequest{
		Input: [][]byte{expect[0].Value},
	})

	if !resp.Success {
		t.Fatalf("leader should accept redirect")
	}

	time.Sleep(4 * HeartbeatIntv)

	leaderCommitted, err := leader.GetLog()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expect, leaderCommitted) {
		t.Fatalf("expect %v, got %v", expect, leaderCommitted)
	}

	followerCommitted, err := follower.GetLog()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expect, followerCommitted) {
		t.Fatalf("expect %v, got %v", expect, followerCommitted)
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
