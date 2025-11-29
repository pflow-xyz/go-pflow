package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// BenchmarkSudoku4x4ODESingleEvaluation benchmarks a single ODE evaluation for 4x4 Sudoku
func BenchmarkSudoku4x4ODESingleEvaluation(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, state, rates := loadSudokuModel(b, modelPath)

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

// BenchmarkSudoku9x9ODESingleEvaluation benchmarks a single ODE evaluation for 9x9 Sudoku
func BenchmarkSudoku9x9ODESingleEvaluation(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, state, rates := loadSudokuModel(b, modelPath)

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

// BenchmarkSudoku4x4ODEWithTighterTolerance benchmarks 4x4 with default (tighter) tolerances
func BenchmarkSudoku4x4ODEWithTighterTolerance(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, state, rates := loadSudokuModel(b, modelPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
		opts := solver.DefaultOptions()
		// Use tighter tolerances (default-like)
		opts.Abstol = 1e-6
		opts.Reltol = 1e-6
		opts.Dt = 0.01
		_ = solver.Solve(prob, solver.Tsit5(), opts)
	}
}

// BenchmarkSudoku9x9ODEWithTighterTolerance benchmarks 9x9 with default (tighter) tolerances
func BenchmarkSudoku9x9ODEWithTighterTolerance(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, state, rates := loadSudokuModel(b, modelPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
		opts := solver.DefaultOptions()
		// Use tighter tolerances (default-like)
		opts.Abstol = 1e-6
		opts.Reltol = 1e-6
		opts.Dt = 0.01
		_ = solver.Solve(prob, solver.Tsit5(), opts)
	}
}

// BenchmarkSudoku9x9ODEShortHorizon benchmarks 9x9 with very short time horizon
func BenchmarkSudoku9x9ODEShortHorizon(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, state, rates := loadSudokuModel(b, modelPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prob := solver.NewProblem(net, state, [2]float64{0, 1.0}, rates)
		opts := solver.DefaultOptions()
		opts.Abstol = 1e-2
		opts.Reltol = 1e-2
		opts.Dt = 0.5
		_ = solver.Solve(prob, solver.Tsit5(), opts)
	}
}

// Helper function to load Sudoku model and create initial state
func loadSudokuModel(b *testing.B, modelPath string) (*petri.PetriNet, map[string]float64, map[string]float64) {
	jsonData, err := os.ReadFile(modelPath)
	if err != nil {
		b.Fatalf("Error reading model %s: %v", modelPath, err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		b.Fatalf("Error parsing model: %v", err)
	}

	// Parse the model metadata to get initial state
	var modelData struct {
		Puzzle struct {
			InitialState [][]int `json:"initial_state"`
		} `json:"puzzle"`
	}
	if err := json.Unmarshal(jsonData, &modelData); err != nil {
		b.Fatalf("Error parsing model metadata: %v", err)
	}

	// Create initial state
	state := createSudokuInitialState(net, modelData.Puzzle.InitialState)

	// Create rates (all transitions have rate 1.0)
	rates := make(map[string]float64)
	for label := range net.Transitions {
		rates[label] = 1.0
	}

	return net, state, rates
}

// Helper to create initial state from puzzle
func createSudokuInitialState(net *petri.PetriNet, puzzle [][]int) map[string]float64 {
	state := make(map[string]float64)

	// Initialize all places
	for label := range net.Places {
		state[label] = 0
	}

	// Set initial state based on puzzle
	size := len(puzzle)
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			cellPlace := formatCellPlace(i, j)
			if puzzle[i][j] == 0 {
				// Empty cell - place token in cell place
				if _, exists := state[cellPlace]; exists {
					state[cellPlace] = 1
				}
			} else {
				// Given digit - mark in history
				digit := puzzle[i][j]
				historyPlace := formatHistoryPlace(digit, i, j)
				if _, exists := state[historyPlace]; exists {
					state[historyPlace] = 1
				}
			}
		}
	}

	return state
}

// Format cell place name (e.g., "P00", "P12")
func formatCellPlace(row, col int) string {
	return "P" + string(rune('0'+row)) + string(rune('0'+col))
}

// Format history place name (e.g., "_D1_00", "_D5_23")
func formatHistoryPlace(digit, row, col int) string {
	return "_D" + string(rune('0'+digit)) + "_" +
		string(rune('0'+row)) + string(rune('0'+col))
}

// BenchmarkSudoku9x9WithEarlyTermination benchmarks move evaluation with early termination
func BenchmarkSudoku9x9WithEarlyTermination(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	// Find all possible moves (empty cells)
	moves := findPossibleMoves(initialState, 9)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Evaluate moves with early termination
		_ = findBestMoveWithEarlyTermination(net, initialState, rates, moves, 20.0)
	}
}

