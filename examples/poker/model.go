package poker

import (
	"github.com/pflow-xyz/go-pflow/petri"
)

// GamePhase represents the current phase of the game
type GamePhase int

const (
	PhasePreflop GamePhase = iota
	PhaseFlop
	PhaseTurn
	PhaseRiver
	PhaseShowdown
	PhaseComplete
)

func (p GamePhase) String() string {
	return [...]string{
		"Pre-flop",
		"Flop",
		"Turn",
		"River",
		"Showdown",
		"Complete",
	}[p]
}

// Action represents a player action
type Action int

const (
	ActionFold Action = iota
	ActionCheck
	ActionCall
	ActionRaise
	ActionAllIn
)

func (a Action) String() string {
	return [...]string{"Fold", "Check", "Call", "Raise", "All-in"}[a]
}

// CreatePokerPetriNet creates a Petri net model for Texas Hold'em
// The model captures:
// - Game phases (pre-flop, flop, turn, river, showdown)
// - Betting rounds within each phase
// - Player actions (fold, check, call, raise, all-in)
// - Win conditions and pot distribution
func CreatePokerPetriNet(numPlayers int) *petri.PetriNet {
	net := petri.Build().
		// === GAME PHASE PLACES ===
		Place("phase_preflop", 1).
		Place("phase_flop", 0).
		Place("phase_turn", 0).
		Place("phase_river", 0).
		Place("phase_showdown", 0).
		Place("phase_complete", 0).

		// === PLAYER STATE PLACES (per player) ===
		// Player 1 (dealer position / button)
		Place("p1_active", 1).     // Player is still in hand
		Place("p1_folded", 0).     // Player has folded
		Place("p1_acted", 0).      // Player has acted this round
		Place("p1_turn", 0).       // It's player 1's turn
		Place("p1_bet", 0).        // Amount bet this round
		Place("p1_chips", 1000).   // Stack size
		Place("p1_pot_share", 0).  // Accumulated pot contributions
		Place("p1_hand_str", 0.5). // Normalized hand strength (0-1)
		Place("p1_wins", 0).       // Win accumulator

		// Player 2
		Place("p2_active", 1).
		Place("p2_folded", 0).
		Place("p2_acted", 0).
		Place("p2_turn", 0).
		Place("p2_bet", 0).
		Place("p2_chips", 1000).
		Place("p2_pot_share", 0).
		Place("p2_hand_str", 0.5).
		Place("p2_wins", 0).

		// === POT AND BETTING PLACES ===
		Place("pot", 0).            // Current pot size
		Place("bet_to_call", 0).    // Current bet amount to call
		Place("min_raise", 2).      // Minimum raise amount (big blind)
		Place("round_complete", 0). // All active players have acted

		// === PHASE TRANSITIONS ===
		Transition("deal_hole").
		Transition("deal_flop").
		Transition("deal_turn").
		Transition("deal_river").
		Transition("to_showdown").
		Transition("end_hand").

		// === PLAYER 1 ACTION TRANSITIONS ===
		Transition("p1_fold").
		Transition("p1_check").
		Transition("p1_call").
		Transition("p1_raise").
		Transition("p1_all_in").

		// === PLAYER 2 ACTION TRANSITIONS ===
		Transition("p2_fold").
		Transition("p2_check").
		Transition("p2_call").
		Transition("p2_raise").
		Transition("p2_all_in").

		// === BETTING ROUND TRANSITIONS ===
		Transition("start_p1_turn").
		Transition("start_p2_turn").
		Transition("complete_round").

		// === WIN TRANSITIONS ===
		Transition("p1_wins_pot").
		Transition("p2_wins_pot").

		// === ARCS ===
		// Phase transitions
		Flow("phase_preflop", "deal_hole", "phase_preflop", 1).
		Flow("phase_preflop", "deal_flop", "phase_flop", 1).
		Flow("phase_flop", "deal_turn", "phase_turn", 1).
		Flow("phase_turn", "deal_river", "phase_river", 1).
		Flow("phase_river", "to_showdown", "phase_showdown", 1).
		Flow("phase_showdown", "end_hand", "phase_complete", 1).

		// Player 1 fold
		Arc("p1_turn", "p1_fold", 1).
		Arc("p1_active", "p1_fold", 1).
		Arc("p1_fold", "p1_folded", 1).
		Arc("p1_fold", "p1_acted", 1).

		// Player 1 check (when bet_to_call == 0)
		Arc("p1_turn", "p1_check", 1).
		Arc("p1_active", "p1_check", 1).
		Arc("p1_check", "p1_active", 1).
		Arc("p1_check", "p1_acted", 1).

		// Player 1 call
		Arc("p1_turn", "p1_call", 1).
		Arc("p1_active", "p1_call", 1).
		Arc("p1_call", "p1_active", 1).
		Arc("p1_call", "p1_acted", 1).

		// Player 1 raise
		Arc("p1_turn", "p1_raise", 1).
		Arc("p1_active", "p1_raise", 1).
		Arc("p1_raise", "p1_active", 1).
		Arc("p1_raise", "p1_acted", 1).

		// Player 1 all-in
		Arc("p1_turn", "p1_all_in", 1).
		Arc("p1_active", "p1_all_in", 1).
		Arc("p1_all_in", "p1_active", 1).
		Arc("p1_all_in", "p1_acted", 1).

		// Player 2 fold
		Arc("p2_turn", "p2_fold", 1).
		Arc("p2_active", "p2_fold", 1).
		Arc("p2_fold", "p2_folded", 1).
		Arc("p2_fold", "p2_acted", 1).

		// Player 2 check
		Arc("p2_turn", "p2_check", 1).
		Arc("p2_active", "p2_check", 1).
		Arc("p2_check", "p2_active", 1).
		Arc("p2_check", "p2_acted", 1).

		// Player 2 call
		Arc("p2_turn", "p2_call", 1).
		Arc("p2_active", "p2_call", 1).
		Arc("p2_call", "p2_active", 1).
		Arc("p2_call", "p2_acted", 1).

		// Player 2 raise
		Arc("p2_turn", "p2_raise", 1).
		Arc("p2_active", "p2_raise", 1).
		Arc("p2_raise", "p2_active", 1).
		Arc("p2_raise", "p2_acted", 1).

		// Player 2 all-in
		Arc("p2_turn", "p2_all_in", 1).
		Arc("p2_active", "p2_all_in", 1).
		Arc("p2_all_in", "p2_active", 1).
		Arc("p2_all_in", "p2_acted", 1).

		// Turn management
		Arc("p1_acted", "start_p2_turn", 1).
		Arc("p2_active", "start_p2_turn", 1).
		Arc("start_p2_turn", "p2_turn", 1).
		Arc("start_p2_turn", "p2_active", 1).

		Arc("p2_acted", "start_p1_turn", 1).
		Arc("p1_active", "start_p1_turn", 1).
		Arc("start_p1_turn", "p1_turn", 1).
		Arc("start_p1_turn", "p1_active", 1).

		// Win detection - Player 1 wins when Player 2 folds
		Arc("p2_folded", "p1_wins_pot", 1).
		Arc("p1_active", "p1_wins_pot", 1).
		Arc("pot", "p1_wins_pot", 1).
		Arc("p1_wins_pot", "p1_wins", 1).

		// Win detection - Player 2 wins when Player 1 folds
		Arc("p1_folded", "p2_wins_pot", 1).
		Arc("p2_active", "p2_wins_pot", 1).
		Arc("pot", "p2_wins_pot", 1).
		Arc("p2_wins_pot", "p2_wins", 1).

		Done()

	return net
}

