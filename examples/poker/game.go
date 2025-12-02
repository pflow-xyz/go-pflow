package poker

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/hypothesis"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Player represents a player in the game
type Player int

const (
	Player1 Player = iota
	Player2
)

func (p Player) String() string {
	return [...]string{"Player 1", "Player 2"}[p]
}

// BettingDecision represents a betting decision with amount
type BettingDecision struct {
	Action Action
	Amount float64
}

// PokerGame represents a Texas Hold'em poker game
type PokerGame struct {
	net           *petri.PetriNet
	engine        *engine.Engine
	rates         map[string]float64
	deck          *Deck
	communityCards []Card
	p1Hole        []Card
	p2Hole        []Card
	phase         GamePhase
	pot           float64
	currentBet    float64
	p1Chips       float64
	p2Chips       float64
	p1Bet         float64 // Amount bet this round
	p2Bet         float64
	p1Folded      bool
	p2Folded      bool
	currentPlayer Player
	actedThisRound map[Player]bool
	winner        *Player
	smallBlind    float64
	bigBlind      float64
	evaluator     *hypothesis.Evaluator
	
	// Card tracking for adversarial analysis
	p1Tracker     *CardTracker // Player 1's view (sees own hole cards + community)
	p2Tracker     *CardTracker // Player 2's view
	p1Aggression  float64      // Running aggression factor for player 1
	p2Aggression  float64      // Running aggression factor for player 2
}

// NewPokerGame creates a new Texas Hold'em game
func NewPokerGame(initialChips, smallBlind, bigBlind float64) *PokerGame {
	net := CreatePokerPetriNet(2)
	rates := DefaultRates()

	// Initialize state
	initialState := make(map[string]float64)
	for placeName := range net.Places {
		initialState[placeName] = 0
	}

	// Set initial values
	initialState["phase_preflop"] = 1
	initialState["p1_active"] = 1
	initialState["p2_active"] = 1
	initialState["p1_chips"] = initialChips
	initialState["p2_chips"] = initialChips
	initialState["min_raise"] = bigBlind

	eng := engine.NewEngine(net, initialState, rates)

	// Create hypothesis evaluator for bet sizing
	scorer := func(final map[string]float64) float64 {
		return final["p1_wins"] - final["p2_wins"]
	}
	eval := hypothesis.NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 10.0).
		WithOptions(solver.FastOptions())

	game := &PokerGame{
		net:            net,
		engine:         eng,
		rates:          rates,
		deck:           NewDeck(),
		communityCards: make([]Card, 0, 5),
		phase:          PhasePreflop,
		pot:            0,
		currentBet:     bigBlind,
		p1Chips:        initialChips,
		p2Chips:        initialChips,
		p1Bet:          0,
		p2Bet:          0,
		p1Folded:       false,
		p2Folded:       false,
		currentPlayer:  Player1,
		actedThisRound: make(map[Player]bool),
		smallBlind:     smallBlind,
		bigBlind:       bigBlind,
		evaluator:      eval,
		p1Tracker:      NewCardTracker(),
		p2Tracker:      NewCardTracker(),
		p1Aggression:   0.5, // Start at neutral
		p2Aggression:   0.5,
	}

	return game
}

// StartHand begins a new hand
func (g *PokerGame) StartHand() {
	// Reset deck and shuffle
	g.deck = NewDeck()
	g.deck.Shuffle()

	// Clear community cards
	g.communityCards = g.communityCards[:0]

	// Post blinds
	g.PostBlinds()

	// Deal hole cards
	g.p1Hole = g.deck.DealN(2)
	g.p2Hole = g.deck.DealN(2)

	// Reset player states
	g.p1Folded = false
	g.p2Folded = false
	g.actedThisRound = make(map[Player]bool)

	// Reset card trackers
	g.p1Tracker = NewCardTracker()
	g.p2Tracker = NewCardTracker()
	g.p1Tracker.SetOurHoleCards(g.p1Hole)
	g.p2Tracker.SetOurHoleCards(g.p2Hole)
	g.p1Tracker.SetPhase(PhasePreflop)
	g.p2Tracker.SetPhase(PhasePreflop)
	
	// Reset aggression factors
	g.p1Aggression = 0.5
	g.p2Aggression = 0.5

	// Update hand strengths in state
	g.UpdateHandStrengths()

	// Player after big blind acts first
	g.currentPlayer = Player1

	// Reset phase
	g.phase = PhasePreflop

	// Update engine state
	g.syncToEngine()
}

// PostBlinds posts the blinds
func (g *PokerGame) PostBlinds() {
	// Player 1 posts small blind
	g.p1Chips -= g.smallBlind
	g.p1Bet = g.smallBlind

	// Player 2 posts big blind
	g.p2Chips -= g.bigBlind
	g.p2Bet = g.bigBlind

	g.pot = g.smallBlind + g.bigBlind
	g.currentBet = g.bigBlind
}

