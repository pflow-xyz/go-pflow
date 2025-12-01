package poker

import (
	"fmt"
	"sort"
)

// CardTracker tracks visible cards and estimates opponent hand ranges
type CardTracker struct {
	knownCards   map[Card]bool // Cards that are visible (community + our hole)
	deadCards    map[Card]bool // Cards that can't be in opponent's hand
	communityCards []Card
	ourHole      []Card
	phase        GamePhase
}

// NewCardTracker creates a new card tracker
func NewCardTracker() *CardTracker {
	return &CardTracker{
		knownCards:     make(map[Card]bool),
		deadCards:      make(map[Card]bool),
		communityCards: make([]Card, 0, 5),
		ourHole:        make([]Card, 0, 2),
	}
}

// SetOurHoleCards sets our hole cards (these are known to us but not opponent)
func (ct *CardTracker) SetOurHoleCards(cards []Card) {
	ct.ourHole = cards
	for _, c := range cards {
		ct.knownCards[c] = true
		ct.deadCards[c] = true // Opponent can't have these
	}
}

// SetCommunityCards sets the community cards (visible to both players)
func (ct *CardTracker) SetCommunityCards(cards []Card) {
	ct.communityCards = cards
	for _, c := range cards {
		ct.knownCards[c] = true
		ct.deadCards[c] = true // Opponent can't have these
	}
}

// SetPhase sets the current game phase
func (ct *CardTracker) SetPhase(phase GamePhase) {
	ct.phase = phase
}

// GetRemainingDeck returns cards not known to be used
func (ct *CardTracker) GetRemainingDeck() []Card {
	remaining := make([]Card, 0, 52-len(ct.deadCards))
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			card := Card{Rank: rank, Suit: suit}
			if !ct.deadCards[card] {
				remaining = append(remaining, card)
			}
		}
	}
	return remaining
}

// HandRangeEstimate represents an estimate of possible hands with probabilities
type HandRangeEstimate struct {
	// Probability distribution over hand types
	HighCardProb     float64
	OnePairProb      float64
	TwoPairProb      float64
	ThreeOfAKindProb float64
	StraightProb     float64
	FlushProb        float64
	FullHouseProb    float64
	FourOfAKindProb  float64
	StraightFlushProb float64
	RoyalFlushProb   float64
	
	// Expected hand strength (0-1)
	EstimatedStrength float64
	
	// Key cards that would help opponent (danger cards)
	DangerCards []Card
	
	// Sample size used for estimation
	SampleSize int
}

// String returns a human-readable description of the hand range
func (hr HandRangeEstimate) String() string {
	return fmt.Sprintf("Est. strength: %.1f%% | Pair: %.1f%% | Two Pair: %.1f%% | Trips: %.1f%% | Straight: %.1f%% | Flush: %.1f%%",
		hr.EstimatedStrength*100,
		hr.OnePairProb*100,
		hr.TwoPairProb*100,
		hr.ThreeOfAKindProb*100,
		hr.StraightProb*100,
		hr.FlushProb*100)
}

