package main

import (
	"github.com/coder/websocket"
	"context"
	"time"
)

type Spectator struct {
	conn *websocket.Conn
	send   chan Message
	master bool
}

type PlayerSpectatorData struct {
	Pos Vec2	 `json:"pos"`
	Dead bool	 `json:"dead"`
	Score uint32 `json:"score"`
}

type SpectatorRoundInfoMessage struct {
	MessageType  MessageType `json:"message_type"`
	WorldSizeX     int32   `json:"world_size_x"`
	WorldSizeY     int32    `json:"world_size_y"`
	NumPlayers     int    `json:"num_players"`
	PlayerData []PlayerSpectatorData `json:"player_info"`
}

type SpectatorRoundDeltaMessage struct {
	MessageType  MessageType `json:"message_type"`
	CurrentTick  uint32      `json:"current_tick"`
	DeltaUpdates []TileDelta `json:"delta_updates"`
	PlayerData   []PlayerSpectatorData `json:"player_info"`
	WorldHash    uint32      `json:"world_hash"`
}

func (sp *Spectator) enqueue(msg Message) {
	select {
		case sp.send <- msg:
		default:
			sp.conn.CloseNow() // queue full: too slow, drop them
	}
}

func (sp *Spectator) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-sp.send:
			wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := sp.conn.Write(wctx, m.typ, m.data)
			cancel()
			if err != nil {
				sp.conn.CloseNow()
				return
			}
		}
	}
}