// UpdateHandStrengths updates the hand strength places in the Petri net using ODE simulation.
// This function computes hand strength through the Petri net ODE dynamics rather than
// directly setting the values, making the hand strength part of the continuous simulation.
// It now includes draw potential based on the cards tracked in the Petri net.
func (g *PokerGame) UpdateHandStrengths() {
	p1Result := EvaluateHand(g.p1Hole, g.communityCards)
	p2Result := EvaluateHand(g.p2Hole, g.communityCards)

	// Compute normalized components for ODE input
	// Score formula: (HandRank * 1000) + (HighCard * 10) + kickers
	// Max score: 9140 (Royal Flush with Ace)
	p1RankNorm := float64(p1Result.Rank) / 9.0      // Normalize rank (0-9) to (0-1)
	p1HighNorm := float64(p1Result.HighCard) / 14.0 // Normalize highcard (2-14) to (~0.14-1)
	p2RankNorm := float64(p2Result.Rank) / 9.0
	p2HighNorm := float64(p2Result.HighCard) / 14.0

	// Update state with ODE input values
	state := g.engine.GetState()

	// Set the input places for ODE computation
	// These will flow through the ODE to compute hand_str
	state["p1_rank_input"] = p1RankNorm
	state["p1_highcard_input"] = p1HighNorm
	state["p2_rank_input"] = p2RankNorm
	state["p2_highcard_input"] = p2HighNorm

	// Sync card memory and compute draw potentials
	g.syncCardMemory(state)

	// Reset delta places for fresh computation
	state["p1_str_delta"] = 0
	state["p2_str_delta"] = 0

	g.engine.SetState(state)

	// Run ODE simulation to compute hand strengths (includes draw potential)
	p1Str, p2Str := g.computeHandStrengthsViaODE()

	// Update state with ODE-computed strengths
	state = g.engine.GetState()
	state["p1_hand_str"] = p1Str
	state["p2_hand_str"] = p2Str
	g.engine.SetState(state)

	// Update rates based on hand strengths
	g.rates = StrengthAdjustedRates(DefaultRates(), p1Str, p2Str)
}

// computeHandStrengthsViaODE runs an ODE simulation to compute hand strengths.
// This models hand strength as a continuous flow through the Petri net,
// where rank, highcard, and draw potential inputs flow through transitions.
//
// The computation considers:
// - Current hand rank (pair, flush, etc.)
// - High card value for tie-breaking
// - Draw potential (flush draws, straight draws, overcards)
// - Completion odds based on remaining cards
func (g *PokerGame) computeHandStrengthsViaODE() (p1Strength, p2Strength float64) {
	state := g.engine.GetState()

	// Create rates that enable hand strength computation
	rates := make(map[string]float64)
	for k, v := range g.rates {
		rates[k] = v
	}
	// Enable hand strength and draw computation transitions
	rates["p1_compute_str"] = 1.0
	rates["p2_compute_str"] = 1.0
	rates["p1_update_str"] = 1.0
	rates["p2_update_str"] = 1.0
	rates["p1_compute_draws"] = 1.0
	rates["p2_compute_draws"] = 1.0

	// Create ODE problem for hand strength computation
	// Use a short time span since we just need the equilibrium
	prob := solver.NewProblem(g.net, state, [2]float64{0, 1.0}, rates)

	// Use fast options since this is a simple computation
	opts := solver.FastOptions()

	// Solve the ODE
	sol := solver.Solve(prob, solver.Tsit5(), opts)

	// Get final state from ODE solution
	finalState := sol.GetFinalState()

	// Extract inputs from state (these drive the ODE)
	p1Rank := state["p1_rank_input"]
	p1High := state["p1_highcard_input"]
	p1DrawPot := state["p1_draw_potential"]
	p1CompOdds := state["p1_completion_odds"]

	p2Rank := state["p2_rank_input"]
	p2High := state["p2_highcard_input"]
	p2DrawPot := state["p2_draw_potential"]
	p2CompOdds := state["p2_completion_odds"]

	// Compute hand strength using the ODE-modeled formula with draw potential:
	// Base strength = (rank_contribution * 0.8) + (highcard_contribution * 0.1) + (draw_potential * 0.1)
	// Adjusted by completion odds when hand can improve
	p1Strength = computeStrengthWithDraws(p1Rank, p1High, finalState["p1_str_delta"], p1DrawPot, p1CompOdds)
	p2Strength = computeStrengthWithDraws(p2Rank, p2High, finalState["p2_str_delta"], p2DrawPot, p2CompOdds)

	// Clamp to valid range [0, 1]
	if p1Strength > 1.0 {
		p1Strength = 1.0
	}
	if p1Strength < 0 {
		p1Strength = 0
	}
	if p2Strength > 1.0 {
		p2Strength = 1.0
	}
	if p2Strength < 0 {
		p2Strength = 0
	}

	return p1Strength, p2Strength
}

// computeStrengthFromODE computes the final hand strength from ODE components.
// The strength formula: strength = (rank * 0.9) + (highcard * 0.1) + delta_adjustment
// This models the poker hand ranking where hand rank dominates (90%) and highcard
// is for tie-breaking (10%), with ODE dynamics providing smooth transitions.
func computeStrengthFromODE(rankNorm, highNorm, deltaAdjust float64) float64 {
	// Weight rank much more heavily than high card (like real poker scoring)
	strength := (rankNorm * 0.9) + (highNorm * 0.1)

	// Apply any ODE-computed adjustment (from state flow dynamics)
	// This allows the ODE to modify strength based on game state
	if deltaAdjust > 0 {
		strength += deltaAdjust * 0.05 // Small adjustment from ODE dynamics
	}

	return strength
}

// computeStrengthWithDraws computes hand strength including draw potential.
// This extends the basic strength calculation to consider incomplete hands
// that could improve with future cards.
//
// The formula:
// - Base: (rank * 0.75) + (highcard * 0.1) + delta_adjustment
// - Draw bonus: draw_potential * completion_odds * 0.15
//
// This allows hands with strong draws (like flush draws) to have higher
// estimated strength even when the current hand is weak.
func computeStrengthWithDraws(rankNorm, highNorm, deltaAdjust, drawPot, compOdds float64) float64 {
	// Base strength from current hand
	baseStrength := (rankNorm * 0.75) + (highNorm * 0.1)

	// Apply ODE delta adjustment
	if deltaAdjust > 0 {
		baseStrength += deltaAdjust * 0.05
	}

	// Add draw potential weighted by completion odds
	// A flush draw with 35% completion odds adds significant equity
	drawBonus := drawPot * compOdds * 0.15

	return baseStrength + drawBonus
}

