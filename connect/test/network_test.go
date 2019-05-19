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

package test_test

import (
	"context"
	"testing"
	"time"

	"github.com/andy-kimball/paxos/connect"
	"github.com/andy-kimball/paxos/connect/test"
	"github.com/stretchr/testify/require"
)

func TestNetwork(t *testing.T) {
	ctx := context.Background()
	network := test.NewNetwork()
	server := test.NewServer()
	network.Listen(ctx, "a", server)

	// Wrap a new client in SyncConn so that the syn lock is acquired before each
	// call to the connect.Conn interface.
	client := &test.SyncConn{}
	client.Inner = network.NewClient(&client.Syn, "a")
	defer client.Close(ctx)

	conn := <-server.AcceptChan

	// Test different interesting interleavings.
	sendToServer(ctx, client, "request1")
	receiveAtServer(t, conn, "request1")
	sendToClient(conn, "reply1")
	receiveAtClient(t, client, "reply1")
	sendToServer(ctx, client, "request2")
	receiveAtServer(t, conn, "request2")
	sendToClient(conn, "reply2")
	receiveAtClient(t, client, "reply2")

	sendToServer(ctx, client, "request3")
	receiveAtServer(t, conn, "request3")
	sendToServer(ctx, client, "request4")
	receiveAtServer(t, conn, "request4")
	sendToClient(conn, "reply3")
	receiveAtClient(t, client, "reply3")
	sendToClient(conn, "reply4")
	receiveAtClient(t, client, "reply4")

	sendToServer(ctx, client, "request5")
	receiveAtServer(t, conn, "request5")
	sendToServer(ctx, client, "request6")
	sendToClient(conn, "reply5")
	receiveAtServer(t, conn, "request6")
	sendToClient(conn, "reply6")
	receiveAtClient(t, client, "reply5")
	receiveAtClient(t, client, "reply6")
}

// Ensure that artificial latency is added when sending messages to server in a
// different, linked network.
func TestNetworkLatency(t *testing.T) {
	// Simulate an east and west data center. Create server in east and client
	// in west.
	ctx := context.Background()
	east := test.NewNetworkWithLatency(time.Millisecond)
	eastServer := test.NewServer()
	east.Listen(ctx, "a", eastServer)
	west := east.NewLinkedNetwork()

	// Wrap a new client in SyncConn so that the syn lock is acquired before each
	// call to the connect.Conn interface.
	client := &test.SyncConn{}
	client.Inner = west.NewClient(&client.Syn, "a")
	defer client.Close(ctx)

	conn := <-eastServer.AcceptChan

	start := time.Now()
	sendToServer(ctx, client, "request")
	receiveAtServer(t, conn, "request")
	elapsed := time.Now().Sub(start)
	require.True(t, elapsed > time.Millisecond)
}

func sendToServer(ctx context.Context, client connect.Conn, text string) {
	client.Send(ctx, &test.TestMessage{Text: text})
}

func receiveAtServer(t *testing.T, conn *test.Conn, expected string) {
	request, ok := (<-conn.RequestChan).(*test.TestMessage)
	require.True(t, ok)
	require.Equal(t, expected, request.Text)
}

func sendToClient(conn *test.Conn, text string) {
	conn.ReplyChan <- &test.TestMessage{Text: text}
}

func receiveAtClient(t *testing.T, client connect.Conn, expected string) {
	reply, ok := client.Recv().(*test.TestMessage)
	require.True(t, ok)
	require.Equal(t, expected, reply.Text)
}
