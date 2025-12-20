// Package karate implements a 2-player karate fighting game using Petri nets.
//
// The game models two fighters with health, stamina, and position.
// Each fighter can punch, kick, block, or move. Actions consume stamina
// and can deal damage if they connect. Blocking reduces incoming damage.
//
// The Petri net models:
// - Fighter health (tokens = HP remaining)
// - Fighter stamina (tokens = stamina points)
// - Fighter position (tokens in position places)
// - Fighter stance (blocking, attacking, neutral)
// - Action history for combo detection
package karate

import (
	"fmt"
	"sync"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/hypothesis"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/stateutil"
)

// Game constants
const (
	MaxHealth  = 100.0
	MaxStamina = 50.0

	// Positions (0=left, 1=center-left, 2=center, 3=center-right, 4=right)
	NumPositions = 5

	// Damage values
	PunchDamage     = 10.0
	KickDamage      = 15.0
	SpecialDamage   = 25.0
	BlockReduction  = 0.5 // Blocks reduce damage by 50%

	// Stamina costs
	PunchStamina   = 5.0
	KickStamina    = 8.0
	SpecialStamina = 15.0
	BlockStamina   = 3.0
	MoveStamina    = 2.0

	// Recovery
	StaminaRecovery = 2.0 // Per tick when not acting
)

// Player identifies a player in the game
type Player int

const (
	Player1 Player = 1
	Player2 Player = 2
)

func (p Player) String() string {
	if p == Player1 {
		return "P1"
	}
	return "P2"
}

func (p Player) Opponent() Player {
	if p == Player1 {
		return Player2
	}
	return Player1
}

// ActionType represents the type of action a player can take
type ActionType string

const (
	ActionPunch   ActionType = "punch"
	ActionKick    ActionType = "kick"
	ActionSpecial ActionType = "special"
	ActionBlock   ActionType = "block"
	ActionMoveL   ActionType = "move_left"
	ActionMoveR   ActionType = "move_right"
	ActionRecover ActionType = "recover"
)

// Action represents a game action
type Action struct {
	Player Player
	Type   ActionType
}

// AIMood represents the AI's current emotional state
type AIMood string

const (
	MoodCalm       AIMood = "calm"       // Balanced play
	MoodAggressive AIMood = "aggressive" // Attacks relentlessly, closes distance
	MoodBored      AIMood = "bored"      // Wants action, will attack
	MoodTired      AIMood = "tired"      // Low stamina, recovers
)

// GameState represents the observable game state
type GameState struct {
	P1Health   float64 `json:"p1_health"`
	P1Stamina  float64 `json:"p1_stamina"`
	P1Position int     `json:"p1_position"`
	P1Blocking bool    `json:"p1_blocking"`

	P2Health   float64 `json:"p2_health"`
	P2Stamina  float64 `json:"p2_stamina"`
	P2Position int     `json:"p2_position"`
	P2Blocking bool    `json:"p2_blocking"`

	Winner     Player `json:"winner,omitempty"`
	GameOver   bool   `json:"game_over"`
	RoundNum   int    `json:"round_num"`
	TurnNum    int    `json:"turn_num"`
	LastAction string `json:"last_action,omitempty"`
	AIMood     AIMood `json:"ai_mood,omitempty"`
}

// Game represents a karate fighting game instance
type Game struct {
	mu sync.RWMutex

	net    *petri.PetriNet
	engine *engine.Engine
	rates  map[string]float64

	// AI evaluator for single-player mode
	aiEval *hypothesis.Evaluator

	// Game state tracking
	roundNum int
	turnNum  int
	gameOver bool
	winner   Player

	// Action history for this turn (both players submit, then resolve)
	pendingActions map[Player]ActionType
}

