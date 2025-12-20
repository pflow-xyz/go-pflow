// Package server provides an HTTP/WebSocket game server for catacombs
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pflow-xyz/go-pflow/examples/catacombs"
	"github.com/pflow-xyz/go-pflow/examples/catacombs/storage"
)

// Server handles HTTP and WebSocket connections
type Server struct {
	mu sync.RWMutex

	// Active game sessions
	sessions map[string]*GameSession

	// All connected clients
	clients map[*Client]bool

	// WebSocket upgrader
	upgrader websocket.Upgrader

	// Debug mode for AI logging
	debug bool

	// SQLite storage for session logging (optional)
	store *storage.Store
}

// GameSession represents an active game
type GameSession struct {
	ID        string
	Game      *catacombs.Game
	Client    *Client
	CreatedAt time.Time
	Mode      string // "normal", "demo", "infinite"
	mu        sync.Mutex
}

// Client represents a connected player
type Client struct {
	ID       string
	Conn     *websocket.Conn
	Session  *GameSession
	mu       sync.Mutex
	sendChan chan []byte
}

// Message types
type MessageType string

const (
	MsgTypeJoin           MessageType = "join"
	MsgTypeJoinDemo       MessageType = "join_demo"
	MsgTypeJoinSeed       MessageType = "join_seed"
	MsgTypeJoinInfinite   MessageType = "join_infinite"
	MsgTypeGameState      MessageType = "game_state"
	MsgTypeAction         MessageType = "action"
	MsgTypeDialogueChoice MessageType = "dialogue_choice"
	MsgTypeUseItem        MessageType = "use_item"
	MsgTypeError          MessageType = "error"
	MsgTypePing           MessageType = "ping"
	MsgTypePong           MessageType = "pong"
	MsgTypeLeave          MessageType = "leave"
	MsgTypeReset          MessageType = "reset"
	// Combat message types
	MsgTypeCombatAction   MessageType = "combat_action"
	MsgTypeSetTarget      MessageType = "set_target"
	MsgTypeSetBodyPart    MessageType = "set_body_part"
	MsgTypeInitiateCombat MessageType = "initiate_combat"
	// AI mode message types
	MsgTypeAIToggle MessageType = "ai_toggle"
	MsgTypeAITick   MessageType = "ai_tick"
)

