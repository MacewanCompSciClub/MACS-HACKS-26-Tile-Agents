package main

type MessageType uint32 

const (
	MsgRoundStart MessageType = 1
	MsgWorldState MessageType = 2
	MsgDeltaState MessageType = 3
	MsgRoundEnd   MessageType = 4
)

type RoundStartMessage struct {
	MessageType    MessageType `json:"message_type"`
	WorldSizeX     int         `json:"world_size_x"`
	WorldSizeY     int         `json:"world_size_y"`
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
	Score	  uint32 `json:"score"`
	Placement uint32 `json:"placement"`
}

type SpectatorRoundInfoMessage struct {
	WorldSizeX     int    `json:"world_size_x"`
	WorldSizeY     int    `json:"world_size_y"`
	NumPlayers     int    `json:"num_players"`
}