// EstimateOpponentRange estimates opponent's likely hand range based on:
// 1. Known cards (community + our hole) - opponent can't have these
// 2. Board texture - what draws are possible
// 3. Betting behavior (aggression factor)
func (ct *CardTracker) EstimateOpponentRange(aggressionFactor float64) HandRangeEstimate {
	remaining := ct.GetRemainingDeck()
	
	// If not enough cards for analysis, return empty estimate
	if len(remaining) < 2 {
		return HandRangeEstimate{}
	}
	
	// Sample opponent hole card combinations
	combinations := getCardCombinations(remaining, 2)
	
	// Track hand type frequencies
	handCounts := make(map[HandRank]int)
	totalStrength := 0.0
	dangerCards := make(map[Card]int)
	
	for _, oppHole := range combinations {
		result := EvaluateHand(oppHole, ct.communityCards)
		handCounts[result.Rank]++
		totalStrength += result.Strength()
		
		// Track which cards contribute to strong hands
		if result.Rank >= OnePair {
			for _, c := range oppHole {
				dangerCards[c]++
			}
		}
	}
	
	total := float64(len(combinations))
	if total == 0 {
		return HandRangeEstimate{}
	}
	
	// Apply aggression factor adjustment
	// Higher aggression suggests stronger range
	strengthAdjustment := 1.0 + (aggressionFactor-0.5)*0.2
	
	estimate := HandRangeEstimate{
		HighCardProb:      float64(handCounts[HighCard]) / total,
		OnePairProb:       float64(handCounts[OnePair]) / total,
		TwoPairProb:       float64(handCounts[TwoPair]) / total,
		ThreeOfAKindProb:  float64(handCounts[ThreeOfAKind]) / total,
		StraightProb:      float64(handCounts[Straight]) / total,
		FlushProb:         float64(handCounts[Flush]) / total,
		FullHouseProb:     float64(handCounts[FullHouse]) / total,
		FourOfAKindProb:   float64(handCounts[FourOfAKind]) / total,
		StraightFlushProb: float64(handCounts[StraightFlush]) / total,
		RoyalFlushProb:    float64(handCounts[RoyalFlush]) / total,
		EstimatedStrength: (totalStrength / total) * strengthAdjustment,
		SampleSize:        len(combinations),
	}
	
	// Cap estimated strength at 1.0
	if estimate.EstimatedStrength > 1.0 {
		estimate.EstimatedStrength = 1.0
	}
	
	// Find top danger cards
	estimate.DangerCards = ct.findDangerCards(dangerCards)
	
	return estimate
}

// findDangerCards returns the cards most likely to help opponent
func (ct *CardTracker) findDangerCards(cardCounts map[Card]int) []Card {
	type cardCount struct {
		card  Card
		count int
	}
	
	counts := make([]cardCount, 0, len(cardCounts))
	for card, count := range cardCounts {
		counts = append(counts, cardCount{card, count})
	}
	
	// Sort by count descending
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].count > counts[j].count
	})
	
	// Return top 5 danger cards
	result := make([]Card, 0, 5)
	for i := 0; i < len(counts) && i < 5; i++ {
		result = append(result, counts[i].card)
	}
	
	return result
}

// AnalyzeBoardTexture analyzes what drawing possibilities exist on the board
type BoardTexture struct {
	// Flush potential
	FlushDraw     bool   // 4 to a flush possible
	FlushComplete bool   // 5 of same suit on board
	FlushSuit     Suit   // Dominant suit if flush draw
	
	// Straight potential
	StraightDraw     bool // 4 to a straight possible (OESD or gutshot)
	StraightComplete bool // Straight possible on board
	
	// Pairing
	PairedBoard  bool // Board has a pair
	TripsOnBoard bool // Board has trips
	
	// High cards
	HighCards    int  // Number of broadway cards (T,J,Q,K,A)
	HasAce       bool
	HasKing      bool
	
	// Connectivity
	Connected    bool // Cards are within 4 ranks
	Rainbow      bool // All different suits
}