// AddAIMoodToNet adds AI mood places and transitions to the game's Petri net.
// The mood is modeled as places with 1-token invariant (exactly one mood active).
// History places track mood transitions for ODE analysis.
//
// Places:
//   - AI_mood_calm, AI_mood_aggressive, AI_mood_bored, AI_mood_tired (current mood)
//   - AI_hist_got_hit, AI_hist_attacked, AI_hist_passive (history counters)
//
// Transitions:
//   - AI_to_aggressive, AI_to_calm, AI_to_bored, AI_to_tired (mood changes)
func AddAIMoodToNet(net *petri.PetriNet) {
	// Mood state places (1-token invariant: exactly one is active)
	net.AddPlace("AI_mood_calm", 1, nil, 500, 100, nil)       // Start calm
	net.AddPlace("AI_mood_aggressive", 0, nil, 500, 150, nil)
	net.AddPlace("AI_mood_bored", 0, nil, 500, 200, nil)
	net.AddPlace("AI_mood_tired", 0, nil, 500, 250, nil)

	// History counters (accumulate over game for analysis)
	net.AddPlace("AI_hist_got_hit", 0, nil, 600, 100, nil)        // Times AI was hit
	net.AddPlace("AI_hist_attacked", 0, nil, 600, 150, nil)       // Times AI attacked
	net.AddPlace("AI_hist_passive", 0, nil, 600, 200, nil)        // Passive turn count
	net.AddPlace("AI_hist_mood_changes", 0, nil, 600, 250, nil)   // Total mood transitions
	net.AddPlace("AI_consecutive_blocks", 0, nil, 600, 300, nil)  // Consecutive blocks (max 2)

	// Mood transition: calm -> aggressive (got hit)
	net.AddTransition("AI_calm_to_aggressive", "default", 550, 125, nil)
	net.AddArc("AI_mood_calm", "AI_calm_to_aggressive", 1, false)
	net.AddArc("AI_calm_to_aggressive", "AI_mood_aggressive", 1, false)
	net.AddArc("AI_calm_to_aggressive", "AI_hist_mood_changes", 1, false)

	// Mood transition: bored -> aggressive (got hit)
	net.AddTransition("AI_bored_to_aggressive", "default", 550, 175, nil)
	net.AddArc("AI_mood_bored", "AI_bored_to_aggressive", 1, false)
	net.AddArc("AI_bored_to_aggressive", "AI_mood_aggressive", 1, false)
	net.AddArc("AI_bored_to_aggressive", "AI_hist_mood_changes", 1, false)

	// Mood transition: tired -> aggressive (got hit - adrenaline!)
	net.AddTransition("AI_tired_to_aggressive", "default", 550, 225, nil)
	net.AddArc("AI_mood_tired", "AI_tired_to_aggressive", 1, false)
	net.AddArc("AI_tired_to_aggressive", "AI_mood_aggressive", 1, false)
	net.AddArc("AI_tired_to_aggressive", "AI_hist_mood_changes", 1, false)

	// Mood transition: aggressive -> calm (attacked, vented)
	net.AddTransition("AI_aggressive_to_calm", "default", 550, 140, nil)
	net.AddArc("AI_mood_aggressive", "AI_aggressive_to_calm", 1, false)
	net.AddArc("AI_aggressive_to_calm", "AI_mood_calm", 1, false)
	net.AddArc("AI_aggressive_to_calm", "AI_hist_mood_changes", 1, false)

	// Mood transition: calm -> bored (passive turns)
	net.AddTransition("AI_calm_to_bored", "default", 550, 160, nil)
	net.AddArc("AI_mood_calm", "AI_calm_to_bored", 1, false)
	net.AddArc("AI_calm_to_bored", "AI_mood_bored", 1, false)
	net.AddArc("AI_calm_to_bored", "AI_hist_mood_changes", 1, false)

	// Mood transition: bored -> calm (attacked)
	net.AddTransition("AI_bored_to_calm", "default", 550, 190, nil)
	net.AddArc("AI_mood_bored", "AI_bored_to_calm", 1, false)
	net.AddArc("AI_bored_to_calm", "AI_mood_calm", 1, false)
	net.AddArc("AI_bored_to_calm", "AI_hist_mood_changes", 1, false)

	// Mood transition: calm -> tired (low stamina)
	net.AddTransition("AI_calm_to_tired", "default", 550, 210, nil)
	net.AddArc("AI_mood_calm", "AI_calm_to_tired", 1, false)
	net.AddArc("AI_calm_to_tired", "AI_mood_tired", 1, false)
	net.AddArc("AI_calm_to_tired", "AI_hist_mood_changes", 1, false)

	// Mood transition: bored -> tired (low stamina)
	net.AddTransition("AI_bored_to_tired", "default", 550, 230, nil)
	net.AddArc("AI_mood_bored", "AI_bored_to_tired", 1, false)
	net.AddArc("AI_bored_to_tired", "AI_mood_tired", 1, false)
	net.AddArc("AI_bored_to_tired", "AI_hist_mood_changes", 1, false)

	// Mood transition: tired -> calm (stamina recovered)
	net.AddTransition("AI_tired_to_calm", "default", 550, 240, nil)
	net.AddArc("AI_mood_tired", "AI_tired_to_calm", 1, false)
	net.AddArc("AI_tired_to_calm", "AI_mood_calm", 1, false)
	net.AddArc("AI_tired_to_calm", "AI_hist_mood_changes", 1, false)
}

