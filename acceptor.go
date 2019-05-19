package paxos

import (
	"context"
	"sync"

	"github.com/andy-kimball/paxos/connect"
)

// Acceptor implements the connect.Server interface so that it can receive
// messages from proposers.
type Acceptor struct {
	syn sync.Mutex
}

func (a *Acceptor) NewClient(syn sync.Locker) connect.Conn {
	return newAcceptorConn(syn, a)
}

func (a *Acceptor) handleTestMessage(request *TestMessage) *TestMessage {
	// Always lock, in case multiple clients are concurrently calling.
	a.syn.Lock()
	defer a.syn.Unlock()

	return &TestMessage{Text: "reply"}
}

// acceptorConn is a single connection to the server (there may be multiple
// outstanding at once).
type acceptorConn struct {
	cond     sync.Cond
	acceptor *Acceptor
	reply    connect.Message
}

func newAcceptorConn(syn sync.Locker, acceptor *Acceptor) *acceptorConn {
	return &acceptorConn{cond: sync.Cond{L: syn}, acceptor: acceptor}
}

func (c *acceptorConn) Send(ctx context.Context, request connect.Message) {
	// Wait for previous reply to be processed by Recv.
	if c.reply != nil {
		c.cond.Wait()
	}

	// Handle incoming request messages.
	switch t := request.(type) {
	case *TestMessage:
		c.reply = c.acceptor.handleTestMessage(t)
	}

	// Signal Recv goroutine that reply is ready.
	c.cond.Signal()
}

func (c *acceptorConn) Recv() connect.Message {
	// Wait for reply to be ready.
	if c.reply == nil {
		c.cond.Wait()
	}

	reply := c.reply
	c.reply = nil

	// Signal Send goroutine that it can process the next request.
	c.cond.Signal()

	return reply
}

func (c *acceptorConn) Close(ctx context.Context) {

}
