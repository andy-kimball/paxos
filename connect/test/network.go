// Copyright 2019 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sync"
	"time"

	"github.com/andy-kimball/paxos/connect"
	"github.com/gogo/protobuf/proto"
)

// Network is a mock network used in tests. It is entirely based in memory and
// is goroutine-safe. Listeners are maintained in a map. Clients "connect" by
// simple lookup into the map by listen URI. Here is an example:
//
//   network := test.NewNetwork()
//   server := &test.Server{}
//   network.Listen("a", server)
//
//   client := &test.SyncConn{}
//   ctx := context.Background()
//   defer client.Close(ctx, true /* drain */)
//   client.Inner = network.NewClient(&client.Syn, "a")
//   client.Send(ctx, &test.TestMessage{Text: text})
//
// The network can have latency associated with it. The NewLinkedNetwork method
// returns another connect.Network that is separated from the original network
// by the amount of latency passed to NewNetworkWithLatency. Messages sent from
// a client in one network to a server listening in a different network are
// delayed by that amount. For example:
//
//   east := test.NewNetworkWithLatency(100 * time.Millisecond)
//   eastServer := &test.Server{}
//   east.Listen("east-server", eastServer)
//
//   west := east.NewLinkedNetwork()
//   westServer := &test.Server{}
//   west.Listen("west-server", westServer)
//
//   client := &test.SyncConn{}
//   ctx := context.Background()
//   defer client.Close(ctx, true /* drain */)
//   client.Inner = east.NewClient(&client.Syn, "west-server")
//   client.Send(ctx, &test.TestMessage{Text: text})
//
// This example simulates servers in two data centers, separated by 100ms of
// latency. It creates a client in the east data center, and sends a message to
// the server in the west data center. The message will take 100ms to "arrive"
// (but with additional random latency). If instead the client sent a message to
// the server in the east data center, there would be no added latency.
type Network struct {
	state *networkState
}

// NewNetwork creates a new test.Network instance, with no latency between
// linked networks.
func NewNetwork() *Network {
	return NewNetworkWithLatency(0)
}

// NewNetworkWithLatency creates a new test.Network instance, with the specified
// amount of latency between linked networks.
func NewNetworkWithLatency(latency time.Duration) *Network {
	return &Network{
		state: &networkState{
			listeners: make(map[string]*listener),
			latency:   latency,
		},
	}
}

// NewLinkedNetwork returns another Network instance that is separated from this
// and other linked networks by the latency provided to NewNetworkWithLatency.
func (n *Network) NewLinkedNetwork() *Network {
	return &Network{state: n.state}
}

// Listen is part of the the connect.Network interface.
func (n *Network) Listen(ctx context.Context, uri string, server connect.Server) error {
	n.state.mutex.Lock()
	defer n.state.mutex.Unlock()

	if _, ok := n.state.listeners[uri]; ok {
		return fmt.Errorf("already listening on %s", uri)
	}
	n.state.listeners[uri] = &listener{network: n, server: server}
	return nil
}

// NewClient is part of the the connect.Network interface.
func (n *Network) NewClient(syn sync.Locker, uri string) connect.Conn {
	return newClient(n, syn, uri)
}

func (n *Network) lookupListener(uri string) *listener {
	n.state.mutex.Lock()
	defer n.state.mutex.Unlock()

	return n.state.listeners[uri]
}

// networkState contains the state shared between multiple linked networks.
type networkState struct {
	mutex     sync.Mutex
	listeners map[string]*listener
	latency   time.Duration
}

type listener struct {
	network *Network
	server  connect.Server
}

type request struct {
	ctx  context.Context
	msg  connect.Message
	sent time.Time
}

// client is a mock client created by the mock network. It maintains a simulated
// network connection to one of the servers maintained by the mock network.
type client struct {
	uri string

	// clientSyn is used to synchronize calls to the client connection. It is
	// separate from serverSyn so that client calls can be concurrent with server
	// calls, just as would be the case in a real configuration.
	clientSyn sync.Locker

	// serverSync is used to synchronize calls to the server connection.
	serverSyn sync.Locker

	server   connect.Conn
	latency  time.Duration
	requests chan *request
	close    chan struct{}
}

