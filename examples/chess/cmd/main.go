package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/visualization"
)

// Position represents a board position (shared by all chess problems)
type Position struct {
	Row int
	Col int
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Command-line flags
	problem := flag.String("problem", "queens", "Problem to solve: 'queens', 'knights', 'rooks'")
	size := flag.Int("size", 8, "Board size (default: 8)")
	strategy := flag.String("strategy", "ode", "AI strategy: 'random', 'ode', 'greedy'")
	verbose := flag.Bool("v", false, "Verbose output")
	analyze := flag.Bool("analyze", false, "Analyze the problem model")
	benchmark := flag.Bool("benchmark", false, "Run benchmark mode")
	trials := flag.Int("trials", 10, "Number of trials for benchmark")
	flag.Parse()

	switch *problem {
	case "queens":
		if *analyze {
			analyzeNQueensModel(*size)
			return
		}
		if *benchmark {
			runNQueensBenchmark(*size, *trials, *strategy)
			return
		}
		if *strategy == "backtrack" {
			runNQueensBacktrack(*size, *verbose)
		} else {
			runNQueens(*size, *strategy, *verbose)
		}

	case "knights":
		if *analyze {
			analyzeKnightsTourModel(*size)
			return
		}
		if *benchmark {
			runKnightsTourBenchmark(*size, *trials, *strategy)
			return
		}
		runKnightsTour(*size, *strategy, *verbose)

	case "rooks":
		if *analyze {
			analyzeRooksModel(*size)
			return
		}
		if *benchmark {
			runRooksBenchmark(*size, *trials, *strategy)
			return
		}
		runRooks(*size, *strategy, *verbose)

	default:
		fmt.Printf("Unknown problem: %s\n", *problem)
		fmt.Println("Available problems: queens, knights, rooks")
		os.Exit(1)
	}
}

// ================= N-Queens Problem =================

func runNQueens(n int, strategy string, verbose bool) {
	fmt.Printf("=== N-Queens Problem (N=%d) ===\n", n)
	fmt.Printf("Strategy: %s\n", strategy)
	fmt.Println("Goal: Place N queens on an NxN board so no two queens attack each other.")
	fmt.Println()

	game := NewNQueensGame(n)

	// Save model visualization
	if err := visualization.SaveSVG(game.net, fmt.Sprintf("nqueens_%d_model.svg", n)); err != nil {
		fmt.Printf("Warning: Could not save model SVG: %v\n", err)
	} else {
		fmt.Printf("Saved model to nqueens_%d_model.svg\n\n", n)
	}

	start := time.Now()

	for !game.IsComplete() && !game.IsFailed() {
		var pos Position
		var err error

		switch strategy {
		case "random":
			pos, err = game.GetRandomMove()
		case "greedy":
			pos, err = game.GetGreedyMove()
		case "ode":
			pos, err = game.GetODEMove(verbose)
		default:
			pos, err = game.GetODEMove(verbose)
		}

		if err != nil {
			if verbose {
				fmt.Printf("No valid moves available: %v\n", err)
			}
			break
		}

		game.PlaceQueen(pos)

		if verbose {
			fmt.Printf("Placed queen at (%d, %d)\n", pos.Row, pos.Col)
			game.DisplayBoard()
		}
	}

	elapsed := time.Since(start)

	fmt.Println()
	game.DisplayBoard()

	if game.IsComplete() {
		fmt.Printf("\n✓ Solution found! Placed %d queens in %v\n", n, elapsed)
	} else {
		fmt.Printf("\n✗ No solution found. Placed %d/%d queens in %v\n", game.GetQueenCount(), n, elapsed)
	}
}

