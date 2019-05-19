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

package connect

import (
	"context"
	"sync"
)

// Message objects are sent and received over a connection. Messages can
// be a piece of data, a command, or anything else; to the connect package, they
// are all just messages to be sent from place to place. Messages can be any
// type of Go object, but the typical case is for them to be generated by
// protobuf. Some components may require the message to implement additional
// interfaces.
type Message interface{}

// Chunkable is implemented by messages that are one chunk in a larger sequence
// of messages. Callers can determine when the last message of a sequence has
// been sent or received by detecting when IsFinal becomes true. If a message
// sequence is canceled or encounters an error while chunking is in progress,
// then an ErrorMessage will interrupt as the final "chunk".
type Chunkable interface {
	// IsFinal is true when this message is the final chunk in a larger sequence
	// of messages.
	IsFinal() bool
}

// Conn represents a communication channel between components. Messages are sent
// and received using a familiar request/reply protocol. Each request must have
// a corresponding reply, except in the case where a connection terminates early
// due to calling Close, or due to an error that terminates the connection. Once
// a connection has returned an error, it will continue to return an error for
// any future requests.
//
// Unlike a protocol like traditional HTTP, requests and replies can be
// pipelined, meaning the next request can be sent before receiving the previous
// reply. This allows many requests to be outstanding at once. In addition,
// large requests and replies can be broken into multiple message chunks and
// fully streamed using the Chunkable interface.
//
// The syn lock must be acquired before calling any of the methods on Conn. In
// addition, the Send and Recv methods are not re-entrant (i.e. a second call
// to Send cannot happen until the first call is complete).
type Conn interface {
	// Send writes a new request message to the connection. The given context can
	// contain tracing information or other context that's tied to the request.
	// The context covers the entire request/reply lifecycle of the message. Once
	// the connection has been closed, Send is a no-op.
	Send(ctx context.Context, request Message)

	// Recv reads a reply to a previous request sent via the Send method. Note
	// that no context parameter is required, since it was provided to the Send
	// method instead. Once the connection has been closed, Recv returns nil.
	Recv() Message

	// Close shuts down the connection. This can only be called once other
	// goroutines are done sending or receiving.
	Close(ctx context.Context)
}

// Server offers a shared service that can be used concurrently by multiple
// clients. Each client is typically synchronized by a different syn lock.
//
// Implementations must be goroutine-safe.
type Server interface {
	// NewClient returns a new connection to the server that is synchronized by
	// the given syn lock. NewClient should never block; any long-running
	// connection establishment should occur lazily during the first call to the
	// Send method. If a connection error occurs, then it will be returned as
	// an ErrorMessage by the Recv method.
	//
	// The returned connection must always be closed. If this happens before
	// receiving a reply to the final request, it cancels any in-progress
	// requests.
	//
	// NewClient can be called concurrently by any number of goroutines.
	NewClient(syn sync.Locker) Conn
}

// Listener binds and unbinds servers to URIs. Only one server on the network
// may be bound to a particular URI at a time. A Router can now route incoming
// connection requests to the appropriate server.
//
// Implementations must be goroutine-safe.
type Listener interface {
	// Listen binds the given server to the given URI. The server is now routable,
	// meaning that remote clients can connect to it via that URI. If the server
	// parameter is nil, then the listener will unbind the server. The URI is now
	// again available for binding.
	//
	// NewClient can be called concurrently by any number of goroutines.
	Listen(ctx context.Context, uri string, server Server) error
}

// Router returns a new connection to the server listening on the given URI.
// Each connection is typically synchronized by a different syn lock. It works
// just like Server, but with the addition of the unique routing URI.
//
// Implementations must be goroutine-safe.
type Router interface {
	// NewClient returns a new connection to the server listening on the given
	// URI. The connection is synchronized by the given syn lock. NewClient
	// should never block; any long-running connection establishment should occur
	// lazily during the first call to the Send method. If a connection error
	// occurs, then it will be returned as an ErrorMessage by the Recv method.
	//
	// The returned connection must always be closed. If this happens before
	// receiving a reply to the final request, it cancels any in-progress
	// requests.
	//
	// NewClient can be called concurrently by any number of goroutines.
	NewClient(syn sync.Locker, uri string) Conn
}

// Network implements the Listener and Router interfaces. Listeners that
// register with a given Network instance will be discoverable by calling the
// Router method on that same instance.
type Network interface {
	Listener
	Router
}

// Error implements the error interface for ErrorMessage.
func (e *ErrorMessage) Error() string {
	return e.Text
}
