package main

import (
	"context"
	"log"
	"github.com/coder/websocket"
	"encoding/json"
	"example/hello/server/shared"
	"errors"
	"fmt"
)

type MessageType = shared.MessageType;

var TOKEN = "9f3a7c1e"

type WorldState struct {
	w, h int32 
}

func GetMessageType(m * Message) (MessageType, error) {
	switch m.typ {
		case websocket.MessageText:
			var head struct { MessageType shared.MessageType `json:"message_type"` }
			if err := json.Unmarshal(m.data, &head); err != nil {
				return 0, err
			}
			return head.MessageType, nil

		case websocket.MessageBinary:
			if len(m.data) < 1 { return 0, errors.New("empty binary message") }
			return MessageType(m.data[0]), nil

		default:
			return 0, fmt.Errorf("unexpected frame type %v", m.typ)
	}
}

func waitWaitForRoundStart(ctx context.Context, client * Client) WorldState {
	startReceived := false
    for {
		m, err := client.nextMessage(ctx)
		if err != nil {
			log.Fatal(err)
		}
		messageType, err := GetMessageType(&m)
		if err != nil { continue; }
		switch messageType {
			case shared.MsgRoundStart:
				log.Printf("got round start")
				startReceived = true
			case shared.MsgWorldState:
				log.Printf("got world state: %d bytes", len(m.data))
				if (startReceived) { return WorldState{0,0} }
        }
    }
}

func main() {
	ctx := context.Background()

	client, err := Connect(ctx, "ws://localhost:8080/ws", TOKEN)
	if err != nil { log.Fatal(err) }
	defer client.conn.CloseNow()

	worldState := waitWaitForRoundStart(ctx, client)
	log.Printf("game started world size: %d %d", worldState.w, worldState.h)

    for {
		m, err := client.nextMessage(ctx)
		if err != nil {
			log.Fatal(err)
		}

		messageType, err := GetMessageType(&m)
		if err != nil { log.Fatal(err); }
		switch messageType {
			case shared.MsgDeltaState: 
				var d shared.DeltaStateMessage
				if err := json.Unmarshal(m.data, &d); err == nil {
					log.Printf("Delta update received for tick %d", d.CurrentTick)
				}
			case shared.MsgRoundEnd:
				var d shared.RoundOverMessage
				if err := json.Unmarshal(m.data, &d); err == nil {
					log.Printf("Round over! placed: %d score: %d", d.Placement, d.Score)
				}
			default:
					log.Printf("fuck")

        }
    }
}
