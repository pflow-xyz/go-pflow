package main

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/solver"
)

// buildAnalysisNet constructs the analysis net for NxN tic-tac-toe following
// the integer reduction paper (Definition 1: Analysis Net).
//
// Entities = board cells (i,j).  Constraints = win lines (rows, cols, diags).
//
// For each entity e_i:
//   - Source place S_i (initial=1)
//   - Accumulator place X_i (initial=0)
//   - Catalytic play transition: S_i -> S_i + X_i (net zero on S, +1 on X)
//
// For each constraint C_j containing entity e_i:
//   - Drain transition: X_i -> ∅  (one per cell-per-winline)
//
// Under mass-action kinetics, equilibrium gives x_i* = 1/n_i where
// n_i = number of constraints containing entity i.
func buildAnalysisNet(n int) *petri.PetriNet {
	net := petri.NewPetriNet()

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			net.AddPlace(fmt.Sprintf("S%d_%d", i, j), 1.0, nil, float64(j)*40, float64(i)*40, nil)
			net.AddPlace(fmt.Sprintf("X%d_%d", i, j), 0.0, nil, float64(j)*40+20, float64(i)*40, nil)
		}
	}

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			t := fmt.Sprintf("play_%d_%d", i, j)
			net.AddTransition(t, "play", float64(j)*40+10, float64(i)*40+10, nil)
			net.AddArc(fmt.Sprintf("S%d_%d", i, j), t, 1.0, false)
			net.AddArc(t, fmt.Sprintf("S%d_%d", i, j), 1.0, false)
			net.AddArc(t, fmt.Sprintf("X%d_%d", i, j), 1.0, false)
		}
	}

	patterns := generateWinPatterns(n)
	for lineIdx, pattern := range patterns {
		for _, pos := range pattern {
			i, j := pos[0], pos[1]
			t := fmt.Sprintf("drain_%d_%d_%d", i, j, lineIdx)
			net.AddTransition(t, "drain", 0, 0, nil)
			net.AddArc(fmt.Sprintf("X%d_%d", i, j), t, 1.0, false)
		}
	}

	return net
}

// integerReduction runs ODE to equilibrium on the analysis net and returns
// the reduced position values (1/concentration, normalized).
func integerReduction(net *petri.PetriNet, n int) ([][]float64, bool) {
	state := net.SetState(nil)
	rates := net.SetRates(nil)

	prob := solver.NewProblem(net, state, [2]float64{0, 200}, rates)
	opts := solver.DefaultOptions()
	opts.Dt = 0.5
	eqOpts := &solver.EquilibriumOptions{
		Tolerance:        1e-6,
		ConsecutiveSteps: 5,
		MinTime:          0.5,
		CheckInterval:    5,
	}

	_, eqResult := solver.SolveUntilEquilibrium(prob, solver.Tsit5(), opts, eqOpts)

	raw := make([][]float64, n)
	for i := 0; i < n; i++ {
		raw[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			raw[i][j] = eqResult.State[fmt.Sprintf("X%d_%d", i, j)]
		}
	}

	values := make([][]float64, n)
	for i := 0; i < n; i++ {
		values[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			if raw[i][j] > 1e-10 {
				values[i][j] = 1.0 / raw[i][j]
			}
		}
	}

	minVal := math.MaxFloat64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if values[i][j] > 1e-10 && values[i][j] < minVal {
				minVal = values[i][j]
			}
		}
	}
	if minVal > 1e-10 && minVal < math.MaxFloat64 {
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				values[i][j] /= minVal
			}
		}
	}

	return values, eqResult.Reached
}

// winPatternsToConstraints converts NxN win patterns to entity-constraint format.
// Returns entity labels and constraint index lists for EntityConstraintCentrality.
func winPatternsToConstraints(n int) ([]string, [][]int) {
	entities := make([]string, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			entities[i*n+j] = fmt.Sprintf("(%d,%d)", i, j)
		}
	}

	patterns := generateWinPatterns(n)
	constraints := make([][]int, len(patterns))
	for k, pat := range patterns {
		indices := make([]int, len(pat))
		for m, pos := range pat {
			indices[m] = pos[0]*n + pos[1]
		}
		constraints[k] = indices
	}

	return entities, constraints
}