// NewGame creates a new karate fighting game
func NewGame() *Game {
	net := BuildKarateNet()
	AddAIMoodToNet(net) // Integrate mood into the Petri net
	initialState := InitialState(net)
	rates := DefaultRates(net)

	eng := engine.NewEngine(net, initialState, rates)

	g := &Game{
		net:            net,
		engine:         eng,
		rates:          rates,
		roundNum:       1,
		turnNum:        1,
		pendingActions: make(map[Player]ActionType),
	}

	// Create AI evaluator for opponent
	// Now considers mood in the evaluation since it's part of the net
	g.aiEval = hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
		// AI plays as Player2, wants to maximize P2 health and minimize P1 health
		// Mood affects evaluation: aggressive mood boosts attack value
		score := final["P2_health"] - final["P1_health"]

		// Bonus for being aggressive when health advantage
		if final["AI_mood_aggressive"] > 0.5 && final["P2_health"] > final["P1_health"] {
			score += 5
		}

		return score
	}).WithTimeSpan(0, 3.0).WithOptions(solver.FastOptions())

	return g
}

// BuildKarateNet constructs the Petri net model for the karate game
func BuildKarateNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// ========================================
	// PLACES
	// ========================================

	// Player 1 places
	net.AddPlace("P1_health", MaxHealth, nil, 100, 100, nil)
	net.AddPlace("P1_stamina", MaxStamina, nil, 100, 150, nil)
	net.AddPlace("P1_blocking", 0, nil, 100, 200, nil)

	// Player 1 positions (one token in current position)
	for i := 0; i < NumPositions; i++ {
		tokens := 0.0
		if i == 1 { // P1 starts at position 1 (center-left)
			tokens = 1.0
		}
		net.AddPlace(fmt.Sprintf("P1_pos%d", i), tokens, nil, float64(50+i*50), 250, nil)
	}

	// Player 1 action history (for combo tracking)
	net.AddPlace("P1_last_punch", 0, nil, 100, 300, nil)
	net.AddPlace("P1_last_kick", 0, nil, 100, 350, nil)

	// Player 2 places
	net.AddPlace("P2_health", MaxHealth, nil, 400, 100, nil)
	net.AddPlace("P2_stamina", MaxStamina, nil, 400, 150, nil)
	net.AddPlace("P2_blocking", 0, nil, 400, 200, nil)

	// Player 2 positions
	for i := 0; i < NumPositions; i++ {
		tokens := 0.0
		if i == 3 { // P2 starts at position 3 (center-right)
			tokens = 1.0
		}
		net.AddPlace(fmt.Sprintf("P2_pos%d", i), tokens, nil, float64(350+i*50), 250, nil)
	}

	// Player 2 action history
	net.AddPlace("P2_last_punch", 0, nil, 400, 300, nil)
	net.AddPlace("P2_last_kick", 0, nil, 400, 350, nil)

	// Win condition places
	net.AddPlace("P1_wins", 0, nil, 100, 400, nil)
	net.AddPlace("P2_wins", 0, nil, 400, 400, nil)

	// Distance indicator (derived from positions)
	net.AddPlace("in_range", 0, nil, 250, 250, nil)

	// ========================================
	// TRANSITIONS
	// ========================================

	// Player 1 actions
	net.AddTransition("P1_punch", "default", 150, 100, nil)
	net.AddTransition("P1_kick", "default", 150, 150, nil)
	net.AddTransition("P1_special", "default", 150, 200, nil)
	net.AddTransition("P1_block", "default", 150, 250, nil)
	net.AddTransition("P1_move_left", "default", 150, 300, nil)
	net.AddTransition("P1_move_right", "default", 150, 350, nil)
	net.AddTransition("P1_recover", "default", 150, 400, nil)

	// Player 2 actions
	net.AddTransition("P2_punch", "default", 350, 100, nil)
	net.AddTransition("P2_kick", "default", 350, 150, nil)
	net.AddTransition("P2_special", "default", 350, 200, nil)
	net.AddTransition("P2_block", "default", 350, 250, nil)
	net.AddTransition("P2_move_left", "default", 350, 300, nil)
	net.AddTransition("P2_move_right", "default", 350, 350, nil)
	net.AddTransition("P2_recover", "default", 350, 400, nil)

	// Damage resolution transitions
	net.AddTransition("P1_hit_P2", "default", 250, 100, nil)
	net.AddTransition("P2_hit_P1", "default", 250, 150, nil)

	// Win transitions
	net.AddTransition("P1_victory", "default", 100, 450, nil)
	net.AddTransition("P2_victory", "default", 400, 450, nil)

	// ========================================
	// ARCS
	// ========================================

	// P1 punch: consumes stamina, produces punch history
	net.AddArc("P1_stamina", "P1_punch", PunchStamina, false)
	net.AddArc("P1_punch", "P1_last_punch", 1, false)

	// P1 kick: consumes more stamina
	net.AddArc("P1_stamina", "P1_kick", KickStamina, false)
	net.AddArc("P1_kick", "P1_last_kick", 1, false)

	// P1 special: consumes high stamina
	net.AddArc("P1_stamina", "P1_special", SpecialStamina, false)

	// P1 block: consumes stamina, produces blocking state
	net.AddArc("P1_stamina", "P1_block", BlockStamina, false)
	net.AddArc("P1_block", "P1_blocking", 1, false)

	// P1 recover: produces stamina
	net.AddArc("P1_recover", "P1_stamina", StaminaRecovery, false)

	// P2 punch
	net.AddArc("P2_stamina", "P2_punch", PunchStamina, false)
	net.AddArc("P2_punch", "P2_last_punch", 1, false)

	// P2 kick
	net.AddArc("P2_stamina", "P2_kick", KickStamina, false)
	net.AddArc("P2_kick", "P2_last_kick", 1, false)

	// P2 special
	net.AddArc("P2_stamina", "P2_special", SpecialStamina, false)

	// P2 block
	net.AddArc("P2_stamina", "P2_block", BlockStamina, false)
	net.AddArc("P2_block", "P2_blocking", 1, false)

	// P2 recover
	net.AddArc("P2_recover", "P2_stamina", StaminaRecovery, false)

	// Damage arcs: P1 attacks damage P2, P2 attacks damage P1
	// In ODE simulation, in_range enables these transitions
	net.AddArc("in_range", "P1_hit_P2", 1, false)
	net.AddArc("P1_hit_P2", "in_range", 1, false) // Keep in_range token
	net.AddArc("P1_last_punch", "P1_hit_P2", 1, false)
	net.AddArc("P1_hit_P2", "P2_health", -PunchDamage, false) // Negative = damage

	net.AddArc("in_range", "P2_hit_P1", 1, false)
	net.AddArc("P2_hit_P1", "in_range", 1, false)
	net.AddArc("P2_last_punch", "P2_hit_P1", 1, false)
	net.AddArc("P2_hit_P1", "P1_health", -PunchDamage, false)

	// Victory conditions
	// P1 wins when P2 health depleted
	net.AddArc("P1_victory", "P1_wins", 1, false)
	// P2 wins when P1 health depleted
	net.AddArc("P2_victory", "P2_wins", 1, false)

	return net
}

