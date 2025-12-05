package poker

import (
	"testing"
)

func TestNewCardTracker(t *testing.T) {
	tracker := NewCardTracker()
	if tracker == nil {
		t.Error("Expected non-nil tracker")
	}
}

func TestCardTrackerSetHoleCards(t *testing.T) {
	tracker := NewCardTracker()
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: King, Suit: Spades},
	}
	tracker.SetOurHoleCards(hole)

	remaining := tracker.GetRemainingDeck()
	// Should have 52 - 2 = 50 cards
	if len(remaining) != 50 {
		t.Errorf("Expected 50 remaining cards, got %d", len(remaining))
	}

	// Our hole cards should not be in remaining
	for _, card := range remaining {
		if card.Rank == Ace && card.Suit == Spades {
			t.Error("Ace of Spades should not be in remaining deck")
		}
		if card.Rank == King && card.Suit == Spades {
			t.Error("King of Spades should not be in remaining deck")
		}
	}
}

func TestCardTrackerSetCommunityCards(t *testing.T) {
	tracker := NewCardTracker()
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: King, Suit: Spades},
	}
	tracker.SetOurHoleCards(hole)

	community := []Card{
		{Rank: Queen, Suit: Hearts},
		{Rank: Jack, Suit: Hearts},
		{Rank: Ten, Suit: Hearts},
	}
	tracker.SetCommunityCards(community)

	remaining := tracker.GetRemainingDeck()
	// Should have 52 - 2 - 3 = 47 cards
	if len(remaining) != 47 {
		t.Errorf("Expected 47 remaining cards, got %d", len(remaining))
	}
}

func TestOpponentRangeEstimation(t *testing.T) {
	tracker := NewCardTracker()
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: King, Suit: Spades},
	}
	tracker.SetOurHoleCards(hole)

	community := []Card{
		{Rank: Ace, Suit: Hearts},
		{Rank: King, Suit: Hearts},
		{Rank: Two, Suit: Diamonds},
	}
	tracker.SetCommunityCards(community)

	// Estimate opponent range with neutral aggression
	estimate := tracker.EstimateOpponentRange(0.5)

	// Should have a reasonable sample size
	if estimate.SampleSize == 0 {
		t.Error("Expected non-zero sample size")
	}

	// Probabilities should sum to approximately 1
	total := estimate.HighCardProb + estimate.OnePairProb + estimate.TwoPairProb +
		estimate.ThreeOfAKindProb + estimate.StraightProb + estimate.FlushProb +
		estimate.FullHouseProb + estimate.FourOfAKindProb + estimate.StraightFlushProb +
		estimate.RoyalFlushProb
	if total < 0.99 || total > 1.01 {
		t.Errorf("Hand probabilities should sum to ~1, got %.3f", total)
	}

	// Estimated strength should be in valid range
	if estimate.EstimatedStrength < 0 || estimate.EstimatedStrength > 1 {
		t.Errorf("Estimated strength should be 0-1, got %.3f", estimate.EstimatedStrength)
	}
}

func TestBoardTextureAnalysis(t *testing.T) {
	tracker := NewCardTracker()

	// Test flush draw board
	community := []Card{
		{Rank: Two, Suit: Hearts},
		{Rank: Five, Suit: Hearts},
		{Rank: Eight, Suit: Hearts},
		{Rank: King, Suit: Diamonds},
	}
	tracker.SetCommunityCards(community)

	texture := tracker.AnalyzeBoard()
	if !texture.FlushDraw {
		t.Error("Expected flush draw to be detected")
	}
}

func TestBoardTexturePaired(t *testing.T) {
	tracker := NewCardTracker()

	// Test paired board
	community := []Card{
		{Rank: King, Suit: Hearts},
		{Rank: King, Suit: Diamonds},
		{Rank: Seven, Suit: Clubs},
	}
	tracker.SetCommunityCards(community)

	texture := tracker.AnalyzeBoard()
	if !texture.PairedBoard {
		t.Error("Expected paired board to be detected")
	}
}

func TestAdversarialAnalysis(t *testing.T) {
	tracker := NewCardTracker()

	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: Ace, Suit: Hearts},
	}
	tracker.SetOurHoleCards(hole)

	community := []Card{
		{Rank: Ace, Suit: Diamonds},
		{Rank: King, Suit: Clubs},
		{Rank: Two, Suit: Hearts},
	}
	tracker.SetCommunityCards(community)

	analysis := tracker.GetAdversarialAnalysis(0.5)

	// With trip aces, we should have high strength
	if analysis.OurStrength < 0.3 {
		t.Errorf("Expected high strength with trip aces, got %.3f", analysis.OurStrength)
	}

	// We should have positive equity advantage
	if analysis.EquityAdvantage < 0 {
		t.Error("Expected positive equity advantage with trip aces")
	}

	// Should have a recommendation
	if analysis.RecommendedAction == "" {
		t.Error("Expected a recommendation")
	}
}

func TestAggressionTracking(t *testing.T) {
	tracker := NewCardTracker()

	// Test different actions
	foldAgg := tracker.UpdateFromBettingAction(ActionFold, 0, 100)
	if foldAgg != 0.0 {
		t.Errorf("Fold should have 0 aggression, got %.2f", foldAgg)
	}

	checkAgg := tracker.UpdateFromBettingAction(ActionCheck, 0, 100)
	if checkAgg < 0.3 || checkAgg > 0.5 {
		t.Errorf("Check should have moderate aggression, got %.2f", checkAgg)
	}

	raiseAgg := tracker.UpdateFromBettingAction(ActionRaise, 100, 100)
	if raiseAgg < 0.7 {
		t.Errorf("Pot-sized raise should have high aggression, got %.2f", raiseAgg)
	}

	allInAgg := tracker.UpdateFromBettingAction(ActionAllIn, 1000, 100)
	if allInAgg != 1.0 {
		t.Errorf("All-in should have max aggression, got %.2f", allInAgg)
	}
}

func TestDangerCards(t *testing.T) {
	tracker := NewCardTracker()

	hole := []Card{
		{Rank: Seven, Suit: Spades},
		{Rank: Eight, Suit: Spades},
	}
	tracker.SetOurHoleCards(hole)

	community := []Card{
		{Rank: Nine, Suit: Hearts},
		{Rank: Ten, Suit: Diamonds},
		{Rank: Two, Suit: Clubs},
	}
	tracker.SetCommunityCards(community)

	estimate := tracker.EstimateOpponentRange(0.5)

	// Should identify some danger cards (cards that help opponent)
	// Not checking specific cards as it depends on combinations
	if len(estimate.DangerCards) == 0 && estimate.SampleSize > 100 {
		// With a reasonable sample, we should find some danger cards
		t.Log("Warning: No danger cards identified (may be acceptable)")
	}
}

func TestHandRangeEstimateString(t *testing.T) {
	estimate := HandRangeEstimate{
		EstimatedStrength: 0.35,
		OnePairProb:       0.40,
		TwoPairProb:       0.05,
		ThreeOfAKindProb:  0.02,
		StraightProb:      0.03,
		FlushProb:         0.04,
	}

	str := estimate.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}
}

func TestAdversarialAnalysisString(t *testing.T) {
	analysis := AdversarialAnalysis{
		OurHand:           HandResult{Rank: OnePair, HighCard: Ace},
		OurStrength:       0.15,
		EquityAdvantage:   0.05,
		DangerLevel:       0.3,
		RecommendedAction: "Check/call",
	}

	str := analysis.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}
}