// GetAvailableActions returns the legal actions for the current player
func (g *PokerGame) GetAvailableActions() []Action {
	if g.IsHandComplete() {
		return []Action{}
	}

	actions := []Action{ActionFold}

	var currentBet, playerBet, playerChips float64
	if g.currentPlayer == Player1 {
		playerBet = g.p1Bet
		playerChips = g.p1Chips
	} else {
		playerBet = g.p2Bet
		playerChips = g.p2Chips
	}
	currentBet = g.currentBet

	toCall := currentBet - playerBet

	if toCall <= 0 {
		actions = append(actions, ActionCheck)
	} else if toCall <= playerChips {
		actions = append(actions, ActionCall)
	}

	if playerChips > toCall+g.bigBlind {
		actions = append(actions, ActionRaise)
	}

	if playerChips > 0 {
		actions = append(actions, ActionAllIn)
	}

	return actions
}

// MakeAction executes a player action
func (g *PokerGame) MakeAction(action Action, amount float64) error {
	if g.IsHandComplete() {
		return fmt.Errorf("hand is complete")
	}

	player := g.currentPlayer
	var playerBet, playerChips *float64
	var playerFolded *bool

	if player == Player1 {
		playerBet = &g.p1Bet
		playerChips = &g.p1Chips
		playerFolded = &g.p1Folded
	} else {
		playerBet = &g.p2Bet
		playerChips = &g.p2Chips
		playerFolded = &g.p2Folded
	}

	toCall := g.currentBet - *playerBet

	// Update opponent's aggression factor (for the OTHER player's tracker)
	// This allows us to estimate opponent's range based on their betting
	aggFactor := g.p1Tracker.UpdateFromBettingAction(action, amount, g.pot)
	if player == Player1 {
		// Player 1 acted, update P2's view of P1's aggression
		g.p1Aggression = (g.p1Aggression + aggFactor) / 2.0
	} else {
		// Player 2 acted, update P1's view of P2's aggression
		g.p2Aggression = (g.p2Aggression + aggFactor) / 2.0
	}

	switch action {
	case ActionFold:
		*playerFolded = true

	case ActionCheck:
		if toCall > 0 {
			return fmt.Errorf("cannot check, must call %v", toCall)
		}

	case ActionCall:
		if toCall > *playerChips {
			toCall = *playerChips // All-in for less
		}
		*playerChips -= toCall
		*playerBet += toCall
		g.pot += toCall

	case ActionRaise:
		minRaise := g.bigBlind
		if amount < toCall+minRaise {
			amount = toCall + minRaise
		}
		if amount > *playerChips {
			amount = *playerChips
		}
		*playerChips -= amount
		*playerBet += amount
		g.pot += amount
		g.currentBet = *playerBet
		// Reset other player's acted status since there's a raise
		g.actedThisRound = make(map[Player]bool)

	case ActionAllIn:
		amount = *playerChips
		*playerChips = 0
		*playerBet += amount
		g.pot += amount
		if *playerBet > g.currentBet {
			g.currentBet = *playerBet
			g.actedThisRound = make(map[Player]bool)
		}
	}

	g.actedThisRound[player] = true

	// Check if betting round is complete
	if g.isBettingRoundComplete() {
		g.advancePhase()
	} else {
		// Switch to next player
		if g.currentPlayer == Player1 && !g.p2Folded {
			g.currentPlayer = Player2
		} else if !g.p1Folded {
			g.currentPlayer = Player1
		}
	}

	g.syncToEngine()
	return nil
}

// isBettingRoundComplete checks if betting round is done
func (g *PokerGame) isBettingRoundComplete() bool {
	// If only one player remains, round is complete
	if g.p1Folded || g.p2Folded {
		return true
	}

	// Both active players must have acted and bets must be equal
	if g.actedThisRound[Player1] && g.actedThisRound[Player2] {
		if g.p1Bet == g.p2Bet {
			return true
		}
	}

	return false
}

// advancePhase moves to the next phase
func (g *PokerGame) advancePhase() {
	// Check for winner by fold
	if g.p1Folded {
		winner := Player2
		g.winner = &winner
		g.phase = PhaseComplete
		return
	}
	if g.p2Folded {
		winner := Player1
		g.winner = &winner
		g.phase = PhaseComplete
		return
	}

	// Reset betting for new round
	g.p1Bet = 0
	g.p2Bet = 0
	g.currentBet = 0
	g.actedThisRound = make(map[Player]bool)
	g.currentPlayer = Player1

	switch g.phase {
	case PhasePreflop:
		// Deal flop
		g.deck.Deal() // Burn card
		g.communityCards = append(g.communityCards, g.deck.DealN(3)...)
		g.phase = PhaseFlop
		g.UpdateHandStrengths()
		g.updateTrackersCommunity()

	case PhaseFlop:
		// Deal turn
		g.deck.Deal() // Burn card
		g.communityCards = append(g.communityCards, g.deck.Deal())
		g.phase = PhaseTurn
		g.UpdateHandStrengths()
		g.updateTrackersCommunity()

	case PhaseTurn:
		// Deal river
		g.deck.Deal() // Burn card
		g.communityCards = append(g.communityCards, g.deck.Deal())
		g.phase = PhaseRiver
		g.UpdateHandStrengths()
		g.updateTrackersCommunity()

	case PhaseRiver:
		// Showdown
		g.phase = PhaseShowdown
		g.determineWinner()
	}
}