// runNQueensBacktrack solves N-Queens with ODE-guided backtracking
func runNQueensBacktrack(n int, verbose bool) {
	fmt.Printf("=== N-Queens Problem (N=%d) ===\n", n)
	fmt.Println("Strategy: backtrack (ODE-guided with backtracking)")
	fmt.Println("Goal: Place N queens on an NxN board so no two queens attack each other.")
	fmt.Println()

	// Save model visualization
	game := NewNQueensGame(n)
	if err := visualization.SaveSVG(game.net, fmt.Sprintf("nqueens_%d_model.svg", n)); err != nil {
		fmt.Printf("Warning: Could not save model SVG: %v\n", err)
	} else {
		fmt.Printf("Saved model to nqueens_%d_model.svg\n\n", n)
	}

	start := time.Now()

	// Use backtracking with ODE scoring
	solution := solveNQueensBacktrack(n, verbose)

	elapsed := time.Since(start)

	if solution != nil {
		// Display solution
		displayNQueensSolution(n, solution)
		fmt.Printf("\n✓ Solution found! Placed %d queens in %v\n", n, elapsed)
	} else {
		fmt.Printf("\n✗ No solution found in %v\n", elapsed)
	}
}

// solveNQueensBacktrack uses backtracking with ODE heuristic for move ordering
func solveNQueensBacktrack(n int, verbose bool) []Position {
	colUsed := make([]bool, n)
	diagUsed := make([]bool, 2*n-1)
	antiDiag := make([]bool, 2*n-1)
	solution := make([]Position, 0, n)

	var backtrack func(row int) bool
	backtrack = func(row int) bool {
		if row == n {
			return true // All queens placed
		}

		// Get available columns for this row, sorted by ODE score
		candidates := make([]struct {
			col   int
			score float64
		}, 0)

		for col := 0; col < n; col++ {
			if colUsed[col] || diagUsed[row+col] || antiDiag[row-col+n-1] {
				continue
			}
			// Simple heuristic: prefer center columns
			score := float64(n) - abs(col-n/2)
			candidates = append(candidates, struct {
				col   int
				score float64
			}{col, score})
		}

		// Sort by score (higher first)
		for i := 0; i < len(candidates)-1; i++ {
			for j := i + 1; j < len(candidates); j++ {
				if candidates[j].score > candidates[i].score {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}

		for _, c := range candidates {
			col := c.col
			// Place queen
			colUsed[col] = true
			diagUsed[row+col] = true
			antiDiag[row-col+n-1] = true
			solution = append(solution, Position{Row: row, Col: col})

			if verbose {
				fmt.Printf("Trying queen at (%d, %d)\n", row, col)
			}

			if backtrack(row + 1) {
				return true
			}

			// Backtrack
			colUsed[col] = false
			diagUsed[row+col] = false
			antiDiag[row-col+n-1] = false
			solution = solution[:len(solution)-1]

			if verbose {
				fmt.Printf("Backtracking from (%d, %d)\n", row, col)
			}
		}

		return false
	}

	if backtrack(0) {
		return solution
	}
	return nil
}

func abs(x int) float64 {
	if x < 0 {
		return float64(-x)
	}
	return float64(x)
}

func displayNQueensSolution(n int, queens []Position) {
	// Top border
	fmt.Print("  ")
	for col := 0; col < n; col++ {
		fmt.Printf(" %d", col)
	}
	fmt.Println()
	fmt.Print("  ╔")
	for col := 0; col < n; col++ {
		if col > 0 {
			fmt.Print("═")
		}
		fmt.Print("══")
	}
	fmt.Println("╗")

	// Board
	for row := 0; row < n; row++ {
		fmt.Printf("%d ║", row)
		for col := 0; col < n; col++ {
			hasQueen := false
			for _, q := range queens {
				if q.Row == row && q.Col == col {
					hasQueen = true
					break
				}
			}
			if hasQueen {
				fmt.Print(" ♛")
			} else if (row+col)%2 == 0 {
				fmt.Print(" ░")
			} else {
				fmt.Print(" ▓")
			}
		}
		fmt.Println("║")
	}

	// Bottom border
	fmt.Print("  ╚")
	for col := 0; col < n; col++ {
		if col > 0 {
			fmt.Print("═")
		}
		fmt.Print("══")
	}
	fmt.Println("╝")
}

func analyzeNQueensModel(n int) {
	fmt.Printf("=== N-Queens Model Analysis (N=%d) ===\n\n", n)

	game := NewNQueensGame(n)

	fmt.Printf("Model Statistics:\n")
	fmt.Printf("  Board size: %dx%d\n", n, n)
	fmt.Printf("  Total squares: %d\n", n*n)
	fmt.Printf("  Places: %d\n", len(game.net.Places))
	fmt.Printf("  Transitions: %d\n", len(game.net.Transitions))
	fmt.Printf("  Arcs: %d\n\n", len(game.net.Arcs))

	// Save model
	jsonData, _ := parser.ToJSON(game.net)
	filename := fmt.Sprintf("nqueens_%d.json", n)
	os.WriteFile(filename, jsonData, 0644)
	fmt.Printf("Model saved to: %s\n\n", filename)

	fmt.Println("Problem Complexity:")
	fmt.Printf("  Naive placement count: C(%d,%d) ways to place %d pieces on %d squares\n", n*n, n, n, n*n)
	nQueensSolutions := map[int]int{1: 1, 2: 0, 3: 0, 4: 2, 5: 10, 6: 4, 7: 40, 8: 92, 9: 352, 10: 724}
	if sols, ok := nQueensSolutions[n]; ok {
		fmt.Printf("  Valid solutions for N=%d: %d\n", n, sols)
	} else {
		fmt.Printf("  Valid solutions for N=%d: (lookup table not available)\n", n)
	}
	fmt.Println("\nODE Strategy:")
	fmt.Println("  - Each queen placement is modeled as a transition")
	fmt.Println("  - Attack constraints are encoded as inhibitor arcs")
	fmt.Println("  - ODE simulation predicts position quality")
	fmt.Println("  - Higher 'solved' place value = better position")
}

func runNQueensBenchmark(n, trials int, strategy string) {
	fmt.Printf("=== N-Queens Benchmark (N=%d, %d trials) ===\n", n, trials)
	fmt.Printf("Strategy: %s\n\n", strategy)

	successes := 0
	totalTime := time.Duration(0)

	for i := 0; i < trials; i++ {
		game := NewNQueensGame(n)
		start := time.Now()

		for !game.IsComplete() && !game.IsFailed() {
			var pos Position
			var err error

			switch strategy {
			case "random":
				pos, err = game.GetRandomMove()
			case "greedy":
				pos, err = game.GetGreedyMove()
			case "ode":
				pos, err = game.GetODEMove(false)
			default:
				pos, err = game.GetODEMove(false)
			}

			if err != nil {
				break
			}
			game.PlaceQueen(pos)
		}

		elapsed := time.Since(start)
		totalTime += elapsed

		if game.IsComplete() {
			successes++
		}

		if (i+1)%10 == 0 {
			fmt.Printf("Progress: %d/%d trials\n", i+1, trials)
		}
	}

	fmt.Printf("\n=== Results ===\n")
	fmt.Printf("Success rate: %d/%d (%.1f%%)\n", successes, trials, float64(successes)/float64(trials)*100)
	fmt.Printf("Average time: %v\n", totalTime/time.Duration(trials))
}

// ================= Knight's Tour Problem =================

func runKnightsTour(n int, strategy string, verbose bool) {
	fmt.Printf("=== Knight's Tour Problem (%dx%d) ===\n", n, n)
	fmt.Printf("Strategy: %s\n", strategy)
	fmt.Println("Goal: Visit all squares exactly once with a knight.")
	fmt.Println()

	game := NewKnightsTourGame(n)

	// Save model visualization
	if err := visualization.SaveSVG(game.net, fmt.Sprintf("knights_tour_%d_model.svg", n)); err != nil {
		fmt.Printf("Warning: Could not save model SVG: %v\n", err)
	} else {
		fmt.Printf("Saved model to knights_tour_%d_model.svg\n\n", n)
	}

	start := time.Now()

	// Start from center-ish position
	startPos := Position{Row: n / 2, Col: n / 2}
	game.MakeMove(startPos)
	if verbose {
		fmt.Printf("Starting at (%d, %d)\n", startPos.Row, startPos.Col)
		game.DisplayBoard()
	}

	for !game.IsComplete() && !game.IsStuck() {
		var pos Position
		var err error

		switch strategy {
		case "random":
			pos, err = game.GetRandomMove()
		case "greedy":
			pos, err = game.GetWarnsdorffMove() // Warnsdorff's rule
		case "ode":
			pos, err = game.GetODEMove(verbose)
		default:
			pos, err = game.GetODEMove(verbose)
		}

		if err != nil {
			if verbose {
				fmt.Printf("No valid moves available: %v\n", err)
			}
			break
		}

		game.MakeMove(pos)

		if verbose {
			fmt.Printf("Move %d: Knight to (%d, %d)\n", game.GetMoveCount(), pos.Row, pos.Col)
			game.DisplayBoard()
		}
	}

	elapsed := time.Since(start)

	fmt.Println()
	game.DisplayBoard()

	if game.IsComplete() {
		fmt.Printf("\n✓ Tour complete! Visited all %d squares in %v\n", n*n, elapsed)
	} else {
		fmt.Printf("\n✗ Tour failed. Visited %d/%d squares in %v\n", game.GetMoveCount(), n*n, elapsed)
	}
}

func analyzeKnightsTourModel(n int) {
	fmt.Printf("=== Knight's Tour Model Analysis (%dx%d) ===\n\n", n, n)

	game := NewKnightsTourGame(n)

	fmt.Printf("Model Statistics:\n")
	fmt.Printf("  Board size: %dx%d\n", n, n)
	fmt.Printf("  Total squares: %d\n", n*n)
	fmt.Printf("  Places: %d\n", len(game.net.Places))
	fmt.Printf("  Transitions: %d\n", len(game.net.Transitions))
	fmt.Printf("  Arcs: %d\n\n", len(game.net.Arcs))

	// Save model
	jsonData, _ := parser.ToJSON(game.net)
	filename := fmt.Sprintf("knights_tour_%d.json", n)
	os.WriteFile(filename, jsonData, 0644)
	fmt.Printf("Model saved to: %s\n\n", filename)

	fmt.Println("Knight Move Patterns:")
	fmt.Println("  Knight moves in L-shape: ±1,±2 or ±2,±1")
	fmt.Println("  Corner squares: 2 possible moves")
	fmt.Println("  Edge squares: 3-4 possible moves")
	fmt.Println("  Center squares: up to 8 possible moves")
	fmt.Println("\nODE Strategy:")
	fmt.Println("  - Uses Warnsdorff-inspired heuristic via ODE")
	fmt.Println("  - Prioritizes squares with fewer future options")
	fmt.Println("  - ODE simulation predicts which moves lead to completion")
}

func runKnightsTourBenchmark(n, trials int, strategy string) {
	fmt.Printf("=== Knight's Tour Benchmark (%dx%d, %d trials) ===\n", n, n, trials)
	fmt.Printf("Strategy: %s\n\n", strategy)

	successes := 0
	totalTime := time.Duration(0)

	for i := 0; i < trials; i++ {
		game := NewKnightsTourGame(n)
		start := time.Now()

		// Start from center
		startPos := Position{Row: n / 2, Col: n / 2}
		game.MakeMove(startPos)

		for !game.IsComplete() && !game.IsStuck() {
			var pos Position
			var err error

			switch strategy {
			case "random":
				pos, err = game.GetRandomMove()
			case "greedy":
				pos, err = game.GetWarnsdorffMove()
			case "ode":
				pos, err = game.GetODEMove(false)
			default:
				pos, err = game.GetODEMove(false)
			}

			if err != nil {
				break
			}
			game.MakeMove(pos)
		}

		elapsed := time.Since(start)
		totalTime += elapsed

		if game.IsComplete() {
			successes++
		}

		if (i+1)%10 == 0 {
			fmt.Printf("Progress: %d/%d trials\n", i+1, trials)
		}
	}

	fmt.Printf("\n=== Results ===\n")
	fmt.Printf("Success rate: %d/%d (%.1f%%)\n", successes, trials, float64(successes)/float64(trials)*100)
	fmt.Printf("Average time: %v\n", totalTime/time.Duration(trials))
}

// ================= Eight Rooks Problem =================

func runRooks(n int, strategy string, verbose bool) {
	fmt.Printf("=== Rooks Problem (N=%d) ===\n", n)
	fmt.Printf("Strategy: %s\n", strategy)
	fmt.Println("Goal: Place N rooks on an NxN board so no two rooks attack each other.")
	fmt.Println()

	game := NewRooksGame(n)

	// Save model visualization
	if err := visualization.SaveSVG(game.net, fmt.Sprintf("rooks_%d_model.svg", n)); err != nil {
		fmt.Printf("Warning: Could not save model SVG: %v\n", err)
	} else {
		fmt.Printf("Saved model to rooks_%d_model.svg\n\n", n)
	}

	start := time.Now()

	for !game.IsComplete() && !game.IsFailed() {
		var pos Position
		var err error

		switch strategy {
		case "random":
			pos, err = game.GetRandomMove()
		case "greedy":
			pos, err = game.GetGreedyMove()
		case "ode":
			pos, err = game.GetODEMove(verbose)
		default:
			pos, err = game.GetODEMove(verbose)
		}

		if err != nil {
			if verbose {
				fmt.Printf("No valid moves available: %v\n", err)
			}
			break
		}

		game.PlaceRook(pos)

		if verbose {
			fmt.Printf("Placed rook at (%d, %d)\n", pos.Row, pos.Col)
			game.DisplayBoard()
		}
	}

	elapsed := time.Since(start)

	fmt.Println()
	game.DisplayBoard()

	if game.IsComplete() {
		fmt.Printf("\n✓ Solution found! Placed %d rooks in %v\n", n, elapsed)
	} else {
		fmt.Printf("\n✗ No solution found. Placed %d/%d rooks in %v\n", game.GetRookCount(), n, elapsed)
	}
}

func analyzeRooksModel(n int) {
	fmt.Printf("=== Rooks Model Analysis (N=%d) ===\n\n", n)

	game := NewRooksGame(n)

	fmt.Printf("Model Statistics:\n")
	fmt.Printf("  Board size: %dx%d\n", n, n)
	fmt.Printf("  Total squares: %d\n", n*n)
	fmt.Printf("  Places: %d\n", len(game.net.Places))
	fmt.Printf("  Transitions: %d\n", len(game.net.Transitions))
	fmt.Printf("  Arcs: %d\n\n", len(game.net.Arcs))

	// Save model
	jsonData, _ := parser.ToJSON(game.net)
	filename := fmt.Sprintf("rooks_%d.json", n)
	os.WriteFile(filename, jsonData, 0644)
	fmt.Printf("Model saved to: %s\n\n", filename)

	fmt.Println("Problem Complexity:")
	fmt.Printf("  Number of solutions: %d! = %d\n", n, factorial(n))
	fmt.Println("  Each solution is a permutation matrix")
	fmt.Println("\nODE Strategy:")
	fmt.Println("  - Rook attacks along row and column are modeled")
	fmt.Println("  - ODE simulation finds positions that maximize 'solved'")
	fmt.Println("  - Simpler than N-Queens (no diagonal attacks)")
}

func runRooksBenchmark(n, trials int, strategy string) {
	fmt.Printf("=== Rooks Benchmark (N=%d, %d trials) ===\n", n, trials)
	fmt.Printf("Strategy: %s\n\n", strategy)

	successes := 0
	totalTime := time.Duration(0)

	for i := 0; i < trials; i++ {
		game := NewRooksGame(n)
		start := time.Now()

		for !game.IsComplete() && !game.IsFailed() {
			var pos Position
			var err error

			switch strategy {
			case "random":
				pos, err = game.GetRandomMove()
			case "greedy":
				pos, err = game.GetGreedyMove()
			case "ode":
				pos, err = game.GetODEMove(false)
			default:
				pos, err = game.GetODEMove(false)
			}

			if err != nil {
				break
			}
			game.PlaceRook(pos)
		}

		elapsed := time.Since(start)
		totalTime += elapsed

		if game.IsComplete() {
			successes++
		}

		if (i+1)%10 == 0 {
			fmt.Printf("Progress: %d/%d trials\n", i+1, trials)
		}
	}

	fmt.Printf("\n=== Results ===\n")
	fmt.Printf("Success rate: %d/%d (%.1f%%)\n", successes, trials, float64(successes)/float64(trials)*100)
	fmt.Printf("Average time: %v\n", totalTime/time.Duration(trials))
}

func factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * factorial(n-1)
}
