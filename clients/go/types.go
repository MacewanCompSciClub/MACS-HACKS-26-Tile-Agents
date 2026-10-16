package main 

import (
	"encoding/json"
	"fmt"
)

type Vec2 struct {
	X, Y int32
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

type TileDelta struct {
	Pos	  Vec2	`json:"pos"`
	Value uint8 `json:"tile_value"`
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
	WorldSizeX     uint32	   `json:"world_size_x"`
	WorldSizeY     uint32	   `json:"world_size_y"`
	NumPlayers     int		   `json:"num_players"`
	ResponseTimeMs int         `json:"response_time_ms"`
}

type DeltaStateMessage struct {
	MessageType  MessageType `json:"message_type"`
	CurrentTick  uint32      `json:"current_tick"`
	LocalPlayerX int32       `json:"local_player_x"`
	LocalPlayerY int32       `json:"local_player_y"`
	DeltaCount   uint32      `json:"delta_count"`
	DeltaUpdates []TileDelta `json:"delta_updates"`
	WorldHash    uint32      `json:"world_hash"`
}

type ClientMoveMessage struct {
	Tick  uint32 `json:"tick"`
	MoveX uint32 `json:"move_x"`
	MoveY uint32 `json:"move_y"`
}

type RoundOverMessage struct {
	MessageType  MessageType `json:"message_type"`
	Score	  	 uint32		 `json:"score"`
	Placement	 uint32	     `json:"placement"`
}
