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
	"hash/crc32"
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

type Role int

const (
	RolePlayer Role = iota
	RoleSpectator
	RoleMasterSpectator
)

type Server struct {
	mu      sync.Mutex
	auth    *authTable
	clients map[string]*Client  // All players connected. Keyed by token.
	spectators []*Spectator
	spectatorView SpectatorView
	scores  map[string]uint32
	clientMessages chan ClientMessage
}

type SpectatorView struct {
	roundInProgress  bool
	roundInfoMSG	 Message // sent only during a round
	worldSnapshotMSG Message
}

type Client struct {
	conn *websocket.Conn
	token string
}

type ClientMessage struct {
	message Message
	token string
}

type Message struct {
	typ websocket.MessageType
	data []byte
}

type PlayerInstance struct {
	token string
	pos Vec2
	nextMovePos Vec2
	dead bool
	score uint32
}

type Move struct {
	playerIdx uint32
	pos Vec2
}

func NewServer(auth * authTable) *Server {
	return &Server{
		auth: auth,
		clients:		make(map[string]*Client),
		clientMessages: make(chan ClientMessage, 64),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

func (s *Server) add_client(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clients[c.token] = c
	log.Printf("connected client. (%d total)", len(s.clients))
	go s.runGame()
}

func (server *Server) add_spectator(spectator * Spectator) {
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.spectatorView.roundInProgress {
		spectator.enqueue(server.spectatorView.roundInfoMSG)
		// catchup.
	}

	server.spectators = append(server.spectators, spectator)
	log.Printf("connected spectator. (%d total)", len(server.spectators))
}

func (s *Server) remove_client(c *Client){
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clients, c.token)
}

func (s *Server) remove_spectator(spectator * Spectator) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, x := range s.spectators {
		if x == spectator {
			last := len(s.spectators) - 1
			s.spectators[i] = s.spectators[last] // swap with the last element
			s.spectators[last] = nil             // let the GC reclaim it
			s.spectators = s.spectators[:last]
			return
		}
	}
}

var acceptOpts = &websocket.AcceptOptions{
	OriginPatterns: []string{"yoursite.com", "localhost:*"},
}

func (s *Server) handlePlayer(w http.ResponseWriter, r *http.Request, token string) {
	conn, err := websocket.Accept(w, r, acceptOpts)
	if err != nil {
		log.Println("accept:", err)
		return
	}
	defer conn.CloseNow()

	c := &Client{conn: conn, token: token}
	s.add_client(c)
	defer s.remove_client(c)

	for {
		typ, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		select {
		case s.clientMessages <- ClientMessage{token: token , message: Message{typ, data}}:
			default:
		}
	}
}

