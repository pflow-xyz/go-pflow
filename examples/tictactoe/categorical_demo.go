package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/examples/tictactoe/categorical"
	"github.com/pflow-xyz/go-pflow/parser"
)

func main() {
	fmt.Println("=== Categorical Game Theory Demo ===")
	fmt.Println("Tic-Tac-Toe as an Open Game Lens")
	fmt.Println()

	// Load Petri net model
	net, rates := loadModel()

	// Create ODE lens
	lens := categorical.ODELens(net, rates)

	fmt.Println("Part 1: Convergent Dynamics")
	fmt.Println("=" + repeat("=", 50))
	demonstrateConvergence(lens)

	fmt.Println("\nPart 2: Oscillating Dynamics")
	fmt.Println("=" + repeat("=", 50))
	demonstrateOscillation(lens)

	fmt.Println("\nPart 3: Lens Composition")
	fmt.Println("=" + repeat("=", 50))
	demonstrateComposition(lens)

	fmt.Println("\nPart 4: Fixed Points")
	fmt.Println("=" + repeat("=", 50))
	demonstrateFixedPoint(lens)
}

func demonstrateConvergence(lens categorical.Lens) {
	fmt.Println("Starting from empty board...")
	fmt.Println("Expecting: Convergence to draw equilibrium")
	fmt.Println()

	// Empty board
	state := categorical.GameState{
		Board: [9]int{0, 0, 0, 0, 0, 0, 0, 0, 0},
		Turn:  1,
	}

	// Analyze dynamics
	analysis := categorical.AnalyzeDynamics(lens, state, 50)

	// Print results
	fmt.Println(analysis.Report())
	fmt.Println()

	// Show trajectory
	if analysis.Type == categorical.Converged {
		fmt.Println("✓ System converged to equilibrium")
		fmt.Printf("  Final state: X wins %.1f%%, O wins %.1f%%, Draw %.1f%%\n",
			analysis.FinalUtility.WinProbX*100,
			analysis.FinalUtility.WinProbO*100,
			analysis.FinalUtility.DrawProb*100)
	}

	fmt.Println("\n" + analysis.PlotTrajectory())
}

func demonstrateOscillation(lens categorical.Lens) {
	fmt.Println("Starting from symmetric position...")
	fmt.Println("X in center, O in corner - 4 equivalent responses")
	fmt.Println()

	// Symmetric position
	state := categorical.GameState{
		Board: [9]int{
			2, 0, 0,
			0, 1, 0,
			0, 0, 0,
		},
		Turn: 1, // X to move
	}

	printBoard(state)
	fmt.Println()

	// Analyze dynamics
	analysis := categorical.AnalyzeDynamics(lens, state, 50)

	// Print results
	fmt.Println(analysis.Report())
	fmt.Println()

	// Show move sequence
	if analysis.Type == categorical.Oscillating {
		fmt.Println("✓ System oscillating in limit cycle")
		fmt.Printf("  Period: %d moves\n", analysis.CyclePeriod)
		fmt.Println("  Move sequence (last cycle):")

		start := len(analysis.MoveHistory) - analysis.CyclePeriod
		if start < 0 {
			start = 0
		}

		for i := start; i < len(analysis.MoveHistory); i++ {
			move := analysis.MoveHistory[i]
			fmt.Printf("    %d. Player %s → Position %d (%s)\n",
				i-start+1,
				playerName(move.Player),
				move.Position,
				positionName(move.Position))
		}
	}

	fmt.Println("\n" + analysis.PlotTrajectory())
}

func demonstrateComposition(lens categorical.Lens) {
	fmt.Println("Composing X strategy with O strategy...")
	fmt.Println()

	// Compose lens with itself (full game loop)
	composedLens := lens.Compose(lens)

	// Start from empty board
	state := categorical.GameState{
		Board: [9]int{0, 0, 0, 0, 0, 0, 0, 0, 0},
		Turn:  1,
	}

	fmt.Println("Playing through composed lens:")
	fmt.Println("(Each step plays both X and O)")
	fmt.Println()

	for i := 0; i < 5; i++ {
		fmt.Printf("Round %d:\n", i+1)

		// Forward through composed lens
		move := composedLens.Play(state)

		fmt.Printf("  Composed move: Player %s → Position %d\n",
			playerName(move.Player), move.Position)

		// Apply move
		state = categorical.ApplyMove(state, move)

		// Check if game over
		if categorical.IsTerminal(state) {
			fmt.Println("  Game ended!")
			break
		}

		fmt.Println()
	}
}

