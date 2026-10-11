package main

import (
	"log"
	"net/http"
	"github.com/coder/websocket"
	"sync"
	"time"
	"errors"
	"context"
	"encoding/json"
	"fmt"
	"encoding/binary"
	"example/hello/server/shared"
)

type Vec2 = shared.Vec2
type TileDelta = shared.TileDelta

type MessageType = shared.MessageType

const (
	MsgRoundStart = shared.MsgRoundStart
	MsgWorldState = shared.MsgWorldState
	MsgDeltaState = shared.MsgDeltaState
	MsgRoundEnd   = shared.MsgRoundEnd
)

type RoundStartMessage = shared.RoundStartMessage
type DeltaStateMessage = shared.DeltaStateMessage
type RoundOverMessage = shared.RoundOverMessage
type SpectatorRoundInfoMessage = shared.SpectatorRoundInfoMessage

type Role int

const (
	RolePlayer Role = iota
	RoleSpectator
)

type Server struct {
	mu      sync.Mutex
	conns   map[string]*Client  // everyone connected
	scores  map[string]uint32
	player_inputs  chan input
}

type Client struct {
	conn *websocket.Conn
	role Role // never changes after creation, so safe to read without the lock
	token string
}


type PlayerInstance struct {
	token string
	pos Vec2
	nextMovePos Vec2
	dead bool
	score uint32
}

type input struct {
	from *websocket.Conn
	data []byte
}

func NewServer() *Server {
	return &Server{
		conns:			make(map[string]*Client),
		player_inputs:  make(chan input, 64),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

func getRole(t string) Role {
	return RolePlayer;
}

func (s *Server) add_client(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.conns[c.token] = c
	log.Printf("connected role=%d (%d total)", c.role, len(s.conns))
	go s.runGame()
}

func (s *Server) remove_client(c *Client){
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.conns, c.token)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Println("accept:", err)
		return
	}
	defer conn.CloseNow()

	
	c := &Client{conn: conn, role: getRole(token), token: token}
	s.add_client(c)
	defer s.remove_client(c)

	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		select {
			case s.player_inputs <- input{from: conn, data: data}:
			default:
		}
	}
}


// Tries to find a client from a token.
func (s *Server) findClient(token string) (*Client, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conns[token]
	return c, ok
}

func (s *Server) initPlayerInstances(spawns []Vec2) []PlayerInstance {
	s.mu.Lock()
	defer s.mu.Unlock()

	var instances []PlayerInstance
	for _, c := range s.conns {
		if c.role != RolePlayer {
			continue // skip spectators
		}
		instances = append(instances, PlayerInstance{token: c.token})
	}

	if len(spawns) < len(instances) {
		panic(fmt.Sprintf("not enough spawn points: have %d, need %d", len(spawns), len(instances)))
	}

	for i := range instances {
		instances[i].pos = spawns[i]
		instances[i].nextMovePos = spawns[i]
	}

	return instances
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("json marshal failed: %v", err))
	}
	return data
}

