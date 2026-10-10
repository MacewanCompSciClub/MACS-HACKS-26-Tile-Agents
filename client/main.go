package main

import (
	"context"
	"log"

	"github.com/coder/websocket"
)

func main() {
	ctx := context.Background()

	c, _, err := websocket.Dial(ctx, "ws://localhost:8080/ws?token=9f3a7c1e", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer c.CloseNow()

	for {
		typ, msg, err := c.Read(ctx)
		if err != nil {
			log.Fatal(err)
		}

		switch typ {
		case websocket.MessageText:
			log.Printf("received text: %s", msg)
		case websocket.MessageBinary:
			log.Printf("received binary: %d bytes", len(msg))
		}
	}
}