func demonstrateFixedPoint(lens categorical.Lens) {
	fmt.Println("Searching for lens fixed points...")
	fmt.Println("(States where play(learn(play(s))) = play(s))")
	fmt.Println()

	// Try several initial states
	testStates := []categorical.GameState{
		// Empty board
		{Board: [9]int{0, 0, 0, 0, 0, 0, 0, 0, 0}, Turn: 1},
		// X in center
		{Board: [9]int{0, 0, 0, 0, 1, 0, 0, 0, 0}, Turn: 2},
		// X and O alternating
		{Board: [9]int{1, 0, 0, 0, 2, 0, 0, 0, 0}, Turn: 1},
	}

	for i, state := range testStates {
		fmt.Printf("Test state %d:\n", i+1)
		printBoard(state)

		// Apply lens composition
		move1 := lens.Play(state)
		state2 := categorical.ApplyMove(state, move1)

		gradient := categorical.ComputeGradient(lens.Learn(state2, categorical.Gradient{
			dUtility: make(map[int]float64),
		}))

		utility := lens.Learn(state, gradient)
		move2 := lens.Play(state)

		if move1.Position == move2.Position {
			fmt.Println("  ✓ Fixed point! play(learn(play(s))) = play(s)")
			fmt.Printf("  Stable move: %d (%s)\n",
				move1.Position, positionName(move1.Position))
			fmt.Printf("  Utility: X=%.2f, O=%.2f, Draw=%.2f\n",
				utility.WinProbX, utility.WinProbO, utility.DrawProb)
		} else {
			fmt.Println("  ✗ Not a fixed point")
			fmt.Printf("  First move: %d, Second move: %d\n",
				move1.Position, move2.Position)
		}

		fmt.Println()
	}
}

// Helper functions

func loadModel() (*parser.PetriNet, map[string]float64) {
	// Try to load the tictactoe model
	jsonData, err := os.ReadFile("tictactoe-ode.jsonld")
	if err != nil {
		// If file doesn't exist, create a simple model
		fmt.Println("Note: Using simplified model (tictactoe-ode.jsonld not found)")
		return createSimpleModel()
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		panic(err)
	}

	// Create rates
	rates := make(map[string]float64)
	for label := range net.Transitions {
		rates[label] = 1.0
	}

	return net, rates
}

func createSimpleModel() (*parser.PetriNet, map[string]float64) {
	// Create a minimal Petri net for demonstration
	modelJSON := `{
		"@context": "https://pflow.dev/context.jsonld",
		"@type": "PetriNet",
		"places": {
			"x_wins": {"initial": 0},
			"o_wins": {"initial": 0},
			"draw": {"initial": 0},
			"x_turn": {"initial": 1},
			"o_turn": {"initial": 0}
		},
		"transitions": {
			"x_move": {
				"inputs": {"x_turn": 1},
				"outputs": {"o_turn": 1}
			},
			"o_move": {
				"inputs": {"o_turn": 1},
				"outputs": {"x_turn": 1}
			},
			"x_wins_trans": {
				"inputs": {"x_turn": 1},
				"outputs": {"x_wins": 1}
			},
			"o_wins_trans": {
				"inputs": {"o_turn": 1},
				"outputs": {"o_wins": 1}
			}
		}
	}`

	net, _ := parser.FromJSON([]byte(modelJSON))

	rates := map[string]float64{
		"x_move":        1.0,
		"o_move":        1.0,
		"x_wins_trans":  0.1,
		"o_wins_trans":  0.1,
	}

	return net, rates
}

func printBoard(state categorical.GameState) {
	fmt.Println("  Board:")
	for i := 0; i < 3; i++ {
		fmt.Print("    ")
		for j := 0; j < 3; j++ {
			idx := i*3 + j
			cell := state.Board[idx]
			if cell == 0 {
				fmt.Printf(" %d ", idx)
			} else if cell == 1 {
				fmt.Print(" X ")
			} else {
				fmt.Print(" O ")
			}
			if j < 2 {
				fmt.Print("|")
			}
		}
		fmt.Println()
		if i < 2 {
			fmt.Println("    -----------")
		}
	}
	fmt.Printf("  Turn: %s\n", playerName(state.Turn))
}

func playerName(player int) string {
	if player == 1 {
		return "X"
	}
	return "O"
}

func positionName(pos int) string {
	names := []string{
		"TL", "T", "TR",
		"L", "C", "R",
		"BL", "B", "BR",
	}
	if pos >= 0 && pos < 9 {
		return names[pos]
	}
	return "?"
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
