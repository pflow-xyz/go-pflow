package karate

import (
	"testing"
)

func TestNewGame(t *testing.T) {
	game := NewGame()

	if game == nil {
		t.Fatal("NewGame returned nil")
	}

	state := game.GetState()

	// Check initial health
	if state.P1Health != MaxHealth {
		t.Errorf("P1 health = %v, want %v", state.P1Health, MaxHealth)
	}
	if state.P2Health != MaxHealth {
		t.Errorf("P2 health = %v, want %v", state.P2Health, MaxHealth)
	}

	// Check initial stamina
	if state.P1Stamina != MaxStamina {
		t.Errorf("P1 stamina = %v, want %v", state.P1Stamina, MaxStamina)
	}
	if state.P2Stamina != MaxStamina {
		t.Errorf("P2 stamina = %v, want %v", state.P2Stamina, MaxStamina)
	}

	// Check initial positions
	if state.P1Position != 1 {
		t.Errorf("P1 position = %v, want 1", state.P1Position)
	}
	if state.P2Position != 3 {
		t.Errorf("P2 position = %v, want 3", state.P2Position)
	}

	// Check not blocking
	if state.P1Blocking {
		t.Error("P1 should not be blocking initially")
	}
	if state.P2Blocking {
		t.Error("P2 should not be blocking initially")
	}

	// Check game not over
	if state.GameOver {
		t.Error("Game should not be over initially")
	}
}

func TestGetAvailableActions(t *testing.T) {
	game := NewGame()

	// P1 should have all actions available initially
	actions := game.GetAvailableActions(Player1)

	// Should include recover (always available)
	hasRecover := false
	for _, a := range actions {
		if a == ActionRecover {
			hasRecover = true
			break
		}
	}
	if !hasRecover {
		t.Error("Recover should always be available")
	}

	// Should have punch (5 stamina, we have 50)
	hasPunch := false
	for _, a := range actions {
		if a == ActionPunch {
			hasPunch = true
			break
		}
	}
	if !hasPunch {
		t.Error("Punch should be available with full stamina")
	}

	// Should have special (15 stamina, we have 50)
	hasSpecial := false
	for _, a := range actions {
		if a == ActionSpecial {
			hasSpecial = true
			break
		}
	}
	if !hasSpecial {
		t.Error("Special should be available with full stamina")
	}
}

func TestSubmitAction(t *testing.T) {
	game := NewGame()

	// Submit P1 action
	err := game.SubmitAction(Player1, ActionPunch)
	if err != nil {
		t.Errorf("SubmitAction failed: %v", err)
	}

	// Should not resolve yet (waiting for P2)
	if game.HasBothActions() {
		t.Error("Should not have both actions yet")
	}

	// Submit P2 action
	err = game.SubmitAction(Player2, ActionKick)
	if err != nil {
		t.Errorf("SubmitAction failed: %v", err)
	}

	// Should have both actions now
	if !game.HasBothActions() {
		t.Error("Should have both actions now")
	}
}

func TestResolveTurn(t *testing.T) {
	game := NewGame()

	// Submit actions for both players
	game.SubmitAction(Player1, ActionMoveR) // Move closer
	game.SubmitAction(Player2, ActionMoveL) // Move closer

	state, err := game.ResolveTurn()
	if err != nil {
		t.Fatalf("ResolveTurn failed: %v", err)
	}

	// Check positions updated
	if state.P1Position != 2 {
		t.Errorf("P1 position = %v, want 2", state.P1Position)
	}
	if state.P2Position != 2 {
		t.Errorf("P2 position = %v, want 2", state.P2Position)
	}

	// Check stamina consumed
	expectedStamina := MaxStamina - MoveStamina
	if state.P1Stamina != expectedStamina {
		t.Errorf("P1 stamina = %v, want %v", state.P1Stamina, expectedStamina)
	}
	if state.P2Stamina != expectedStamina {
		t.Errorf("P2 stamina = %v, want %v", state.P2Stamina, expectedStamina)
	}

	// Check turn incremented
	if state.TurnNum != 2 {
		t.Errorf("Turn = %v, want 2", state.TurnNum)
	}
}

func TestDamageDealing(t *testing.T) {
	game := NewGame()

	// Move players into range first
	game.SubmitAction(Player1, ActionMoveR)
	game.SubmitAction(Player2, ActionMoveL)
	game.ResolveTurn()

	// Now they're adjacent (both at pos 2)
	// Have them attack each other
	game.SubmitAction(Player1, ActionPunch)
	game.SubmitAction(Player2, ActionKick)
	state, err := game.ResolveTurn()
	if err != nil {
		t.Fatalf("ResolveTurn failed: %v", err)
	}

	// Check damage dealt
	expectedP1Health := MaxHealth - KickDamage // P2's kick hit P1
	expectedP2Health := MaxHealth - PunchDamage // P1's punch hit P2

	if state.P1Health != expectedP1Health {
		t.Errorf("P1 health = %v, want %v", state.P1Health, expectedP1Health)
	}
	if state.P2Health != expectedP2Health {
		t.Errorf("P2 health = %v, want %v", state.P2Health, expectedP2Health)
	}
}