// InitialState returns the initial game state
func InitialState(net *petri.PetriNet) map[string]float64 {
	state := net.SetState(nil)

	// Override with explicit initial values
	state["P1_health"] = MaxHealth
	state["P1_stamina"] = MaxStamina
	state["P1_blocking"] = 0

	state["P2_health"] = MaxHealth
	state["P2_stamina"] = MaxStamina
	state["P2_blocking"] = 0

	// Clear all positions first
	for i := 0; i < NumPositions; i++ {
		state[fmt.Sprintf("P1_pos%d", i)] = 0
		state[fmt.Sprintf("P2_pos%d", i)] = 0
	}

	// Set starting positions
	state["P1_pos1"] = 1 // P1 starts center-left
	state["P2_pos3"] = 1 // P2 starts center-right

	// Initial distance: 2 positions apart
	state["in_range"] = 0 // Not in range initially (need to be adjacent)

	// Clear history and wins
	state["P1_last_punch"] = 0
	state["P1_last_kick"] = 0
	state["P2_last_punch"] = 0
	state["P2_last_kick"] = 0
	state["P1_wins"] = 0
	state["P2_wins"] = 0

	// AI mood state (1-token invariant: exactly one active)
	state["AI_mood_calm"] = 1 // Start calm
	state["AI_mood_aggressive"] = 0
	state["AI_mood_bored"] = 0
	state["AI_mood_tired"] = 0

	// AI history counters
	state["AI_hist_got_hit"] = 0
	state["AI_hist_attacked"] = 0
	state["AI_hist_passive"] = 0
	state["AI_hist_mood_changes"] = 0
	state["AI_consecutive_blocks"] = 0

	return state
}

// DefaultRates returns default transition rates
func DefaultRates(net *petri.PetriNet) map[string]float64 {
	rates := make(map[string]float64)
	for trans := range net.Transitions {
		rates[trans] = 1.0
	}
	return rates
}

// GetState returns the current observable game state
func (g *Game) GetState() GameState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	raw := g.engine.GetState()

	return GameState{
		P1Health:   raw["P1_health"],
		P1Stamina:  raw["P1_stamina"],
		P1Position: g.getPosition(raw, Player1),
		P1Blocking: raw["P1_blocking"] > 0.5,

		P2Health:   raw["P2_health"],
		P2Stamina:  raw["P2_stamina"],
		P2Position: g.getPosition(raw, Player2),
		P2Blocking: raw["P2_blocking"] > 0.5,

		Winner:   g.winner,
		GameOver: g.gameOver,
		RoundNum: g.roundNum,
		TurnNum:  g.turnNum,
		AIMood:   g.getAIMood(),
	}
}

