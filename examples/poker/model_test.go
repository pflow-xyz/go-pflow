package poker

import (
	"testing"
)

func TestCreatePokerPetriNet(t *testing.T) {
	net := CreatePokerPetriNet(2)

	// Check that we have places for phases
	phases := []string{"phase_preflop", "phase_flop", "phase_turn", "phase_river", "phase_showdown", "phase_complete"}
	for _, phase := range phases {
		if _, ok := net.Places[phase]; !ok {
			t.Errorf("Missing phase place: %s", phase)
		}
	}

	// Check player places exist
	playerPlaces := []string{"p1_active", "p1_folded", "p1_hand_str", "p2_active", "p2_folded", "p2_hand_str"}
	for _, place := range playerPlaces {
		if _, ok := net.Places[place]; !ok {
			t.Errorf("Missing player place: %s", place)
		}
	}

	// Check win places exist
	if _, ok := net.Places["p1_wins"]; !ok {
		t.Error("Missing p1_wins place")
	}
	if _, ok := net.Places["p2_wins"]; !ok {
		t.Error("Missing p2_wins place")
	}
}

func TestDeckCardPlaces(t *testing.T) {
	net := CreatePokerPetriNet(2)

	// Check that all 52 deck card places exist
	cardCount := 0
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			placeName := CardPlaceName(suit, rank)
			if _, ok := net.Places[placeName]; !ok {
				t.Errorf("Missing deck card place: %s", placeName)
			} else {
				cardCount++
			}
		}
	}

	if cardCount != 52 {
		t.Errorf("Expected 52 deck card places, got %d", cardCount)
	}
}

func TestPlayerCardMemoryPlaces(t *testing.T) {
	net := CreatePokerPetriNet(2)

	// Check P1 card memory places
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			p1Place := P1CardPlaceName(suit, rank)
			if _, ok := net.Places[p1Place]; !ok {
				t.Errorf("Missing P1 card memory place: %s", p1Place)
			}

			p2Place := P2CardPlaceName(suit, rank)
			if _, ok := net.Places[p2Place]; !ok {
				t.Errorf("Missing P2 card memory place: %s", p2Place)
			}

			commPlace := CommunityCardPlaceName(suit, rank)
			if _, ok := net.Places[commPlace]; !ok {
				t.Errorf("Missing community card memory place: %s", commPlace)
			}
		}
	}
}

func TestDrawPotentialPlaces(t *testing.T) {
	net := CreatePokerPetriNet(2)

	// Check draw potential places exist
	drawPlaces := []string{
		"p1_flush_draw", "p1_straight_draw", "p1_overcards",
		"p2_flush_draw", "p2_straight_draw", "p2_overcards",
		"p1_draw_potential", "p2_draw_potential",
		"p1_completion_odds", "p2_completion_odds",
	}

	for _, place := range drawPlaces {
		if _, ok := net.Places[place]; !ok {
			t.Errorf("Missing draw potential place: %s", place)
		}
	}
}

func TestSuitCountPlaces(t *testing.T) {
	net := CreatePokerPetriNet(2)

	// Check suit count places exist
	suitPlaces := []string{
		"p1_clubs", "p1_diamonds", "p1_hearts", "p1_spades",
		"p2_clubs", "p2_diamonds", "p2_hearts", "p2_spades",
		"comm_clubs", "comm_diamonds", "comm_hearts", "comm_spades",
	}

	for _, place := range suitPlaces {
		if _, ok := net.Places[place]; !ok {
			t.Errorf("Missing suit count place: %s", place)
		}
	}
}

func TestCardCountPlaces(t *testing.T) {
	net := CreatePokerPetriNet(2)

	// Check card count places exist
	countPlaces := []string{"deck_count", "p1_hole_count", "p2_hole_count", "community_count"}

	for _, place := range countPlaces {
		if _, ok := net.Places[place]; !ok {
			t.Errorf("Missing card count place: %s", place)
		}
	}

	// deck_count should start at 52
	if len(net.Places["deck_count"].Initial) == 0 || net.Places["deck_count"].Initial[0] != 52.0 {
		t.Errorf("deck_count should be initialized to 52")
	}
}

func TestHandStrengthODEPlaces(t *testing.T) {
	net := CreatePokerPetriNet(2)

	// Check that hand strength ODE places exist
	odePlaces := []string{
		"p1_rank_input", "p1_highcard_input", "p1_str_delta",
		"p2_rank_input", "p2_highcard_input", "p2_str_delta",
	}
	for _, place := range odePlaces {
		if _, ok := net.Places[place]; !ok {
			t.Errorf("Missing hand strength ODE place: %s", place)
		}
	}
}

func TestHandStrengthODETransitions(t *testing.T) {
	net := CreatePokerPetriNet(2)

	// Check that hand strength ODE transitions exist
	odeTransitions := []string{
		"p1_compute_str", "p2_compute_str",
		"p1_update_str", "p2_update_str",
		"p1_compute_draws", "p2_compute_draws",
	}
	for _, trans := range odeTransitions {
		if _, ok := net.Transitions[trans]; !ok {
			t.Errorf("Missing hand strength ODE transition: %s", trans)
		}
	}
}