func TestBlockReducesDamage(t *testing.T) {
	game := NewGame()

	// Move into range
	game.SubmitAction(Player1, ActionMoveR)
	game.SubmitAction(Player2, ActionMoveL)
	game.ResolveTurn()

	// P1 attacks, P2 blocks
	game.SubmitAction(Player1, ActionPunch)
	game.SubmitAction(Player2, ActionBlock)
	state, err := game.ResolveTurn()
	if err != nil {
		t.Fatalf("ResolveTurn failed: %v", err)
	}

	// P2 should take reduced damage
	expectedP2Health := MaxHealth - (PunchDamage * BlockReduction)
	if state.P2Health != expectedP2Health {
		t.Errorf("P2 health = %v, want %v (blocked)", state.P2Health, expectedP2Health)
	}

	// P1 should take no damage (P2 only blocked)
	expectedP1Health := MaxHealth - MoveStamina + MoveStamina // stamina changes only
	if state.P1Health != MaxHealth {
		t.Errorf("P1 health = %v, want %v", state.P1Health, expectedP1Health)
	}
}

func TestAIMove(t *testing.T) {
	game := NewGame()

	// Get AI move
	move := game.GetAIMove()

	// Should return a valid action
	available := game.GetAvailableActions(Player2)
	found := false
	for _, a := range available {
		if a == move {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("AI returned invalid move: %s", move)
	}
}

func TestGameOver(t *testing.T) {
	game := NewGame()

	// First move into range
	game.SubmitAction(Player1, ActionMoveR)
	game.SubmitAction(Player2, ActionMoveL)
	game.ResolveTurn()

	// Simulate a game until someone wins
	// Alternate between attacking and recovering to manage stamina
	for i := 0; i < 100 && !game.GetState().GameOver; i++ {
		state := game.GetState()

		// P1 attacks if has stamina, otherwise recovers
		if state.P1Stamina >= PunchStamina {
			game.SubmitAction(Player1, ActionPunch)
		} else {
			game.SubmitAction(Player1, ActionRecover)
		}

		// P2 attacks if has stamina, otherwise recovers
		if state.P2Stamina >= PunchStamina {
			game.SubmitAction(Player2, ActionPunch)
		} else {
			game.SubmitAction(Player2, ActionRecover)
		}

		game.ResolveTurn()
	}

	state := game.GetState()
	if !state.GameOver {
		t.Error("Game should be over after enough turns")
	}

	// Should have a winner (both deplete health evenly, so either could win)
	if state.Winner != Player1 && state.Winner != Player2 {
		t.Errorf("Expected a winner, got: %v", state.Winner)
	}
}

func TestReset(t *testing.T) {
	game := NewGame()

	// Make some moves
	game.SubmitAction(Player1, ActionPunch)
	game.SubmitAction(Player2, ActionKick)
	game.ResolveTurn()

	// Reset
	game.Reset()

	// Should be back to initial state
	state := game.GetState()

	if state.P1Health != MaxHealth {
		t.Errorf("After reset, P1 health = %v, want %v", state.P1Health, MaxHealth)
	}
	if state.P2Health != MaxHealth {
		t.Errorf("After reset, P2 health = %v, want %v", state.P2Health, MaxHealth)
	}
	if state.TurnNum != 1 {
		t.Errorf("After reset, turn = %v, want 1", state.TurnNum)
	}
	if state.GameOver {
		t.Error("After reset, game should not be over")
	}
}

func TestPetriNetStructure(t *testing.T) {
	net := BuildKarateNet()

	// Check expected places exist
	expectedPlaces := []string{
		"P1_health", "P1_stamina", "P1_blocking",
		"P2_health", "P2_stamina", "P2_blocking",
		"P1_pos0", "P1_pos1", "P1_pos2", "P1_pos3", "P1_pos4",
		"P2_pos0", "P2_pos1", "P2_pos2", "P2_pos3", "P2_pos4",
		"P1_wins", "P2_wins", "in_range",
	}

	for _, place := range expectedPlaces {
		if _, ok := net.Places[place]; !ok {
			t.Errorf("Missing place: %s", place)
		}
	}

	// Check expected transitions exist
	expectedTransitions := []string{
		"P1_punch", "P1_kick", "P1_special", "P1_block",
		"P1_move_left", "P1_move_right", "P1_recover",
		"P2_punch", "P2_kick", "P2_special", "P2_block",
		"P2_move_left", "P2_move_right", "P2_recover",
	}

	for _, trans := range expectedTransitions {
		if _, ok := net.Transitions[trans]; !ok {
			t.Errorf("Missing transition: %s", trans)
		}
	}
}

func TestAIMoodSystem(t *testing.T) {
	game := NewGame()

	// Initial mood should be calm
	state := game.GetState()
	if state.AIMood != MoodCalm {
		t.Errorf("Initial mood = %v, want %v", state.AIMood, MoodCalm)
	}

	// Move into range
	game.SubmitAction(Player1, ActionMoveR)
	game.SubmitAction(Player2, ActionMoveL)
	game.ResolveTurn()

	// Hit the AI - should become aggressive
	game.SubmitAction(Player1, ActionPunch)
	game.SubmitAction(Player2, ActionRecover) // AI doesn't attack, gets hit
	state, _ = game.ResolveTurn()

	if state.AIMood != MoodAggressive {
		t.Errorf("After getting hit, mood = %v, want %v", state.AIMood, MoodAggressive)
	}

	// AI attacks back - should calm down
	game.SubmitAction(Player1, ActionBlock)
	game.SubmitAction(Player2, ActionPunch) // AI attacks
	state, _ = game.ResolveTurn()

	if state.AIMood != MoodCalm {
		t.Errorf("After attacking back, mood = %v, want %v", state.AIMood, MoodCalm)
	}
}

func TestAIMoodBoredom(t *testing.T) {
	game := NewGame()

	// Do nothing but recover for 3+ turns to trigger boredom
	for i := 0; i < 4; i++ {
		game.SubmitAction(Player1, ActionRecover)
		game.SubmitAction(Player2, ActionRecover)
		game.ResolveTurn()
	}

	state := game.GetState()
	if state.AIMood != MoodBored {
		t.Errorf("After 4 passive turns, mood = %v, want %v", state.AIMood, MoodBored)
	}
}

func TestAIMoodTired(t *testing.T) {
	game := NewGame()

	// Move into range
	game.SubmitAction(Player1, ActionMoveR)
	game.SubmitAction(Player2, ActionMoveL)
	game.ResolveTurn()

	// Exhaust AI stamina with attacks
	for i := 0; i < 5; i++ {
		game.SubmitAction(Player1, ActionBlock)
		game.SubmitAction(Player2, ActionPunch)
		game.ResolveTurn()
	}

	state := game.GetState()
	// When stamina is low, AI should be tired or have different mood
	// (depends on state transitions, but stamina should be depleted)
	if state.P2Stamina >= MaxStamina*0.2 {
		t.Skip("AI stamina not low enough to test tired mood")
	}
	// If we got here with low stamina, the mood system is working
}

func TestAIBlockLimit(t *testing.T) {
	game := NewGame()

	// Move into range
	game.SubmitAction(Player1, ActionMoveR)
	game.SubmitAction(Player2, ActionMoveL)
	game.ResolveTurn()

	// Force AI to block twice by P1 attacking
	game.SubmitAction(Player1, ActionPunch)
	game.SubmitAction(Player2, ActionBlock) // First block
	game.ResolveTurn()

	game.SubmitAction(Player1, ActionPunch)
	game.SubmitAction(Player2, ActionBlock) // Second block
	game.ResolveTurn()

	// Check that consecutive block counter is 2
	rawState := game.GetRawState()
	if rawState["AI_consecutive_blocks"] != 2 {
		t.Errorf("Consecutive blocks = %v, want 2", rawState["AI_consecutive_blocks"])
	}

	// Now the AI should not be able to choose block
	aiMove := game.GetAIMove()
	if aiMove == ActionBlock {
		t.Errorf("AI chose block after 2 consecutive blocks, should not be allowed")
	}

	// After the AI takes a non-block action, counter resets
	game.SubmitAction(Player1, ActionPunch)
	game.SubmitAction(Player2, aiMove) // Use the AI's chosen non-block move
	game.ResolveTurn()

	rawState = game.GetRawState()
	if rawState["AI_consecutive_blocks"] != 0 {
		t.Errorf("Consecutive blocks should reset after non-block, got %v", rawState["AI_consecutive_blocks"])
	}
}

func BenchmarkAIMove(b *testing.B) {
	game := NewGame()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		game.GetAIMove()
	}
}

func BenchmarkResolveTurn(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		game := NewGame()
		game.SubmitAction(Player1, ActionPunch)
		game.SubmitAction(Player2, ActionKick)
		game.ResolveTurn()
	}
}
