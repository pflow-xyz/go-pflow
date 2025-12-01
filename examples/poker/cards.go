package poker

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// Suit represents a card suit
type Suit int

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

func (s Suit) String() string {
	return [...]string{"♣", "♦", "♥", "♠"}[s]
}

// Rank represents a card rank
type Rank int

const (
	Two Rank = iota + 2
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
)

func (r Rank) String() string {
	if r >= Two && r <= Ten {
		return fmt.Sprintf("%d", int(r))
	}
	return map[Rank]string{
		Jack:  "J",
		Queen: "Q",
		King:  "K",
		Ace:   "A",
	}[r]
}

// Card represents a playing card
type Card struct {
	Rank Rank
	Suit Suit
}

func (c Card) String() string {
	return fmt.Sprintf("%s%s", c.Rank.String(), c.Suit.String())
}

// Deck represents a deck of cards
type Deck struct {
	cards []Card
}

// NewDeck creates a standard 52-card deck
func NewDeck() *Deck {
	cards := make([]Card, 0, 52)
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			cards = append(cards, Card{Rank: rank, Suit: suit})
		}
	}
	return &Deck{cards: cards}
}

// Shuffle randomizes the deck
func (d *Deck) Shuffle() {
	rand.Shuffle(len(d.cards), func(i, j int) {
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	})
}

// Deal removes and returns the top card.
// Panics if deck is empty - this is intentional as dealing from an empty deck
// indicates a programming error (standard 52-card deck should never be exhausted
// in a single hand of Texas Hold'em which needs at most 9 cards).
func (d *Deck) Deal() Card {
	if len(d.cards) == 0 {
		panic("deck is empty - this should never happen in normal gameplay")
	}
	card := d.cards[0]
	d.cards = d.cards[1:]
	return card
}

// DealN removes and returns n cards from the top
func (d *Deck) DealN(n int) []Card {
	cards := make([]Card, n)
	for i := 0; i < n; i++ {
		cards[i] = d.Deal()
	}
	return cards
}

// Remaining returns the number of cards left
func (d *Deck) Remaining() int {
	return len(d.cards)
}

// HandRank represents the strength of a poker hand
type HandRank int

const (
	HighCard HandRank = iota
	OnePair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
	RoyalFlush
)

func (h HandRank) String() string {
	return [...]string{
		"High Card",
		"One Pair",
		"Two Pair",
		"Three of a Kind",
		"Straight",
		"Flush",
		"Full House",
		"Four of a Kind",
		"Straight Flush",
		"Royal Flush",
	}[h]
}

// HandResult represents the evaluated hand
type HandResult struct {
	Rank     HandRank
	HighCard Rank
	Kickers  []Rank
	Cards    []Card // Best 5 cards
}

// Score returns a numeric score for comparison (higher is better)
func (h HandResult) Score() float64 {
	// Base score from hand rank (0-9) * 1000
	score := float64(h.Rank) * 1000.0

	// Add high card value (2-14) * 10
	score += float64(h.HighCard) * 10.0

	// Add kickers (for tie-breaking)
	for i, k := range h.Kickers {
		score += float64(k) / float64(i+1)
	}

	return score
}

// Strength returns a normalized strength (0-1) for betting decisions
func (h HandResult) Strength() float64 {
	// Max possible score: RoyalFlush (9*1000) + Ace (14*10) = 9140
	return h.Score() / 9140.0
}

func (h HandResult) String() string {
	return fmt.Sprintf("%s (%s high)", h.Rank.String(), h.HighCard.String())
}

// EvaluateHand evaluates the best 5-card hand from hole cards + community cards
func EvaluateHand(hole []Card, community []Card) HandResult {
	allCards := make([]Card, 0, len(hole)+len(community))
	allCards = append(allCards, hole...)
	allCards = append(allCards, community...)

	if len(allCards) < 5 {
		// Not enough cards, return high card
		sort.Slice(allCards, func(i, j int) bool {
			return allCards[i].Rank > allCards[j].Rank
		})
		kickers := make([]Rank, 0)
		for i := 1; i < len(allCards); i++ {
			kickers = append(kickers, allCards[i].Rank)
		}
		highCard := Two
		if len(allCards) > 0 {
			highCard = allCards[0].Rank
		}
		return HandResult{
			Rank:     HighCard,
			HighCard: highCard,
			Kickers:  kickers,
			Cards:    allCards,
		}
	}

	// Generate all 5-card combinations and find best
	bestResult := HandResult{Rank: HighCard, HighCard: Two}
	combinations := getCombinations(allCards, 5)

	for _, combo := range combinations {
		result := evaluate5Cards(combo)
		if result.Score() > bestResult.Score() {
			bestResult = result
		}
	}

	return bestResult
}