// getAIMood returns the current AI mood from the Petri net state
func (g *Game) getAIMood() AIMood {
	state := g.engine.GetState()
	return getMoodFromState(state)
}

// getMoodFromState extracts the AI mood from a raw state map
func getMoodFromState(state map[string]float64) AIMood {
	if state["AI_mood_aggressive"] > 0.5 {
		return MoodAggressive
	}
	if state["AI_mood_bored"] > 0.5 {
		return MoodBored
	}
	if state["AI_mood_tired"] > 0.5 {
		return MoodTired
	}
	return MoodCalm
}

func (g *Game) getPosition(state map[string]float64, player Player) int {
	prefix := "P1_pos"
	if player == Player2 {
		prefix = "P2_pos"
	}

	for i := 0; i < NumPositions; i++ {
		if state[fmt.Sprintf("%s%d", prefix, i)] > 0.5 {
			return i
		}
	}
	return 2 // Default to center
}

// GetAvailableActions returns valid actions for a player
func (g *Game) GetAvailableActions(player Player) []ActionType {
	g.mu.RLock()
	defer g.mu.RUnlock()

	state := g.engine.GetState()
	prefix := "P1"
	if player == Player2 {
		prefix = "P2"
	}

	stamina := state[fmt.Sprintf("%s_stamina", prefix)]
	pos := g.getPosition(state, player)

	actions := []ActionType{}

	// Always can recover
	actions = append(actions, ActionRecover)

	// Movement
	if pos > 0 && stamina >= MoveStamina {
		actions = append(actions, ActionMoveL)
	}
	if pos < NumPositions-1 && stamina >= MoveStamina {
		actions = append(actions, ActionMoveR)
	}

	// Combat actions
	if stamina >= PunchStamina {
		actions = append(actions, ActionPunch)
	}
	if stamina >= KickStamina {
		actions = append(actions, ActionKick)
	}
	if stamina >= SpecialStamina {
		actions = append(actions, ActionSpecial)
	}
	if stamina >= BlockStamina {
		actions = append(actions, ActionBlock)
	}

	return actions
}

// SubmitAction submits an action for a player
func (g *Game) SubmitAction(player Player, action ActionType) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameOver {
		return fmt.Errorf("game is over")
	}

	// Validate action is available
	available := false
	for _, a := range g.getAvailableActionsLocked(player) {
		if a == action {
			available = true
			break
		}
	}
	if !available {
		return fmt.Errorf("action %s not available for %s", action, player)
	}

	g.pendingActions[player] = action

	return nil
}

func (g *Game) getAvailableActionsLocked(player Player) []ActionType {
	state := g.engine.GetState()
	prefix := "P1"
	if player == Player2 {
		prefix = "P2"
	}

	stamina := state[fmt.Sprintf("%s_stamina", prefix)]
	pos := g.getPosition(state, player)

	actions := []ActionType{ActionRecover}

	if pos > 0 && stamina >= MoveStamina {
		actions = append(actions, ActionMoveL)
	}
	if pos < NumPositions-1 && stamina >= MoveStamina {
		actions = append(actions, ActionMoveR)
	}
	if stamina >= PunchStamina {
		actions = append(actions, ActionPunch)
	}
	if stamina >= KickStamina {
		actions = append(actions, ActionKick)
	}
	if stamina >= SpecialStamina {
		actions = append(actions, ActionSpecial)
	}
	if stamina >= BlockStamina {
		actions = append(actions, ActionBlock)
	}

	return actions
}

// getAIAvailableActionsLocked returns available actions for the AI (Player2)
// with additional constraints like blocking limit (max 2 consecutive blocks)
func (g *Game) getAIAvailableActionsLocked() []ActionType {
	actions := g.getAvailableActionsLocked(Player2)
	state := g.engine.GetState()

	// Remove block if AI has already blocked 2 times in a row
	consecutiveBlocks := int(state["AI_consecutive_blocks"])
	if consecutiveBlocks >= 2 {
		filtered := make([]ActionType, 0, len(actions))
		for _, a := range actions {
			if a != ActionBlock {
				filtered = append(filtered, a)
			}
		}
		return filtered
	}

	return actions
}

// HasBothActions returns true if both players have submitted actions
func (g *Game) HasBothActions() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, p1 := g.pendingActions[Player1]
	_, p2 := g.pendingActions[Player2]
	return p1 && p2
}