func TestHandStrengthODERates(t *testing.T) {
	rates := DefaultRates()

	// Check that hand strength ODE rates are present
	odeRates := []string{
		"p1_compute_str", "p2_compute_str",
		"p1_update_str", "p2_update_str",
		"p1_compute_draws", "p2_compute_draws",
	}
	for _, rate := range odeRates {
		if _, ok := rates[rate]; !ok {
			t.Errorf("Missing hand strength ODE rate: %s", rate)
		}
	}

	// Check that ODE rates are 1.0 (enabled by default)
	for _, rate := range odeRates {
		if rates[rate] != 1.0 {
			t.Errorf("Hand strength ODE rate %s should be 1.0, got %f", rate, rates[rate])
		}
	}
}

func TestDefaultRates(t *testing.T) {
	rates := DefaultRates()

	// Check that all action rates are present
	actions := []string{"p1_fold", "p1_check", "p1_call", "p1_raise", "p1_all_in",
		"p2_fold", "p2_check", "p2_call", "p2_raise", "p2_all_in"}

	for _, action := range actions {
		if _, ok := rates[action]; !ok {
			t.Errorf("Missing rate for action: %s", action)
		}
	}

	// Action rates should be between 0 and 1
	for _, action := range actions {
		if rates[action] < 0 || rates[action] > 1 {
			t.Errorf("Rate for %s should be between 0 and 1, got %f", action, rates[action])
		}
	}
}

func TestStrengthAdjustedRates(t *testing.T) {
	baseRates := DefaultRates()

	// With high hand strength, fold rate should decrease
	highStrength := StrengthAdjustedRates(baseRates, 0.9, 0.5)
	if highStrength["p1_fold"] >= baseRates["p1_fold"] {
		t.Error("High strength should decrease fold rate")
	}

	// With low hand strength, fold rate should remain high
	lowStrength := StrengthAdjustedRates(baseRates, 0.1, 0.5)
	if lowStrength["p1_fold"] <= highStrength["p1_fold"] {
		t.Error("Low strength should have higher fold rate than high strength")
	}

	// With high hand strength, raise rate should increase
	if highStrength["p1_raise"] <= baseRates["p1_raise"] {
		t.Error("High strength should increase raise rate")
	}
}

func TestPotOddsAdjustment(t *testing.T) {
	rates := DefaultRates()

	// Good pot odds (pot big relative to bet)
	goodOdds := PotOddsAdjustment(rates, 100, 10, 0.3)
	if goodOdds["p1_call"] <= rates["p1_call"] {
		t.Error("Good pot odds should increase call rate")
	}

	// Bad pot odds (bet big relative to pot)
	badOdds := PotOddsAdjustment(rates, 10, 100, 0.1)
	if badOdds["p1_fold"] <= rates["p1_fold"] {
		t.Error("Bad pot odds should increase fold rate")
	}
}

func TestGamePhaseString(t *testing.T) {
	tests := []struct {
		phase    GamePhase
		expected string
	}{
		{PhasePreflop, "Pre-flop"},
		{PhaseFlop, "Flop"},
		{PhaseTurn, "Turn"},
		{PhaseRiver, "River"},
		{PhaseShowdown, "Showdown"},
		{PhaseComplete, "Complete"},
	}

	for _, tt := range tests {
		if tt.phase.String() != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.phase.String())
		}
	}
}

func TestActionString(t *testing.T) {
	tests := []struct {
		action   Action
		expected string
	}{
		{ActionFold, "Fold"},
		{ActionCheck, "Check"},
		{ActionCall, "Call"},
		{ActionRaise, "Raise"},
		{ActionAllIn, "All-in"},
	}

	for _, tt := range tests {
		if tt.action.String() != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.action.String())
		}
	}
}

func TestCardPlaceNaming(t *testing.T) {
	// Test card place name generation
	tests := []struct {
		suit     Suit
		rank     Rank
		expected string
	}{
		{Clubs, Two, "deck_c_2"},
		{Diamonds, Ace, "deck_d_14"},
		{Hearts, King, "deck_h_13"},
		{Spades, Ten, "deck_s_10"},
	}

	for _, tt := range tests {
		got := CardPlaceName(tt.suit, tt.rank)
		if got != tt.expected {
			t.Errorf("CardPlaceName(%v, %v) = %s, want %s", tt.suit, tt.rank, got, tt.expected)
		}
	}
}

func TestPlayerCardPlaceNaming(t *testing.T) {
	// Test P1/P2 card place naming
	p1Name := P1CardPlaceName(Hearts, Ace)
	if p1Name != "p1_card_h_14" {
		t.Errorf("P1CardPlaceName = %s, want p1_card_h_14", p1Name)
	}

	p2Name := P2CardPlaceName(Spades, King)
	if p2Name != "p2_card_s_13" {
		t.Errorf("P2CardPlaceName = %s, want p2_card_s_13", p2Name)
	}

	commName := CommunityCardPlaceName(Diamonds, Queen)
	if commName != "comm_card_d_12" {
		t.Errorf("CommunityCardPlaceName = %s, want comm_card_d_12", commName)
	}
}
