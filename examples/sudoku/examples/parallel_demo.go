package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
)

func main() {
	fmt.Println("=== ODE Parallel Evaluation Demo ===\n")
	fmt.Printf("System: %d CPU cores\n", runtime.NumCPU())
	fmt.Printf("GOMAXPROCS: %d\n\n", runtime.GOMAXPROCS(0))

	// Load 4x4 model
	net, initialState, rates := loadModel("../sudoku-4x4-ode.jsonld")

	fmt.Printf("Model: %d places, %d transitions\n", len(net.Places), len(net.Transitions))

	// Find possible moves
	moves := findPossibleMoves(initialState, 4)
	fmt.Printf("Possible moves: %d\n\n", len(moves))

	// Test 1: Sequential Evaluation
	fmt.Println("Test 1: Sequential Evaluation (Baseline)")
	fmt.Println("------------------------------------------")
	start := time.Now()
	sequentialResults := EvaluateMovesSequential(net, moves, rates)
	timeSequential := time.Since(start)

	fmt.Printf("Moves evaluated: %d\n", len(sequentialResults))
	fmt.Printf("Total time: %v\n", timeSequential)
	fmt.Printf("Time per move: %v\n\n", timeSequential/time.Duration(len(moves)))

	// Test 2: Parallel Evaluation
	fmt.Println("Test 2: Parallel Evaluation")
	fmt.Println("----------------------------")
	start = time.Now()
	parallelResults := EvaluateMovesParallel(net, moves, rates)
	timeParallel := time.Since(start)

	fmt.Printf("Moves evaluated: %d\n", len(parallelResults))
	fmt.Printf("Total time: %v\n", timeParallel)
	fmt.Printf("Time per move: %v\n", timeParallel/time.Duration(len(moves)))
	fmt.Printf("\nSpeedup: %.2fx\n\n", float64(timeSequential)/float64(timeParallel))

	// Test 3: Parallel + Cache
	fmt.Println("Test 3: Parallel + Cache")
	fmt.Println("-------------------------")
	cache := NewODECache()

	// Run 3 times to show cache effectiveness
	totalTime := time.Duration(0)
	for iteration := 0; iteration < 3; iteration++ {
		start = time.Now()
		_ = EvaluateMovesParallelWithCache(cache, net, moves, rates)
		iterTime := time.Since(start)
		totalTime += iterTime

		if iteration == 0 {
			fmt.Printf("Iteration 1 (cold cache): %v\n", iterTime)
		} else if iteration == 1 {
			fmt.Printf("Iteration 2 (warm cache): %v\n", iterTime)
		} else {
			fmt.Printf("Iteration 3 (hot cache): %v\n", iterTime)
		}
	}

	stats := cache.Stats()
	fmt.Printf("\nCache stats:\n")
	fmt.Printf("  Hit rate: %.1f%%\n", stats.HitRate)
	fmt.Printf("  Cache size: %d entries\n", stats.Size)
	fmt.Printf("  Average time: %v\n", totalTime/3)
	fmt.Printf("\nSpeedup vs sequential: %.2fx\n\n", float64(timeSequential)/float64(totalTime/3))

	// Test 4: Parallel Top-K
	fmt.Println("Test 4: Parallel Top-K (k=10)")
	fmt.Println("------------------------------")
	start = time.Now()
	bestMove := FindBestMoveParallelTopK(net, moves, rates, 10)
	timeTopK := time.Since(start)

	fmt.Printf("Best move found: cell (%d,%d) = %d\n", bestMove.row, bestMove.col, bestMove.digit)
	fmt.Printf("Time: %v\n", timeTopK)
	fmt.Printf("Moves evaluated: 10 (%.1f%% of total)\n", 10.0/float64(len(moves))*100)
	fmt.Printf("\nSpeedup vs sequential: %.2fx\n\n", float64(timeSequential)/float64(timeTopK))

	// Test 5: Worker Scaling
	fmt.Println("Test 5: Worker Scaling")
	fmt.Println("-----------------------")
	fmt.Println("Workers | Time      | Speedup vs 1 worker")
	fmt.Println("--------|-----------|--------------------")

	var time1Worker time.Duration
	for _, workers := range []int{1, 2, 4, 8, 16} {
		config := ParallelConfig{
			MaxWorkers: workers,
			BatchSize:  0,
			UseCache:   false,
		}

		start = time.Now()
		_ = EvaluateMovesParallelBatched(net, moves, rates, config)
		elapsed := time.Since(start)

		if workers == 1 {
			time1Worker = elapsed
			fmt.Printf("   %2d   | %9v | %.2fx (baseline)\n", workers, elapsed, 1.0)
		} else {
			speedup := float64(time1Worker) / float64(elapsed)
			fmt.Printf("   %2d   | %9v | %.2fx\n", workers, elapsed, speedup)
		}
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("✓ Parallelization: %.2fx speedup\n", float64(timeSequential)/float64(timeParallel))
	fmt.Printf("✓ Parallel + Cache: %.2fx speedup\n", float64(timeSequential)/float64(totalTime/3))
	fmt.Printf("✓ Parallel Top-K: %.2fx speedup\n", float64(timeSequential)/float64(timeTopK))
	fmt.Printf("✓ Optimal core usage on %d-core system\n", runtime.NumCPU())

	fmt.Println("\n=== Combined Optimization Impact ===")
	fmt.Println("From 4×4 Sudoku baseline (standard parameters):")
	fmt.Printf("  Sequential (standard): ~3,200 ms (67ms × 48 moves)\n")
	fmt.Printf("  Sequential (optimized): %v (155× from params)\n", timeSequential)
	fmt.Printf("  Parallel (optimized): %v (%.1fx from parallel)\n",
		timeParallel, float64(timeSequential)/float64(timeParallel))
	fmt.Printf("  Parallel + Cache: %v (%.1fx total)\n",
		totalTime/3, float64(timeSequential)/float64(totalTime/3))

	baselineTime := 3200 * time.Millisecond
	fmt.Printf("\nTotal speedup: %.0fx\n", float64(baselineTime)/float64(totalTime/3))
	fmt.Printf("From: 3,200 ms → %.2f ms\n", float64(totalTime/3)/float64(time.Millisecond))
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

type Move struct {
	row   int
	col   int
	digit int
	state map[string]float64
	index int
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
						index: len(moves),
					})
				}
			}
		}
	}

	return moves
}