// updateTrackersCommunity updates both player's trackers with community cards
func (g *PokerGame) updateTrackersCommunity() {
	g.p1Tracker.SetCommunityCards(g.communityCards)
	g.p2Tracker.SetCommunityCards(g.communityCards)
	g.p1Tracker.SetPhase(g.phase)
	g.p2Tracker.SetPhase(g.phase)
}

// determineWinner compares hands at showdown
func (g *PokerGame) determineWinner() {
	p1Result := EvaluateHand(g.p1Hole, g.communityCards)
	p2Result := EvaluateHand(g.p2Hole, g.communityCards)

	if p1Result.Score() > p2Result.Score() {
		winner := Player1
		g.winner = &winner
	} else if p2Result.Score() > p1Result.Score() {
		winner := Player2
		g.winner = &winner
	}
	// On tie, split pot (winner remains nil)

	g.phase = PhaseComplete
}

// IsHandComplete returns true if the hand is over
func (g *PokerGame) IsHandComplete() bool {
	return g.phase == PhaseComplete
}

// GetWinner returns the winner (nil for split pot)
func (g *PokerGame) GetWinner() *Player {
	return g.winner
}

// GetPhase returns the current phase
func (g *PokerGame) GetPhase() GamePhase {
	return g.phase
}

// GetPot returns the current pot size
func (g *PokerGame) GetPot() float64 {
	return g.pot
}

// GetCurrentPlayer returns the current player
func (g *PokerGame) GetCurrentPlayer() Player {
	return g.currentPlayer
}

// GetPlayerChips returns a player's chip count
func (g *PokerGame) GetPlayerChips(p Player) float64 {
	if p == Player1 {
		return g.p1Chips
	}
	return g.p2Chips
}

// GetPlayerHole returns a player's hole cards
func (g *PokerGame) GetPlayerHole(p Player) []Card {
	if p == Player1 {
		return g.p1Hole
	}
	return g.p2Hole
}

// GetCommunityCards returns the community cards
func (g *PokerGame) GetCommunityCards() []Card {
	return g.communityCards
}

// GetToCall returns the amount needed to call for the current player
func (g *PokerGame) GetToCall() float64 {
	if g.currentPlayer == Player1 {
		return g.currentBet - g.p1Bet
	}
	return g.currentBet - g.p2Bet
}

// GetHandResult returns the evaluated hand for a player
func (g *PokerGame) GetHandResult(p Player) HandResult {
	hole := g.GetPlayerHole(p)
	return EvaluateHand(hole, g.communityCards)
}

// GetAdversarialAnalysis returns adversarial analysis for the current player
// This estimates what hands the opponent likely has based on visible cards and betting
func (g *PokerGame) GetAdversarialAnalysis(p Player) AdversarialAnalysis {
	if p == Player1 {
		return g.p1Tracker.GetAdversarialAnalysis(g.p2Aggression)
	}
	return g.p2Tracker.GetAdversarialAnalysis(g.p1Aggression)
}

// GetOpponentAggression returns the tracked aggression factor for the opponent
func (g *PokerGame) GetOpponentAggression(p Player) float64 {
	if p == Player1 {
		return g.p2Aggression // P1 wants to know P2's aggression
	}
	return g.p1Aggression // P2 wants to know P1's aggression
}

// GetBoardTexture returns the board texture analysis
func (g *PokerGame) GetBoardTexture(p Player) BoardTexture {
	if p == Player1 {
		return g.p1Tracker.AnalyzeBoard()
	}
	return g.p2Tracker.AnalyzeBoard()
}

// syncToEngine updates the Petri net engine state
func (g *PokerGame) syncToEngine() {
	state := g.engine.GetState()

	// Update phase places
	state["phase_preflop"] = 0
	state["phase_flop"] = 0
	state["phase_turn"] = 0
	state["phase_river"] = 0
	state["phase_showdown"] = 0
	state["phase_complete"] = 0
	
	switch g.phase {
	case PhasePreflop:
		state["phase_preflop"] = 1
	case PhaseFlop:
		state["phase_flop"] = 1
	case PhaseTurn:
		state["phase_turn"] = 1
	case PhaseRiver:
		state["phase_river"] = 1
	case PhaseShowdown:
		state["phase_showdown"] = 1
	case PhaseComplete:
		state["phase_complete"] = 1
	}

	// Update player states
	if g.p1Folded {
		state["p1_active"] = 0
		state["p1_folded"] = 1
	} else {
		state["p1_active"] = 1
		state["p1_folded"] = 0
	}

	if g.p2Folded {
		state["p2_active"] = 0
		state["p2_folded"] = 1
	} else {
		state["p2_active"] = 1
		state["p2_folded"] = 0
	}

	// Update betting state
	state["pot"] = g.pot
	state["bet_to_call"] = g.currentBet
	state["p1_bet"] = g.p1Bet
	state["p2_bet"] = g.p2Bet
	state["p1_chips"] = g.p1Chips
	state["p2_chips"] = g.p2Chips

	// Update turn
	if g.currentPlayer == Player1 {
		state["p1_turn"] = 1
		state["p2_turn"] = 0
	} else {
		state["p1_turn"] = 0
		state["p2_turn"] = 1
	}

	// Update win places
	if g.winner != nil {
		if *g.winner == Player1 {
			state["p1_wins"] = 1
		} else {
			state["p2_wins"] = 1
		}
	}

	// Sync card memory to Petri net
	g.syncCardMemory(state)

	g.engine.SetState(state)
}

