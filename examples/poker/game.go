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

// UpdateHandStrengths updates the hand strength places in the Petri net
func (g *PokerGame) UpdateHandStrengths() {
	p1Result := EvaluateHand(g.p1Hole, g.communityCards)
	p2Result := EvaluateHand(g.p2Hole, g.communityCards)

	// Update state with normalized strengths
	state := g.engine.GetState()
	state["p1_hand_str"] = p1Result.Strength()
	state["p2_hand_str"] = p2Result.Strength()
	g.engine.SetState(state)

	// Update rates based on hand strengths
	g.rates = StrengthAdjustedRates(DefaultRates(), p1Result.Strength(), p2Result.Strength())
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

	g.engine.SetState(state)
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
