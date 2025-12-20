// Package server provides an HTTP/WebSocket game server for doom
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
	"github.com/pflow-xyz/go-pflow/examples/doom"
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
}

// GameSession represents an active game
type GameSession struct {
	ID        string
	Game      *doom.Game
	Client    *Client
	CreatedAt time.Time
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
	MsgTypeJoin      MessageType = "join"
	MsgTypeGameState MessageType = "game_state"
	MsgTypeAction    MessageType = "action"
	MsgTypeError     MessageType = "error"
	MsgTypePing      MessageType = "ping"
	MsgTypePong      MessageType = "pong"
	MsgTypeLeave     MessageType = "leave"
	MsgTypeReset     MessageType = "reset"
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

// ErrorPayload for errors
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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
		s.handleJoin(client)

	case MsgTypeAction:
		s.handleAction(client, msg.Payload)

	case MsgTypeReset:
		s.handleReset(client)

	case MsgTypePing:
		s.sendMessage(client, MsgTypePong, nil)

	case MsgTypeLeave:
		s.handleLeave(client)

	default:
		s.sendError(client, "unknown_type", fmt.Sprintf("Unknown message type: %s", msg.Type))
	}
}

func (s *Server) handleJoin(client *Client) {
	// Create a new game session
	session := s.createSession(client)
	client.Session = session

	log.Printf("Client %s started game session %s", client.ID, session.ID)

	// Log initial state with ASCII map
	state := session.Game.GetState()
	logGameState(session.ID, "JOIN", state)

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
	actionType := doom.ActionType(action.Action)
	if err := session.Game.ProcessAction(actionType); err != nil {
		s.sendError(client, "action_error", err.Error())
		return
	}

	// Log state with ASCII map
	state := session.Game.GetState()
	logGameState(session.ID, action.Action, state)

	// Send updated state
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
		s.mu.Lock()
		delete(s.sessions, client.Session.ID)
		s.mu.Unlock()
		client.Session = nil
	}
}

func (s *Server) createSession(client *Client) *GameSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := &GameSession{
		ID:        generateID(),
		Game:      doom.NewGame(),
		Client:    client,
		CreatedAt: time.Now(),
	}

	s.sessions[session.ID] = session
	return session
}

func (s *Server) removeClient(client *Client) {
	s.handleLeave(client)

	s.mu.Lock()
	delete(s.clients, client)
	s.mu.Unlock()

	log.Printf("Client %s disconnected", client.ID)
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

// renderASCIIMap renders the game state as an ASCII map for logging
func renderASCIIMap(state doom.GameState) string {
	var sb strings.Builder

	// Create a character grid from the tiles
	grid := make([][]rune, state.MapHeight)
	for y := 0; y < state.MapHeight; y++ {
		grid[y] = make([]rune, state.MapWidth)
		for x := 0; x < state.MapWidth; x++ {
			switch state.Tiles[y][x] {
			case 0: // Floor
				grid[y][x] = '.'
			case 1: // Wall
				grid[y][x] = '#'
			case 2: // Door
				grid[y][x] = '+'
			case 3: // Locked door
				grid[y][x] = 'L'
			case 4: // Exit
				grid[y][x] = 'E'
			default:
				grid[y][x] = '?'
			}
		}
	}

	// Place items
	for _, item := range state.Items {
		if item.Picked {
			continue
		}
		ix, iy := int(item.X), int(item.Y)
		if ix >= 0 && ix < state.MapWidth && iy >= 0 && iy < state.MapHeight {
			switch item.Type {
			case 0: // Health
				grid[iy][ix] = 'h'
			case 1: // Armor
				grid[iy][ix] = 'a'
			case 2: // Ammo
				grid[iy][ix] = 'b'
			case 3: // Shotgun
				grid[iy][ix] = 'S'
			case 4: // Red key
				grid[iy][ix] = 'k'
			case 5: // Blue key
				grid[iy][ix] = 'K'
			default:
				grid[iy][ix] = 'i'
			}
		}
	}

	// Place enemies
	for _, enemy := range state.Enemies {
		if enemy.State == 3 { // Dead
			continue
		}
		ex, ey := int(enemy.X), int(enemy.Y)
		if ex >= 0 && ex < state.MapWidth && ey >= 0 && ey < state.MapHeight {
			switch enemy.State {
			case 0: // Idle
				grid[ey][ex] = 'e'
			case 1: // Alert
				grid[ey][ex] = 'E'
			case 2: // Attacking
				grid[ey][ex] = '!'
			default:
				grid[ey][ex] = 'e'
			}
		}
	}

	// Place player with direction indicator
	px, py := int(state.Player.X), int(state.Player.Y)
	if px >= 0 && px < state.MapWidth && py >= 0 && py < state.MapHeight {
		// Use direction arrows based on angle
		angle := state.Player.Angle
		var playerChar rune
		if angle < 0.785 || angle >= 5.497 { // East (right)
			playerChar = '>'
		} else if angle < 2.356 { // South (down)
			playerChar = 'v'
		} else if angle < 3.927 { // West (left)
			playerChar = '<'
		} else { // North (up)
			playerChar = '^'
		}
		grid[py][px] = playerChar
	}

	// Render grid
	sb.WriteString("\n")
	for y := 0; y < state.MapHeight; y++ {
		sb.WriteString("    ")
		for x := 0; x < state.MapWidth; x++ {
			sb.WriteRune(grid[y][x])
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// logGameState logs the current game state with ASCII map
func logGameState(sessionID string, action string, state doom.GameState) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n=== Session %s | Action: %s ===\n", sessionID[:8], action))
	sb.WriteString(fmt.Sprintf("    Player: HP=%3.0f  Armor=%2.0f  Ammo=%2.0f  Pos=(%.1f,%.1f)\n",
		state.Player.Health, state.Player.Armor, state.Player.Ammo,
		state.Player.X, state.Player.Y))

	// Show keys/weapons
	items := []string{}
	if state.Player.HasShotgun {
		items = append(items, "Shotgun")
	}
	if state.Player.HasKeyRed {
		items = append(items, "RedKey")
	}
	if state.Player.HasKeyBlue {
		items = append(items, "BlueKey")
	}
	if len(items) > 0 {
		sb.WriteString(fmt.Sprintf("    Items: %s\n", strings.Join(items, ", ")))
	}

	// Show enemy status
	aliveEnemies := 0
	for _, e := range state.Enemies {
		if e.State != 3 { // Not dead
			aliveEnemies++
		}
	}
	sb.WriteString(fmt.Sprintf("    Enemies: %d alive  Kills: %d\n", aliveEnemies, state.KillCount))

	// Show message if any
	if state.Message != "" {
		sb.WriteString(fmt.Sprintf("    >>> %s <<<\n", state.Message))
	}

	// Render ASCII map
	sb.WriteString(renderASCIIMap(state))

	// Legend
	sb.WriteString("    Legend: # wall  + door  L locked  E exit  >v<^ player\n")
	sb.WriteString("            e enemy  ! attacking  h health  b ammo  k key  S shotgun\n")

	log.Print(sb.String())
}
