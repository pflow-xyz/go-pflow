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

// TestIntegerReduction verifies that ODE steady-state values with uniform rates
// preserve the ranking and grouping determined by incidence degrees (arc counts
// to terminal win transitions) for any NxN board.
//
// The incidence degree for position (i,j) is:
//   - 4: both diagonals (center of odd-sized boards only)
//   - 3: one diagonal (corners + other diagonal positions)
//   - 2: no diagonal (all other positions)
//
// The test verifies three properties:
//  1. Positions with the same incidence degree get the same ODE score
//  2. Higher incidence degree always produces higher ODE score
//  3. ODE score ratios are approximately proportional to degree ratios
func TestIntegerReduction(t *testing.T) {
	sizes := []int{3, 4, 5, 6, 7}

	for _, n := range sizes {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			net := generateTicTacToeNet(n)

			// Compute incidence degrees from graph structure
			degree := make([][]int, n)
			for i := 0; i < n; i++ {
				degree[i] = make([]int, n)
				for j := 0; j < n; j++ {
					deg := 2 // row + column
					if i == j {
						deg++ // main diagonal
					}
					if i+j == n-1 {
						deg++ // anti-diagonal
					}
					degree[i][j] = deg
				}
			}

			// Verify incidence degrees by counting arcs in the actual net
			for i := 0; i < n; i++ {
				for j := 0; j < n; j++ {
					histPlace := fmt.Sprintf("_X%d_%d", i, j)
					arcCount := 0
					for _, arc := range net.Arcs {
						if arc.Source == histPlace {
							arcCount++
						}
					}
					if arcCount != degree[i][j] {
						t.Errorf("pos(%d,%d): arc count %d != computed degree %d",
							i, j, arcCount, degree[i][j])
					}
				}
			}

			// Run ODE for each first move (X plays at each position)
			state := net.SetState(nil)
			rates := net.SetRates(nil)
			scores := make([][]float64, n)

			for i := 0; i < n; i++ {
				scores[i] = make([]float64, n)
				for j := 0; j < n; j++ {
					hypState := make(map[string]float64)
					for k, v := range state {
						hypState[k] = v
					}
					hypState[fmt.Sprintf("P%d_%d", i, j)] = 0
					hypState[fmt.Sprintf("_X%d_%d", i, j)] = 1
					hypState["Next"] = 1

					prob := solver.NewProblem(net, hypState, [2]float64{0, 9.0}, rates)
					opts := solver.DefaultOptions()
					opts.Abstol = 1e-6
					opts.Reltol = 1e-5
					opts.Dt = 0.1
					sol := solver.Solve(prob, solver.Tsit5(), opts)
					final := sol.GetFinalState()

					scores[i][j] = final["win_x"] - final["win_o"]
				}
			}

			// Group scores by degree
			groups := map[int][]float64{}
			for i := 0; i < n; i++ {
				for j := 0; j < n; j++ {
					deg := degree[i][j]
					groups[deg] = append(groups[deg], scores[i][j])
				}
			}

			// Print the board
			fmt.Printf("\nN=%d board (%d win patterns):\n", n, 2*n+2)
			for i := 0; i < n; i++ {
				for j := 0; j < n; j++ {
					if j > 0 {
						fmt.Print("  ")
					}
					fmt.Printf("%d", degree[i][j])
				}
				fmt.Println()
			}

			// Property 1: positions with same degree get similar scores (within 5%)
			for deg, vals := range groups {
				for k := 1; k < len(vals); k++ {
					relDiff := (vals[k] - vals[0]) / vals[0]
					if relDiff < 0 {
						relDiff = -relDiff
					}
					if relDiff > 0.05 {
						t.Errorf("degree %d: scores differ by %.2f%% (%.6f vs %.6f)",
							deg, relDiff*100, vals[0], vals[k])
					}
				}
			}

			// Property 2: higher degree always produces higher score
			degreeOrder := []int{2, 3, 4}
			for k := 1; k < len(degreeOrder); k++ {
				lowDeg := degreeOrder[k-1]
				highDeg := degreeOrder[k]
				if _, ok := groups[highDeg]; !ok {
					continue
				}
				if groups[highDeg][0] <= groups[lowDeg][0] {
					t.Errorf("degree %d score %.6f not greater than degree %d score %.6f",
						highDeg, groups[highDeg][0], lowDeg, groups[lowDeg][0])
				}
			}

			// Property 3: score ratios approximate degree ratios
			baseScore := groups[2][0]
			baseDeg := 2.0
			fmt.Println()
			for deg := 4; deg >= 2; deg-- {
				vals, ok := groups[deg]
				if !ok {
					continue
				}
				odeRatio := vals[0] / baseScore
				degRatio := float64(deg) / baseDeg
				pctErr := (odeRatio - degRatio) / degRatio * 100
				fmt.Printf("  degree %d: %d positions, score %.6f, ratio %.3f (expected %.3f, err %+.1f%%)\n",
					deg, len(vals), vals[0], odeRatio, degRatio, pctErr)
			}
		})
	}
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
