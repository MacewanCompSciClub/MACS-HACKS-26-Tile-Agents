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
	messages  chan Message 
	readError chan error
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
        readError: make(chan error, 1),
    }

    // Reader goroutine: only calls Read, never blocks on game logic,
    go func() {
        defer close(c.messages)
        for {
            typ, data, err := conn.Read(ctx)
            if err != nil {
                c.readError <- err
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
			if !ok { return Message{}, <-c.readError }
			return m, nil
		case <-ctx.Done():
			return Message{}, ctx.Err()
	}
}