// BenchmarkSudoku9x9WithoutEarlyTermination benchmarks exhaustive move evaluation
func BenchmarkSudoku9x9WithoutEarlyTermination(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	// Find all possible moves (empty cells)
	moves := findPossibleMoves(initialState, 9)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Evaluate ALL moves exhaustively
		_ = findBestMoveExhaustive(net, initialState, rates, moves)
	}
}

// BenchmarkSudoku4x4WithEarlyTermination benchmarks 4x4 with early termination
func BenchmarkSudoku4x4WithEarlyTermination(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = findBestMoveWithEarlyTermination(net, initialState, rates, moves, 8.0)
	}
}

// Helper: Find possible moves (empty cells with candidate digits)
func findPossibleMoves(state map[string]float64, size int) []Move {
	moves := make([]Move, 0)

	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			cellPlace := formatCellPlace(i, j)
			// If cell is empty (has a token)
			if state[cellPlace] > 0.5 {
				// Try each digit
				for digit := 1; digit <= size; digit++ {
					newState := make(map[string]float64)
					for k, v := range state {
						newState[k] = v
					}
					// Place the digit
					newState[cellPlace] = 0
					histPlace := formatHistoryPlace(digit, i, j)
					newState[histPlace] = 1

					moves = append(moves, Move{
						row:    i,
						col:    j,
						digit:  digit,
						state:  newState,
						index:  len(moves),
					})
				}
			}
		}
	}

	return moves
}

// Move represents a possible digit placement
type Move struct {
	row    int
	col    int
	digit  int
	state  map[string]float64
	index  int
}

// findBestMoveWithEarlyTermination stops when a "good enough" move is found
func findBestMoveWithEarlyTermination(net *petri.PetriNet, initialState map[string]float64,
	rates map[string]float64, moves []Move, threshold float64) Move {

	bestMove := moves[0]
	bestScore := -1000.0

	for _, move := range moves {
		// Evaluate this move with optimized parameters
		prob := solver.NewProblem(net, move.state, [2]float64{0, 1.0}, rates)
		opts := solver.DefaultOptions()
		opts.Abstol = 1e-2
		opts.Reltol = 1e-2
		opts.Dt = 0.5
		sol := solver.Solve(prob, solver.Tsit5(), opts)

		score := sol.GetFinalState()["solved"]

		if score > bestScore {
			bestScore = score
			bestMove = move
		}

		// Early termination: if score exceeds threshold, stop searching
		if score >= threshold {
			return move
		}
	}

	return bestMove
}

// findBestMoveExhaustive evaluates ALL moves (no early termination)
func findBestMoveExhaustive(net *petri.PetriNet, initialState map[string]float64,
	rates map[string]float64, moves []Move) Move {

	bestMove := moves[0]
	bestScore := -1000.0

	for _, move := range moves {
		// Evaluate this move with optimized parameters
		prob := solver.NewProblem(net, move.state, [2]float64{0, 1.0}, rates)
		opts := solver.DefaultOptions()
		opts.Abstol = 1e-2
		opts.Reltol = 1e-2
		opts.Dt = 0.5
		sol := solver.Solve(prob, solver.Tsit5(), opts)

		score := sol.GetFinalState()["solved"]

		if score > bestScore {
			bestScore = score
			bestMove = move
		}
		// No early termination - always evaluate all moves
	}

	return bestMove
}

// BenchmarkSudoku4x4WithCache benchmarks with caching
func BenchmarkSudoku4x4WithCache(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	cache := NewODECache()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate playing multiple games with same starting position
		// This creates cache hits
		for game := 0; game < 3; game++ {
			moves := findPossibleMoves(initialState, 4)

			// Evaluate moves with cache
			for j := 0; j < len(moves) && j < 10; j++ {
				_ = EvaluateWithCache(cache, net, moves[j].state, rates)
			}
		}
	}
	b.StopTimer()

	stats := cache.Stats()
	b.ReportMetric(stats.HitRate, "hit_rate_%")
	b.ReportMetric(float64(stats.Size), "cache_entries")
}

// BenchmarkSudoku4x4WithoutCache benchmarks without caching
func BenchmarkSudoku4x4WithoutCache(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Same workload but no cache
		for game := 0; game < 3; game++ {
			moves := findPossibleMoves(initialState, 4)

			for j := 0; j < len(moves) && j < 10; j++ {
				prob := solver.NewProblem(net, moves[j].state, [2]float64{0, 1.0}, rates)
				opts := solver.DefaultOptions()
				opts.Abstol = 1e-2
				opts.Reltol = 1e-2
				opts.Dt = 0.5
				sol := solver.Solve(prob, solver.Tsit5(), opts)
				_ = sol.GetFinalState()["solved"]
			}
		}
	}
}

