package main

import (
	"runtime"
	"sync"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// MoveResult holds the result of evaluating a single move
type MoveResult struct {
	Move  Move
	Score float64
	Index int
}

// EvaluateMovesParallel evaluates multiple moves in parallel
func EvaluateMovesParallel(net *petri.PetriNet, moves []Move, rates map[string]float64) []MoveResult {
	results := make([]MoveResult, len(moves))
	var wg sync.WaitGroup
	resultChan := make(chan MoveResult, len(moves))

	// Launch goroutines for each move
	for i, move := range moves {
		wg.Add(1)
		go func(idx int, m Move) {
			defer wg.Done()

			// Evaluate with optimized parameters
			prob := solver.NewProblem(net, m.state, [2]float64{0, 1.0}, rates)
			opts := solver.DefaultOptions()
			opts.Abstol = 1e-2
			opts.Reltol = 1e-2
			opts.Dt = 0.5
			sol := solver.Solve(prob, solver.Tsit5(), opts)

			score := sol.GetFinalState()["solved"]

			resultChan <- MoveResult{
				Move:  m,
				Score: score,
				Index: idx,
			}
		}(i, move)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for result := range resultChan {
		results[result.Index] = result
	}

	return results
}

// EvaluateMovesParallelWithCache evaluates moves in parallel with caching
func EvaluateMovesParallelWithCache(cache *ODECache, net *petri.PetriNet, moves []Move, rates map[string]float64) []MoveResult {
	results := make([]MoveResult, len(moves))
	var wg sync.WaitGroup
	resultChan := make(chan MoveResult, len(moves))

	// Launch goroutines for each move
	for i, move := range moves {
		wg.Add(1)
		go func(idx int, m Move) {
			defer wg.Done()

			// Check cache first
			var score float64
			if cached, hit := cache.Get(m.state); hit {
				score = cached
			} else {
				// Cache miss - evaluate with ODE
				prob := solver.NewProblem(net, m.state, [2]float64{0, 1.0}, rates)
				opts := solver.DefaultOptions()
				opts.Abstol = 1e-2
				opts.Reltol = 1e-2
				opts.Dt = 0.5
				sol := solver.Solve(prob, solver.Tsit5(), opts)

				score = sol.GetFinalState()["solved"]
				cache.Put(m.state, score, sol.GetFinalState())
			}

			resultChan <- MoveResult{
				Move:  m,
				Score: score,
				Index: idx,
			}
		}(i, move)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for result := range resultChan {
		results[result.Index] = result
	}

	return results
}

// FindBestMoveParallel finds the best move using parallel evaluation
func FindBestMoveParallel(net *petri.PetriNet, moves []Move, rates map[string]float64) Move {
	if len(moves) == 0 {
		panic("no moves to evaluate")
	}

	results := EvaluateMovesParallel(net, moves, rates)

	bestIdx := 0
	bestScore := results[0].Score

	for i, result := range results {
		if result.Score > bestScore {
			bestScore = result.Score
			bestIdx = i
		}
	}

	return moves[bestIdx]
}

// FindBestMoveParallelTopK evaluates only top K moves in parallel
func FindBestMoveParallelTopK(net *petri.PetriNet, moves []Move, rates map[string]float64, k int) Move {
	if len(moves) == 0 {
		panic("no moves to evaluate")
	}

	// Limit to k moves
	evaluateCount := k
	if evaluateCount > len(moves) {
		evaluateCount = len(moves)
	}

	results := EvaluateMovesParallel(net, moves[:evaluateCount], rates)

	bestIdx := 0
	bestScore := results[0].Score

	for i, result := range results {
		if result.Score > bestScore {
			bestScore = result.Score
			bestIdx = i
		}
	}

	return moves[bestIdx]
}

// ParallelConfig holds configuration for parallel evaluation
type ParallelConfig struct {
	MaxWorkers int  // Maximum number of parallel workers (0 = use GOMAXPROCS)
	BatchSize  int  // Number of moves to evaluate per batch
	UseCache   bool // Whether to use caching
}

// DefaultParallelConfig returns default parallel configuration
func DefaultParallelConfig() ParallelConfig {
	return ParallelConfig{
		MaxWorkers: runtime.GOMAXPROCS(0),
		BatchSize:  0, // 0 = evaluate all at once
		UseCache:   true,
	}
}

// EvaluateMovesParallelBatched evaluates moves in batches to control parallelism
func EvaluateMovesParallelBatched(net *petri.PetriNet, moves []Move, rates map[string]float64, config ParallelConfig) []MoveResult {
	results := make([]MoveResult, len(moves))

	// Determine batch size
	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = len(moves)
	}

	// Limit concurrent goroutines
	semaphore := make(chan struct{}, config.MaxWorkers)
	var wg sync.WaitGroup
	resultChan := make(chan MoveResult, len(moves))

	// Process in batches
	for i := 0; i < len(moves); i++ {
		wg.Add(1)

		// Acquire semaphore
		semaphore <- struct{}{}

		go func(idx int, m Move) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			// Evaluate with optimized parameters
			prob := solver.NewProblem(net, m.state, [2]float64{0, 1.0}, rates)
			opts := solver.DefaultOptions()
			opts.Abstol = 1e-2
			opts.Reltol = 1e-2
			opts.Dt = 0.5
			sol := solver.Solve(prob, solver.Tsit5(), opts)

			score := sol.GetFinalState()["solved"]

			resultChan <- MoveResult{
				Move:  m,
				Score: score,
				Index: idx,
			}
		}(i, moves[i])
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for result := range resultChan {
		results[result.Index] = result
	}

	return results
}

// EvaluateMovesSequential evaluates moves sequentially (for comparison)
func EvaluateMovesSequential(net *petri.PetriNet, moves []Move, rates map[string]float64) []MoveResult {
	results := make([]MoveResult, len(moves))

	for i, move := range moves {
		// Evaluate with optimized parameters
		prob := solver.NewProblem(net, move.state, [2]float64{0, 1.0}, rates)
		opts := solver.DefaultOptions()
		opts.Abstol = 1e-2
		opts.Reltol = 1e-2
		opts.Dt = 0.5
		sol := solver.Solve(prob, solver.Tsit5(), opts)

		score := sol.GetFinalState()["solved"]

		results[i] = MoveResult{
			Move:  move,
			Score: score,
			Index: i,
		}
	}

	return results
}