// TestSpectralVsIntegerReduction compares eigenvector centrality with
// integer reduction for NxN tic-tac-toe.
//
// Three centrality methods are compared:
//  1. Integer Reduction (ODE equilibrium): values = 1/n_i, exact rationals
//  2. Eigenvector centrality of B·B^T: entity co-occurrence via shared win-lines
//  3. Bipartite eigenvector centrality: full analysis net graph
func TestSpectralVsIntegerReduction(t *testing.T) {
	sizes := []int{3, 5, 7}

	for _, n := range sizes {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			net := buildAnalysisNet(n)
			patterns := generateWinPatterns(n)

			places, trans, arcs := netStats(net)
			fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
			fmt.Printf("  N=%d  Analysis Net: %d places, %d transitions, %d arcs\n", n, places, trans, arcs)
			fmt.Printf("  Win patterns: %d (rows=%d, cols=%d, diags=2)\n", len(patterns), n, n)
			fmt.Printf("═══════════════════════════════════════════════════════════════\n")

			// ---------- Incidence degree (closed-form) ----------
			degree := make([][]int, n)
			for i := 0; i < n; i++ {
				degree[i] = make([]int, n)
				for j := 0; j < n; j++ {
					deg := 2
					if i == j {
						deg++
					}
					if i+j == n-1 {
						deg++
					}
					degree[i][j] = deg
				}
			}

			fmt.Printf("\n  Incidence Degree n_i (drain count per cell):\n")
			printIntBoard(n, degree, "    ")

			// ---------- Method 1: Integer reduction ----------
			irValues, reached := integerReduction(net, n)
			fmt.Printf("\n  1. Integer Reduction (1/n_i via ODE, converged=%v):\n", reached)
			printBoard(n, irValues, "    ")

			// ---------- Method 2: Entity-constraint eigenvector centrality ----------
			entities, constraints := winPatternsToConstraints(n)
			ecResult := reachability.EntityConstraintCentrality(entities, constraints, 1000, 1e-12)
			fmt.Printf("\n  2. Eigenvector Centrality of B·Bᵀ (λ=%.4f, %d iters):\n",
				ecResult.Eigenvalue, ecResult.Iterations)

			ecValues := make([][]float64, n)
			for i := 0; i < n; i++ {
				ecValues[i] = make([]float64, n)
				for j := 0; j < n; j++ {
					ecValues[i][j] = ecResult.Centrality[fmt.Sprintf("(%d,%d)", i, j)]
				}
			}
			printBoard(n, ecValues, "    ")

			// ---------- Method 3: Normalize eigenvector to compare with IR ----------
			// Normalize EV centrality so minimum non-zero value = 1.0 (same as IR)
			ecNorm := make([][]float64, n)
			minEC := math.MaxFloat64
			for i := 0; i < n; i++ {
				ecNorm[i] = make([]float64, n)
				for j := 0; j < n; j++ {
					ecNorm[i][j] = ecValues[i][j]
					if ecNorm[i][j] > 1e-10 && ecNorm[i][j] < minEC {
						minEC = ecNorm[i][j]
					}
				}
			}
			if minEC > 1e-10 && minEC < math.MaxFloat64 {
				for i := 0; i < n; i++ {
					for j := 0; j < n; j++ {
						ecNorm[i][j] /= minEC
					}
				}
			}
			fmt.Printf("\n  3. EV Centrality normalized (min=1.0, comparable to IR):\n")
			printBoard(n, ecNorm, "    ")

			// ---------- Build B·B^T and show matrix structure ----------
			if n == 3 {
				fmt.Printf("\n  B·Bᵀ co-occurrence matrix (3×3 only):\n")
				ne := n * n
				nc := len(constraints)
				B := make([][]float64, ne)
				for i := range B {
					B[i] = make([]float64, nc)
				}
				for j, c := range constraints {
					for _, idx := range c {
						B[idx][j] = 1
					}
				}
				M := make([][]float64, ne)
				for i := range M {
					M[i] = make([]float64, ne)
					for j := range M[i] {
						for k := 0; k < nc; k++ {
							M[i][j] += B[i][k] * B[j][k]
						}
					}
				}
				for i := 0; i < ne; i++ {
					fmt.Printf("    ")
					for j := 0; j < ne; j++ {
						fmt.Printf("%3.0f", M[i][j])
					}
					fmt.Printf("  ← %s (n=%d)\n", entities[i], degree[i/n][i%n])
				}
			}

			// ---------- Compare rankings ----------
			fmt.Printf("\n  ─── Ranking Comparison ───\n")

			type cellScore struct {
				i, j   int
				degree int
				ir     float64
				ec     float64
				ecNorm float64
			}
			cells := make([]cellScore, 0, n*n)
			for i := 0; i < n; i++ {
				for j := 0; j < n; j++ {
					cells = append(cells, cellScore{
						i: i, j: j,
						degree: degree[i][j],
						ir:     irValues[i][j],
						ec:     ecValues[i][j],
						ecNorm: ecNorm[i][j],
					})
				}
			}

			sort.Slice(cells, func(a, b int) bool {
				return cells[a].ir > cells[b].ir
			})

			fmt.Printf("\n  %5s  %6s  %10s  %10s  %10s\n",
				"Cell", "Degree", "IntReduce", "EV(raw)", "EV(norm)")
			fmt.Printf("  %5s  %6s  %10s  %10s  %10s\n",
				"─────", "──────", "──────────", "──────────", "──────────")

			degIR := map[int]float64{}
			degEC := map[int]float64{}
			degEN := map[int]float64{}
			degCount := map[int]int{}

			for _, c := range cells {
				fmt.Printf("  (%d,%d)  %4d    %8.4f    %8.6f    %8.4f\n",
					c.i, c.j, c.degree, c.ir, c.ec, c.ecNorm)
				degIR[c.degree] += c.ir
				degEC[c.degree] += c.ec
				degEN[c.degree] += c.ecNorm
				degCount[c.degree]++
			}

			fmt.Printf("\n  ─── Averages by Degree Group ───\n")
			fmt.Printf("  %6s  %5s  %10s  %10s  %10s  %10s\n",
				"Degree", "Count", "IntReduce", "EV(raw)", "EV(norm)", "IR/EV ratio")

			degs := []int{}
			for d := range degCount {
				degs = append(degs, d)
			}
			sort.Sort(sort.Reverse(sort.IntSlice(degs)))

			for _, d := range degs {
				cnt := float64(degCount[d])
				avgIR := degIR[d] / cnt
				avgEC := degEC[d] / cnt
				avgEN := degEN[d] / cnt
				ratio := 0.0
				if avgEN > 1e-10 {
					ratio = avgIR / avgEN
				}
				fmt.Printf("  %4d    %3d    %8.4f    %8.6f    %8.4f    %8.4f\n",
					d, degCount[d], avgIR, avgEC, avgEN, ratio)
			}

			// ---------- Verify ranking agreement ----------
			fmt.Printf("\n  ─── Ranking Agreement ───\n")
			allAgree := true

			for _, d1 := range degs {
				for _, d2 := range degs {
					if d1 <= d2 {
						continue
					}
					avgIR1 := degIR[d1] / float64(degCount[d1])
					avgIR2 := degIR[d2] / float64(degCount[d2])
					avgEN1 := degEN[d1] / float64(degCount[d1])
					avgEN2 := degEN[d2] / float64(degCount[d2])

					irOK := avgIR1 > avgIR2
					ecOK := avgEN1 > avgEN2

					sym := func(ok bool) string {
						if ok {
							return "yes"
						}
						return "NO"
					}

					fmt.Printf("  deg %d > deg %d:  IR=%s  EV=%s\n",
						d1, d2, sym(irOK), sym(ecOK))

					if !irOK {
						t.Errorf("integer reduction: degree %d not ranked above %d", d1, d2)
						allAgree = false
					}
					if !ecOK {
						t.Errorf("eigenvector centrality: degree %d not ranked above %d", d1, d2)
						allAgree = false
					}
				}
			}

			if allAgree {
				fmt.Printf("\n  RESULT: Both methods produce identical ranking.\n")
			}
		})
	}
}

func printBoard(n int, values [][]float64, prefix string) {
	for i := 0; i < n; i++ {
		fmt.Printf("%s", prefix)
		for j := 0; j < n; j++ {
			if j > 0 {
				fmt.Printf("  ")
			}
			fmt.Printf("%6.3f", values[i][j])
		}
		fmt.Println()
	}
}

func printIntBoard(n int, values [][]int, prefix string) {
	for i := 0; i < n; i++ {
		fmt.Printf("%s", prefix)
		for j := 0; j < n; j++ {
			if j > 0 {
				fmt.Printf("  ")
			}
			fmt.Printf("%d", values[i][j])
		}
		fmt.Println()
	}
}
