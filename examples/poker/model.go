package poker

import (
	"fmt"

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

// CardPlaceName returns the place name for a card in the deck
// Format: "deck_<suit>_<rank>" e.g., "deck_c_2" for 2 of clubs
func CardPlaceName(suit Suit, rank Rank) string {
	suitChar := []string{"c", "d", "h", "s"}[suit]
	return fmt.Sprintf("deck_%s_%d", suitChar, int(rank))
}

// P1CardPlaceName returns the place name for a card in player 1's hand
func P1CardPlaceName(suit Suit, rank Rank) string {
	suitChar := []string{"c", "d", "h", "s"}[suit]
	return fmt.Sprintf("p1_card_%s_%d", suitChar, int(rank))
}

// P2CardPlaceName returns the place name for a card in player 2's hand
func P2CardPlaceName(suit Suit, rank Rank) string {
	suitChar := []string{"c", "d", "h", "s"}[suit]
	return fmt.Sprintf("p2_card_%s_%d", suitChar, int(rank))
}

// CommunityCardPlaceName returns the place name for a community card
func CommunityCardPlaceName(suit Suit, rank Rank) string {
	suitChar := []string{"c", "d", "h", "s"}[suit]
	return fmt.Sprintf("comm_card_%s_%d", suitChar, int(rank))
}

// CreatePokerPetriNet creates a Petri net model for Texas Hold'em
// The model captures:
// - Complete 52-card deck with places for each card
// - Card memory for player hands and community cards
// - Game phases (pre-flop, flop, turn, river, showdown)
// - Betting rounds within each phase
// - Player actions (fold, check, call, raise, all-in)
// - Win conditions and pot distribution
// - Hand strength computation via ODE with draw potential
func CreatePokerPetriNet(numPlayers int) *petri.PetriNet {
	net := petri.NewPetriNet()

	// === DECK PLACES ===
	// Each card in the 52-card deck has a place
	// Token value 1 = card available in deck, 0 = card dealt
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			placeName := CardPlaceName(suit, rank)
			net.AddPlace(placeName, 1.0, nil, float64(rank)*30, float64(suit)*30, nil)
		}
	}

	// === PLAYER 1 CARD MEMORY PLACES ===
	// Each card can be in P1's hand (hole cards)
	// Token value 1 = P1 has this card, 0 = P1 doesn't have it
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			placeName := P1CardPlaceName(suit, rank)
			net.AddPlace(placeName, 0.0, nil, float64(rank)*30+500, float64(suit)*30, nil)
		}
	}

	// === PLAYER 2 CARD MEMORY PLACES ===
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			placeName := P2CardPlaceName(suit, rank)
			net.AddPlace(placeName, 0.0, nil, float64(rank)*30+1000, float64(suit)*30, nil)
		}
	}

	// === COMMUNITY CARD MEMORY PLACES ===
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			placeName := CommunityCardPlaceName(suit, rank)
			net.AddPlace(placeName, 0.0, nil, float64(rank)*30+1500, float64(suit)*30, nil)
		}
	}

	// === DRAW POTENTIAL PLACES ===
	// These places track drawing possibilities for ODE computation
	// P1 draw potential
	net.AddPlace("p1_flush_draw", 0.0, nil, 100, 200, nil)     // 4 to a flush
	net.AddPlace("p1_straight_draw", 0.0, nil, 100, 230, nil)  // Open-ended straight draw
	net.AddPlace("p1_gutshot_draw", 0.0, nil, 100, 260, nil)   // Gutshot straight draw
	net.AddPlace("p1_pair_draw", 0.0, nil, 100, 290, nil)      // Drawing to a pair
	net.AddPlace("p1_overcards", 0.0, nil, 100, 320, nil)      // Overcards to board

	// P2 draw potential
	net.AddPlace("p2_flush_draw", 0.0, nil, 200, 200, nil)
	net.AddPlace("p2_straight_draw", 0.0, nil, 200, 230, nil)
	net.AddPlace("p2_gutshot_draw", 0.0, nil, 200, 260, nil)
	net.AddPlace("p2_pair_draw", 0.0, nil, 200, 290, nil)
	net.AddPlace("p2_overcards", 0.0, nil, 200, 320, nil)

	// === CARD COUNT MEMORY PLACES ===
	// Track the number of cards in each location
	net.AddPlace("deck_count", 52.0, nil, 100, 400, nil)      // Cards remaining in deck
	net.AddPlace("p1_hole_count", 0.0, nil, 100, 430, nil)    // P1's hole cards
	net.AddPlace("p2_hole_count", 0.0, nil, 100, 460, nil)    // P2's hole cards
	net.AddPlace("community_count", 0.0, nil, 100, 490, nil)  // Community cards

	// === SUIT COUNT PLACES (for flush draws) ===
	// P1 suit counts (hole cards)
	net.AddPlace("p1_clubs", 0.0, nil, 300, 200, nil)
	net.AddPlace("p1_diamonds", 0.0, nil, 300, 230, nil)
	net.AddPlace("p1_hearts", 0.0, nil, 300, 260, nil)
	net.AddPlace("p1_spades", 0.0, nil, 300, 290, nil)

	// P2 suit counts (hole cards)
	net.AddPlace("p2_clubs", 0.0, nil, 400, 200, nil)
	net.AddPlace("p2_diamonds", 0.0, nil, 400, 230, nil)
	net.AddPlace("p2_hearts", 0.0, nil, 400, 260, nil)
	net.AddPlace("p2_spades", 0.0, nil, 400, 290, nil)

	// Community suit counts
	net.AddPlace("comm_clubs", 0.0, nil, 500, 200, nil)
	net.AddPlace("comm_diamonds", 0.0, nil, 500, 230, nil)
	net.AddPlace("comm_hearts", 0.0, nil, 500, 260, nil)
	net.AddPlace("comm_spades", 0.0, nil, 500, 290, nil)

	// === GAME PHASE PLACES ===
	net.AddPlace("phase_preflop", 1.0, nil, 100, 500, nil)
	net.AddPlace("phase_flop", 0.0, nil, 200, 500, nil)
	net.AddPlace("phase_turn", 0.0, nil, 300, 500, nil)
	net.AddPlace("phase_river", 0.0, nil, 400, 500, nil)
	net.AddPlace("phase_showdown", 0.0, nil, 500, 500, nil)
	net.AddPlace("phase_complete", 0.0, nil, 600, 500, nil)

	// === PLAYER STATE PLACES ===
	// Player 1
	net.AddPlace("p1_active", 1.0, nil, 100, 600, nil)
	net.AddPlace("p1_folded", 0.0, nil, 100, 630, nil)
	net.AddPlace("p1_acted", 0.0, nil, 100, 660, nil)
	net.AddPlace("p1_turn", 0.0, nil, 100, 690, nil)
	net.AddPlace("p1_bet", 0.0, nil, 100, 720, nil)
	net.AddPlace("p1_chips", 1000.0, nil, 100, 750, nil)
	net.AddPlace("p1_pot_share", 0.0, nil, 100, 780, nil)
	net.AddPlace("p1_hand_str", 0.5, nil, 100, 810, nil)
	net.AddPlace("p1_wins", 0.0, nil, 100, 840, nil)

	// Player 2
	net.AddPlace("p2_active", 1.0, nil, 200, 600, nil)
	net.AddPlace("p2_folded", 0.0, nil, 200, 630, nil)
	net.AddPlace("p2_acted", 0.0, nil, 200, 660, nil)
	net.AddPlace("p2_turn", 0.0, nil, 200, 690, nil)
	net.AddPlace("p2_bet", 0.0, nil, 200, 720, nil)
	net.AddPlace("p2_chips", 1000.0, nil, 200, 750, nil)
	net.AddPlace("p2_pot_share", 0.0, nil, 200, 780, nil)
	net.AddPlace("p2_hand_str", 0.5, nil, 200, 810, nil)
	net.AddPlace("p2_wins", 0.0, nil, 200, 840, nil)

	// === HAND STRENGTH ODE INPUT PLACES ===
	// P1 ODE inputs
	net.AddPlace("p1_rank_input", 0.0, nil, 300, 600, nil)
	net.AddPlace("p1_highcard_input", 0.0, nil, 300, 630, nil)
	net.AddPlace("p1_str_delta", 0.0, nil, 300, 660, nil)
	net.AddPlace("p1_draw_potential", 0.0, nil, 300, 690, nil)  // Combined draw value
	net.AddPlace("p1_completion_odds", 0.0, nil, 300, 720, nil) // Odds of completing draws

	// P2 ODE inputs
	net.AddPlace("p2_rank_input", 0.0, nil, 400, 600, nil)
	net.AddPlace("p2_highcard_input", 0.0, nil, 400, 630, nil)
	net.AddPlace("p2_str_delta", 0.0, nil, 400, 660, nil)
	net.AddPlace("p2_draw_potential", 0.0, nil, 400, 690, nil)
	net.AddPlace("p2_completion_odds", 0.0, nil, 400, 720, nil)

	// === POT AND BETTING PLACES ===
	net.AddPlace("pot", 0.0, nil, 300, 500, nil)
	net.AddPlace("bet_to_call", 0.0, nil, 350, 500, nil)
	net.AddPlace("min_raise", 2.0, nil, 400, 500, nil)
	net.AddPlace("round_complete", 0.0, nil, 450, 500, nil)

	// === TRANSITIONS ===
	// Phase transitions
	net.AddTransition("deal_hole", "default", 150, 500, nil)
	net.AddTransition("deal_flop", "default", 250, 500, nil)
	net.AddTransition("deal_turn", "default", 350, 500, nil)
	net.AddTransition("deal_river", "default", 450, 500, nil)
	net.AddTransition("to_showdown", "default", 550, 500, nil)
	net.AddTransition("end_hand", "default", 650, 500, nil)

	// Player 1 actions
	net.AddTransition("p1_fold", "default", 100, 900, nil)
	net.AddTransition("p1_check", "default", 150, 900, nil)
	net.AddTransition("p1_call", "default", 200, 900, nil)
	net.AddTransition("p1_raise", "default", 250, 900, nil)
	net.AddTransition("p1_all_in", "default", 300, 900, nil)

	// Player 2 actions
	net.AddTransition("p2_fold", "default", 100, 950, nil)
	net.AddTransition("p2_check", "default", 150, 950, nil)
	net.AddTransition("p2_call", "default", 200, 950, nil)
	net.AddTransition("p2_raise", "default", 250, 950, nil)
	net.AddTransition("p2_all_in", "default", 300, 950, nil)

	// Betting round transitions
	net.AddTransition("start_p1_turn", "default", 350, 900, nil)
	net.AddTransition("start_p2_turn", "default", 350, 950, nil)
	net.AddTransition("complete_round", "default", 400, 900, nil)

	// Win transitions
	net.AddTransition("p1_wins_pot", "default", 450, 900, nil)
	net.AddTransition("p2_wins_pot", "default", 450, 950, nil)

	// Hand strength ODE transitions
	net.AddTransition("p1_compute_str", "default", 300, 750, nil)
	net.AddTransition("p2_compute_str", "default", 400, 750, nil)
	net.AddTransition("p1_update_str", "default", 300, 780, nil)
	net.AddTransition("p2_update_str", "default", 400, 780, nil)

	// Draw potential computation transitions
	net.AddTransition("p1_compute_draws", "default", 300, 810, nil)
	net.AddTransition("p2_compute_draws", "default", 400, 810, nil)

	// === ARCS ===
	// Phase transitions
	net.AddArc("phase_preflop", "deal_hole", 1.0, false)
	net.AddArc("deal_hole", "phase_preflop", 1.0, false)
	net.AddArc("phase_preflop", "deal_flop", 1.0, false)
	net.AddArc("deal_flop", "phase_flop", 1.0, false)
	net.AddArc("phase_flop", "deal_turn", 1.0, false)
	net.AddArc("deal_turn", "phase_turn", 1.0, false)
	net.AddArc("phase_turn", "deal_river", 1.0, false)
	net.AddArc("deal_river", "phase_river", 1.0, false)
	net.AddArc("phase_river", "to_showdown", 1.0, false)
	net.AddArc("to_showdown", "phase_showdown", 1.0, false)
	net.AddArc("phase_showdown", "end_hand", 1.0, false)
	net.AddArc("end_hand", "phase_complete", 1.0, false)

	// Player 1 fold
	net.AddArc("p1_turn", "p1_fold", 1.0, false)
	net.AddArc("p1_active", "p1_fold", 1.0, false)
	net.AddArc("p1_fold", "p1_folded", 1.0, false)
	net.AddArc("p1_fold", "p1_acted", 1.0, false)

	// Player 1 check
	net.AddArc("p1_turn", "p1_check", 1.0, false)
	net.AddArc("p1_active", "p1_check", 1.0, false)
	net.AddArc("p1_check", "p1_active", 1.0, false)
	net.AddArc("p1_check", "p1_acted", 1.0, false)

	// Player 1 call
	net.AddArc("p1_turn", "p1_call", 1.0, false)
	net.AddArc("p1_active", "p1_call", 1.0, false)
	net.AddArc("p1_call", "p1_active", 1.0, false)
	net.AddArc("p1_call", "p1_acted", 1.0, false)

	// Player 1 raise
	net.AddArc("p1_turn", "p1_raise", 1.0, false)
	net.AddArc("p1_active", "p1_raise", 1.0, false)
	net.AddArc("p1_raise", "p1_active", 1.0, false)
	net.AddArc("p1_raise", "p1_acted", 1.0, false)

	// Player 1 all-in
	net.AddArc("p1_turn", "p1_all_in", 1.0, false)
	net.AddArc("p1_active", "p1_all_in", 1.0, false)
	net.AddArc("p1_all_in", "p1_active", 1.0, false)
	net.AddArc("p1_all_in", "p1_acted", 1.0, false)

	// Player 2 fold
	net.AddArc("p2_turn", "p2_fold", 1.0, false)
	net.AddArc("p2_active", "p2_fold", 1.0, false)
	net.AddArc("p2_fold", "p2_folded", 1.0, false)
	net.AddArc("p2_fold", "p2_acted", 1.0, false)

	// Player 2 check
	net.AddArc("p2_turn", "p2_check", 1.0, false)
	net.AddArc("p2_active", "p2_check", 1.0, false)
	net.AddArc("p2_check", "p2_active", 1.0, false)
	net.AddArc("p2_check", "p2_acted", 1.0, false)

	// Player 2 call
	net.AddArc("p2_turn", "p2_call", 1.0, false)
	net.AddArc("p2_active", "p2_call", 1.0, false)
	net.AddArc("p2_call", "p2_active", 1.0, false)
	net.AddArc("p2_call", "p2_acted", 1.0, false)

	// Player 2 raise
	net.AddArc("p2_turn", "p2_raise", 1.0, false)
	net.AddArc("p2_active", "p2_raise", 1.0, false)
	net.AddArc("p2_raise", "p2_active", 1.0, false)
	net.AddArc("p2_raise", "p2_acted", 1.0, false)

	// Player 2 all-in
	net.AddArc("p2_turn", "p2_all_in", 1.0, false)
	net.AddArc("p2_active", "p2_all_in", 1.0, false)
	net.AddArc("p2_all_in", "p2_active", 1.0, false)
	net.AddArc("p2_all_in", "p2_acted", 1.0, false)

	// Turn management
	net.AddArc("p1_acted", "start_p2_turn", 1.0, false)
	net.AddArc("p2_active", "start_p2_turn", 1.0, false)
	net.AddArc("start_p2_turn", "p2_turn", 1.0, false)
	net.AddArc("start_p2_turn", "p2_active", 1.0, false)

	net.AddArc("p2_acted", "start_p1_turn", 1.0, false)
	net.AddArc("p1_active", "start_p1_turn", 1.0, false)
	net.AddArc("start_p1_turn", "p1_turn", 1.0, false)
	net.AddArc("start_p1_turn", "p1_active", 1.0, false)

	// Win detection
	net.AddArc("p2_folded", "p1_wins_pot", 1.0, false)
	net.AddArc("p1_active", "p1_wins_pot", 1.0, false)
	net.AddArc("pot", "p1_wins_pot", 1.0, false)
	net.AddArc("p1_wins_pot", "p1_wins", 1.0, false)

	net.AddArc("p1_folded", "p2_wins_pot", 1.0, false)
	net.AddArc("p2_active", "p2_wins_pot", 1.0, false)
	net.AddArc("pot", "p2_wins_pot", 1.0, false)
	net.AddArc("p2_wins_pot", "p2_wins", 1.0, false)

	// Hand strength ODE arcs
	net.AddArc("p1_rank_input", "p1_compute_str", 1.0, false)
	net.AddArc("p1_highcard_input", "p1_compute_str", 1.0, false)
	net.AddArc("p1_draw_potential", "p1_compute_str", 1.0, false)
	net.AddArc("p1_compute_str", "p1_str_delta", 1.0, false)
	net.AddArc("p1_str_delta", "p1_update_str", 1.0, false)
	net.AddArc("p1_update_str", "p1_hand_str", 1.0, false)

	net.AddArc("p2_rank_input", "p2_compute_str", 1.0, false)
	net.AddArc("p2_highcard_input", "p2_compute_str", 1.0, false)
	net.AddArc("p2_draw_potential", "p2_compute_str", 1.0, false)
	net.AddArc("p2_compute_str", "p2_str_delta", 1.0, false)
	net.AddArc("p2_str_delta", "p2_update_str", 1.0, false)
	net.AddArc("p2_update_str", "p2_hand_str", 1.0, false)

	// Draw potential computation arcs
	// P1 draw inputs
	net.AddArc("p1_flush_draw", "p1_compute_draws", 1.0, false)
	net.AddArc("p1_straight_draw", "p1_compute_draws", 1.0, false)
	net.AddArc("p1_overcards", "p1_compute_draws", 1.0, false)
	net.AddArc("p1_compute_draws", "p1_draw_potential", 1.0, false)

	// P2 draw inputs
	net.AddArc("p2_flush_draw", "p2_compute_draws", 1.0, false)
	net.AddArc("p2_straight_draw", "p2_compute_draws", 1.0, false)
	net.AddArc("p2_overcards", "p2_compute_draws", 1.0, false)
	net.AddArc("p2_compute_draws", "p2_draw_potential", 1.0, false)

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
		"p1_fold":   0.2,  // Fold rate decreases with hand strength
		"p1_check":  0.3,  // Check when possible
		"p1_call":   0.3,  // Call rate based on pot odds
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

		// Hand strength ODE transitions
		"p1_compute_str": 1.0,
		"p2_compute_str": 1.0,
		"p1_update_str":  1.0,
		"p2_update_str":  1.0,

		// Draw potential computation transitions
		"p1_compute_draws": 1.0,
		"p2_compute_draws": 1.0,
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
