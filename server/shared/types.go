package shared 

import (
	"encoding/json"
	"fmt"
)

type Vec2 struct {
	X, Y int32
}

type TileDelta struct {
	Pos Vec2	`json:"pos"`
	Value uint8 `json:"tile_value"`
}

func (v Vec2) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]int32{v.X, v.Y})
}

func (v *Vec2) UnmarshalJSON(b []byte) error {
	var a []int32
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	if len(a) != 2 {
		return fmt.Errorf("Vec2: expected 2 elements, got %d", len(a))
	}
	v.X, v.Y = a[0], a[1]
	return nil
}

type MessageType uint32 

const (
	MsgRoundStart MessageType = 1
	MsgWorldState MessageType = 2
	MsgDeltaState MessageType = 3
	MsgRoundEnd   MessageType = 4
)

type RoundStartMessage struct {
	MessageType    MessageType `json:"message_type"`
	WorldSizeX     int32  `json:"world_size_x"`
	WorldSizeY     int32  `json:"world_size_y"`
	NumPlayers     int    `json:"num_players"`
	ResponseTimeMs int    `json:"response_time_ms"`
}

type DeltaStateMessage struct {
	MessageType  MessageType `json:"message_type"`
	CurrentTick  uint32      `json:"current_tick"`
	LocalPlayerX int32       `json:"local_player_x"`
	LocalPlayerY int32       `json:"local_player_y"`
	DeltaCount   uint32      `json:"delta_count"`
	DeltaUpdates []TileDelta `json:"delta_updates"`
	WorldHash    uint64      `json:"world_hash"`
}

type RoundOverMessage struct {
	MessageType  MessageType `json:"message_type"`
	Score	  uint32 `json:"score"`
	Placement uint32 `json:"placement"`
}

type SpectatorRoundInfoMessage struct {
	WorldSizeX     int    `json:"world_size_x"`
	WorldSizeY     int    `json:"world_size_y"`
	NumPlayers     int    `json:"num_players"`
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("json marshal failed: %v", err))
	}
	return data
}
