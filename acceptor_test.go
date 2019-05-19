package paxos

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/andy-kimball/paxos/connect/test"
)

func TestAcceptor(t *testing.T) {
	ctx := context.Background()

	// Create acceptor.
	acceptor := &Acceptor{}

	// Create client of acceptor.
	client := test.SyncConn{}
	client.Inner = acceptor.NewClient(&client.Syn)
	defer client.Close(ctx)

	// Send message to acceptor and receive reply.
	client.Send(ctx, &TestMessage{Text: "request"})
	reply := client.Recv().(*TestMessage)

	// Verify reply.
	require.Equal(t, "reply", reply.Text)
}
