package main

import (
	"context"
	"log"
	"github.com/coder/websocket"
	"encoding/json"
	"errors"
	"fmt"
	"encoding/binary"
	"hash/crc32"
	"math/rand"
	"flag"
)

var DEFAULT_TOKEN = "9f3a7c1e"

type WorldState struct {
	w, h uint32 
	playerX, playerY uint32
	currentTick uint32
	tiles []uint8
}

func GetMessageType(m * Message) (MessageType, error) {
	switch m.typ {
		case websocket.MessageText:
			var head struct { MessageType MessageType `json:"message_type"` }
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

func deserializeRoundStartMessage(m Message) RoundStartMessage {
	var roundStartMSG RoundStartMessage
	mustUnmarshal(m.data, &roundStartMSG)
	return roundStartMSG
}

func deserializeWorldStateMessage(msg []byte, w, h uint32) []byte {
	const header uint32 = 1 + 4
	if len(msg) != int(header+w*h) {
		log.Panicf("short world state: got %d bytes, want %d", len(msg), header+w*h)
	}
	if MessageType(msg[0]) != MsgWorldState {
		log.Panicf("mismatched message type: got %d, want %d", msg[0], MsgWorldState)
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
			case MsgRoundStart:
				var roundStartMSG = deserializeRoundStartMessage(m)
				state.w = roundStartMSG.WorldSizeX
				state.h = roundStartMSG.WorldSizeY
				startReceived = true
			case MsgWorldState:
				if (startReceived) {
					state.tiles = deserializeWorldStateMessage(m.data, state.w, state.h)
					return state
				}
        }
    }
}

func applyTileDeltas(world * WorldState, tileDeltas []TileDelta, worldHash uint32) {
	for i := range tileDeltas {
		t := &tileDeltas[i]
		world.tiles[t.Pos.Y * int32(world.w) + t.Pos.X]= t.Value
	}

	var hash uint32 = crc32.ChecksumIEEE(world.tiles)
	if (hash != worldHash) { panic("WORLD HASH DOES NOT MATCH") }
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
			case MsgDeltaState:
				var d DeltaStateMessage
				if err := json.Unmarshal(m.data, &d); err == nil {
					log.Printf("Delta update received for tick %d Player pos: %d %d", d.CurrentTick, d.LocalPlayerX, d.LocalPlayerY)
					world.playerX = uint32(d.LocalPlayerX)
					world.playerY = uint32(d.LocalPlayerY)
					applyTileDeltas(world, d.DeltaUpdates, d.WorldHash)
				}
			case MsgRoundEnd:
				var d RoundOverMessage
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

func sendMoveToServer(ctx context.Context, client * Client, x uint32, y uint32, tick uint32) {
	data, err := json.Marshal(ClientMoveMessage{Tick: tick, MoveX: x, MoveY: y});
	if err != nil {
		panic(fmt.Sprintf("json marshal failed: %v", err))
	}
	log.Print("sending move")
	client.sendMessage(ctx, Message{typ: websocket.MessageText, data: data})
}

func isPosInBounds(x, y int32, world *WorldState) bool {
	return x >= 0 && y >= 0 && x < int32(world.w) && y < int32(world.h)
}

func getTileHealth(world *WorldState, pos Vec2) uint32 {
	if !isPosInBounds(pos.X, pos.Y, world) {
		panic("position not in world bounds")
	}
	return uint32(world.tiles[pos.Y*int32(world.w) + pos.X] & 0b00000111)
}

func getRandomMovePos(pos Vec2, world *WorldState) Vec2 {
	candidates := [4]Vec2{
		{X: pos.X - 1, Y: pos.Y}, // left
		{X: pos.X + 1, Y: pos.Y}, // right
		{X: pos.X, Y: pos.Y - 1}, // up
		{X: pos.X, Y: pos.Y + 1}, // down
	}

	var valid [4]Vec2
	n := 0
	for _, c := range candidates {
		if isPosInBounds(c.X, c.Y, world) {
			if (getTileHealth(world, c) > 1) {
				valid[n] = c
				n++
			}
		}
	}

	if n == 0 {
		return pos // boxed in: stay put
	}
	return valid[rand.Int() % n]
}

func main() {
	ctx := context.Background()

	token := flag.String("token", DEFAULT_TOKEN, "auth token for the game server")
	log.Printf("Starting client. (token: %s)", *token)
	flag.Parse()

	//var address string = "ws://localhost:8080/ws"
	var address string = "ws://hack.macewancsclub.ca:8080/ws"
	client, err := Connect(ctx, address, *token)
	if err != nil { log.Fatal(err) }
	defer client.conn.CloseNow()

	var worldState WorldState = waitWaitForRoundStart(ctx, client)
	log.Printf("game started world size: %d %d", worldState.w, worldState.h)

    for {
		// Get the latest server world update
		var roundOver bool = getNextWorldUpdate(ctx, client, &worldState)
		if (roundOver) { break; }

		// Calculate where to move 
		var movePos = getRandomMovePos(Vec2{int32(worldState.playerX), int32(worldState.playerY)}, &worldState) 
		// var movePos = Vec2{67,69}

		// Send move to the server
		sendMoveToServer(ctx, client, uint32(movePos.X), uint32(movePos.Y), worldState.currentTick)
	}

	log.Print("exiting")
}