// ResolveTurn resolves the current turn with both actions
func (g *Game) ResolveTurn() (GameState, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.pendingActions) < 2 {
		return GameState{}, fmt.Errorf("waiting for both players to submit actions")
	}

	p1Action := g.pendingActions[Player1]
	p2Action := g.pendingActions[Player2]

	state := g.engine.GetState()
	oldP2Health := state["P2_health"]

	// Apply actions
	state = g.applyAction(state, Player1, p1Action)
	state = g.applyAction(state, Player2, p2Action)

	// Update distance/range calculation
	p1Pos := g.getPosition(state, Player1)
	p2Pos := g.getPosition(state, Player2)
	distance := p2Pos - p1Pos
	if distance < 0 {
		distance = -distance
	}
	if distance <= 1 {
		state["in_range"] = 1
	} else {
		state["in_range"] = 0
	}

	// Resolve damage
	state = g.resolveDamage(state, p1Action, p2Action)

	// Clear blocking at end of turn
	state["P1_blocking"] = 0
	state["P2_blocking"] = 0

	// Clear action history
	state["P1_last_punch"] = 0
	state["P1_last_kick"] = 0
	state["P2_last_punch"] = 0
	state["P2_last_kick"] = 0

	// Cap values
	state["P1_health"] = clamp(state["P1_health"], 0, MaxHealth)
	state["P1_stamina"] = clamp(state["P1_stamina"], 0, MaxStamina)
	state["P2_health"] = clamp(state["P2_health"], 0, MaxHealth)
	state["P2_stamina"] = clamp(state["P2_stamina"], 0, MaxStamina)

	// Update AI mood based on what happened (modifies state in place)
	g.updateAIMood(state, oldP2Health, p1Action, p2Action)

	// Update engine state (includes mood changes)
	g.engine.SetState(state)

	// Check for winner
	if state["P1_health"] <= 0 {
		g.gameOver = true
		g.winner = Player2
		state["P2_wins"] = 1
	} else if state["P2_health"] <= 0 {
		g.gameOver = true
		g.winner = Player1
		state["P1_wins"] = 1
	}

	// Clear pending actions and increment turn
	g.pendingActions = make(map[Player]ActionType)
	g.turnNum++

	return g.getStateLocked(state, fmt.Sprintf("%s:%s vs %s:%s", Player1, p1Action, Player2, p2Action)), nil
}

// updateAIMood updates the AI mood in the Petri net state based on game events.
// Mood transitions are modeled as direct state changes, firing the appropriate
// Petri net transition conceptually (updating history counters).
func (g *Game) updateAIMood(state map[string]float64, oldP2Health float64, p1Action, p2Action ActionType) {
	p2Health := state["P2_health"]
	p2Stamina := state["P2_stamina"]

	currentMood := getMoodFromState(state)
	passiveTurns := int(state["AI_hist_passive"])

	// Track consecutive blocks
	if p2Action == ActionBlock {
		state["AI_consecutive_blocks"]++
	} else {
		state["AI_consecutive_blocks"] = 0 // Reset on any other action
	}

	// Check if AI got hit
	gotHit := p2Health < oldP2Health
	if gotHit {
		state["AI_hist_got_hit"]++
		state["AI_hist_passive"] = 0 // Reset passive counter
	}

	// Check if AI did an attack
	didAttack := p2Action == ActionPunch || p2Action == ActionKick || p2Action == ActionSpecial
	if didAttack {
		state["AI_hist_attacked"]++
		state["AI_hist_passive"] = 0 // Reset passive counter
	}

	// Track passive turns (no combat)
	isCombat := p1Action == ActionPunch || p1Action == ActionKick || p1Action == ActionSpecial || didAttack
	if !isCombat {
		state["AI_hist_passive"]++
		passiveTurns = int(state["AI_hist_passive"])
	}

	// Determine new mood based on events and current mood
	newMood := currentMood

	switch currentMood {
	case MoodCalm:
		if gotHit || p2Health < MaxHealth*0.2 {
			// Got hit or low health -> aggressive
			newMood = MoodAggressive
		} else if p2Stamina < MaxStamina*0.2 {
			// Low stamina -> tired
			newMood = MoodTired
		} else if passiveTurns >= 3 {
			// Bored after 3 passive turns
			newMood = MoodBored
		}

	case MoodAggressive:
		if didAttack {
			// Vented aggression -> calm
			newMood = MoodCalm
		}
		// Aggressive ignores low stamina - keeps fighting!

	case MoodBored:
		if gotHit {
			// Got hit while bored -> aggressive
			newMood = MoodAggressive
		} else if didAttack {
			// Finally attacked -> calm
			newMood = MoodCalm
		} else if p2Stamina < MaxStamina*0.2 {
			// Low stamina -> tired
			newMood = MoodTired
		}

	case MoodTired:
		if gotHit {
			// Got hit -> adrenaline! -> aggressive
			newMood = MoodAggressive
		} else if p2Stamina >= MaxStamina*0.5 {
			// Stamina recovered -> calm
			newMood = MoodCalm
		}
	}

	// Apply mood change to state if changed
	if newMood != currentMood {
		// Clear old mood
		state["AI_mood_calm"] = 0
		state["AI_mood_aggressive"] = 0
		state["AI_mood_bored"] = 0
		state["AI_mood_tired"] = 0

		// Set new mood
		switch newMood {
		case MoodCalm:
			state["AI_mood_calm"] = 1
		case MoodAggressive:
			state["AI_mood_aggressive"] = 1
		case MoodBored:
			state["AI_mood_bored"] = 1
		case MoodTired:
			state["AI_mood_tired"] = 1
		}

		// Increment mood change counter
		state["AI_hist_mood_changes"]++
	}
}