// evaluate5Cards evaluates exactly 5 cards
func evaluate5Cards(cards []Card) HandResult {
	// Sort cards by rank (descending)
	sorted := make([]Card, len(cards))
	copy(sorted, cards)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Rank > sorted[j].Rank
	})

	// Count ranks and suits
	rankCount := make(map[Rank]int)
	suitCount := make(map[Suit]int)
	for _, c := range cards {
		rankCount[c.Rank]++
		suitCount[c.Suit]++
	}

	// Check for flush
	isFlush := false
	for _, count := range suitCount {
		if count >= 5 {
			isFlush = true
			break
		}
	}

	// Check for straight
	isStraight, straightHigh := checkStraight(sorted)

	// Check for royal flush
	if isFlush && isStraight && straightHigh == Ace {
		return HandResult{Rank: RoyalFlush, HighCard: Ace, Cards: sorted}
	}

	// Check for straight flush
	if isFlush && isStraight {
		return HandResult{Rank: StraightFlush, HighCard: straightHigh, Cards: sorted}
	}

	// Count of counts
	pairs := 0
	threes := 0
	fours := 0
	var pairRanks, threeRanks, fourRanks []Rank

	for rank, count := range rankCount {
		switch count {
		case 2:
			pairs++
			pairRanks = append(pairRanks, rank)
		case 3:
			threes++
			threeRanks = append(threeRanks, rank)
		case 4:
			fours++
			fourRanks = append(fourRanks, rank)
		}
	}

	// Sort pair/three ranks descending
	sort.Slice(pairRanks, func(i, j int) bool { return pairRanks[i] > pairRanks[j] })
	sort.Slice(threeRanks, func(i, j int) bool { return threeRanks[i] > threeRanks[j] })
	sort.Slice(fourRanks, func(i, j int) bool { return fourRanks[i] > fourRanks[j] })

	// Get kickers (cards not in pairs/threes/fours)
	usedRanks := make(map[Rank]bool)
	for _, r := range pairRanks {
		usedRanks[r] = true
	}
	for _, r := range threeRanks {
		usedRanks[r] = true
	}
	for _, r := range fourRanks {
		usedRanks[r] = true
	}
	var kickers []Rank
	for _, c := range sorted {
		if !usedRanks[c.Rank] {
			kickers = append(kickers, c.Rank)
		}
	}

	// Four of a kind
	if fours > 0 {
		return HandResult{Rank: FourOfAKind, HighCard: fourRanks[0], Kickers: kickers, Cards: sorted}
	}

	// Full house
	if threes > 0 && pairs > 0 {
		return HandResult{Rank: FullHouse, HighCard: threeRanks[0], Kickers: pairRanks, Cards: sorted}
	}

	// Flush
	if isFlush {
		return HandResult{Rank: Flush, HighCard: sorted[0].Rank, Kickers: kickers, Cards: sorted}
	}

	// Straight
	if isStraight {
		return HandResult{Rank: Straight, HighCard: straightHigh, Cards: sorted}
	}

	// Three of a kind
	if threes > 0 {
		return HandResult{Rank: ThreeOfAKind, HighCard: threeRanks[0], Kickers: kickers, Cards: sorted}
	}

	// Two pair
	if pairs >= 2 {
		return HandResult{Rank: TwoPair, HighCard: pairRanks[0], Kickers: append([]Rank{pairRanks[1]}, kickers...), Cards: sorted}
	}

	// One pair
	if pairs == 1 {
		return HandResult{Rank: OnePair, HighCard: pairRanks[0], Kickers: kickers, Cards: sorted}
	}

	// High card
	return HandResult{Rank: HighCard, HighCard: sorted[0].Rank, Kickers: kickers[1:], Cards: sorted}
}

// checkStraight checks if cards form a straight
func checkStraight(sorted []Card) (bool, Rank) {
	if len(sorted) < 5 {
		return false, Two
	}

	// Get unique ranks
	ranks := make([]Rank, 0)
	seen := make(map[Rank]bool)
	for _, c := range sorted {
		if !seen[c.Rank] {
			ranks = append(ranks, c.Rank)
			seen[c.Rank] = true
		}
	}

	if len(ranks) < 5 {
		return false, Two
	}

	// Sort ranks descending
	sort.Slice(ranks, func(i, j int) bool {
		return ranks[i] > ranks[j]
	})

	// Check for regular straight
	for i := 0; i <= len(ranks)-5; i++ {
		if ranks[i]-ranks[i+4] == 4 {
			return true, ranks[i]
		}
	}

	// Check for wheel (A-2-3-4-5)
	if seen[Ace] && seen[Two] && seen[Three] && seen[Four] && seen[Five] {
		return true, Five
	}

	return false, Two
}

// getCombinations returns all n-element combinations of cards
func getCombinations(cards []Card, n int) [][]Card {
	if n > len(cards) {
		return nil
	}
	if n == len(cards) {
		return [][]Card{cards}
	}

	var result [][]Card
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}

	for {
		combo := make([]Card, n)
		for i, idx := range indices {
			combo[i] = cards[idx]
		}
		result = append(result, combo)

		// Find rightmost element that can be incremented
		i := n - 1
		for i >= 0 && indices[i] == i+len(cards)-n {
			i--
		}
		if i < 0 {
			break
		}

		indices[i]++
		for j := i + 1; j < n; j++ {
			indices[j] = indices[j-1] + 1
		}
	}

	return result
}

// FormatCards formats a slice of cards as a string
func FormatCards(cards []Card) string {
	strs := make([]string, len(cards))
	for i, c := range cards {
		strs[i] = c.String()
	}
	return strings.Join(strs, " ")
}