// DefaultRates returns transition rates for poker actions
// Rates are based on typical action frequencies and hand strength
func DefaultRates() map[string]float64 {
	return map[string]float64{
		// Phase transitions (controlled by game logic)
		"deal_hole":    0.0,
		"deal_flop":    0.0,
		"deal_turn":    0.0,
		"deal_river":   0.0,
		"to_showdown":  0.0,
		"end_hand":     0.0,

		// Player 1 actions (base rates, modified by hand strength)
		"p1_fold":   0.2, // Fold rate decreases with hand strength
		"p1_check":  0.3, // Check when possible
		"p1_call":   0.3, // Call rate based on pot odds
		"p1_raise":  0.15, // Raise with strong hands
		"p1_all_in": 0.05, // All-in with premium hands

		// Player 2 actions
		"p2_fold":   0.2,
		"p2_check":  0.3,
		"p2_call":   0.3,
		"p2_raise":  0.15,
		"p2_all_in": 0.05,

		// Turn management
		"start_p1_turn":  1.0,
		"start_p2_turn":  1.0,
		"complete_round": 1.0,

		// Win transitions (enabled by game state)
		"p1_wins_pot": 0.0,
		"p2_wins_pot": 0.0,
	}
}

// StrengthAdjustedRates returns rates adjusted for hand strength
// Higher hand strength increases raise/call rates, decreases fold rate
func StrengthAdjustedRates(baseRates map[string]float64, p1Strength, p2Strength float64) map[string]float64 {
	rates := make(map[string]float64)
	for k, v := range baseRates {
		rates[k] = v
	}

	// Adjust Player 1 action rates based on hand strength
	// Fold rate decreases with strength (strong hands don't fold)
	rates["p1_fold"] = baseRates["p1_fold"] * (1.0 - p1Strength)
	// Call/raise rates increase with strength
	rates["p1_call"] = baseRates["p1_call"] * (0.5 + p1Strength*0.5)
	rates["p1_raise"] = baseRates["p1_raise"] * p1Strength * 2.0
	rates["p1_all_in"] = baseRates["p1_all_in"] * p1Strength * 3.0

	// Adjust Player 2 action rates
	rates["p2_fold"] = baseRates["p2_fold"] * (1.0 - p2Strength)
	rates["p2_call"] = baseRates["p2_call"] * (0.5 + p2Strength*0.5)
	rates["p2_raise"] = baseRates["p2_raise"] * p2Strength * 2.0
	rates["p2_all_in"] = baseRates["p2_all_in"] * p2Strength * 3.0

	return rates
}