// syncCardMemory updates the card memory places in the Petri net state
// This tracks which cards are in each player's hand, community, and remaining in deck
func (g *PokerGame) syncCardMemory(state map[string]float64) {
	// Reset all deck places to 1 (available)
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			deckPlace := CardPlaceName(suit, rank)
			p1Place := P1CardPlaceName(suit, rank)
			p2Place := P2CardPlaceName(suit, rank)
			commPlace := CommunityCardPlaceName(suit, rank)
			state[deckPlace] = 1.0
			state[p1Place] = 0.0
			state[p2Place] = 0.0
			state[commPlace] = 0.0
		}
	}

	// Reset suit counts
	state["p1_clubs"] = 0
	state["p1_diamonds"] = 0
	state["p1_hearts"] = 0
	state["p1_spades"] = 0
	state["p2_clubs"] = 0
	state["p2_diamonds"] = 0
	state["p2_hearts"] = 0
	state["p2_spades"] = 0
	state["comm_clubs"] = 0
	state["comm_diamonds"] = 0
	state["comm_hearts"] = 0
	state["comm_spades"] = 0

	// Mark P1's hole cards
	for _, card := range g.p1Hole {
		deckPlace := CardPlaceName(card.Suit, card.Rank)
		p1Place := P1CardPlaceName(card.Suit, card.Rank)
		state[deckPlace] = 0.0 // Remove from deck
		state[p1Place] = 1.0   // Add to P1's hand

		// Update suit counts
		switch card.Suit {
		case Clubs:
			state["p1_clubs"]++
		case Diamonds:
			state["p1_diamonds"]++
		case Hearts:
			state["p1_hearts"]++
		case Spades:
			state["p1_spades"]++
		}
	}

	// Mark P2's hole cards
	for _, card := range g.p2Hole {
		deckPlace := CardPlaceName(card.Suit, card.Rank)
		p2Place := P2CardPlaceName(card.Suit, card.Rank)
		state[deckPlace] = 0.0 // Remove from deck
		state[p2Place] = 1.0   // Add to P2's hand

		// Update suit counts
		switch card.Suit {
		case Clubs:
			state["p2_clubs"]++
		case Diamonds:
			state["p2_diamonds"]++
		case Hearts:
			state["p2_hearts"]++
		case Spades:
			state["p2_spades"]++
		}
	}

	// Mark community cards
	for _, card := range g.communityCards {
		deckPlace := CardPlaceName(card.Suit, card.Rank)
		commPlace := CommunityCardPlaceName(card.Suit, card.Rank)
		state[deckPlace] = 0.0 // Remove from deck
		state[commPlace] = 1.0 // Add to community

		// Update suit counts
		switch card.Suit {
		case Clubs:
			state["comm_clubs"]++
		case Diamonds:
			state["comm_diamonds"]++
		case Hearts:
			state["comm_hearts"]++
		case Spades:
			state["comm_spades"]++
		}
	}

	// Update card counts
	state["deck_count"] = 52.0 - float64(len(g.p1Hole)+len(g.p2Hole)+len(g.communityCards))
	state["p1_hole_count"] = float64(len(g.p1Hole))
	state["p2_hole_count"] = float64(len(g.p2Hole))
	state["community_count"] = float64(len(g.communityCards))

	// Compute and set draw potentials
	g.computeDrawPotentials(state)
}

// computeDrawPotentials calculates drawing possibilities for ODE computation
func (g *PokerGame) computeDrawPotentials(state map[string]float64) {
	// P1 draws
	p1FlushDraw, p1StraightDraw, p1Overcards := g.analyzeDraws(g.p1Hole)
	state["p1_flush_draw"] = p1FlushDraw
	state["p1_straight_draw"] = p1StraightDraw
	state["p1_overcards"] = p1Overcards
	state["p1_draw_potential"] = (p1FlushDraw*0.35 + p1StraightDraw*0.32 + p1Overcards*0.12) // Weighted by outs

	// P2 draws
	p2FlushDraw, p2StraightDraw, p2Overcards := g.analyzeDraws(g.p2Hole)
	state["p2_flush_draw"] = p2FlushDraw
	state["p2_straight_draw"] = p2StraightDraw
	state["p2_overcards"] = p2Overcards
	state["p2_draw_potential"] = (p2FlushDraw*0.35 + p2StraightDraw*0.32 + p2Overcards*0.12)

	// Compute completion odds based on cards remaining
	cardsTocome := 0
	switch g.phase {
	case PhasePreflop:
		cardsTocome = 5 // Flop + turn + river
	case PhaseFlop:
		cardsTocome = 2 // Turn + river
	case PhaseTurn:
		cardsTocome = 1 // River
	default:
		cardsTocome = 0
	}

	// Rough completion odds (simplified)
	if cardsTocome > 0 {
		deckSize := state["deck_count"]
		// For flush draw: 9 outs, for straight draw: 8 outs, for overcards: 6 outs
		p1Outs := p1FlushDraw*9 + p1StraightDraw*8 + p1Overcards*6
		p2Outs := p2FlushDraw*9 + p2StraightDraw*8 + p2Overcards*6

		// Approximate odds = 1 - (no hit on any card)
		if deckSize > 0 {
			state["p1_completion_odds"] = 1.0 - math.Pow((deckSize-p1Outs)/deckSize, float64(cardsTocome))
			state["p2_completion_odds"] = 1.0 - math.Pow((deckSize-p2Outs)/deckSize, float64(cardsTocome))
		}
	} else {
		state["p1_completion_odds"] = 0
		state["p2_completion_odds"] = 0
	}
}