func newClient(network *Network, syn sync.Locker, uri string) *client {
	c := &client{
		uri:       uri,
		clientSyn: syn,
		serverSyn: &sync.Mutex{},
		requests:  make(chan *request, 2),
		close:     make(chan struct{}),
	}

	// Try to "connect" to the server.
	if listener := network.lookupListener(c.uri); listener != nil {
		c.server = listener.server.NewClient(c.serverSyn)
		if listener.network != network {
			// This simulates a cross data-center call, so add latency.
			c.latency = network.state.latency
		}
	} else {
		text := fmt.Sprintf("could not connect to server %s", c.uri)
		c.server = connect.NewErrorClient(&connect.ErrorMessage{Text: text}, c.serverSyn)
	}

	go func() {
		closed := false
		for !closed {
			select {
			case request := <-c.requests:
				// Simulate wire latency by inserting requested delay before
				// calling Write on the server.
				latency := randomizeLatency(c.latency)
				delay := request.sent.Add(latency).Sub(time.Now())
				if delay > 0 {
					time.Sleep(delay)
				}

				// Serialize message and then deserialize it in order to simulate
				// putting it on the wire.
				bytes, err := request.msg.(proto.Marshaler).Marshal()
				if err != nil {
					panic(err)
				}

				newMsg := reflect.New(reflect.TypeOf(request.msg).Elem()).Interface().(connect.Message)
				err = newMsg.(proto.Unmarshaler).Unmarshal(bytes)
				if err != nil {
					panic(err)
				}

				c.serverSyn.Lock()
				c.server.Send(request.ctx, newMsg)
				c.serverSyn.Unlock()

			case <-c.close:
				closed = true
			}
		}
	}()

	return c
}

// Send is part of the the connect.Conn interface.
func (c *client) Send(ctx context.Context, msg connect.Message) {
	// Allow other goroutines to use client, as if waiting for the request message
	// to be sent to the network layer.
	c.clientSyn.Unlock()
	defer c.clientSyn.Lock()

	// Simulate flow control by allowing a limited number of concurrent messages
	// (i.e. the number of buffered messages allowed by the channel).
	select {
	case c.requests <- &request{ctx: ctx, msg: msg, sent: time.Now()}:
	case <-c.close:
	}
}

// Recv is part of the the connect.Conn interface.
func (c *client) Recv() connect.Message {
	// Allow other goroutines to use client, as if waiting for a reply message
	// to be received from the network layer.
	c.clientSyn.Unlock()
	defer c.clientSyn.Lock()

	// Don't bother with simulated latency, since Send path already has it.
	c.serverSyn.Lock()
	defer c.serverSyn.Unlock()
	return c.server.Recv()
}

// Close is part of the the connect.Conn interface.
func (c *client) Close(ctx context.Context) {
	// Allow other goroutines to use client, as if waiting for the network
	// connection to be closed.
	c.clientSyn.Unlock()
	defer c.clientSyn.Lock()

	// Signal to other goroutines that the client is closing down.
	close(c.close)

	// Close the server.
	c.serverSyn.Lock()
	defer c.serverSyn.Unlock()
	c.server.Close(ctx)
}

// randomizeLatency returns a latency between minLatency and minLatency * 10,
// with the chance of higher latencies exponentially decreasing:
//
//   >= minLatency * 1 and < minLatency * 2 = 50% chance
//   >= minLatency * 2 and < minLatency * 3 = 25% chance
//   >= minLatency * 3 and < minLatency * 4 = 12.5% chance
//   etc.
//
func randomizeLatency(minLatency time.Duration) time.Duration {
	if minLatency == 0 {
		return 0
	}

	// Generate number between 1 and 10, distributed as described above.
	mult := time.Duration(10.0-math.Log2(float64(rand.Intn(1024)))) + 1

	rnd := time.Duration(rand.Int63n(int64(minLatency)))
	return minLatency*mult + rnd
}
