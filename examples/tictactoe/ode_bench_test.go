package tictactoe

import (
	"fmt"
	"os"
	"testing"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// BenchmarkTicTacToeODESingleEvaluation benchmarks a single ODE evaluation
func BenchmarkTicTacToeODESingleEvaluation(b *testing.B) {
	// Load the Petri net model
	modelPath := "../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld"
	jsonData, err := os.ReadFile(modelPath)
	if err != nil {
		b.Fatalf("Error reading model: %v", err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		b.Fatalf("Error parsing model: %v", err)
	}

	// Create a typical mid-game state
	state := createMidGameState(net)
	rates := createRates(net, 1.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
		opts := solver.DefaultOptions()
		opts.Abstol = 1e-4
		opts.Reltol = 1e-3
		opts.Dt = 0.2
		_ = solver.Solve(prob, solver.Tsit5(), opts)
	}
}

// BenchmarkTicTacToeODEMoveEvaluation benchmarks evaluating all 9 possible first moves
func BenchmarkTicTacToeODEMoveEvaluation(b *testing.B) {
	modelPath := "../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld"
	jsonData, err := os.ReadFile(modelPath)
	if err != nil {
		b.Fatalf("Error reading model: %v", err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		b.Fatalf("Error parsing model: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Evaluate all 9 opening moves
		state := createInitialState(net)
		rates := createRates(net, 1.0)

		for row := 0; row < 3; row++ {
			for col := 0; col < 3; col++ {
				// Create hypothetical state for this move
				hypState := make(map[string]float64)
				for k, v := range state {
					hypState[k] = v
				}

				position := fmt.Sprintf("P%d%d", row, col)
				hypState[position] = 0
				hypState[fmt.Sprintf("_X%d%d", row, col)] = 1
				hypState["Next"] = 1

				// Run ODE evaluation
				prob := solver.NewProblem(net, hypState, [2]float64{0, 3.0}, rates)
				opts := solver.DefaultOptions()
				opts.Abstol = 1e-4
				opts.Reltol = 1e-3
				opts.Dt = 0.2
				_ = solver.Solve(prob, solver.Tsit5(), opts)
			}
		}
	}
}

// Helper function to create initial state
func createInitialState(net *petri.PetriNet) map[string]float64 {
	state := make(map[string]float64)

	// Initialize all places to 0
	for label := range net.Places {
		state[label] = 0
	}

	// Set up initial board state - all positions available
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			state[fmt.Sprintf("P%d%d", i, j)] = 1
		}
	}

	// X goes first
	state["Next"] = 0

	return state
}

// Helper function to create a mid-game state
func createMidGameState(net *petri.PetriNet) map[string]float64 {
	state := make(map[string]float64)

	// Initialize all places to 0
	for label := range net.Places {
		state[label] = 0
	}

	// Set up a mid-game state (X has played center, O has played corner)
	// P11 is empty (center taken)
	state["P00"] = 1 // Empty
	state["P01"] = 1 // Empty
	state["P02"] = 0 // O played here
	state["P10"] = 1 // Empty
	state["P11"] = 0 // X played here (center)
	state["P12"] = 1 // Empty
	state["P20"] = 1 // Empty
	state["P21"] = 1 // Empty
	state["P22"] = 1 // Empty

	// Set history
	state["_X11"] = 1 // X played center
	state["_O02"] = 1 // O played top-right

	// Next turn marker
	state["Next"] = 1

	return state
}

// Helper function to create rates map
func createRates(net *petri.PetriNet, rate float64) map[string]float64 {
	rates := make(map[string]float64)
	for label := range net.Transitions {
		rates[label] = rate
	}
	return rates
}