// AnalyzeBoard analyzes the texture of community cards
func (ct *CardTracker) AnalyzeBoard() BoardTexture {
	texture := BoardTexture{}
	
	if len(ct.communityCards) == 0 {
		return texture
	}
	
	// Count suits
	suitCounts := make(map[Suit]int)
	for _, c := range ct.communityCards {
		suitCounts[c.Suit]++
	}
	
	// Check flush potential
	for suit, count := range suitCounts {
		if count >= 5 {
			texture.FlushComplete = true
			texture.FlushSuit = suit
		} else if count >= 3 {
			texture.FlushDraw = true
			texture.FlushSuit = suit
		}
	}
	
	// Check rainbow
	texture.Rainbow = len(suitCounts) == len(ct.communityCards)
	
	// Count ranks
	rankCounts := make(map[Rank]int)
	for _, c := range ct.communityCards {
		rankCounts[c.Rank]++
	}
	
	// Check pairing
	for _, count := range rankCounts {
		if count >= 3 {
			texture.TripsOnBoard = true
			texture.PairedBoard = true
		} else if count >= 2 {
			texture.PairedBoard = true
		}
	}
	
	// Check high cards
	highRanks := []Rank{Ten, Jack, Queen, King, Ace}
	for _, c := range ct.communityCards {
		for _, hr := range highRanks {
			if c.Rank == hr {
				texture.HighCards++
				if c.Rank == Ace {
					texture.HasAce = true
				}
				if c.Rank == King {
					texture.HasKing = true
				}
				break
			}
		}
	}
	
	// Check connectivity (simplified - check if max - min rank <= 4)
	if len(ct.communityCards) >= 3 {
		ranks := make([]Rank, len(ct.communityCards))
		for i, c := range ct.communityCards {
			ranks[i] = c.Rank
		}
		sort.Slice(ranks, func(i, j int) bool { return ranks[i] < ranks[j] })
		
		// Check for straight draw potential
		maxRank := ranks[len(ranks)-1]
		minRank := ranks[0]
		if maxRank-minRank <= 4 {
			texture.Connected = true
			texture.StraightDraw = true
		}
		
		// Check if straight is possible on board
		if len(ranks) >= 5 {
			isStraight, _ := checkStraight(ct.communityCards)
			texture.StraightComplete = isStraight
		}
	}
	
	return texture
}

// getCardCombinations returns all 2-card combinations
func getCardCombinations(cards []Card, n int) [][]Card {
	if n != 2 {
		return getCombinations(cards, n)
	}
	
	// Optimized for n=2 case (most common for opponent hole cards)
	combinations := make([][]Card, 0, len(cards)*(len(cards)-1)/2)
	for i := 0; i < len(cards); i++ {
		for j := i + 1; j < len(cards); j++ {
			combinations = append(combinations, []Card{cards[i], cards[j]})
		}
	}
	return combinations
}

// UpdateFromBettingAction adjusts estimates based on opponent's betting
func (ct *CardTracker) UpdateFromBettingAction(action Action, betSize, potSize float64) float64 {
	// Returns an aggression factor (0-1) based on action
	switch action {
	case ActionFold:
		return 0.0 // Very weak
	case ActionCheck:
		return 0.4 // Slightly weak or trapping
	case ActionCall:
		return 0.5 // Neutral
	case ActionRaise:
		ratio := betSize / potSize
		if ratio > 1.0 {
			return 0.9 // Very strong
		}
		return 0.5 + ratio*0.3 // 0.5-0.8
	case ActionAllIn:
		return 1.0 // Maximum strength or bluff
	}
	return 0.5
}

// AdversarialAnalysis provides a complete adversarial analysis of the current situation
type AdversarialAnalysis struct {
	OurHand           HandResult
	OurStrength       float64
	OpponentEstimate  HandRangeEstimate
	BoardTexture      BoardTexture
	EquityAdvantage   float64 // Our strength - opponent estimated strength
	DangerLevel       float64 // 0-1, how dangerous the board is for us
	RecommendedAction string
}

// GetAdversarialAnalysis provides full analysis considering opponent's likely range
func (ct *CardTracker) GetAdversarialAnalysis(aggressionFactor float64) AdversarialAnalysis {
	analysis := AdversarialAnalysis{}
	
	// Evaluate our hand
	analysis.OurHand = EvaluateHand(ct.ourHole, ct.communityCards)
	analysis.OurStrength = analysis.OurHand.Strength()
	
	// Estimate opponent range
	analysis.OpponentEstimate = ct.EstimateOpponentRange(aggressionFactor)
	
	// Analyze board texture
	analysis.BoardTexture = ct.AnalyzeBoard()
	
	// Calculate equity advantage
	analysis.EquityAdvantage = analysis.OurStrength - analysis.OpponentEstimate.EstimatedStrength
	
	// Calculate danger level
	analysis.DangerLevel = ct.calculateDangerLevel(analysis.BoardTexture, analysis.OpponentEstimate)
	
	// Generate recommendation
	analysis.RecommendedAction = ct.generateRecommendation(analysis)
	
	return analysis
}