// analyzeDraws analyzes draw potential for a hand
// Returns normalized values (0-1) for flush draw, straight draw, and overcards
func (g *PokerGame) analyzeDraws(hole []Card) (flushDraw, straightDraw, overcards float64) {
	if len(g.communityCards) == 0 {
		// Preflop - check for suited/connected
		if len(hole) >= 2 {
			if hole[0].Suit == hole[1].Suit {
				flushDraw = 0.5 // Suited - potential flush draw
			}
			rankDiff := int(hole[0].Rank) - int(hole[1].Rank)
			if rankDiff < 0 {
				rankDiff = -rankDiff
			}
			if rankDiff <= 4 && rankDiff > 0 {
				straightDraw = 0.5 // Connected - potential straight draw
			}
			// Overcards to a random board
			if hole[0].Rank >= Jack && hole[1].Rank >= Jack {
				overcards = 0.8
			} else if hole[0].Rank >= Jack || hole[1].Rank >= Jack {
				overcards = 0.4
			}
		}
		return
	}

	// Count suits including community
	suitCounts := make(map[Suit]int)
	for _, c := range hole {
		suitCounts[c.Suit]++
	}
	for _, c := range g.communityCards {
		suitCounts[c.Suit]++
	}

	// Check for flush draw (4 to a flush)
	for _, count := range suitCounts {
		if count == 4 {
			flushDraw = 1.0
		} else if count == 3 && len(g.communityCards) <= 3 {
			flushDraw = 0.5 // Backdoor flush draw
		}
	}

	// Check for straight draw (simplified)
	allCards := append([]Card{}, hole...)
	allCards = append(allCards, g.communityCards...)
	straightDraw = g.checkStraightDraw(allCards)

	// Check for overcards
	maxCommunity := Two
	for _, c := range g.communityCards {
		if c.Rank > maxCommunity {
			maxCommunity = c.Rank
		}
	}
	overCount := 0
	for _, c := range hole {
		if c.Rank > maxCommunity {
			overCount++
		}
	}
	if overCount == 2 {
		overcards = 1.0
	} else if overCount == 1 {
		overcards = 0.5
	}

	return
}

// checkStraightDraw checks for open-ended or gutshot straight draws
// Open-ended: 4 consecutive cards that can complete on either end (8 outs)
// Gutshot: 4 cards with one gap in a 5-card range (4 outs)
func (g *PokerGame) checkStraightDraw(cards []Card) float64 {
	if len(cards) < 4 {
		return 0
	}

	// Get unique ranks and convert to sorted slice
	ranks := make(map[Rank]bool)
	for _, c := range cards {
		ranks[c.Rank] = true
	}

	// Check for straight draws by looking at 5-card windows
	// For a valid straight, we need 5 consecutive ranks (e.g., 5-6-7-8-9)
	for start := Two; start <= Ten; start++ {
		// Count cards in this 5-card window
		count := 0
		var gaps []Rank
		for r := start; r <= start+4 && r <= Ace; r++ {
			if ranks[r] {
				count++
			} else {
				gaps = append(gaps, r)
			}
		}

		if count == 4 && len(gaps) == 1 {
			// We have 4 of 5 cards needed for a straight
			gap := gaps[0]

			// Check if it's open-ended (gap is at end) or gutshot (gap in middle)
			if gap == start || gap == start+4 {
				// Open-ended: missing card is at one end
				// Check if the other end is also available (true open-ended = 8 outs)
				if gap == start && start > Two && !ranks[start-1] {
					return 1.0 // Can complete on both ends
				}
				if gap == start+4 && start+5 <= Ace && !ranks[start+5] {
					return 1.0 // Can complete on both ends
				}
				// One-ended straight draw (4 outs)
				return 0.7
			}
			// Gutshot: missing card is in the middle (4 outs)
			return 0.5
		}
	}

	// Special case: wheel draw (A-2-3-4-5)
	wheelCards := 0
	for _, r := range []Rank{Ace, Two, Three, Four, Five} {
		if ranks[r] {
			wheelCards++
		}
	}
	if wheelCards == 4 {
		return 0.5 // Gutshot wheel draw
	}

	return 0
}

// AI Strategies

// GetRandomAction returns a random legal action
func (g *PokerGame) GetRandomAction() BettingDecision {
	actions := g.GetAvailableActions()
	if len(actions) == 0 {
		return BettingDecision{Action: ActionFold, Amount: 0}
	}

	action := actions[rand.Intn(len(actions))]
	amount := 0.0

	if action == ActionRaise {
		toCall := g.GetToCall()
		minRaise := toCall + g.bigBlind
		maxRaise := g.GetPlayerChips(g.currentPlayer)
		if maxRaise > minRaise {
			amount = minRaise + rand.Float64()*(maxRaise-minRaise)
		} else {
			amount = maxRaise
		}
	}

	return BettingDecision{Action: action, Amount: amount}
}

