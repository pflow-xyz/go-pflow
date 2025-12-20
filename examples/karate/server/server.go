// Package server provides an HTTP/WebSocket game server for karate fighting game
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pflow-xyz/go-pflow/examples/karate"
)

// Server handles HTTP and WebSocket connections for the game
type Server struct {
	mu sync.RWMutex

	// Active game sessions
	sessions map[string]*GameSession

	// Matchmaking queue
	matchQueue []*Client

	// All connected clients
	clients map[*Client]bool

	// WebSocket upgrader
	upgrader websocket.Upgrader
}

// GameSession represents an active game between players
type GameSession struct {
	ID        string
	Game      *karate.Game
	Player1   *Client
	Player2   *Client // nil if playing against AI
	IsVsAI    bool
	CreatedAt time.Time

	mu sync.Mutex
}

// Client represents a connected player
type Client struct {
	ID       string
	Conn     *websocket.Conn
	Session  *GameSession
	Player   karate.Player
	Ready    bool
	InQueue  bool
	mu       sync.Mutex
	sendChan chan []byte
}

// Message types for WebSocket communication
type MessageType string

const (
	MsgTypeJoin         MessageType = "join"
	MsgTypeMatchmaking  MessageType = "matchmaking"
	MsgTypeMatchFound   MessageType = "match_found"
	MsgTypeGameState    MessageType = "game_state"
	MsgTypeAction       MessageType = "action"
	MsgTypeActionResult MessageType = "action_result"
	MsgTypeError        MessageType = "error"
	MsgTypeChat         MessageType = "chat"
	MsgTypeReady        MessageType = "ready"
	MsgTypePing         MessageType = "ping"
	MsgTypePong         MessageType = "pong"
	MsgTypeLeave        MessageType = "leave"
	MsgTypeGameOver     MessageType = "game_over"
)