// PositionAdjustedRates adds position-based adjustments
// Later positions (button) can play more aggressively
func PositionAdjustedRates(rates map[string]float64, isButton bool) map[string]float64 {
	if !isButton {
		return rates
	}

	adjusted := make(map[string]float64)
	for k, v := range rates {
		adjusted[k] = v
	}

	// Button position gets 20% boost to aggressive actions
	adjusted["p1_raise"] *= 1.2
	adjusted["p1_all_in"] *= 1.2
	adjusted["p1_fold"] *= 0.8

	return adjusted
}

// PotOddsAdjustment adjusts call rate based on pot odds
// pot_odds = pot / bet_to_call
// If pot odds > hand equity, calling is profitable
func PotOddsAdjustment(rates map[string]float64, pot, betToCall, handEquity float64) map[string]float64 {
	adjusted := make(map[string]float64)
	for k, v := range rates {
		adjusted[k] = v
	}

	if betToCall <= 0 {
		return adjusted
	}

	potOdds := pot / betToCall

	// Guard against division by zero (0 equity means never call)
	if handEquity <= 0 {
		adjusted["p1_call"] *= 0.1
		adjusted["p1_fold"] *= 2.0
		return adjusted
	}

	// If pot odds are good relative to equity, increase call rate
	// Pot odds > 1/equity means calling is profitable long-term
	if potOdds > (1.0 / handEquity) {
		adjusted["p1_call"] *= 1.5
		adjusted["p1_fold"] *= 0.5
	} else {
		// Bad pot odds, more likely to fold
		adjusted["p1_call"] *= 0.7
		adjusted["p1_fold"] *= 1.3
	}

	return adjusted
}