// GetODEAction uses ODE simulation to estimate the best action
func (g *PokerGame) GetODEAction(verbose bool) BettingDecision {
	actions := g.GetAvailableActions()
	if len(actions) == 0 {
		return BettingDecision{Action: ActionFold, Amount: 0}
	}

	// Get current hand strength
	result := g.GetHandResult(g.currentPlayer)
	strength := result.Strength()

	// Get adversarial analysis
	analysis := g.GetAdversarialAnalysis(g.currentPlayer)

	// Simple strategy based on hand strength
	toCall := g.GetToCall()
	potOdds := g.pot / (g.pot + toCall)
	
	if verbose {
		fmt.Printf("  Hand: %s (strength: %.3f)\n", result.String(), strength)
		fmt.Printf("  Pot: %.0f, To Call: %.0f, Pot Odds: %.1f%%\n", g.pot, toCall, potOdds*100)
		fmt.Printf("  Opponent aggression: %.1f%%, Est. opponent strength: %.1f%%\n",
			g.GetOpponentAggression(g.currentPlayer)*100, analysis.OpponentEstimate.EstimatedStrength*100)
		fmt.Printf("  Equity advantage: %+.1f%%, Board danger: %.1f%%\n",
			analysis.EquityAdvantage*100, analysis.DangerLevel*100)
		if len(analysis.OpponentEstimate.DangerCards) > 0 {
			fmt.Printf("  Watch for: %s\n", FormatCards(analysis.OpponentEstimate.DangerCards))
		}
	}

	// Create hypothetical states for each action and simulate
	baseState := g.engine.GetState()
	bestAction := ActionFold
	bestScore := math.Inf(-1)
	bestAmount := 0.0

	for _, action := range actions {
		amount := 0.0
		if action == ActionRaise {
			// Try a few raise sizes
			toCall := g.GetToCall()
			chips := g.GetPlayerChips(g.currentPlayer)
			amounts := []float64{
				toCall + g.bigBlind,           // Min raise
				toCall + g.pot*0.5,            // Half pot
				toCall + g.pot,                // Pot
			}
			for _, amt := range amounts {
				if amt > chips {
					continue
				}
				score := g.evaluateActionWithAnalysis(baseState, action, amt, analysis, verbose)
				if score > bestScore {
					bestScore = score
					bestAction = action
					bestAmount = amt
				}
			}
		} else if action == ActionAllIn {
			amount = g.GetPlayerChips(g.currentPlayer)
			score := g.evaluateActionWithAnalysis(baseState, action, amount, analysis, verbose)
			if score > bestScore {
				bestScore = score
				bestAction = action
				bestAmount = amount
			}
		} else {
			score := g.evaluateActionWithAnalysis(baseState, action, 0, analysis, verbose)
			if score > bestScore {
				bestScore = score
				bestAction = action
				bestAmount = 0
			}
		}
	}

	if verbose {
		fmt.Printf("  Best action: %s", bestAction)
		if bestAmount > 0 {
			fmt.Printf(" (%.0f)", bestAmount)
		}
		fmt.Printf(" with score %.3f\n", bestScore)
	}

	return BettingDecision{Action: bestAction, Amount: bestAmount}
}

// Expected Value (EV) calculation constants
const (
	// foldPenaltyFactor is the fraction of pot considered "lost" when folding.
	// Set to 1/4 because folding gives up equity while limiting losses.
	foldPenaltyFactor = 0.25

	// baseFoldEquity is the base probability opponent folds to a raise.
	// This is a simplified model; in practice it varies by opponent tendencies.
	baseFoldEquity = 0.3

	// allInVariancePenalty reduces all-in EV to account for high variance.
	// Risk-averse play prefers lower variance with similar expected value.
	allInVariancePenalty = 0.9
)

// evaluateAction simulates an action and returns expected value
func (g *PokerGame) evaluateAction(baseState map[string]float64, action Action, amount float64, verbose bool) float64 {
	// Get current hand strength
	result := g.GetHandResult(g.currentPlayer)
	strength := result.Strength()

	// Calculate pot odds
	toCall := g.GetToCall()
	potAfter := g.pot

	switch action {
	case ActionFold:
		// Expected value of folding is losing current investment
		// We use a fraction of the pot as the penalty since we're giving up equity
		ev := -g.pot * foldPenaltyFactor
		if verbose {
			fmt.Printf("    %s: EV = %.0f (lost investment)\n", action, ev)
		}
		return ev

	case ActionCheck:
		// Expected value based on hand strength
		ev := strength * g.pot
		if verbose {
			fmt.Printf("    %s: EV = %.1f (hand strength %.3f × pot %.0f)\n", action, ev, strength, g.pot)
		}
		return ev

	case ActionCall:
		// Pot odds calculation
		potAfter = g.pot + toCall
		ev := strength*potAfter - (1-strength)*toCall
		if verbose {
			fmt.Printf("    %s: EV = %.1f (%.3f × %.0f - %.3f × %.0f)\n", action, ev, strength, potAfter, 1-strength, toCall)
		}
		return ev

	case ActionRaise:
		// Raising can win pot immediately (fold equity) or build pot
		// Fold equity decreases with our hand strength (weaker hands = more fold equity value)
		foldEquity := baseFoldEquity * (1 - strength)
		potAfter = g.pot + amount
		ev := foldEquity*g.pot + (1-foldEquity)*(strength*potAfter-(1-strength)*amount)
		if verbose {
			fmt.Printf("    %s %.0f: EV = %.1f (fold equity %.1f%%, pot after %.0f)\n", action, amount, ev, foldEquity*100, potAfter)
		}
		return ev

	case ActionAllIn:
		// All-in is high variance - apply penalty for risk-averse play
		potAfter = g.pot + amount
		ev := strength * potAfter * allInVariancePenalty
		if verbose {
			fmt.Printf("    %s: EV = %.1f (strength %.3f × pot %.0f × %.1f variance penalty)\n",
				action, ev, strength, potAfter, allInVariancePenalty)
		}
		return ev
	}

	return 0
}

