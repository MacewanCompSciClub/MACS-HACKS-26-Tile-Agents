package main

import (
	"context"
	"github.com/coder/websocket"
)

type Message struct {
	typ websocket.MessageType
	data []byte
}

type Client struct {
	conn * websocket.Conn
	messages chan Message 
	err      error
}

func Connect(ctx context.Context, url string, token string) (*Client, error) {
    fullURL := url + "?token=" + token
    conn, _, err := websocket.Dial(ctx, fullURL, nil)
    if err != nil {
        return nil, err
    }
	// Must set this to receive the entire inital world state.
    conn.SetReadLimit(50000)

    c := &Client{
        conn:      conn,
        messages:  make(chan Message, 1024),
    }

    // Reader goroutine: only calls Read, never blocks on game logic,
    go func() {
        defer close(c.messages)
        for {
            typ, data, err := conn.Read(ctx)
            if err != nil {
                c.err = err
                return
            }
            c.messages <- Message{typ, data}
        }
    }()

    return c, nil
}

// next blocks until the next message arrives
func (c *Client) nextMessage(ctx context.Context) (Message, error) {
	select {
		case m, ok := <-c.messages:
			if !ok { return Message{}, c.err }
			return m, nil
		case <-ctx.Done():
			return Message{}, ctx.Err()
	}
}

// Non blocking returns false if no new messages.
func (c * Client) pollMessage() (Message, bool, error) {
	select {
		case m, ok := <-c.messages:
			if !ok { return Message{}, false, c.err }
			return m, true, nil
		default:
			return Message{}, false, nil
	}
}

func (c * Client) nextMessages(ctx context.Context) ([]Message, error) {
	var messages []Message
	
	m, err := c.nextMessage(ctx)
	if (err != nil) { return nil, err }
	messages = append(messages, m)

	for {
		m, ok, err := c.pollMessage()
		if err != nil {
			// Connection closed mid-drain. Deliver what we already pulled
			// out; the next call will return c.err.
			return messages, nil
		}
		if !ok {
			return messages, nil
		}
		messages = append(messages, m)
	}
}