// Message is the envelope for all WebSocket messages
type Message struct {
	Type      MessageType     `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// JoinPayload for joining a game
type JoinPayload struct {
	PlayerID string `json:"player_id"`
	Mode     string `json:"mode"` // "ai" or "pvp"
}

// ActionPayload for submitting an action
type ActionPayload struct {
	Action string `json:"action"`
}

// MatchFoundPayload when a match is found
type MatchFoundPayload struct {
	SessionID string `json:"session_id"`
	Player    int    `json:"player"`    // 1 or 2
	Opponent  string `json:"opponent"`  // "AI" or player ID
	IsVsAI    bool   `json:"is_vs_ai"`
}

// ErrorPayload for errors
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewServer creates a new game server
func NewServer() *Server {
	return &Server{
		sessions:   make(map[string]*GameSession),
		clients:    make(map[*Client]bool),
		matchQueue: make([]*Client, 0),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
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
	case "/api/sessions":
		s.handleSessions(w, r)
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
		"queue":    len(s.matchQueue),
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]map[string]any, 0)
	for id, session := range s.sessions {
		sessions = append(sessions, map[string]any{
			"id":         id,
			"is_vs_ai":   session.IsVsAI,
			"created_at": session.CreatedAt,
			"state":      session.Game.GetState(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
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
		s.handleJoin(client, msg.Payload)

	case MsgTypeMatchmaking:
		s.handleMatchmaking(client)

	case MsgTypeAction:
		s.handleAction(client, msg.Payload)

	case MsgTypePing:
		s.sendMessage(client, MsgTypePong, nil)

	case MsgTypeLeave:
		s.handleLeave(client)

	case MsgTypeReady:
		s.handleReady(client)

	default:
		s.sendError(client, "unknown_type", fmt.Sprintf("Unknown message type: %s", msg.Type))
	}
}

func (s *Server) handleJoin(client *Client, payload json.RawMessage) {
	var join JoinPayload
	if err := json.Unmarshal(payload, &join); err != nil {
		s.sendError(client, "invalid_payload", "Invalid join payload")
		return
	}

	if join.PlayerID != "" {
		client.ID = join.PlayerID
	}

	if join.Mode == "ai" {
		// Create game against AI immediately
		session := s.createSession(client, nil, true)
		client.Session = session
		client.Player = karate.Player1

		s.sendMessage(client, MsgTypeMatchFound, MatchFoundPayload{
			SessionID: session.ID,
			Player:    1,
			Opponent:  "AI",
			IsVsAI:    true,
		})

		// Send initial game state
		s.sendGameState(client)
	} else {
		// Add to matchmaking queue
		s.handleMatchmaking(client)
	}
}

func (s *Server) handleMatchmaking(client *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already in queue
	for _, c := range s.matchQueue {
		if c == client {
			s.sendError(client, "already_queued", "Already in matchmaking queue")
			return
		}
	}

	// Add to queue
	client.InQueue = true
	s.matchQueue = append(s.matchQueue, client)

	log.Printf("Client %s joined matchmaking queue (queue size: %d)", client.ID, len(s.matchQueue))

	// Try to match
	if len(s.matchQueue) >= 2 {
		p1 := s.matchQueue[0]
		p2 := s.matchQueue[1]
		s.matchQueue = s.matchQueue[2:]

		p1.InQueue = false
		p2.InQueue = false

		session := s.createSessionLocked(p1, p2, false)

		p1.Session = session
		p1.Player = karate.Player1

		p2.Session = session
		p2.Player = karate.Player2

		// Notify both players
		s.sendMessage(p1, MsgTypeMatchFound, MatchFoundPayload{
			SessionID: session.ID,
			Player:    1,
			Opponent:  p2.ID,
			IsVsAI:    false,
		})

		s.sendMessage(p2, MsgTypeMatchFound, MatchFoundPayload{
			SessionID: session.ID,
			Player:    2,
			Opponent:  p1.ID,
			IsVsAI:    false,
		})

		// Send initial game state to both
		s.sendGameState(p1)
		s.sendGameState(p2)

		log.Printf("Match created: %s vs %s (session %s)", p1.ID, p2.ID, session.ID)
	}
}

func (s *Server) handleAction(client *Client, payload json.RawMessage) {
	if client.Session == nil {
		s.sendError(client, "no_session", "Not in a game session")
		return
	}

	var action ActionPayload
	if err := json.Unmarshal(payload, &action); err != nil {
		s.sendError(client, "invalid_payload", "Invalid action payload")
		return
	}

	session := client.Session
	session.mu.Lock()
	defer session.mu.Unlock()

	// Submit player action
	actionType := karate.ActionType(action.Action)
	if err := session.Game.SubmitAction(client.Player, actionType); err != nil {
		s.sendError(client, "invalid_action", err.Error())
		return
	}

	log.Printf("Session %s: %s submitted %s", session.ID, client.Player, action.Action)

	// If playing vs AI, get AI move and resolve
	if session.IsVsAI {
		// Get AI mood before move
		preMood := session.Game.GetState().AIMood
		aiMove := session.Game.GetAIMove()
		log.Printf("Session %s: %s AI chose %s", session.ID, moodEmoji(preMood), aiMove)
		if err := session.Game.SubmitAction(karate.Player2, aiMove); err != nil {
			log.Printf("AI action error: %v", err)
		}
	}

	// If both actions submitted, resolve turn
	if session.Game.HasBothActions() {
		// Track mood before resolve for change detection
		var preMood karate.AIMood
		if session.IsVsAI {
			preMood = session.Game.GetState().AIMood
		}

		state, err := session.Game.ResolveTurn()
		if err != nil {
			s.sendError(client, "resolve_error", err.Error())
			return
		}

		// Log mood change if playing vs AI
		if session.IsVsAI && state.AIMood != preMood {
			log.Printf("Session %s: AI mood %s → %s %s",
				session.ID, moodEmoji(preMood), moodEmoji(state.AIMood), moodDescription(state.AIMood))
		}

		// Send updated state to all players with their available actions
		s.broadcastGameState(session, state)

		// Check for game over
		if state.GameOver {
			s.broadcastToSession(session, MsgTypeGameOver, map[string]any{
				"winner": state.Winner,
				"state":  state,
			})
		}
	} else {
		// Acknowledge action received, waiting for opponent
		s.sendMessage(client, MsgTypeActionResult, map[string]any{
			"status":  "pending",
			"message": "Action received, waiting for opponent",
		})
	}
}

func (s *Server) handleReady(client *Client) {
	client.mu.Lock()
	client.Ready = true
	client.mu.Unlock()

	if client.Session != nil {
		s.sendGameState(client)
	}
}

func (s *Server) handleLeave(client *Client) {
	if client.Session != nil {
		session := client.Session

		// Notify opponent if in PvP
		if !session.IsVsAI {
			var opponent *Client
			if session.Player1 == client {
				opponent = session.Player2
			} else {
				opponent = session.Player1
			}

			if opponent != nil {
				s.sendMessage(opponent, MsgTypeGameOver, map[string]any{
					"reason": "opponent_left",
					"winner": opponent.Player,
				})
			}
		}

		// Remove session
		s.mu.Lock()
		delete(s.sessions, session.ID)
		s.mu.Unlock()

		client.Session = nil
	}

	// Remove from queue if present
	s.mu.Lock()
	for i, c := range s.matchQueue {
		if c == client {
			s.matchQueue = append(s.matchQueue[:i], s.matchQueue[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
}

func (s *Server) createSession(p1, p2 *Client, vsAI bool) *GameSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createSessionLocked(p1, p2, vsAI)
}

func (s *Server) createSessionLocked(p1, p2 *Client, vsAI bool) *GameSession {
	session := &GameSession{
		ID:        generateID(),
		Game:      karate.NewGame(),
		Player1:   p1,
		Player2:   p2,
		IsVsAI:    vsAI,
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
	availableActions := client.Session.Game.GetAvailableActions(client.Player)

	s.sendMessage(client, MsgTypeGameState, map[string]any{
		"state":             state,
		"available_actions": availableActions,
		"your_player":       client.Player,
	})
}

func (s *Server) broadcastToSession(session *GameSession, msgType MessageType, payload any) {
	if session.Player1 != nil {
		s.sendMessage(session.Player1, msgType, payload)
	}
	if session.Player2 != nil {
		s.sendMessage(session.Player2, msgType, payload)
	}
}

func (s *Server) broadcastGameState(session *GameSession, state karate.GameState) {
	// Send to Player1 with their available actions
	if session.Player1 != nil {
		availableActions := session.Game.GetAvailableActions(karate.Player1)
		s.sendMessage(session.Player1, MsgTypeGameState, map[string]any{
			"state":             state,
			"available_actions": availableActions,
			"your_player":       karate.Player1,
		})
	}
	// Send to Player2 with their available actions (if human)
	if session.Player2 != nil {
		availableActions := session.Game.GetAvailableActions(karate.Player2)
		s.sendMessage(session.Player2, MsgTypeGameState, map[string]any{
			"state":             state,
			"available_actions": availableActions,
			"your_player":       karate.Player2,
		})
	}
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

// moodEmoji returns an emoji representing the AI's current mood
func moodEmoji(mood karate.AIMood) string {
	switch mood {
	case karate.MoodCalm:
		return "😌"
	case karate.MoodAggressive:
		return "😡"
	case karate.MoodBored:
		return "😴"
	case karate.MoodTired:
		return "😫"
	default:
		return "🤖"
	}
}

// moodDescription returns a description of the AI's mood
func moodDescription(mood karate.AIMood) string {
	switch mood {
	case karate.MoodCalm:
		return "(focused)"
	case karate.MoodAggressive:
		return "(wants revenge!)"
	case karate.MoodBored:
		return "(looking for action!)"
	case karate.MoodTired:
		return "(needs rest)"
	default:
		return ""
	}
}
