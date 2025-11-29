package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// SudokuAI demonstrates practical AI with early termination strategies
func main() {
	fmt.Println("=== Sudoku AI with Early Termination ===\n")

	// Load model
	modelPath := "sudoku-4x4-ode.jsonld" // Use 4x4 for faster demo
	net, initialState, rates := loadModel(modelPath)

	fmt.Printf("Model: %d places, %d transitions\n", len(net.Places), len(net.Transitions))

	// Test different AI strategies
	strategies := []struct {
		name string
		fn   func(*petri.PetriNet, map[string]float64, map[string]float64, []Move) (Move, Stats)
	}{
		{"Exhaustive (no early termination)", findBestMoveExhaustive},
		{"Fixed Threshold (score ≥ 8)", findBestMoveFixedThreshold},
		{"Top-K (evaluate best 5)", findBestMoveTopK},
		{"Adaptive (20% sample limit)", findBestMoveAdaptive},
		{"Random (baseline)", findBestMoveRandom},
	}

	// Run multiple trials
	trials := 5
	for _, strategy := range strategies {
		fmt.Printf("\n%s:\n", strategy.name)
		totalEvaluations := 0
		totalTime := time.Duration(0)

		for trial := 0; trial < trials; trial++ {
			// Reset to initial state
			state := copyState(initialState)
			moves := findPossibleMoves(state, 4)

			start := time.Now()
			_, stats := strategy.fn(net, state, rates, moves)
			elapsed := time.Since(start)

			totalEvaluations += stats.MovesEvaluated
			totalTime += elapsed
		}

		avgEvaluations := float64(totalEvaluations) / float64(trials)
		avgTime := totalTime / time.Duration(trials)
		fmt.Printf("  Avg evaluations: %.1f\n", avgEvaluations)
		fmt.Printf("  Avg time: %v\n", avgTime)

		if avgEvaluations > 0 {
			totalMoves := float64(len(findPossibleMoves(initialState, 4)))
			fmt.Printf("  Speedup: %.2fx (%.1f%% of moves evaluated)\n",
				totalMoves/avgEvaluations, avgEvaluations/totalMoves*100)
		}
	}
}

// Stats tracks evaluation statistics
type Stats struct {
	MovesEvaluated int
	BestScore      float64
}

// Move represents a possible digit placement
type Move struct {
	row   int
	col   int
	digit int
	state map[string]float64
	score float64 // Cached score
}

// Exhaustive evaluation - evaluate all moves
func findBestMoveExhaustive(net *petri.PetriNet, state map[string]float64,
	rates map[string]float64, moves []Move) (Move, Stats) {

	bestMove := moves[0]
	bestScore := -1000.0
	evaluated := 0

	for i := range moves {
		score := evaluateMove(net, moves[i].state, rates)
		moves[i].score = score
		evaluated++

		if score > bestScore {
			bestScore = score
			bestMove = moves[i]
		}
	}

	return bestMove, Stats{MovesEvaluated: evaluated, BestScore: bestScore}
}

// Fixed threshold - stop when finding move above threshold
func findBestMoveFixedThreshold(net *petri.PetriNet, state map[string]float64,
	rates map[string]float64, moves []Move) (Move, Stats) {

	threshold := 8.0 // For 4x4: 12 constraints total, 8 is good
	bestMove := moves[0]
	bestScore := -1000.0
	evaluated := 0

	for i := range moves {
		score := evaluateMove(net, moves[i].state, rates)
		moves[i].score = score
		evaluated++

		if score > bestScore {
			bestScore = score
			bestMove = moves[i]
		}

		// Early termination
		if score >= threshold {
			return bestMove, Stats{MovesEvaluated: evaluated, BestScore: bestScore}
		}
	}

	return bestMove, Stats{MovesEvaluated: evaluated, BestScore: bestScore}
}

// Top-K - only evaluate top K moves (requires ordering heuristic)
func findBestMoveTopK(net *petri.PetriNet, state map[string]float64,
	rates map[string]float64, moves []Move) (Move, Stats) {

	k := 5 // Evaluate only 5 moves
	if k > len(moves) {
		k = len(moves)
	}

	// Shuffle for random sampling (in practice, use heuristic ordering)
	rand.Shuffle(len(moves), func(i, j int) {
		moves[i], moves[j] = moves[j], moves[i]
	})

	bestMove := moves[0]
	bestScore := -1000.0

	for i := 0; i < k; i++ {
		score := evaluateMove(net, moves[i].state, rates)
		moves[i].score = score

		if score > bestScore {
			bestScore = score
			bestMove = moves[i]
		}
	}

	return bestMove, Stats{MovesEvaluated: k, BestScore: bestScore}
}

// Adaptive - evaluate until hitting sample limit or finding good move
func findBestMoveAdaptive(net *petri.PetriNet, state map[string]float64,
	rates map[string]float64, moves []Move) (Move, Stats) {

	maxEvaluations := len(moves) / 5 // Evaluate max 20%
	if maxEvaluations < 3 {
		maxEvaluations = 3
	}

	// Shuffle for random sampling
	rand.Shuffle(len(moves), func(i, j int) {
		moves[i], moves[j] = moves[j], moves[i]
	})

	bestMove := moves[0]
	bestScore := -1000.0
	evaluated := 0
	improvementMargin := 1.5

	for evaluated < maxEvaluations && evaluated < len(moves) {
		score := evaluateMove(net, moves[evaluated].state, rates)
		moves[evaluated].score = score

		improvement := score - bestScore
		if score > bestScore {
			bestScore = score
			bestMove = moves[evaluated]

			// Early termination on significant improvement
			if evaluated > 2 && improvement > improvementMargin {
				evaluated++
				return bestMove, Stats{MovesEvaluated: evaluated, BestScore: bestScore}
			}
		}

		evaluated++
	}

	return bestMove, Stats{MovesEvaluated: evaluated, BestScore: bestScore}
}

// Random - pick random move (no evaluation)
func findBestMoveRandom(net *petri.PetriNet, state map[string]float64,
	rates map[string]float64, moves []Move) (Move, Stats) {

	randomMove := moves[rand.Intn(len(moves))]
	score := evaluateMove(net, randomMove.state, rates)

	return randomMove, Stats{MovesEvaluated: 1, BestScore: score}
}

// Helper functions

func evaluateMove(net *petri.PetriNet, state map[string]float64, rates map[string]float64) float64 {
	prob := solver.NewProblem(net, state, [2]float64{0, 1.0}, rates)
	opts := solver.DefaultOptions()
	opts.Abstol = 1e-2
	opts.Reltol = 1e-2
	opts.Dt = 0.5
	sol := solver.Solve(prob, solver.Tsit5(), opts)
	return sol.GetFinalState()["solved"]
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

func copyState(state map[string]float64) map[string]float64 {
	newState := make(map[string]float64)
	for k, v := range state {
		newState[k] = v
	}
	return newState
}