// calculateDangerLevel determines how dangerous the board is for our hand
func (ct *CardTracker) calculateDangerLevel(texture BoardTexture, oppEstimate HandRangeEstimate) float64 {
	danger := 0.0
	
	// Flush draws are dangerous
	if texture.FlushDraw {
		danger += 0.15
	}
	if texture.FlushComplete {
		danger += 0.25
	}
	
	// Straight possibilities
	if texture.StraightDraw {
		danger += 0.1
	}
	if texture.StraightComplete {
		danger += 0.2
	}
	
	// Paired boards help trips/full houses
	if texture.PairedBoard {
		danger += 0.15
	}
	if texture.TripsOnBoard {
		danger += 0.3
	}
	
	// High cards favor opponents with broadway
	danger += float64(texture.HighCards) * 0.05
	
	// Opponent's estimated made hand probability
	madeHandProb := oppEstimate.OnePairProb + oppEstimate.TwoPairProb +
		oppEstimate.ThreeOfAKindProb + oppEstimate.StraightProb +
		oppEstimate.FlushProb + oppEstimate.FullHouseProb +
		oppEstimate.FourOfAKindProb + oppEstimate.StraightFlushProb +
		oppEstimate.RoyalFlushProb
	
	danger += madeHandProb * 0.3
	
	// Cap at 1.0
	if danger > 1.0 {
		danger = 1.0
	}
	
	return danger
}

// generateRecommendation provides a strategic recommendation
func (ct *CardTracker) generateRecommendation(analysis AdversarialAnalysis) string {
	advantage := analysis.EquityAdvantage
	danger := analysis.DangerLevel
	strength := analysis.OurStrength
	
	// Strong hand with advantage
	if strength > 0.5 && advantage > 0.1 {
		if danger > 0.5 {
			return "Value bet cautiously - board is wet"
		}
		return "Value bet for max extraction"
	}
	
	// Strong hand but dangerous board
	if strength > 0.3 && danger > 0.6 {
		return "Check/call - let opponent bluff"
	}
	
	// Medium hand with slight advantage
	if advantage > 0 && strength > 0.15 {
		if danger < 0.4 {
			return "Thin value bet or check/call"
		}
		return "Check/fold to aggression"
	}
	
	// Weak hand with disadvantage
	if advantage < -0.1 {
		if strength < 0.1 {
			return "Fold to any bet"
		}
		return "Check/fold unless getting good pot odds"
	}
	
	// Marginal situation
	return "Check and evaluate opponent action"
}

// String provides a formatted output of the analysis
func (a AdversarialAnalysis) String() string {
	result := fmt.Sprintf("=== Adversarial Analysis ===\n")
	result += fmt.Sprintf("Our Hand: %s (%.1f%% strength)\n", a.OurHand.String(), a.OurStrength*100)
	result += fmt.Sprintf("Opponent Range: %s\n", a.OpponentEstimate.String())
	result += fmt.Sprintf("Equity Advantage: %+.1f%%\n", a.EquityAdvantage*100)
	result += fmt.Sprintf("Board Danger: %.1f%%\n", a.DangerLevel*100)
	result += fmt.Sprintf("Recommendation: %s\n", a.RecommendedAction)
	
	if len(a.OpponentEstimate.DangerCards) > 0 {
		result += fmt.Sprintf("Watch for: %s\n", FormatCards(a.OpponentEstimate.DangerCards))
	}
	
	return result
}
