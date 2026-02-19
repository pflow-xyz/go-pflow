package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// generateTicTacToeNet builds an NxN tic-tac-toe Petri net programmatically.
//
// Places:
//   - P{i}_{j}  : cell availability (initial=1)
//   - _X{i}_{j} : X move history (initial=0)
//   - _O{i}_{j} : O move history (initial=0)
//   - Next       : turn marker (0=X, 1=O)
//   - win_x      : X win detector
//   - win_o      : O win detector
//
// Transitions:
//   - PlayX{i}_{j} : X plays cell (i,j)
//   - PlayO{i}_{j} : O plays cell (i,j)
//   - WinX_*       : X win conditions (rows, cols, diags)
//   - WinO_*       : O win conditions (rows, cols, diags)
func generateTicTacToeNet(n int) *petri.PetriNet {
	net := petri.NewPetriNet()

	// Cell places
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			net.AddPlace(fmt.Sprintf("P%d_%d", i, j), 1.0, nil, float64(j)*30, float64(i)*30, nil)
		}
	}

	// History places
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			net.AddPlace(fmt.Sprintf("_X%d_%d", i, j), 0.0, nil, float64(j)*30, float64(n+i)*30, nil)
			net.AddPlace(fmt.Sprintf("_O%d_%d", i, j), 0.0, nil, float64(j)*30, float64(2*n+i)*30, nil)
		}
	}

	// Control places
	net.AddPlace("Next", 0.0, nil, float64(n)*30, 0, nil)
	net.AddPlace("win_x", 0.0, nil, float64(n+1)*30, 0, nil)
	net.AddPlace("win_o", 0.0, nil, float64(n+2)*30, 0, nil)

	// Move transitions: PlayX consumes cell, produces X history
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			tx := fmt.Sprintf("PlayX%d_%d", i, j)
			net.AddTransition(tx, "move_x", float64(j)*30, float64(i)*30+15, nil)
			net.AddArc(fmt.Sprintf("P%d_%d", i, j), tx, 1.0, false)
			net.AddArc(tx, fmt.Sprintf("_X%d_%d", i, j), 1.0, false)

			to := fmt.Sprintf("PlayO%d_%d", i, j)
			net.AddTransition(to, "move_o", float64(j)*30, float64(i)*30+15, nil)
			net.AddArc(fmt.Sprintf("P%d_%d", i, j), to, 1.0, false)
			net.AddArc(to, fmt.Sprintf("_O%d_%d", i, j), 1.0, false)
		}
	}

	// Win patterns: rows, columns, 2 diagonals
	winPatterns := generateWinPatterns(n)

	for idx, pattern := range winPatterns {
		// X win
		txLabel := fmt.Sprintf("WinX_%d", idx)
		net.AddTransition(txLabel, "win", float64(n+1)*30, float64(idx)*30, nil)
		for _, pos := range pattern {
			net.AddArc(fmt.Sprintf("_X%d_%d", pos[0], pos[1]), txLabel, 1.0, false)
		}
		net.AddArc(txLabel, "win_x", 1.0, false)

		// O win
		toLabel := fmt.Sprintf("WinO_%d", idx)
		net.AddTransition(toLabel, "win", float64(n+2)*30, float64(idx)*30, nil)
		for _, pos := range pattern {
			net.AddArc(fmt.Sprintf("_O%d_%d", pos[0], pos[1]), toLabel, 1.0, false)
		}
		net.AddArc(toLabel, "win_o", 1.0, false)
	}

	return net
}

// generateWinPatterns returns all N-in-a-row winning lines for an NxN board.
func generateWinPatterns(n int) [][][2]int {
	patterns := make([][][2]int, 0, 2*n+2)

	// Rows
	for i := 0; i < n; i++ {
		row := make([][2]int, n)
		for j := 0; j < n; j++ {
			row[j] = [2]int{i, j}
		}
		patterns = append(patterns, row)
	}

	// Columns
	for j := 0; j < n; j++ {
		col := make([][2]int, n)
		for i := 0; i < n; i++ {
			col[i] = [2]int{i, j}
		}
		patterns = append(patterns, col)
	}

	// Main diagonal
	diag := make([][2]int, n)
	for i := 0; i < n; i++ {
		diag[i] = [2]int{i, i}
	}
	patterns = append(patterns, diag)

	// Anti-diagonal
	anti := make([][2]int, n)
	for i := 0; i < n; i++ {
		anti[i] = [2]int{i, n - 1 - i}
	}
	patterns = append(patterns, anti)

	return patterns
}

func netStats(net *petri.PetriNet) (int, int, int) {
	return len(net.Places), len(net.Transitions), len(net.Arcs)
}

// TestNxNTicTacToeBenchmark runs a timed benchmark across board sizes and prints a table.
func TestNxNTicTacToeBenchmark(t *testing.T) {
	sizes := []int{3, 5, 10, 15, 20, 25, 30, 35, 40}

	fmt.Println()
	fmt.Println("NxN Tic-Tac-Toe Petri Net ODE Benchmark (Go)")
	fmt.Println("==============================================")
	fmt.Printf("%-5s %8s %8s %8s %12s %12s %12s\n",
		"N", "Places", "Trans", "Arcs", "Build", "Solve", "Total")
	fmt.Println("---------------------------------------------------------------------")

	for _, n := range sizes {
		// Build
		buildStart := time.Now()
		net := generateTicTacToeNet(n)
		buildDur := time.Since(buildStart)

		places, trans, arcs := netStats(net)

		// Solve
		state := net.SetState(nil)
		rates := net.SetRates(nil)

		solveStart := time.Now()
		prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
		opts := solver.DefaultOptions()
		opts.Abstol = 1e-4
		opts.Reltol = 1e-3
		opts.Dt = 0.2
		sol := solver.Solve(prob, solver.Tsit5(), opts)
		solveDur := time.Since(solveStart)

		final := sol.GetFinalState()
		_ = final

		totalDur := buildDur + solveDur

		fmt.Printf("%-5d %8d %8d %8d %12s %12s %12s\n",
			n, places, trans, arcs,
			formatDuration(buildDur),
			formatDuration(solveDur),
			formatDuration(totalDur))

		// Bail if a single size exceeds 10s to keep under 1 min total
		if totalDur > 120*time.Second {
			fmt.Printf("  (stopping: exceeded 10s for N=%d)\n", n)
			break
		}
	}

	fmt.Println()
}

func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%.0fns", float64(d.Nanoseconds()))
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d.Nanoseconds())/1000)
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Nanoseconds())/1e6)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// Standard Go benchmarks for `go test -bench`
func BenchmarkNxN3(b *testing.B)  { benchmarkNxN(b, 3) }
func BenchmarkNxN4(b *testing.B)  { benchmarkNxN(b, 4) }
func BenchmarkNxN5(b *testing.B)  { benchmarkNxN(b, 5) }
func BenchmarkNxN6(b *testing.B)  { benchmarkNxN(b, 6) }
func BenchmarkNxN7(b *testing.B)  { benchmarkNxN(b, 7) }
func BenchmarkNxN8(b *testing.B)  { benchmarkNxN(b, 8) }
func BenchmarkNxN10(b *testing.B) { benchmarkNxN(b, 10) }

func benchmarkNxN(b *testing.B, n int) {
	net := generateTicTacToeNet(n)
	state := net.SetState(nil)
	rates := net.SetRates(nil)

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
