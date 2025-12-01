package poker

import (
	"testing"
)

func TestNewDeck(t *testing.T) {
	deck := NewDeck()
	if deck.Remaining() != 52 {
		t.Errorf("Expected 52 cards, got %d", deck.Remaining())
	}
}

func TestDeckShuffle(t *testing.T) {
	deck1 := NewDeck()
	deck2 := NewDeck()

	deck1.Shuffle()

	// Decks should be different after shuffle (with very high probability)
	different := false
	for i := 0; i < 52; i++ {
		c1 := deck1.Deal()
		c2 := deck2.Deal()
		if c1.Rank != c2.Rank || c1.Suit != c2.Suit {
			different = true
			break
		}
	}

	if !different {
		t.Error("Shuffled deck should be different from unshuffled deck")
	}
}

func TestDeckDeal(t *testing.T) {
	deck := NewDeck()
	deck.Deal()
	if deck.Remaining() != 51 {
		t.Errorf("Expected 51 cards after dealing one, got %d", deck.Remaining())
	}
}

func TestCardString(t *testing.T) {
	card := Card{Rank: Ace, Suit: Spades}
	if card.String() != "A♠" {
		t.Errorf("Expected A♠, got %s", card.String())
	}

	card = Card{Rank: Ten, Suit: Hearts}
	if card.String() != "10♥" {
		t.Errorf("Expected 10♥, got %s", card.String())
	}
}

func TestHandRankString(t *testing.T) {
	tests := []struct {
		rank     HandRank
		expected string
	}{
		{HighCard, "High Card"},
		{OnePair, "One Pair"},
		{TwoPair, "Two Pair"},
		{ThreeOfAKind, "Three of a Kind"},
		{Straight, "Straight"},
		{Flush, "Flush"},
		{FullHouse, "Full House"},
		{FourOfAKind, "Four of a Kind"},
		{StraightFlush, "Straight Flush"},
		{RoyalFlush, "Royal Flush"},
	}

	for _, tt := range tests {
		if tt.rank.String() != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.rank.String())
		}
	}
}

func TestEvaluateHighCard(t *testing.T) {
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: King, Suit: Hearts},
	}
	community := []Card{
		{Rank: Two, Suit: Diamonds},
		{Rank: Five, Suit: Clubs},
		{Rank: Eight, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != HighCard {
		t.Errorf("Expected High Card, got %s", result.Rank)
	}
	if result.HighCard != Ace {
		t.Errorf("Expected Ace high, got %s", result.HighCard)
	}
}

func TestEvaluateOnePair(t *testing.T) {
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: Ace, Suit: Hearts},
	}
	community := []Card{
		{Rank: Two, Suit: Diamonds},
		{Rank: Five, Suit: Clubs},
		{Rank: Eight, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != OnePair {
		t.Errorf("Expected One Pair, got %s", result.Rank)
	}
	if result.HighCard != Ace {
		t.Errorf("Expected Aces, got %s", result.HighCard)
	}
}

func TestEvaluateTwoPair(t *testing.T) {
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: Ace, Suit: Hearts},
	}
	community := []Card{
		{Rank: King, Suit: Diamonds},
		{Rank: King, Suit: Clubs},
		{Rank: Eight, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != TwoPair {
		t.Errorf("Expected Two Pair, got %s", result.Rank)
	}
	if result.HighCard != Ace {
		t.Errorf("Expected Aces high, got %s", result.HighCard)
	}
}

func TestEvaluateThreeOfAKind(t *testing.T) {
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: Ace, Suit: Hearts},
	}
	community := []Card{
		{Rank: Ace, Suit: Diamonds},
		{Rank: Five, Suit: Clubs},
		{Rank: Eight, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != ThreeOfAKind {
		t.Errorf("Expected Three of a Kind, got %s", result.Rank)
	}
}

func TestEvaluateStraight(t *testing.T) {
	hole := []Card{
		{Rank: Ten, Suit: Spades},
		{Rank: Jack, Suit: Hearts},
	}
	community := []Card{
		{Rank: Queen, Suit: Diamonds},
		{Rank: King, Suit: Clubs},
		{Rank: Ace, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != Straight {
		t.Errorf("Expected Straight, got %s", result.Rank)
	}
	if result.HighCard != Ace {
		t.Errorf("Expected Ace high straight, got %s", result.HighCard)
	}
}

func TestEvaluateFlush(t *testing.T) {
	hole := []Card{
		{Rank: Two, Suit: Hearts},
		{Rank: Five, Suit: Hearts},
	}
	community := []Card{
		{Rank: Eight, Suit: Hearts},
		{Rank: Jack, Suit: Hearts},
		{Rank: Ace, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != Flush {
		t.Errorf("Expected Flush, got %s", result.Rank)
	}
}

func TestEvaluateFullHouse(t *testing.T) {
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: Ace, Suit: Hearts},
	}
	community := []Card{
		{Rank: Ace, Suit: Diamonds},
		{Rank: King, Suit: Clubs},
		{Rank: King, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != FullHouse {
		t.Errorf("Expected Full House, got %s", result.Rank)
	}
}

func TestEvaluateFourOfAKind(t *testing.T) {
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: Ace, Suit: Hearts},
	}
	community := []Card{
		{Rank: Ace, Suit: Diamonds},
		{Rank: Ace, Suit: Clubs},
		{Rank: King, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != FourOfAKind {
		t.Errorf("Expected Four of a Kind, got %s", result.Rank)
	}
}

func TestEvaluateStraightFlush(t *testing.T) {
	hole := []Card{
		{Rank: Six, Suit: Hearts},
		{Rank: Seven, Suit: Hearts},
	}
	community := []Card{
		{Rank: Eight, Suit: Hearts},
		{Rank: Nine, Suit: Hearts},
		{Rank: Ten, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != StraightFlush {
		t.Errorf("Expected Straight Flush, got %s", result.Rank)
	}
}

func TestEvaluateRoyalFlush(t *testing.T) {
	hole := []Card{
		{Rank: Ten, Suit: Hearts},
		{Rank: Jack, Suit: Hearts},
	}
	community := []Card{
		{Rank: Queen, Suit: Hearts},
		{Rank: King, Suit: Hearts},
		{Rank: Ace, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != RoyalFlush {
		t.Errorf("Expected Royal Flush, got %s", result.Rank)
	}
}

func TestHandStrength(t *testing.T) {
	// High card should have low strength
	highCard := HandResult{Rank: HighCard, HighCard: Ace}
	if highCard.Strength() > 0.2 {
		t.Errorf("High card strength should be low, got %f", highCard.Strength())
	}

	// Royal flush should have high strength
	royal := HandResult{Rank: RoyalFlush, HighCard: Ace}
	if royal.Strength() < 0.9 {
		t.Errorf("Royal flush strength should be high, got %f", royal.Strength())
	}

	// Royal flush should beat high card
	if highCard.Score() >= royal.Score() {
		t.Error("Royal flush should have higher score than high card")
	}
}

func TestWheelStraight(t *testing.T) {
	hole := []Card{
		{Rank: Ace, Suit: Spades},
		{Rank: Two, Suit: Hearts},
	}
	community := []Card{
		{Rank: Three, Suit: Diamonds},
		{Rank: Four, Suit: Clubs},
		{Rank: Five, Suit: Hearts},
	}

	result := EvaluateHand(hole, community)
	if result.Rank != Straight {
		t.Errorf("Expected Straight (wheel), got %s", result.Rank)
	}
	if result.HighCard != Five {
		t.Errorf("Expected Five high (wheel), got %s", result.HighCard)
	}
}