func (g *Game) applyAction(state map[string]float64, player Player, action ActionType) map[string]float64 {
	prefix := "P1"
	if player == Player2 {
		prefix = "P2"
	}

	switch action {
	case ActionPunch:
		state[fmt.Sprintf("%s_stamina", prefix)] -= PunchStamina
		state[fmt.Sprintf("%s_last_punch", prefix)] = 1

	case ActionKick:
		state[fmt.Sprintf("%s_stamina", prefix)] -= KickStamina
		state[fmt.Sprintf("%s_last_kick", prefix)] = 1

	case ActionSpecial:
		state[fmt.Sprintf("%s_stamina", prefix)] -= SpecialStamina

	case ActionBlock:
		state[fmt.Sprintf("%s_stamina", prefix)] -= BlockStamina
		state[fmt.Sprintf("%s_blocking", prefix)] = 1

	case ActionMoveL:
		state[fmt.Sprintf("%s_stamina", prefix)] -= MoveStamina
		pos := g.getPosition(state, player)
		if pos > 0 {
			state[fmt.Sprintf("%s_pos%d", prefix, pos)] = 0
			state[fmt.Sprintf("%s_pos%d", prefix, pos-1)] = 1
		}

	case ActionMoveR:
		state[fmt.Sprintf("%s_stamina", prefix)] -= MoveStamina
		pos := g.getPosition(state, player)
		if pos < NumPositions-1 {
			state[fmt.Sprintf("%s_pos%d", prefix, pos)] = 0
			state[fmt.Sprintf("%s_pos%d", prefix, pos+1)] = 1
		}

	case ActionRecover:
		state[fmt.Sprintf("%s_stamina", prefix)] += StaminaRecovery
	}

	return state
}

func (g *Game) resolveDamage(state map[string]float64, p1Action, p2Action ActionType) map[string]float64 {
	inRange := state["in_range"] > 0.5

	// P1 attacks P2
	if inRange {
		damage := g.getDamage(p1Action)
		if damage > 0 {
			if state["P2_blocking"] > 0.5 {
				damage *= BlockReduction
			}
			state["P2_health"] -= damage
		}

		// P2 attacks P1
		damage = g.getDamage(p2Action)
		if damage > 0 {
			if state["P1_blocking"] > 0.5 {
				damage *= BlockReduction
			}
			state["P1_health"] -= damage
		}
	}

	return state
}

func (g *Game) getDamage(action ActionType) float64 {
	switch action {
	case ActionPunch:
		return PunchDamage
	case ActionKick:
		return KickDamage
	case ActionSpecial:
		return SpecialDamage
	default:
		return 0
	}
}

func (g *Game) getStateLocked(raw map[string]float64, lastAction string) GameState {
	return GameState{
		P1Health:   raw["P1_health"],
		P1Stamina:  raw["P1_stamina"],
		P1Position: g.getPosition(raw, Player1),
		P1Blocking: raw["P1_blocking"] > 0.5,

		P2Health:   raw["P2_health"],
		P2Stamina:  raw["P2_stamina"],
		P2Position: g.getPosition(raw, Player2),
		P2Blocking: raw["P2_blocking"] > 0.5,

		Winner:     g.winner,
		GameOver:   g.gameOver,
		RoundNum:   g.roundNum,
		TurnNum:    g.turnNum,
		LastAction: lastAction,
		AIMood:     g.getAIMood(),
	}
}

