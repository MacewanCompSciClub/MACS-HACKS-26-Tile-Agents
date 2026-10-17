package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Role int

const (
	RolePlayer Role = iota
	RoleSpectator
	RoleMasterSpectator
)

type Server struct {
	mu               sync.Mutex
	auth             *authTable
	clients          map[string]*Client // All players connected. Keyed by token.
	spectators       []*Spectator
	spectatorView    SpectatorView
	scores           map[string]uint32
	clientMessages   chan ClientMessage
	roundInProgresss bool
}

type SpectatorView struct {
	roundInProgress  bool
	roundInfoMSG     Message // sent only during a round
	worldSnapshotMSG Message
}

type Client struct {
	conn  *websocket.Conn
	token string
}

type ClientMessage struct {
	message Message
	token   string
}

type Message struct {
	typ  websocket.MessageType
	data []byte
}

type PlayerInstance struct {
	token       string
	pos         Vec2
	nextMovePos Vec2
	dead        bool
	score       uint32
	name		string
}

type Move struct {
	playerIdx uint32
	pos       Vec2
}

func NewServer(auth *authTable) *Server {
	return &Server{
		auth:           auth,
		clients:        make(map[string]*Client),
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
	if !s.roundInProgresss {
		s.roundInProgresss = true
		go s.runGame()
	}
}

func (server *Server) add_spectator(spectator *Spectator) {
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.spectatorView.roundInProgress {
		spectator.enqueue(server.spectatorView.roundInfoMSG)
		spectator.enqueue(server.spectatorView.worldSnapshotMSG)
	}

	server.spectators = append(server.spectators, spectator)
	log.Printf("connected spectator. (%d total)", len(server.spectators))
}

func (s *Server) remove_client(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.clients, c.token)
}

func (s *Server) remove_spectator(spectator *Spectator) {
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
	OriginPatterns: []string{"*.macewancsclub.ca", "macewancsclub.ca", "localhost:*"},
}

