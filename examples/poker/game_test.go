package poker

import (
	"testing"
)

func TestNewPokerGame(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)

	if game.GetPlayerChips(Player1) != 1000 {
		t.Errorf("Expected Player 1 to have 1000 chips, got %.0f", game.GetPlayerChips(Player1))
	}
	if game.GetPlayerChips(Player2) != 1000 {
		t.Errorf("Expected Player 2 to have 1000 chips, got %.0f", game.GetPlayerChips(Player2))
	}
}

func TestStartHand(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)
	game.StartHand()

	// Check blinds were posted
	pot := game.GetPot()
	if pot != 3 { // Small blind (1) + Big blind (2)
		t.Errorf("Expected pot of 3, got %.0f", pot)
	}

	// Check hole cards were dealt
	p1Hole := game.GetPlayerHole(Player1)
	if len(p1Hole) != 2 {
		t.Errorf("Expected Player 1 to have 2 hole cards, got %d", len(p1Hole))
	}

	p2Hole := game.GetPlayerHole(Player2)
	if len(p2Hole) != 2 {
		t.Errorf("Expected Player 2 to have 2 hole cards, got %d", len(p2Hole))
	}

	// Check phase is pre-flop
	if game.GetPhase() != PhasePreflop {
		t.Errorf("Expected phase to be Pre-flop, got %s", game.GetPhase())
	}
}

func TestGetAvailableActions(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)
	game.StartHand()

	actions := game.GetAvailableActions()

	// Should have multiple actions available
	if len(actions) < 2 {
		t.Error("Expected at least 2 actions available")
	}

	// Fold should always be available
	hasFold := false
	for _, a := range actions {
		if a == ActionFold {
			hasFold = true
			break
		}
	}
	if !hasFold {
		t.Error("Fold should always be available")
	}
}

func TestMakeActionFold(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)
	game.StartHand()

	// Player 1 folds
	err := game.MakeAction(ActionFold, 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Game should be complete
	if !game.IsHandComplete() {
		t.Error("Expected hand to be complete after fold")
	}

	// Player 2 should be winner
	winner := game.GetWinner()
	if winner == nil || *winner != Player2 {
		t.Error("Expected Player 2 to win after Player 1 folds")
	}
}

func TestMakeActionCall(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)
	game.StartHand()

	toCall := game.GetToCall()
	if toCall != 1 { // Player 1 needs to call 1 to match big blind
		t.Errorf("Expected to call 1, got %.0f", toCall)
	}

	// Player 1 calls
	err := game.MakeAction(ActionCall, 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should not be Player 1's turn anymore
	if game.GetCurrentPlayer() == Player1 {
		t.Error("Expected turn to switch after call")
	}
}

func TestPhaseAdvancement(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)
	game.StartHand()

	// Player 1 calls
	game.MakeAction(ActionCall, 0)

	// Player 2 checks
	game.MakeAction(ActionCheck, 0)

	// Should now be flop
	if game.GetPhase() != PhaseFlop {
		t.Errorf("Expected phase to be Flop, got %s", game.GetPhase())
	}

	// Should have 3 community cards
	community := game.GetCommunityCards()
	if len(community) != 3 {
		t.Errorf("Expected 3 community cards, got %d", len(community))
	}
}

func TestCompleteHand(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)
	game.StartHand()

	// Play through all phases
	phases := []GamePhase{PhasePreflop, PhaseFlop, PhaseTurn, PhaseRiver}

	for _, expectedPhase := range phases {
		if game.GetPhase() != expectedPhase {
			t.Errorf("Expected phase %s, got %s", expectedPhase, game.GetPhase())
		}

		if game.IsHandComplete() {
			break
		}

		// Both players check/call through
		actions := game.GetAvailableActions()
		for _, a := range actions {
			if a == ActionCheck || a == ActionCall {
				game.MakeAction(a, 0)
				break
			}
		}

		if game.IsHandComplete() {
			break
		}

		actions = game.GetAvailableActions()
		for _, a := range actions {
			if a == ActionCheck || a == ActionCall {
				game.MakeAction(a, 0)
				break
			}
		}
	}

	// Hand should be complete
	if !game.IsHandComplete() {
		t.Error("Expected hand to be complete after playing through all phases")
	}

	// Should have 5 community cards at showdown
	community := game.GetCommunityCards()
	if len(community) != 5 {
		t.Errorf("Expected 5 community cards at showdown, got %d", len(community))
	}
}

func TestHandEvaluation(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)
	game.StartHand()

	// Get hand results
	p1Result := game.GetHandResult(Player1)
	p2Result := game.GetHandResult(Player2)

	// Both should have valid hand ranks
	if p1Result.Rank < HighCard || p1Result.Rank > RoyalFlush {
		t.Error("Player 1 hand rank out of range")
	}
	if p2Result.Rank < HighCard || p2Result.Rank > RoyalFlush {
		t.Error("Player 2 hand rank out of range")
	}

	// Strengths should be in valid range
	if p1Result.Strength() < 0 || p1Result.Strength() > 1 {
		t.Error("Player 1 strength out of range")
	}
	if p2Result.Strength() < 0 || p2Result.Strength() > 1 {
		t.Error("Player 2 strength out of range")
	}
}

func TestAIDecision(t *testing.T) {
	game := NewPokerGame(1000, 1, 2)
	game.StartHand()

	// Test random AI
	randomDecision := game.GetRandomAction()
	if randomDecision.Action < ActionFold || randomDecision.Action > ActionAllIn {
		t.Error("Random AI returned invalid action")
	}

	// Test ODE AI
	odeDecision := game.GetODEAction(false)
	if odeDecision.Action < ActionFold || odeDecision.Action > ActionAllIn {
		t.Error("ODE AI returned invalid action")
	}
}

func TestPlayerString(t *testing.T) {
	if Player1.String() != "Player 1" {
		t.Errorf("Expected 'Player 1', got '%s'", Player1.String())
	}
	if Player2.String() != "Player 2" {
		t.Errorf("Expected 'Player 2', got '%s'", Player2.String())
	}
}
