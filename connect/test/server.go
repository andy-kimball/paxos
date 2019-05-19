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
	"sync"

	"github.com/andy-kimball/paxos/connect"
)

// Server is a mock server used in tests. It queues incoming connections and
// waits for callers to accept them by calling Recv on the AcceptChan. Each
// incoming *test.Conn connection has RequestChan and ReplyChan channels that
// callers can use to receive and send messages over the connection. Here is an
// example of usage:
//
//   network := test.NewNetwork()
//   server := test.NewServer()
//   network.Listen("a", 0, server)
//
//   client := &test.SyncConn{}
//   ctx := context.Background()
//   defer client.Close(ctx, true /* drain */)
//   client.Inner = network.NewClient(&syncConn.Syn, "a")
//   client.Send(ctx, &test.TestMessage{Text: text})
//
//   conn := <-server.AcceptChan
//   request := <-conn.RequestChan
//   conn.ReplyChan <- &test.TestMessage{Text: text}
//
//   reply := client.Recv()
//
type Server struct {
	AcceptChan chan *Conn
}

// NewServer constructs a new instance of test.Server.
func NewServer() *Server {
	return &Server{AcceptChan: make(chan *Conn, 2)}
}

// NewClient is part of the the connect.Server interface.
func (s *Server) NewClient(syn sync.Locker) connect.Conn {
	c := &Conn{
		syn:         syn,
		RequestChan: make(chan connect.Message, 2),
		ReplyChan:   make(chan connect.Message, 2),
		CloseChan:   make(chan struct{}),
	}

	// Put the new connection in the accept queue, waiting for caller to accept
	// retrieve it.
	s.AcceptChan <- c

	return c
}

// Conn is a mock connection used in tests. See the header comment for Server to
// see example usage.
type Conn struct {
	syn         sync.Locker
	RequestChan chan connect.Message
	ReplyChan   chan connect.Message
	CloseChan   chan struct{}
}

// Send is part of the the connect.Conn interface.
func (c *Conn) Send(ctx context.Context, msg connect.Message) {
	c.syn.Unlock()
	defer c.syn.Lock()

	select {
	case c.RequestChan <- msg:
	case <-c.CloseChan:
	}
}

// Recv is part of the the connect.Conn interface.
func (c *Conn) Recv() connect.Message {
	c.syn.Unlock()
	defer c.syn.Lock()

	select {
	case reply := <-c.ReplyChan:
		return reply

	case <-c.CloseChan:
		return &connect.ErrorMessage{Text: "test server is closed"}
	}
}

// Close is part of the the connect.Conn interface.
func (c *Conn) Close(ctx context.Context) {
	close(c.CloseChan)
}
