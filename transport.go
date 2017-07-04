// Package raftndn implements raft node communication on top of NDN.
//
// This is designed to work with github.com/coreos/etcd/raft.
package raftndn

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/coreos/etcd/raft/raftpb"
	"github.com/go-ndn/mux"
	"github.com/go-ndn/ndn"
)

// Transport sends and receives raftpb.Message on top of NDN.
type Transport struct {
	Prefix string
	NodeID uint64
	*mux.Publisher
	*mux.Fetcher
	ndn.Face

	// See README.md for example.
	RecvMessage <-chan raftpb.Message
}

// Send sends raft message to msg.To.
func (t *Transport) Send(msg raftpb.Message) error {
	b, err := msg.Marshal()
	if err != nil {
		return err
	}

	name := fmt.Sprintf("%s/%d/message/%d/%08x", t.Prefix, t.NodeID, time.Now().UnixNano()/1000000, rand.Uint32())

	err = t.Publish(&ndn.Data{
		Name:    ndn.NewName(name),
		Content: b,
	})
	if err != nil {
		return err
	}
	_, err = t.Fetch(t, &ndn.Interest{
		Name: mux.Notify(fmt.Sprintf("%s/%d/listen", t.Prefix, msg.To), name),
	})
	if err != nil {
		return err
	}

	return nil
}

// Config is used to initialize Transport.
type Config struct {
	// Prefix is the prefix for the raft application.
	// For example, "/coreos/etcd".
	Prefix string
	// NodeID should match raft.Config.ID.
	// Prefix and NodeID are used to uniquely identify one raft node.
	NodeID uint64
	Conn   net.Conn
	Key    ndn.Key
	// CacheSize is the internal NDN packet buffer. This value should be
	// proportional to the rate of raft.Node.Tick().
	CacheSize int
}

// New creates NDN raft transport.
func New(config *Config) *Transport {
	cache := ndn.NewCache(config.CacheSize)

	recv := make(chan *ndn.Interest)

	recvMessage := make(chan raftpb.Message)

	t := &Transport{
		Prefix:      config.Prefix,
		NodeID:      config.NodeID,
		Face:        ndn.NewFace(config.Conn, recv),
		Publisher:   mux.NewPublisher(cache),
		Fetcher:     mux.NewFetcher(),
		RecvMessage: recvMessage,
	}

	// verify checksum
	t.Fetcher.Use(mux.ChecksumVerifier)
	// assemble chunks
	t.Fetcher.Use(mux.Assembler)

	// segment
	t.Publisher.Use(mux.Segmentor(4096))

	m := mux.New()
	m.Use(mux.RawCacher(cache, false))

	// served from cache
	m.HandleFunc(
		fmt.Sprintf("%s/%d/message", config.Prefix, config.NodeID),
		func(w ndn.Sender, i *ndn.Interest) error { return nil },
	)
	m.Handle(mux.Listener(
		fmt.Sprintf("%s/%d/listen", config.Prefix, config.NodeID),
		func(locator string, w ndn.Sender, i *ndn.Interest) error {
			var msg raftpb.Message

			content, err := t.Fetch(w, &ndn.Interest{
				Name: ndn.NewName(locator),
			})
			if err != nil {
				return err
			}
			err = msg.Unmarshal(content)
			if err != nil {
				return err
			}
			err = w.SendData(&ndn.Data{
				Name: i.Name,
			})
			if err != nil {
				return err
			}
			recvMessage <- msg
			return nil
		},
	))

	go m.Run(t, recv, config.Key)

	return t
}
