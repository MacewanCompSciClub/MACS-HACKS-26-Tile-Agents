package main

import (
	"context"
	"log"
	"github.com/coder/websocket"
	"encoding/json"
	"example/hello/server/shared"
	"errors"
	"fmt"
	"encoding/binary"
	"hash/crc32"
)

type MessageType = shared.MessageType;

var TOKEN = "9f3a7c1e"

type WorldState struct {
	w, h uint32 
	playerX, playerY uint32
	currentTick uint32
	tiles []uint8
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

func mustUnmarshal(data []byte, d any) {
	if err := json.Unmarshal(data, &d); err != nil {
		panic(fmt.Sprintf("json unmarshal failed: %v", err))
	}
}

func deserializeRoundStartMessage(m Message) shared.RoundStartMessage {
	var roundStartMSG shared.RoundStartMessage
	mustUnmarshal(m.data, &roundStartMSG)
	return roundStartMSG
}

func deserializeWorldStateMessage(msg []byte, w, h uint32) []byte {
	const header uint32 = 1 + 4
	if len(msg) != int(header+w*h) {
		log.Panicf("short world state: got %d bytes, want %d", len(msg), header+w*h)
	}
	if MessageType(msg[0]) != shared.MsgWorldState {
		log.Panicf("mismatched message type: got %d, want %d", msg[0], shared.MsgWorldState)
	}
	var tick = binary.LittleEndian.Uint32(msg[1:5])
	if (tick != 1) {
		log.Panicf("wrong world state tick: got %d, want 1", tick)
	}

	return msg[5 : 5+w*h]
}

func waitWaitForRoundStart(ctx context.Context, client * Client) WorldState {
	startReceived := false
	var state WorldState
    for {
		m, err := client.nextMessage(ctx)
		if err != nil {
			log.Fatal(err)
		}

		messageType, err := GetMessageType(&m)
		if err != nil { continue; }

		switch messageType {
			case shared.MsgRoundStart:
				var roundStartMSG = deserializeRoundStartMessage(m)
				state.w = roundStartMSG.WorldSizeX
				state.h = roundStartMSG.WorldSizeY
				startReceived = true
			case shared.MsgWorldState:
				if (startReceived) {
					state.tiles = deserializeWorldStateMessage(m.data, state.w, state.h)
					return state
				}
        }
    }
}

func applyTileDeltas(world * WorldState, tileDeltas []shared.TileDelta, worldHash uint32) {
	for i := range tileDeltas {
		t := &tileDeltas[i]
		world.tiles[t.Pos.Y + t.Pos.X * int32(world.w)] = t.Value
	}

	var hash uint32 = crc32.ChecksumIEEE(world.tiles)
	if (hash != worldHash) { panic("HASH DOES NOT MATCH") }
}

func getNextWorldUpdate(ctx context.Context, client * Client, world * WorldState) bool {
	var messages, err = client.nextMessages(ctx) 
	if err != nil { log.Fatal(err) }

	for i := range messages {
			m := &messages[i]

			messageType, err := GetMessageType(m)
			if err != nil {
				log.Fatal(err)
			}
			switch messageType {
			case shared.MsgDeltaState:
				var d shared.DeltaStateMessage
				if err := json.Unmarshal(m.data, &d); err == nil {
					log.Printf("Delta update received for tick %d", d.CurrentTick)
					applyTileDeltas(world, d.DeltaUpdates, d.WorldHash)
				}
			case shared.MsgRoundEnd:
				var d shared.RoundOverMessage
				if err := json.Unmarshal(m.data, &d); err == nil {
					log.Printf("Round over! placed: %d score: %d", d.Placement, d.Score)
					return true
				}
			default:
				log.Print("unexpected message type received in game loop...")
			}
	}
	return false
}

func sendMoveToServer(x uint32, y uint32, tick uint32) {

}

func main() {
	ctx := context.Background()

	client, err := Connect(ctx, "ws://localhost:8080/ws", TOKEN)
	if err != nil { log.Fatal(err) }
	defer client.conn.CloseNow()

	var worldState WorldState = waitWaitForRoundStart(ctx, client)
	log.Printf("game started world size: %d %d", worldState.w, worldState.h)

    for {
		// Get the latest server world update
		var roundOver bool = getNextWorldUpdate(ctx, client, &worldState)
		if (roundOver) { break; }

		// Calculate where to move 
		var moveX, moveY uint32 = worldState.playerX, worldState.playerY

		// Send move to the server
		sendMoveToServer(moveX, moveY, worldState.currentTick)
	}

	log.Print("exiting")
}