// Message envelope
type Message struct {
	Type      MessageType     `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// ActionPayload for player actions
type ActionPayload struct {
	Action string `json:"action"`
}

// DialoguePayload for dialogue choices
type DialoguePayload struct {
	Choice int `json:"choice"`
}

// ItemPayload for item usage
type ItemPayload struct {
	Index int `json:"index"`
}

// ErrorPayload for errors
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CombatActionPayload for combat actions
type CombatActionPayload struct {
	Action string `json:"action"` // "attack", "aimed_shot", "move", "use_item", "end_turn", "flee"
}

// SetTargetPayload for selecting enemy target
type SetTargetPayload struct {
	EnemyID string `json:"enemy_id"`
}

// SetBodyPartPayload for selecting body part to target
type SetBodyPartPayload struct {
	BodyPart int `json:"body_part"` // 0=torso, 1=head, 2=left_arm, etc.
}

// AITogglePayload for enabling/disabling AI mode
type AITogglePayload struct {
	Enabled bool `json:"enabled"`
}

// JoinSeedPayload for joining with a specific seed
type JoinSeedPayload struct {
	Seed   int64 `json:"seed"`
	AISeed int64 `json:"ai_seed,omitempty"`
}

// JoinInfinitePayload for joining in infinite mode (optional seed)
type JoinInfinitePayload struct {
	Seed   int64 `json:"seed,omitempty"`
	AISeed int64 `json:"ai_seed,omitempty"`
}

// NewServer creates a new game server
func NewServer() *Server {
	return &Server{
		sessions: make(map[string]*GameSession),
		clients:  make(map[*Client]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// SetDebug enables or disables AI debug logging
func (s *Server) SetDebug(debug bool) {
	s.debug = debug
	if debug {
		log.Println("AI debug logging enabled")
	}
}

// SetStore sets the SQLite storage for session logging
func (s *Server) SetStore(store *storage.Store) {
	s.store = store
	if store != nil {
		log.Println("SQLite session logging enabled")
	}
}

// GetStore returns the storage instance
func (s *Server) GetStore() *storage.Store {
	return s.store
}

// ServeHTTP handles HTTP requests
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/ws":
		s.handleWebSocket(w, r)
	case "/health":
		s.handleHealth(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"game":     "catacombs",
		"sessions": len(s.sessions),
		"clients":  len(s.clients),
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:       generateID(),
		Conn:     conn,
		sendChan: make(chan []byte, 256),
	}

	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	log.Printf("Client %s connected", client.ID)

	// Start send goroutine
	go client.writePump()

	// Handle messages
	s.handleClient(client)
}

func (s *Server) handleClient(client *Client) {
	defer func() {
		s.removeClient(client)
		client.Conn.Close()
		close(client.sendChan)
	}()

	client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msgBytes, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Client %s read error: %v", client.ID, err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			s.sendError(client, "invalid_message", "Could not parse message")
			continue
		}

		s.handleMessage(client, &msg)
	}
}

func (s *Server) handleMessage(client *Client, msg *Message) {
	switch msg.Type {
	case MsgTypeJoin:
		s.handleJoinWithSeed(client, false, 0)

	case MsgTypeJoinDemo:
		s.handleJoinWithSeed(client, true, 0)

	case MsgTypeJoinSeed:
		s.handleJoinSeed(client, msg.Payload)

	case MsgTypeJoinInfinite:
		s.handleJoinInfinite(client, msg.Payload)

	case MsgTypeAction:
		s.handleAction(client, msg.Payload)

	case MsgTypeDialogueChoice:
		s.handleDialogueChoice(client, msg.Payload)

	case MsgTypeUseItem:
		s.handleUseItem(client, msg.Payload)

	case MsgTypeReset:
		s.handleReset(client)

	case MsgTypePing:
		s.sendMessage(client, MsgTypePong, nil)

	case MsgTypeLeave:
		s.handleLeave(client)

	case MsgTypeCombatAction:
		s.handleCombatAction(client, msg.Payload)

	case MsgTypeSetTarget:
		s.handleSetTarget(client, msg.Payload)

	case MsgTypeSetBodyPart:
		s.handleSetBodyPart(client, msg.Payload)

	case MsgTypeInitiateCombat:
		s.handleInitiateCombat(client)

	case MsgTypeAIToggle:
		s.handleAIToggle(client, msg.Payload)

	case MsgTypeAITick:
		s.handleAITick(client)

	default:
		s.sendError(client, "unknown_type", fmt.Sprintf("Unknown message type: %s", msg.Type))
	}
}

func (s *Server) handleJoinSeed(client *Client, payload json.RawMessage) {
	var seedPayload JoinSeedPayload
	if err := json.Unmarshal(payload, &seedPayload); err != nil {
		s.sendError(client, "invalid_payload", fmt.Sprintf("Invalid seed payload: %v", err))
		return
	}
	s.handleJoinWithOptions(client, false, seedPayload.Seed, seedPayload.AISeed, false)
}

func (s *Server) handleJoinInfinite(client *Client, payload json.RawMessage) {
	var infinitePayload JoinInfinitePayload
	if payload != nil {
		json.Unmarshal(payload, &infinitePayload) // Optional payload
	}
	s.handleJoinWithOptions(client, false, infinitePayload.Seed, infinitePayload.AISeed, true)
}

func (s *Server) handleJoinWithSeed(client *Client, demoMode bool, seed int64) {
	s.handleJoinWithOptions(client, demoMode, seed, 0, false)
}

func (s *Server) handleJoinWithOptions(client *Client, demoMode bool, seed int64, aiSeed int64, infinite bool) {
	// Create a new game session with options
	session := s.createSessionWithOptions(client, demoMode, seed, aiSeed, infinite)
	client.Session = session

	modeStr := "NORMAL"
	if demoMode {
		modeStr = "DEMO"
	}
	if seed != 0 {
		modeStr = fmt.Sprintf("SEEDED(%d)", seed)
	}
	if infinite {
		modeStr = "INFINITE"
		if seed != 0 {
			modeStr = fmt.Sprintf("INFINITE(seed=%d)", seed)
		}
	}
	log.Printf("Client %s started %s game session %s", client.ID, modeStr, session.ID)

	// Log initial state
	state := session.Game.GetState()
	logGameState(session.ID, "JOIN", state, session.Game)

	// Send initial game state
	s.sendGameState(client)
}

func (s *Server) handleAction(client *Client, payload json.RawMessage) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	var action ActionPayload
	if err := json.Unmarshal(payload, &action); err != nil {
		s.sendError(client, "invalid_payload", fmt.Sprintf("Invalid action payload: %v", err))
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	// Process action
	actionType := catacombs.ActionType(action.Action)
	if err := session.Game.ProcessAction(actionType); err != nil {
		s.sendError(client, "action_error", err.Error())
		return
	}

	// Log state
	state := session.Game.GetState()
	logGameState(session.ID, action.Action, state, session.Game)

	// Log to SQLite
	s.logActionToStore(session, action.Action)

	// Send updated state
	s.sendGameState(client)
}

func (s *Server) handleDialogueChoice(client *Client, payload json.RawMessage) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	var choice DialoguePayload
	if err := json.Unmarshal(payload, &choice); err != nil {
		s.sendError(client, "invalid_payload", fmt.Sprintf("Invalid dialogue payload: %v", err))
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	if err := session.Game.ProcessDialogueChoice(choice.Choice); err != nil {
		s.sendError(client, "dialogue_error", err.Error())
		return
	}

	// Log state
	state := session.Game.GetState()
	logGameState(session.ID, fmt.Sprintf("dialogue_choice_%d", choice.Choice), state, session.Game)

	s.sendGameState(client)
}

func (s *Server) handleUseItem(client *Client, payload json.RawMessage) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	var item ItemPayload
	if err := json.Unmarshal(payload, &item); err != nil {
		s.sendError(client, "invalid_payload", fmt.Sprintf("Invalid item payload: %v", err))
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	if err := session.Game.UseItem(item.Index); err != nil {
		s.sendError(client, "item_error", err.Error())
		return
	}

	state := session.Game.GetState()
	logGameState(session.ID, fmt.Sprintf("use_item_%d", item.Index), state, session.Game)

	s.sendGameState(client)
}

func (s *Server) handleReset(client *Client) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	session := client.Session
	session.mu.Lock()
	session.Game.Reset()
	session.mu.Unlock()

	log.Printf("Client %s reset game", client.ID)

	s.sendGameState(client)
}

func (s *Server) handleLeave(client *Client) {
	if client.Session != nil {
		// End session in SQLite before removing
		if s.store != nil {
			game := client.Session.Game
			if err := s.store.EndSession(
				client.Session.ID,
				game.Level,
				game.Player.Health,
				game.AI.ActionCount,
				game.GameOver,
				game.Victory,
			); err != nil {
				log.Printf("Failed to end session in SQLite: %v", err)
			}
		}

		s.mu.Lock()
		delete(s.sessions, client.Session.ID)
		s.mu.Unlock()
		client.Session = nil
	}
}

func (s *Server) handleCombatAction(client *Client, payload json.RawMessage) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	var action CombatActionPayload
	if err := json.Unmarshal(payload, &action); err != nil {
		s.sendError(client, "invalid_payload", fmt.Sprintf("Invalid combat action payload: %v", err))
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	// Convert string action to ActionType and process
	actionType := catacombs.ActionType(action.Action)
	if err := session.Game.ProcessCombatAction(actionType, nil); err != nil {
		s.sendError(client, "combat_error", err.Error())
		return
	}

	state := session.Game.GetState()
	logGameState(session.ID, fmt.Sprintf("combat_%s", action.Action), state, session.Game)

	s.sendGameState(client)
}

func (s *Server) handleSetTarget(client *Client, payload json.RawMessage) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	var target SetTargetPayload
	if err := json.Unmarshal(payload, &target); err != nil {
		s.sendError(client, "invalid_payload", fmt.Sprintf("Invalid target payload: %v", err))
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	session.Game.SetTargetEnemy(target.EnemyID)

	state := session.Game.GetState()
	logGameState(session.ID, fmt.Sprintf("set_target_%s", target.EnemyID), state, session.Game)

	s.sendGameState(client)
}

func (s *Server) handleSetBodyPart(client *Client, payload json.RawMessage) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	var part SetBodyPartPayload
	if err := json.Unmarshal(payload, &part); err != nil {
		s.sendError(client, "invalid_payload", fmt.Sprintf("Invalid body part payload: %v", err))
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	session.Game.SetTargetPart(catacombs.BodyPart(part.BodyPart))

	state := session.Game.GetState()
	logGameState(session.ID, fmt.Sprintf("set_bodypart_%d", part.BodyPart), state, session.Game)

	s.sendGameState(client)
}

func (s *Server) handleInitiateCombat(client *Client) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	session.Game.InitiateCombat()

	state := session.Game.GetState()
	logGameState(session.ID, "initiate_combat", state, session.Game)

	s.sendGameState(client)
}

func (s *Server) handleAIToggle(client *Client, payload json.RawMessage) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	var toggle AITogglePayload
	if err := json.Unmarshal(payload, &toggle); err != nil {
		s.sendError(client, "invalid_payload", fmt.Sprintf("Invalid AI toggle payload: %v", err))
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	if toggle.Enabled {
		session.Game.EnableAI()
		log.Printf("Session %s: AI enabled", session.ID[:8])
	} else {
		session.Game.DisableAI()
		log.Printf("Session %s: AI disabled", session.ID[:8])
	}

	s.sendGameState(client)
}

func (s *Server) handleAITick(client *Client) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	// Only tick if AI is enabled
	if !session.Game.AI.Enabled {
		return
	}

	// Log AI state before tick if debug mode enabled
	if s.debug {
		game := session.Game
		log.Printf("[AI DEBUG] Level=%d ActionCount=%d Pos=(%d,%d) HP=%d/%d Mode=%s Target=%s",
			game.Level, game.AI.ActionCount,
			game.Player.X, game.Player.Y,
			game.Player.Health, game.Player.MaxHealth,
			game.AI.Mode, game.AI.Target)
	}

	// Execute one AI action
	action := session.Game.AITick()

	// Log AI action result if debug mode enabled
	if s.debug && action != "" {
		game := session.Game
		log.Printf("[AI DEBUG] Action=%s NewPos=(%d,%d) NewMode=%s",
			action, game.Player.X, game.Player.Y, game.AI.Mode)
	}

	state := session.Game.GetState()
	if action != "" {
		logGameState(session.ID, fmt.Sprintf("ai_%s", action), state, session.Game)
		// Log to SQLite
		s.logActionToStore(session, fmt.Sprintf("ai_%s", action))
	}

	// Update session periodically (every 10 ticks or on game over)
	if session.Game.AI.ActionCount%10 == 0 || session.Game.GameOver {
		s.updateSessionInStore(session)
	}

	s.sendGameState(client)
}

func (s *Server) createSession(client *Client, demoMode bool) *GameSession {
	return s.createSessionWithOptions(client, demoMode, 0, 0, false)
}

func (s *Server) createSessionWithSeed(client *Client, demoMode bool, seed int64) *GameSession {
	return s.createSessionWithOptions(client, demoMode, seed, 0, false)
}

func (s *Server) createSessionWithOptions(client *Client, demoMode bool, seed int64, aiSeed int64, infinite bool) *GameSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	var game *catacombs.Game
	var mode string

	if demoMode {
		game = catacombs.NewDemoGame()
		mode = "demo"
	} else if infinite {
		params := catacombs.DefaultParams()
		if seed != 0 {
			params.Seed = seed
		}
		if aiSeed != 0 {
			params.AISeed = aiSeed
		}
		game = catacombs.NewGameWithParams(params)
		game.EnableInfiniteMode()
		mode = "infinite"
	} else if seed != 0 || aiSeed != 0 {
		params := catacombs.DefaultParams()
		if seed != 0 {
			params.Seed = seed
		}
		if aiSeed != 0 {
			params.AISeed = aiSeed
		}
		game = catacombs.NewGameWithParams(params)
		mode = "normal"
	} else {
		game = catacombs.NewGame()
		mode = "normal"
	}

	session := &GameSession{
		ID:        generateID(),
		Game:      game,
		Client:    client,
		CreatedAt: time.Now(),
		Mode:      mode,
	}

	s.sessions[session.ID] = session

	// Log session to SQLite if storage is enabled
	if s.store != nil {
		if err := s.store.CreateSession(session.ID, game.Seed, mode); err != nil {
			log.Printf("Failed to log session to SQLite: %v", err)
		}
	}

	return session
}

func (s *Server) removeClient(client *Client) {
	s.handleLeave(client)

	s.mu.Lock()
	delete(s.clients, client)
	s.mu.Unlock()

	log.Printf("Client %s disconnected", client.ID)
}

// logActionToStore logs a game action to SQLite storage
func (s *Server) logActionToStore(session *GameSession, actionName string) {
	if s.store == nil {
		return
	}

	game := session.Game

	// Count alive enemies
	enemiesAlive := 0
	for _, e := range game.Enemies {
		if e.State != catacombs.StateDead {
			enemiesAlive++
		}
	}

	// Get AI info if enabled
	var aiMode, aiTarget string
	if game.AI.Enabled {
		aiMode = game.AI.Mode
		aiTarget = game.AI.Target
	}

	action := &storage.Action{
		SessionID:    session.ID,
		Tick:         game.AI.ActionCount,
		Level:        game.Level,
		Action:       actionName,
		PlayerX:      game.Player.X,
		PlayerY:      game.Player.Y,
		PlayerHP:     game.Player.Health,
		PlayerMaxHP:  game.Player.MaxHealth,
		AIMode:       aiMode,
		AITarget:     aiTarget,
		EnemiesAlive: enemiesAlive,
		InCombat:     game.Combat.Active,
	}

	if err := s.store.LogAction(action); err != nil {
		log.Printf("Failed to log action to SQLite: %v", err)
	}
}

// updateSessionInStore updates the session record in SQLite
func (s *Server) updateSessionInStore(session *GameSession) {
	if s.store == nil {
		return
	}

	game := session.Game
	if err := s.store.UpdateSession(
		session.ID,
		game.Level,
		game.Player.Health,
		game.AI.ActionCount,
		game.AI.Enabled,
		game.GameOver,
		game.Victory,
	); err != nil {
		log.Printf("Failed to update session in SQLite: %v", err)
	}
}

func (s *Server) sendMessage(client *Client, msgType MessageType, payload any) {
	var payloadBytes json.RawMessage
	if payload != nil {
		var err error
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			log.Printf("Error marshaling payload: %v", err)
			return
		}
	}

	msg := Message{
		Type:      msgType,
		Payload:   payloadBytes,
		Timestamp: time.Now().UnixMilli(),
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	select {
	case client.sendChan <- msgBytes:
	default:
		log.Printf("Client %s send buffer full", client.ID)
	}
}

func (s *Server) sendError(client *Client, code, message string) {
	s.sendMessage(client, MsgTypeError, ErrorPayload{
		Code:    code,
		Message: message,
	})
}

func (s *Server) sendGameState(client *Client) {
	if client.Session == nil {
		return
	}

	state := client.Session.Game.GetState()
	availableActions := client.Session.Game.GetAvailableActions()

	s.sendMessage(client, MsgTypeGameState, map[string]any{
		"state":             state,
		"available_actions": availableActions,
	})
}

func (client *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-client.sendChan:
			if !ok {
				return
			}

			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Client %s write error: %v", client.ID, err)
				return
			}

		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

var idCounter int64
var idMu sync.Mutex

func generateID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), idCounter)
}

// logGameState logs the current game state with ASCII map
func logGameState(sessionID string, action string, state catacombs.GameState, game *catacombs.Game) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n=== Session %s | Action: %s | Level %d | Turn %d ===\n",
		sessionID[:8], action, state.Level, state.Turn))
	sb.WriteString(fmt.Sprintf("    Player: HP=%3d/%3d  MP=%2d/%2d  Gold=%3d  XP=%3d  Lvl=%d\n",
		state.Player.Health, state.Player.MaxHealth,
		state.Player.Mana, state.Player.MaxMana,
		state.Player.Gold, state.Player.XP, state.Player.Level))
	sb.WriteString(fmt.Sprintf("    Pos: (%d,%d)  Atk=%d  Def=%d\n",
		state.Player.X, state.Player.Y, state.Player.Attack, state.Player.Defense))

	// Inventory
	if len(state.Player.Inventory) > 0 {
		items := make([]string, len(state.Player.Inventory))
		for i, item := range state.Player.Inventory {
			items[i] = item.Name
		}
		sb.WriteString(fmt.Sprintf("    Inventory: %s\n", strings.Join(items, ", ")))
	}

	// Active quests
	if len(state.Player.ActiveQuests) > 0 {
		sb.WriteString(fmt.Sprintf("    Quests: %s\n", strings.Join(state.Player.ActiveQuests, ", ")))
	}

	// Enemy count
	aliveEnemies := 0
	for _, e := range state.Enemies {
		if e.State != 6 { // Not dead
			aliveEnemies++
		}
	}
	sb.WriteString(fmt.Sprintf("    Enemies: %d alive  NPCs: %d\n", aliveEnemies, len(state.NPCs)))

	// Message
	if state.Message != "" {
		sb.WriteString(fmt.Sprintf("    >>> %s <<<\n", state.Message))
	}

	// Dialogue state
	if state.InDialogue && state.DialogueData != nil {
		sb.WriteString(fmt.Sprintf("    [DIALOGUE] %s: %s\n", state.DialogueData.Speaker, state.DialogueData.Text))
	}

	// ASCII map
	sb.WriteString("\n")
	sb.WriteString(game.ToASCII())

	// Legend
	sb.WriteString("    Legend: @ player  # wall  . floor  + door  L locked  > down  < up\n")
	sb.WriteString("            $ merchant  + healer  ? quest  N npc  s/z/g/O enemies\n")
	sb.WriteString("            ! potion  * gold  k key  $ chest  _ altar\n")

	log.Print(sb.String())
}