// GetAIMove returns the AI's chosen action for Player2
// The AI's mood influences decision making:
//   - calm: Uses ODE evaluation for balanced play
//   - aggressive: Closes distance and attacks relentlessly
//   - bored: Forces an attack to create action
//   - tired: Always recovers stamina
//
// Additional constraints:
//   - Cannot block more than 2 times in a row
func (g *Game) GetAIMove() ActionType {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Use AI-specific available actions (respects block limit)
	available := g.getAIAvailableActionsLocked()
	if len(available) == 0 {
		return ActionRecover
	}

	mood := g.getAIMood()
	currentState := g.engine.GetState()

	// Calculate distance to opponent
	p1Pos := g.getPosition(currentState, Player1)
	p2Pos := g.getPosition(currentState, Player2)
	distance := p2Pos - p1Pos
	if distance < 0 {
		distance = -distance
	}
	inRange := distance <= 1

	// Helper to check if action is available
	hasAction := func(target ActionType) bool {
		for _, a := range available {
			if a == target {
				return true
			}
		}
		return false
	}

	// Mood-based overrides for personality
	switch mood {
	case MoodTired:
		// When tired, always recover
		return ActionRecover

	case MoodBored:
		// When bored, force action - attack if in range, otherwise close distance
		if inRange {
			for _, a := range available {
				if a == ActionPunch || a == ActionKick || a == ActionSpecial {
					return a
				}
			}
		}
		// Move toward opponent (P2 moves left toward P1)
		if hasAction(ActionMoveL) {
			return ActionMoveL
		}

	case MoodAggressive:
		// When aggressive, close distance and attack
		if !inRange {
			// Not in range - move toward opponent
			if hasAction(ActionMoveL) {
				return ActionMoveL
			}
		}
		// In range - attack with strongest available
		if hasAction(ActionSpecial) {
			return ActionSpecial
		}
		if hasAction(ActionKick) {
			return ActionKick
		}
		if hasAction(ActionPunch) {
			return ActionPunch
		}

	}

	// Calm mood: Tactical AI with movement considerations

	// If not in range, consider moving toward opponent
	if !inRange && mood == MoodCalm {
		// 50% chance to close distance when calm and out of range
		if g.turnNum%2 == 0 && hasAction(ActionMoveL) {
			return ActionMoveL
		}
	}

	// Build candidate moves for ODE evaluation
	var updates []map[string]float64
	for _, action := range available {
		update := g.actionToStateUpdate(Player2, action, currentState)
		updates = append(updates, update)
	}

	// Find best move using ODE evaluation
	bestIdx, _ := g.aiEval.FindBestParallel(currentState, updates)
	if bestIdx < 0 || bestIdx >= len(available) {
		return available[0]
	}

	return available[bestIdx]
}

func (g *Game) actionToStateUpdate(player Player, action ActionType, currentState map[string]float64) map[string]float64 {
	// Create a hypothetical state update
	update := make(map[string]float64)
	prefix := "P1"
	if player == Player2 {
		prefix = "P2"
	}

	stamina := currentState[fmt.Sprintf("%s_stamina", prefix)]
	pos := g.getPosition(currentState, player)

	switch action {
	case ActionPunch:
		update[fmt.Sprintf("%s_stamina", prefix)] = stamina - PunchStamina
		update[fmt.Sprintf("%s_last_punch", prefix)] = 1

	case ActionKick:
		update[fmt.Sprintf("%s_stamina", prefix)] = stamina - KickStamina
		update[fmt.Sprintf("%s_last_kick", prefix)] = 1

	case ActionSpecial:
		update[fmt.Sprintf("%s_stamina", prefix)] = stamina - SpecialStamina

	case ActionBlock:
		update[fmt.Sprintf("%s_stamina", prefix)] = stamina - BlockStamina
		update[fmt.Sprintf("%s_blocking", prefix)] = 1

	case ActionMoveL:
		update[fmt.Sprintf("%s_stamina", prefix)] = stamina - MoveStamina
		if pos > 0 {
			update[fmt.Sprintf("%s_pos%d", prefix, pos)] = 0
			update[fmt.Sprintf("%s_pos%d", prefix, pos-1)] = 1
		}

	case ActionMoveR:
		update[fmt.Sprintf("%s_stamina", prefix)] = stamina - MoveStamina
		if pos < NumPositions-1 {
			update[fmt.Sprintf("%s_pos%d", prefix, pos)] = 0
			update[fmt.Sprintf("%s_pos%d", prefix, pos+1)] = 1
		}

	case ActionRecover:
		newStam := stamina + StaminaRecovery
		if newStam > MaxStamina {
			newStam = MaxStamina
		}
		update[fmt.Sprintf("%s_stamina", prefix)] = newStam
	}

	return update
}

// Reset resets the game to initial state
func (g *Game) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	initial := InitialState(g.net)
	g.engine.SetState(initial)
	g.roundNum = 1
	g.turnNum = 1
	g.gameOver = false
	g.winner = 0
	g.pendingActions = make(map[Player]ActionType)
}

// GetNet returns the underlying Petri net (for visualization)
func (g *Game) GetNet() *petri.PetriNet {
	return g.net
}

// GetRawState returns the raw engine state
func (g *Game) GetRawState() map[string]float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return stateutil.Copy(g.engine.GetState())
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