// BenchmarkSudoku9x9WithCache benchmarks 9x9 with caching
func BenchmarkSudoku9x9WithCache(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	cache := NewODECache()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Multiple games from same position
		for game := 0; game < 3; game++ {
			moves := findPossibleMoves(initialState, 9)

			// Evaluate first 20 moves (with cache hits on repeats)
			for j := 0; j < len(moves) && j < 20; j++ {
				_ = EvaluateWithCache(cache, net, moves[j].state, rates)
			}
		}
	}
	b.StopTimer()

	stats := cache.Stats()
	b.ReportMetric(stats.HitRate, "hit_rate_%")
	b.ReportMetric(float64(stats.Size), "cache_entries")
}

// BenchmarkCacheHashingSpeed tests the hashing performance
func BenchmarkCacheHashingSpeed(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	_, initialState, _ := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 9)
	states := make([]map[string]float64, len(moves))
	for i := range moves {
		states[i] = moves[i].state
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, state := range states {
			_ = HashState(state)
		}
	}
}

// BenchmarkLRUCacheWithEviction benchmarks LRU cache with limited size
func BenchmarkLRUCacheWithEviction(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	cache := NewLRUODECache(50) // Limited to 50 entries

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Generate more than 50 unique states to test eviction
		for iter := 0; iter < 5; iter++ {
			moves := findPossibleMoves(initialState, 4)

			for j := 0; j < len(moves) && j < 15; j++ {
				// Check cache
				if score, hit := cache.Get(moves[j].state); !hit {
					// Evaluate
					prob := solver.NewProblem(net, moves[j].state, [2]float64{0, 1.0}, rates)
					opts := solver.DefaultOptions()
					opts.Abstol = 1e-2
					opts.Reltol = 1e-2
					opts.Dt = 0.5
					sol := solver.Solve(prob, solver.Tsit5(), opts)
					score = sol.GetFinalState()["solved"]

					// Store in cache
					cache.Put(moves[j].state, score, sol.GetFinalState())
				} else {
					_ = score
				}
			}
		}
	}
	b.StopTimer()

	stats := cache.Stats()
	b.ReportMetric(stats.HitRate, "hit_rate_%")
}

// BenchmarkSudoku4x4Parallel benchmarks parallel move evaluation for 4x4
func BenchmarkSudoku4x4Parallel(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateMovesParallel(net, moves, rates)
	}
}

// BenchmarkSudoku4x4Sequential benchmarks sequential move evaluation for comparison
func BenchmarkSudoku4x4Sequential(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateMovesSequential(net, moves, rates)
	}
}

// BenchmarkSudoku9x9Parallel benchmarks parallel move evaluation for 9x9
func BenchmarkSudoku9x9Parallel(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 9)
	// Limit to first 20 moves for reasonable benchmark time
	if len(moves) > 20 {
		moves = moves[:20]
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateMovesParallel(net, moves, rates)
	}
}

// BenchmarkSudoku9x9Sequential benchmarks sequential evaluation for 9x9
func BenchmarkSudoku9x9Sequential(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 9)
	// Limit to first 20 moves for reasonable benchmark time
	if len(moves) > 20 {
		moves = moves[:20]
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateMovesSequential(net, moves, rates)
	}
}

// BenchmarkSudoku4x4ParallelTopK benchmarks parallel Top-K evaluation
func BenchmarkSudoku4x4ParallelTopK(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FindBestMoveParallelTopK(net, moves, rates, 10)
	}
}

// BenchmarkSudoku9x9ParallelTopK benchmarks parallel Top-K for 9x9
func BenchmarkSudoku9x9ParallelTopK(b *testing.B) {
	modelPath := "sudoku-9x9-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 9)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FindBestMoveParallelTopK(net, moves, rates, 20)
	}
}

// BenchmarkSudoku4x4ParallelWithCache benchmarks parallel + cache
func BenchmarkSudoku4x4ParallelWithCache(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	cache := NewODECache()
	moves := findPossibleMoves(initialState, 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EvaluateMovesParallelWithCache(cache, net, moves, rates)
	}
	b.StopTimer()

	stats := cache.Stats()
	b.ReportMetric(stats.HitRate, "hit_rate_%")
}

// BenchmarkParallelScaling tests speedup with different worker counts
func BenchmarkParallelScaling(b *testing.B) {
	modelPath := "sudoku-4x4-ode.jsonld"
	net, initialState, rates := loadSudokuModel(b, modelPath)

	moves := findPossibleMoves(initialState, 4)

	for _, workers := range []int{1, 2, 4, 8, 16} {
		b.Run(b.Name()+"_workers_"+string(rune('0'+workers)), func(b *testing.B) {
			config := ParallelConfig{
				MaxWorkers: workers,
				BatchSize:  0,
				UseCache:   false,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = EvaluateMovesParallelBatched(net, moves, rates, config)
			}
		})
	}
}
