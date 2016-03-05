package raft

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/go-ndn/log"
	"github.com/go-ndn/ndn"
	"github.com/go-ndn/packet"
)

func TestVoteInmem(t *testing.T) {
	testVote(t,
		func(opt *Option) Transport {
			return NewInmemTransport(opt.Name)
		},
		func(opt *Option) Store {
			return NewInmemStore()
		},
	)
}

func TestVoteNDN(t *testing.T) {
	testVote(t,
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

func testVote(t *testing.T, transport func(*Option) Transport, store func(*Option) Store) {
	server1Opt := &Option{
		Name: "server1",
		Peer: []string{"server2"},
	}
	server1Opt.Transport = transport(server1Opt)
	server1Opt.Store = store(server1Opt)

	server1, err := NewServer(server1Opt)
	if err != nil {
		t.Fatal(err)
	}
	server1.Log = append(server1.Log, LogEntry{
		Term: 0,
	})

	server2Opt := &Option{
		Name: "server2",
		Peer: []string{"server1"},
	}
	server2Opt.Transport = transport(server2Opt)
	server2Opt.Store = store(server2Opt)

	server2, err := NewServer(server2Opt)
	if err != nil {
		t.Fatal(err)
	}

	go server1.Start()
	go server2.Start()

	time.Sleep(2 * HeartbeatTimeout)

	server1.Stop()
	server2.Stop()

	if !(server1.State == Candidate || server2.State == Candidate) {
		t.Fatalf("expect one candidate, got server1: %v, server2: %v", server1.State, server2.State)
	}

	go server1.Start()
	go server2.Start()

	time.Sleep(4 * ElectionTimeout)

	server1.Stop()
	server2.Stop()

	if !(server1.State == Leader && server2.State == Follower) {
		t.Fatalf("expect server1 to be leader, got server1: %v, server2: %v", server1.State, server2.State)
	}

	if server1.Term == 0 {
		t.Fatalf("expect server1 term > 0")
	}

	if server2.Term == 0 {
		t.Fatalf("expect server2 term > 0")
	}

	if closer, ok := server1Opt.Transport.(io.Closer); ok {
		closer.Close()
	}
	if closer, ok := server1Opt.Store.(io.Closer); ok {
		closer.Close()
	}

	if closer, ok := server2Opt.Transport.(io.Closer); ok {
		closer.Close()
	}
	if closer, ok := server2Opt.Store.(io.Closer); ok {
		closer.Close()
	}
}