// evaluateActionWithAnalysis evaluates an action using adversarial analysis
func (g *PokerGame) evaluateActionWithAnalysis(baseState map[string]float64, action Action, amount float64, analysis AdversarialAnalysis, verbose bool) float64 {
	// Get current hand strength (using our actual strength vs estimated opponent strength)
	strength := analysis.OurStrength
	oppStrength := analysis.OpponentEstimate.EstimatedStrength
	dangerLevel := analysis.DangerLevel
	
	// Adjust our effective strength based on equity advantage
	// If we have an advantage, we're effectively stronger
	effectiveStrength := strength
	if analysis.EquityAdvantage > 0 {
		effectiveStrength = strength + analysis.EquityAdvantage * 0.5
	} else {
		effectiveStrength = strength + analysis.EquityAdvantage * 0.3
	}
	if effectiveStrength > 1.0 {
		effectiveStrength = 1.0
	}
	if effectiveStrength < 0 {
		effectiveStrength = 0
	}

	// Calculate pot odds
	toCall := g.GetToCall()
	potAfter := g.pot

	switch action {
	case ActionFold:
		// Fold EV is higher when board is dangerous or opponent is aggressive
		foldPenalty := foldPenaltyFactor
		// If board is dangerous and opponent is aggressive, folding is less bad
		if dangerLevel > 0.5 && g.GetOpponentAggression(g.currentPlayer) > 0.6 {
			foldPenalty *= 0.5
		}
		ev := -g.pot * foldPenalty
		if verbose {
			fmt.Printf("    %s: EV = %.0f (adjusted for danger %.1f%%)\n", action, ev, dangerLevel*100)
		}
		return ev

	case ActionCheck:
		// Expected value based on effective strength and danger
		ev := effectiveStrength * g.pot * (1.0 - dangerLevel*0.3)
		if verbose {
			fmt.Printf("    %s: EV = %.1f (eff. strength %.3f × pot %.0f × danger adj)\n", action, ev, effectiveStrength, g.pot)
		}
		return ev

	case ActionCall:
		// Pot odds calculation using estimated winning probability
		potAfter = g.pot + toCall
		// Win prob is based on our strength vs opponent's estimated strength
		winProb := effectiveStrength / (effectiveStrength + oppStrength + 0.001)
		ev := winProb*potAfter - (1-winProb)*toCall
		if verbose {
			fmt.Printf("    %s: EV = %.1f (win prob %.1f%% vs opp strength %.3f)\n", action, ev, winProb*100, oppStrength)
		}
		return ev

	case ActionRaise:
		// Fold equity is higher when opponent is weak/passive and board is dry
		foldEquity := baseFoldEquity
		if oppStrength < 0.3 {
			foldEquity += 0.2 // Weak opponents fold more
		}
		if dangerLevel < 0.3 {
			foldEquity += 0.1 // Dry boards have more fold equity
		}
		if g.GetOpponentAggression(g.currentPlayer) < 0.4 {
			foldEquity += 0.1 // Passive opponents fold more
		}
		if foldEquity > 0.7 {
			foldEquity = 0.7 // Cap fold equity
		}
		
		potAfter = g.pot + amount
		winProb := effectiveStrength / (effectiveStrength + oppStrength + 0.001)
		ev := foldEquity*g.pot + (1-foldEquity)*(winProb*potAfter-(1-winProb)*amount)
		if verbose {
			fmt.Printf("    %s %.0f: EV = %.1f (fold equity %.1f%%, win prob %.1f%%)\n", action, amount, ev, foldEquity*100, winProb*100)
		}
		return ev

	case ActionAllIn:
		// All-in is riskier when board is dangerous
		potAfter = g.pot + amount
		winProb := effectiveStrength / (effectiveStrength + oppStrength + 0.001)
		variancePenalty := allInVariancePenalty - dangerLevel*0.1 // More penalty on dangerous boards
		ev := winProb * potAfter * variancePenalty
		if verbose {
			fmt.Printf("    %s: EV = %.1f (win prob %.1f%%, danger-adjusted variance %.2f)\n",
				action, ev, winProb*100, variancePenalty)
		}
		return ev
	}

	return 0
}

// PrintGameState prints the current game state
func (g *PokerGame) PrintGameState() {
	fmt.Printf("\n=== %s ===\n", g.phase)
	fmt.Printf("Pot: %.0f | Current Bet: %.0f\n", g.pot, g.currentBet)
	fmt.Printf("Community: %s\n", FormatCards(g.communityCards))
	fmt.Println()

	// Player 1
	p1Result := g.GetHandResult(Player1)
	fmt.Printf("Player 1: %s | Chips: %.0f | Bet: %.0f", FormatCards(g.p1Hole), g.p1Chips, g.p1Bet)
	if g.p1Folded {
		fmt.Print(" [FOLDED]")
	}
	fmt.Printf(" | %s\n", p1Result.String())

	// Player 2
	p2Result := g.GetHandResult(Player2)
	fmt.Printf("Player 2: %s | Chips: %.0f | Bet: %.0f", FormatCards(g.p2Hole), g.p2Chips, g.p2Bet)
	if g.p2Folded {
		fmt.Print(" [FOLDED]")
	}
	fmt.Printf(" | %s\n", p2Result.String())

	if !g.IsHandComplete() {
		fmt.Printf("\n%s's turn\n", g.currentPlayer)
	} else if g.winner != nil {
		fmt.Printf("\n%s wins!\n", *g.winner)
	} else {
		fmt.Println("\nSplit pot!")
	}
}
