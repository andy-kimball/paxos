package connect

import (
	"context"
	"sync"
)

// ErrorClient is a trivial implementation of the Client interface that replies
// to any message with the configured error message.
type ErrorClient struct {
	err     *ErrorMessage
	syn     sync.Locker
	replies chan Message
	close   chan struct{}
}

// NewErrorClient returns a new instance of the error client that replies with
// the given error message and is synchronized by the given syn lock.
func NewErrorClient(err *ErrorMessage, syn sync.Locker) *ErrorClient {
	return &ErrorClient{
		err:     err,
		syn:     syn,
		replies: make(chan Message),
		close:   make(chan struct{}),
	}
}

// Send is part of the the connect.Conn interface.
func (c *ErrorClient) Send(ctx context.Context, msg Message) {
	// Unlock "syn" so that another goroutine can call Recv even if this
	// goroutine is blocked writing to the c.replies channel.
	c.syn.Unlock()
	defer c.syn.Lock()

	select {
	case c.replies <- c.err:
	case <-c.close:
	}
}

// Recv is part of the the connect.Conn interface.
func (c *ErrorClient) Recv() Message {
	// Unlock "syn" so that another goroutine can call Send even if this
	// goroutine is blocked reading the c.replies channel.
	c.syn.Unlock()
	defer c.syn.Lock()

	var reply Message
	select {
	case reply = <-c.replies:
	case <-c.close:
		reply = &ErrorMessage{Text: "server is closed"}
	}

	return reply
}

// Close is part of the the connect.Conn interface.
func (c *ErrorClient) Close(ctx context.Context) {
	close(c.close)
}