func (s *Server) handlePlayer(w http.ResponseWriter, r *http.Request, token string) {
	conn, err := websocket.Accept(w, r, acceptOpts)
	if err != nil {
		log.Println("accept:", err)
		return
	}
	defer conn.CloseNow()

	log.Printf("Accepting connection from token: %s", token)

	c := &Client{conn: conn, token: token}
	s.add_client(c)
	defer s.remove_client(c)

	for {
		typ, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		select {
		case s.clientMessages <- ClientMessage{token: token, message: Message{typ, data}}:
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
		instances = append(instances,
						   PlayerInstance{token: c.token, name: s.auth.playerByToken[c.token]})
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

func (s *Server) sendToAllPlayers(players []PlayerInstance, msg Message) {
	for i := range players {
		p := &players[i]
		log.Printf("sending to token=%s pos=%+v", p.token, p.pos)
		if err := s.sendToPlayer(p.token, msg.typ, msg.data); err != nil {
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

func newRoundStartMessage(numPlayers int, responseTimeMS int, worldSizeX uint32, worldSizeY uint32) Message {
	return Message{websocket.MessageText,
		mustMarshal(RoundStartMessage{
			MessageType:    MsgRoundStart,
			WorldSizeX:     worldSizeX,
			WorldSizeY:     worldSizeY,
			NumPlayers:     numPlayers,
			ResponseTimeMs: responseTimeMS})}
}

func newWorldSnapshotMessage(world *WorldState, tick uint32) Message {
	buf := make([]byte, 1+4+(world.w*world.h))
	buf[0] = uint8(MsgWorldState)
	binary.LittleEndian.PutUint32(buf[1:5], tick)
	copy(buf[5:], world.latestSnapshot())
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
	return mustMarshal(RoundOverMessage{MessageType: MsgRoundEnd, Score: score, Placement: placement})
}

func newSpectatorRoundInfoMessage(world *WorldState, players []PlayerInstance) Message {
	var playerData = spectatorPlayerInfoSnapshot(players)

	var roundInfo = mustMarshal(SpectatorRoundInfoMessage{
		MessageType: MsgRoundStart,
		WorldSizeX:  world.w,
		WorldSizeY:  world.h,
		NumPlayers:  len(players),
		PlayerData:  playerData,
	})
	return Message{websocket.MessageText, roundInfo}
}

func newSpectatorRoundDeltaMessage(world *WorldState, tick uint32, tileDeltas []TileDelta, worldHash uint32, players []PlayerInstance) Message {
	var playerData = spectatorPlayerInfoSnapshot(players)

	var roundInfo = mustMarshal(SpectatorRoundDeltaMessage{
		MessageType:  MsgDeltaState,
		CurrentTick:  tick,
		DeltaUpdates: tileDeltas,
		PlayerData:   playerData,
		WorldHash:    worldHash,
	})
	return Message{websocket.MessageText, roundInfo}
}

func updateTileDeltas(world *WorldState, tileDeltas []TileDelta) []TileDelta {
	tileDeltas = tileDeltas[:0]

	a, b := world.snapshotBufs[0], world.snapshotBufs[1]
	for i := range a {
		if a[i] != b[i] {
			tileDeltas = append(tileDeltas, TileDelta{world.tilePosition(int32(i)), world.snapshotBufs[world.front^1][i]})
		}
	}

	return tileDeltas
}

func spectatorPlayerInfoSnapshot(players []PlayerInstance) []PlayerSpectatorData {
	out := make([]PlayerSpectatorData, len(players))
	for i := range players {
		p := &players[i]
		out[i] = PlayerSpectatorData{
			Pos:   p.pos,
			Score: p.score,
			Dead:  p.dead,
			Name:  p.name,
		}
	}
	return out
}

func (s *Server) updateSpectatorView(world *WorldState, tick uint32, players []PlayerInstance, tileDeltas []TileDelta, worldHash uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spectatorView.roundInProgress = true

	var playerData = spectatorPlayerInfoSnapshot(players)

	var roundInfo = mustMarshal(SpectatorRoundInfoMessage{
		MessageType: MsgRoundStart,
		WorldSizeX:  world.w,
		WorldSizeY:  world.h,
		NumPlayers:  len(players),
		PlayerData:  playerData,
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
		if players[i].token == token {
			return true, uint32(i)
		}
	}
	return false, 0
}

func (s *Server) LogSpectator(format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	log.Print(text)
	s.sendToSpectators(newSpectatorLogMessage(text))
}

func (s *Server) getValidPlayerMoves(moves []Move, world *WorldState, players []PlayerInstance, currentTick uint32) []Move {
	moves = moves[:0]
	for {
		select {
		case msg := <-s.clientMessages:
			// Check if message is from player in game.
			ok, idx := getPlayerWithToken(players, msg.token)
			if !ok {
				log.Printf("ignoring message from player not in game (token: %s)", msg.token)
				continue
			}
			if players[idx].dead {
				log.Printf("ignoring message from dead player (token: %s)", msg.token)
				continue
			}

			if msg.message.typ != websocket.MessageText {
				log.Printf("ignoring non text message (token: %s)", msg.token)
				continue
			}
			var moveMessage ClientMoveMessage
			if err := json.Unmarshal(msg.message.data, &moveMessage); err != nil {
				log.Printf("failed to unmarshal move message (token: %s)", msg.token)
				continue
			}
			var movePos = Vec2{X: int32(moveMessage.MoveX), Y: int32(moveMessage.MoveY)}
			if !world.isValidMovePos(players[idx].pos, movePos) {
				s.LogSpectator("invalid move pos %v from player at %v (token: %s)",
					movePos, players[idx].pos, msg.token)
				continue
			}
			log.Printf("got a move pos %v from player (token: %s)", movePos, msg.token)
			moves = append(moves, Move{playerIdx: idx, pos: movePos})
		default:
			return moves
		}
	}
}

func applyMove(move Move, world *WorldState, players []PlayerInstance) {
	players[move.playerIdx].pos = move.pos
}

// sharedPositions returns position -> indices of players at that position,
// only for positions held by 2+ players.
func getPlayersOnSameTile(players []PlayerInstance) map[Vec2][]int {
	groups := make(map[Vec2][]int)
	for i, p := range players {
		groups[p.pos] = append(groups[p.pos], i)
	}
	for pos, idxs := range groups {
		if len(idxs) < 2 {
			delete(groups, pos)
		}
	}
	return groups
}

func (s *Server) runGame() {
	log.Println("first player connected, starting in 5s")
	time.Sleep(3 * time.Second)

	log.Println("game started")
	responseTimeMS := 330
	const maxTileHealth uint8 = 7

	var spawnPoints = [8]Vec2{
		{2, 2},
		{61, 2},
		{2, 61},
		{61, 61}, // bottom right 
		{31, 2}, // top mid
		{2, 31}, // left mid
		{32, 61}, // bottom mid
		{61, 31}, // left mid
	}
	// Initalize players with round robin spawn points.
	players := s.initPlayerInstances(spawnPoints[:])
	world := initWorld(Vec2{X: 64, Y: 64}, maxTileHealth, players[:])

	tileDeltas := make([]TileDelta, 0, 512) // len 0, capacity 64, one allocation up front
	playerMoves := make([]Move, 0, len(players))

	// send initial world state
	roundStartMSG := newRoundStartMessage(len(players), responseTimeMS, uint32(world.h), uint32(world.w))
	initialWorldStateMSG := newWorldSnapshotMessage(world, 1)
	log.Printf("sending game start and world state message")

	// send world state to spectators
	s.sendToSpectators(newSpectatorRoundInfoMessage(world, players))
	s.sendToSpectators(newWorldSnapshotMessage(world, 1))

	s.sendToAllPlayers(players[:], roundStartMSG)
	s.sendToAllPlayers(players[:], initialWorldStateMSG)

	var currentTick uint32 = 1
	for {
		// Send delta update messages to all players
		world.buildSnapshot(players)
		var worldHash = world.getSnapshotHash()

		tileDeltas = updateTileDeltas(world, tileDeltas)
		for i := range players {
			p := &players[i]
			if p.dead {
				continue
			}
			deltaStateMSG := newDeltaStateMessage(currentTick, p.pos, tileDeltas[:], worldHash)
			s.sendToPlayer(p.token, websocket.MessageText, deltaStateMSG)
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

		// Check if any players are on the same tile after moving
		for pos, indexes := range getPlayersOnSameTile(players) {
			fmt.Println(pos, indexes) // {1 2} [0 2]
			log.Printf("Multiple players at: %v", pos)
			// KILL KILL KILL
			for _, idx := range indexes {
				p := &players[idx] // pointer, so changes affect the real player
				p.dead = true
				s.sendToPlayer(p.token, websocket.MessageText, newRoundOverMessage(p.score, 1))
				log.Printf("Player (%s) has died from being too fat. Score: %d", p.token, p.score)
			}
			world.tiles[world.tileIndex(pos)] = 0
		}

		// Decay tiles.
		for i := range world.tiles {
			tileHealth := world.tiles[i]
			if tileHealth < maxTileHealth && tileHealth > 0 {
				tileHealth -= 1
			}
			world.tiles[i] = tileHealth
		}

		// Check if players are on dead tiles
		for i := range players {
			p := &players[i]
			if p.dead {
				continue
			}
			tileHealth := world.tiles[world.tileIndex(p.pos)]
			switch tileHealth {
			case 0:
				p.dead = true
				s.sendToPlayer(p.token, websocket.MessageText, newRoundOverMessage(p.score, 1))
				log.Printf("Player (%s) has died. Score: %d", p.token, p.score)
			case 7: // Start decaying tile.
				world.tiles[world.tileIndex(p.pos)] -= 1
			default:
			}
		}

		// Add 1 to each alive players score and check for round over.
		numAlive := 0
		for i := range players {
			if !players[i].dead {
				players[i].score += 1
				numAlive++
			}
		}
		// Check if round is over.
		if numAlive <= 0 {
			log.Printf("Round over!")
			world.buildSnapshot(players)
			worldHash = world.getSnapshotHash()

			tileDeltas = updateTileDeltas(world, tileDeltas)
			s.sendToSpectators(newSpectatorRoundDeltaMessage(world, currentTick, tileDeltas, worldHash, players[:]))
			s.updateSpectatorView(world, currentTick, players, tileDeltas, worldHash)
			break
		}
	}

	s.mu.Lock()
	s.roundInProgresss = false
	s.mu.Unlock()
}
