package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// EarlyTerminationDemo demonstrates the speedup from early termination
func main() {
	fmt.Println("=== Early Termination Demo ===\n")

	// Load 9x9 Sudoku model
	modelPath := "sudoku-9x9-ode.jsonld"
	net, initialState, rates := loadModel(modelPath)

	fmt.Printf("Loaded model: %d places, %d transitions\n", len(net.Places), len(net.Transitions))

	// Find possible moves
	moves := findPossibleMoves(initialState, 9)
	fmt.Printf("Found %d possible moves\n\n", len(moves))

	// Test different threshold values
	thresholds := []float64{15.0, 18.0, 20.0, 22.0, 25.0}

	for _, threshold := range thresholds {
		fmt.Printf("Testing threshold: %.1f\n", threshold)

		movesEvaluated := 0
		bestScore := -1000.0

		for i, move := range moves {
			// Evaluate this move
			prob := solver.NewProblem(net, move.state, [2]float64{0, 1.0}, rates)
			opts := solver.DefaultOptions()
			opts.Abstol = 1e-2
			opts.Reltol = 1e-2
			opts.Dt = 0.5
			sol := solver.Solve(prob, solver.Tsit5(), opts)

			score := sol.GetFinalState()["solved"]
			movesEvaluated++

			if score > bestScore {
				bestScore = score
			}

			// Early termination check
			if score >= threshold {
				fmt.Printf("  Found good move after %d/%d evaluations (%.1f%% searched)\n",
					movesEvaluated, len(moves), float64(movesEvaluated)/float64(len(moves))*100)
				fmt.Printf("  Score: %.2f (threshold: %.1f)\n", score, threshold)
				fmt.Printf("  Speedup: %.2fx\n\n", float64(len(moves))/float64(movesEvaluated))
				break
			}

			// No good move found
			if i == len(moves)-1 {
				fmt.Printf("  No move exceeded threshold\n")
				fmt.Printf("  Best score found: %.2f (threshold: %.1f)\n", bestScore, threshold)
				fmt.Printf("  Evaluated all %d moves\n\n", movesEvaluated)
			}
		}
	}

	// Demonstrate adaptive early termination
	fmt.Println("=== Adaptive Early Termination ===")
	fmt.Println("Strategy: Accept first move better than current best + margin\n")

	movesEvaluated := adaptiveEarlyTermination(net, initialState, rates, moves)
	fmt.Printf("Evaluated %d/%d moves (%.1f%% searched)\n",
		movesEvaluated, len(moves), float64(movesEvaluated)/float64(len(moves))*100)
	fmt.Printf("Speedup: %.2fx\n", float64(len(moves))/float64(movesEvaluated))
}

// Move represents a possible digit placement
type Move struct {
	row   int
	col   int
	digit int
	state map[string]float64
}

func loadModel(path string) (*petri.PetriNet, map[string]float64, map[string]float64) {
	jsonData, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		panic(err)
	}

	var modelData struct {
		Puzzle struct {
			InitialState [][]int `json:"initial_state"`
		} `json:"puzzle"`
	}
	json.Unmarshal(jsonData, &modelData)

	state := createInitialState(net, modelData.Puzzle.InitialState)

	rates := make(map[string]float64)
	for label := range net.Transitions {
		rates[label] = 1.0
	}

	return net, state, rates
}

func createInitialState(net *petri.PetriNet, puzzle [][]int) map[string]float64 {
	state := make(map[string]float64)

	for label := range net.Places {
		state[label] = 0
	}

	size := len(puzzle)
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			cellPlace := fmt.Sprintf("P%d%d", i, j)
			if puzzle[i][j] == 0 {
				if _, exists := state[cellPlace]; exists {
					state[cellPlace] = 1
				}
			} else {
				digit := puzzle[i][j]
				histPlace := fmt.Sprintf("_D%d_%d%d", digit, i, j)
				if _, exists := state[histPlace]; exists {
					state[histPlace] = 1
				}
			}
		}
	}

	return state
}

func findPossibleMoves(state map[string]float64, size int) []Move {
	moves := make([]Move, 0)

	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			cellPlace := fmt.Sprintf("P%d%d", i, j)
			if state[cellPlace] > 0.5 {
				for digit := 1; digit <= size; digit++ {
					newState := make(map[string]float64)
					for k, v := range state {
						newState[k] = v
					}
					newState[cellPlace] = 0
					histPlace := fmt.Sprintf("_D%d_%d%d", digit, i, j)
					newState[histPlace] = 1

					moves = append(moves, Move{
						row:   i,
						col:   j,
						digit: digit,
						state: newState,
					})
				}
			}
		}
	}

	return moves
}

func adaptiveEarlyTermination(net *petri.PetriNet, initialState map[string]float64,
	rates map[string]float64, moves []Move) int {

	bestScore := -1000.0
	movesEvaluated := 0
	improvementMargin := 2.0 // Accept move if it's 2.0 better than current best

	for _, move := range moves {
		prob := solver.NewProblem(net, move.state, [2]float64{0, 1.0}, rates)
		opts := solver.DefaultOptions()
		opts.Abstol = 1e-2
		opts.Reltol = 1e-2
		opts.Dt = 0.5
		sol := solver.Solve(prob, solver.Tsit5(), opts)

		score := sol.GetFinalState()["solved"]
		movesEvaluated++

		if score > bestScore {
			improvement := score - bestScore
			bestScore = score

			fmt.Printf("  Move %d: score=%.2f (improvement: +%.2f)\n",
				movesEvaluated, score, improvement)

			// Early termination: accept if significant improvement
			if movesEvaluated > 3 && improvement > improvementMargin {
				fmt.Printf("  → Accepting move (good improvement)\n")
				return movesEvaluated
			}
		}

		// Also check if we've found a near-optimal move
		if score >= 22.0 { // High score for typical puzzle state
			fmt.Printf("  → Accepting move (high absolute score: %.2f)\n", score)
			return movesEvaluated
		}

		// Safety: don't evaluate more than 20% of moves
		if movesEvaluated >= len(moves)/5 {
			fmt.Printf("  → Stopping after %d moves (20%% limit)\n", movesEvaluated)
			return movesEvaluated
		}
	}

	return movesEvaluated
}