func (s *Server) sendToPlayer(token string, typ websocket.MessageType, data []byte) error {
	c, ok := s.findClient(token)
	if !ok {
		return errors.New("player not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.conn.Write(ctx, typ, data)
}

func (s *Server) sendToAllPlayers(players []PlayerInstance, typ websocket.MessageType, data []byte) {
	for i := range players {
		p := &players[i]
		log.Printf("sending to token=%s pos=%+v", p.token, p.pos)
		if err := s.sendToPlayer(p.token, typ, data); err != nil {
			log.Printf("send to %s failed: %v", p.token, err)
		}
	}
}

func newRoundStartMessage(numPlayers int, responseTimeMS int) []byte {
	return mustMarshal(RoundStartMessage{
		MessageType:    MsgRoundStart,
		WorldSizeX:     8,
		WorldSizeY:     8,
		NumPlayers:     numPlayers,
		ResponseTimeMs: responseTimeMS,
	})
}

func newWorldStateMessage(world * WorldState, tick uint32, players []PlayerInstance) []byte {
	buf := make([]byte, 1+4+(world.w*world.h))
	buf[0] = uint8(MsgWorldState)
	binary.LittleEndian.PutUint32(buf[1:5], tick)
	copy(buf[5:], world.cells)
	return buf
}

func newDeltaStateMessage(tick uint32, localPlayerPos Vec2, tileDeltas []TileDelta, worldHash uint64) []byte {
	if tileDeltas == nil {
		tileDeltas = []TileDelta{} // marshals to [] instead of null
	}
	return mustMarshal(DeltaStateMessage{
		MessageType:  MsgDeltaState,
		CurrentTick:  tick,
		LocalPlayerX: localPlayerPos.X,
		LocalPlayerY: localPlayerPos.Y,
		DeltaCount:   uint32(len(tileDeltas)),
		DeltaUpdates: tileDeltas,
		WorldHash:    worldHash,
	})
}

func newRoundOverMessage(score uint32, placement uint32) []byte {
	return mustMarshal(RoundOverMessage{MessageType: MsgRoundEnd, Score:  score, Placement:  placement})
}

func updateTileDeltas(world * WorldState, tileDeltas []TileDelta) []TileDelta {
	tileDeltas = tileDeltas[:0]
	for v := range world.updated {
		tileDeltas = append(tileDeltas, TileDelta{v, world.cells[world.tileIndex(v)]})
	}
	clear(world.updated)

	return tileDeltas
}

func findPlayerAt(tilePos Vec2, players []PlayerInstance) *PlayerInstance {
	for i := range players {
		if players[i].pos == tilePos {
			return &players[i]
		}
	}
	panic(fmt.Sprintf("findPlayerAt: no player at tile %+v", tilePos))
}


func (s *Server) runGame() {
	log.Println("first player connected, starting in 5s")
	time.Sleep(3 * time.Second)

	log.Println("game started")
	responseTimeMS := 200
	const maxTileHealth uint8 = 7

	var spawnPoints = [4]Vec2{
		{2, 2},
		{2, 5},
		{5, 2},
		{5, 5},
	}
	// Initalize players with round robin spawn points.
	players := s.initPlayerInstances(spawnPoints[:])
	world := initWorld(Vec2{X:8, Y:8}, maxTileHealth, players[:])

	tileDeltas := make([]TileDelta, 0, 512) // len 0, capacity 64, one allocation up front

	// send initial world state

	roundStartMSG := newRoundStartMessage(len(players), responseTimeMS);
	initialWorldStateMSG := newWorldStateMessage(world, 1, players[:])
	log.Printf("sending game start and world state message")

	s.sendToAllPlayers(players[:], websocket.MessageText,   roundStartMSG)
	s.sendToAllPlayers(players[:], websocket.MessageBinary, initialWorldStateMSG)

	var currentTick uint32 = 1
	for {
		// Send delta update messages to all players
		for i := range players {
			p := &players[i]
			if (p.dead) { continue; }
			tileDeltas = updateTileDeltas(world, tileDeltas)
			deltaStateMSG := newDeltaStateMessage(currentTick, p.pos, tileDeltas[:], 67)
			s.sendToPlayer(p.token, websocket.MessageText, deltaStateMSG);
		}

		time.Sleep(time.Duration(responseTimeMS) * time.Millisecond)
		currentTick++
		// Process Input
		for i := range world.cells {
			tilePos := world.tilePosition(int32(i))
			tileHealth := getTileHealth(world.cells[i])
			playerOnTile := isPlayerOnTile(world.cells[i])
			if ((tileHealth < maxTileHealth && tileHealth > 0) || (tileHealth == maxTileHealth && playerOnTile)) {
				tileHealth -= 1
				// check if player is on tile and kill it
				if (playerOnTile && tileHealth == 0) {
					var p *PlayerInstance = findPlayerAt(tilePos, players);
					p.dead = true;
					playerOnTile = false;
					s.sendToPlayer(p.token, websocket.MessageText, newRoundOverMessage(p.score, 1))
				}
				world.updateTile(tilePos, makeTile(tileHealth, playerOnTile));
			}
		}

		// Add 1 to each alive players score.
		for i := range players { if (!players[i].dead) { players[i].score += 1; } }
		// Update World
		// - check for player deaths and game over
	}

	log.Printf("sending update for tick 1")

}