func (server *Server) handleSpectator(w http.ResponseWriter, r *http.Request, masterSpectator bool) {
	conn, err := websocket.Accept(w, r, acceptOpts)
	if err != nil {
		log.Println("accept:", err)
		return
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	s := &Spectator{conn: conn, send: make(chan Message, 32), master: masterSpectator}
	server.add_spectator(s)
	defer server.remove_spectator(s)

	if !masterSpectator {
		ctx = conn.CloseRead(ctx)
		s.writeLoop(ctx)
		return
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	role, ok := s.auth.getRole(token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		log.Printf("rejected connection: invalid token %q from %s", token, r.RemoteAddr)
		return
	}

	switch role {
		case RolePlayer:
			s.handlePlayer(w, r, token)
		case RoleSpectator:
			s.handleSpectator(w, r, false)
		case RoleMasterSpectator:
			s.handleSpectator(w, r, true)
	}
}


// Tries to find a client from a token.
func (s *Server) findClient(token string) (*Client, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.clients[token]
	return c, ok
}

func (s *Server) initPlayerInstances(spawns []Vec2) []PlayerInstance {
	s.mu.Lock()
	defer s.mu.Unlock()

	var instances []PlayerInstance
	for _, c := range s.clients {
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

func (s *Server) sendToSpectators(message Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, spectator := range s.spectators {
		spectator.enqueue(message)
	}
}

func newRoundStartMessage(numPlayers int, responseTimeMS int, worldSizeX uint32, worldSizeY uint32) []byte {
	return mustMarshal(RoundStartMessage{
		MessageType:    MsgRoundStart,
		WorldSizeX:     worldSizeX,
		WorldSizeY:     worldSizeY,
		NumPlayers:     numPlayers,
		ResponseTimeMs: responseTimeMS,
	})
}

func newWorldStateMessage(world * WorldState, tick uint32) []byte {
	buf := make([]byte, 1+4+(world.w*world.h))
	buf[0] = uint8(MsgWorldState)
	binary.LittleEndian.PutUint32(buf[1:5], tick)
	copy(buf[5:], world.cells)
	return buf
}

func newWorldSnapshotMessage(world * WorldState, tick uint32) Message {
	buf := make([]byte, 1+4+(world.w*world.h))
	buf[0] = uint8(MsgWorldState)
	binary.LittleEndian.PutUint32(buf[1:5], tick)
	copy(buf[5:], world.cells)
	return Message{websocket.MessageBinary, buf}
}

func newDeltaStateMessage(tick uint32, localPlayerPos Vec2, tileDeltas []TileDelta, worldHash uint32) []byte {
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

func newSpectatorRoundInfoMessage(world * WorldState, players []PlayerInstance) Message {
	var playerData = spectatorPlayerInfoSnapshot(players)

	var roundInfo = mustMarshal(SpectatorRoundInfoMessage{
				MessageType:    MsgRoundStart,
				WorldSizeX:     world.w,
				WorldSizeY:     world.h,
				NumPlayers:     len(players),
				PlayerData:		playerData,
	})
	return Message{websocket.MessageText, roundInfo}
}

func newSpectatorRoundDeltaMessage(world * WorldState, tick uint32, tileDeltas []TileDelta,worldHash uint32, players []PlayerInstance) Message {
	var playerData = spectatorPlayerInfoSnapshot(players)

	var roundInfo = mustMarshal(SpectatorRoundDeltaMessage{
				MessageType:    MsgDeltaState,
				CurrentTick:    tick,
				DeltaUpdates:   tileDeltas,
				PlayerData:		playerData,
				WorldHash:		worldHash,
	})
	return Message{websocket.MessageText, roundInfo}
}

func updateTileDeltas(world * WorldState, tileDeltas []TileDelta) []TileDelta {
	tileDeltas = tileDeltas[:0]
	for v := range world.updated {
		tileDeltas = append(tileDeltas, TileDelta{v, world.cells[world.tileIndex(v)]})
	}
	clear(world.updated)

	return tileDeltas
}

func spectatorPlayerInfoSnapshot(players []PlayerInstance) []PlayerSpectatorData {
	out := make([]PlayerSpectatorData, len(players))
	for i := range players {
		p := &players[i]
		out[i] = PlayerSpectatorData{
			Pos: p.pos,
			Score: p.score,
			Dead:  p.dead,
		}
	}
	return out
}

func (s * Server) updateSpectatorView(world * WorldState, tick uint32, players []PlayerInstance, tileDeltas []TileDelta, worldHash uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spectatorView.roundInProgress = true

	var playerData = spectatorPlayerInfoSnapshot(players)

	var roundInfo = mustMarshal(SpectatorRoundInfoMessage{
				MessageType:    MsgRoundStart,
				WorldSizeX:     world.w,
				WorldSizeY:     world.h,
				NumPlayers:     len(players),
				PlayerData:		playerData,
	})

	s.spectatorView.roundInfoMSG = Message{websocket.MessageText, roundInfo}
	s.spectatorView.worldSnapshotMSG = newWorldSnapshotMessage(world, tick)
}

func findPlayerAt(tilePos Vec2, players []PlayerInstance) *PlayerInstance {
	for i := range players {
		if players[i].pos == tilePos {
			return &players[i]
		}
	}
	panic(fmt.Sprintf("findPlayerAt: no player at tile %+v", tilePos))
}

func getPlayerWithToken(players []PlayerInstance, token string) (bool, uint32) {
	for i := range players {
		if players[i].token == token{
			return true, uint32(i)
		}
	}
	return false, 0
}

func (s * Server) getValidPlayerMoves(moves []Move, world * WorldState, players []PlayerInstance, currentTick uint32) []Move {
	moves = moves[:0]
    for {
        select {
			case msg := <-s.clientMessages:
				// Check if message is from player in game.
				ok, idx := getPlayerWithToken(players, msg.token)
				if (!ok) {
					log.Printf("ignoring message from player not in game (token: %s)", msg.token)
					continue;
				}
				if players[idx].dead {
					log.Printf("ignoring message from dead player (token: %s)", msg.token)
					continue;
				}

				if (msg.message.typ != websocket.MessageText) {
					log.Printf("ignoring non text message (token: %s)", msg.token)
					continue;
				}
				var moveMessage shared.ClientMoveMessage
				if err := json.Unmarshal(msg.message.data, &moveMessage); err != nil {
					log.Printf("failed to unmarshal move message (token: %s)", msg.token)
					continue;
				}
				var movePos = Vec2{X: int32(moveMessage.MoveX), Y: int32(moveMessage.MoveY)}
				if (!world.isValidMovePos(movePos)) {
					log.Printf("invalid move pos %v from player (token: %s)", movePos, msg.token)
					continue;
				}
				log.Printf("got a move pos %v from player (token: %s)", movePos, msg.token)
				moves = append(moves, Move{playerIdx: idx, pos: movePos})
			default:
				return moves
        }
    }
}

func applyMove(move Move, world * WorldState, players []PlayerInstance) {
	var fromPos = players[move.playerIdx].pos
	var fromTile = world.cells[world.tileIndex(players[move.playerIdx].pos)]
	var toTile = world.cells[world.tileIndex(move.pos)]
	// Assert that there is actually a player where we are moving from.
	if (!isPlayerOnTile(fromTile)) {
		panic(fmt.Sprintf("player from tile mismatch. The tile the player is moving from doesn't canonically have a player on it. Player: %v Tile: %v", players[move.playerIdx].pos,fromPos))
	}
	// Double check the move is valid.
	if (!(getTileHealth(toTile) > 0)) {
		panic(fmt.Sprintf("player trying to move to destroyed tile: %v", move.pos))
	}
	world.updateTile(fromPos,  fromTile & 0b0111);
	world.updateTile(move.pos, toTile   | 0b1000);
	players[move.playerIdx].pos = move.pos;
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
	world := initWorld(Vec2{X:64, Y:64}, maxTileHealth, players[:])

	tileDeltas := make([]TileDelta, 0, 512) // len 0, capacity 64, one allocation up front
	playerMoves := make([]Move, 0, len(players))

	// send initial world state
	roundStartMSG := newRoundStartMessage(len(players), responseTimeMS, uint32(world.h), uint32(world.w));
	initialWorldStateMSG := newWorldStateMessage(world, 1)
	log.Printf("sending game start and world state message")

	// send world state to spectators
	s.sendToSpectators(newSpectatorRoundInfoMessage(world, players))
	s.sendToSpectators(newWorldSnapshotMessage(world, 1))

	s.sendToAllPlayers(players[:], websocket.MessageText,   roundStartMSG)
	s.sendToAllPlayers(players[:], websocket.MessageBinary, initialWorldStateMSG)

	var currentTick uint32 = 1
	for {
		// Send delta update messages to all players
		var worldHash uint32 = crc32.ChecksumIEEE(world.cells)

		tileDeltas = updateTileDeltas(world, tileDeltas)
		for i := range players {
			p := &players[i]
			if (p.dead) { continue; }
			deltaStateMSG := newDeltaStateMessage(currentTick, p.pos, tileDeltas[:], worldHash)
			s.sendToPlayer(p.token, websocket.MessageText, deltaStateMSG);
		}

		s.sendToSpectators(newSpectatorRoundDeltaMessage(world, currentTick, tileDeltas, worldHash, players[:]))
		s.updateSpectatorView(world, currentTick, players, tileDeltas, worldHash)
		time.Sleep(time.Duration(responseTimeMS) * time.Millisecond)
		currentTick++

		// Process Input
		playerMoves = s.getValidPlayerMoves(playerMoves, world, players, currentTick)

		// Apply moves
		for i := range playerMoves {
			applyMove(playerMoves[i], world, players)
		}

		// update world.
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
