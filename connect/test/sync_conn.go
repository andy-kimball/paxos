package test

import (
	"context"
	"sync"

	"github.com/andy-kimball/paxos/connect"
)

// SyncConn wraps another connection in order to add syn locking to each method
// in the connect.Conn interface. Before each call to the inner connection,
// SyncConn acquires the syn lock and then releases it after each call. This
// makes connections easier to use during testing, since Lock/Unlock doesn't
// need to be explicitly invoked each time Send/Recv/Close calls are made. In
// production code, SyncConn is not necessary, since connections are used in a
// context where the syn lock has already been acquired. Usage example:
//
//   client := &test.SyncConn{}
//   ctx := context.Background()
//   defer client.Close(ctx, true /* drain */)
//   client.Inner = network.NewClient(&client.Syn, "a")
//   client.Send(ctx, &test.TestMessage{Text: text})
//
type SyncConn struct {
	Syn   sync.Mutex
	Inner connect.Conn
}

// Send is part of the the connect.Conn interface.
func (c *SyncConn) Send(ctx context.Context, msg connect.Message) {
	c.Syn.Lock()
	defer c.Syn.Unlock()
	c.Inner.Send(ctx, msg)
}

// Recv is part of the the connect.Conn interface.
func (c *SyncConn) Recv() connect.Message {
	c.Syn.Lock()
	defer c.Syn.Unlock()
	return c.Inner.Recv()
}

// Close is part of the the connect.Conn interface.
func (c *SyncConn) Close(ctx context.Context) {
	c.Syn.Lock()
	defer c.Syn.Unlock()
	c.Inner.Close(ctx)
}
