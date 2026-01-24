package metamodel

import (
	"fmt"
	"testing"

	"github.com/pflow-xyz/go-pflow/tokenmodel/dsl"
	mpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// TestODEConversion verifies the metamodel can be converted to a Petri net for ODE simulation
func TestODEConversion(t *testing.T) {
	// Get schema from struct
	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	// Convert to metamodel/petri.Model
	model := mpetri.FromSchema(schema)
	if model == nil {
		t.Fatal("FromSchema returned nil")
	}

	// Validate model
	if err := model.Validate(); err != nil {
		t.Fatalf("Model validation failed: %v", err)
	}

	// Convert to petri.PetriNet for ODE
	net := model.ToPetriNet()
	if net == nil {
		t.Fatal("ToPetriNet returned nil")
	}

	t.Logf("Places: %d, Transitions: %d, Arcs: %d",
		len(net.Places), len(net.Transitions), len(net.Arcs))

	// Verify structure
	if len(net.Places) != 30 {
		t.Errorf("expected 30 places, got %d", len(net.Places))
	}
	if len(net.Transitions) != 34 {
		t.Errorf("expected 34 transitions, got %d", len(net.Transitions))
	}
	if len(net.Arcs) != 118 {
		t.Errorf("expected 118 arcs, got %d", len(net.Arcs))
	}
}

// TestODESimulation runs a basic ODE simulation on the tic-tac-toe model
func TestODESimulation(t *testing.T) {
	schema, _ := dsl.SchemaFromStruct(TicTacToe{})
	model := mpetri.FromSchema(schema)
	net := model.ToPetriNet()
	rates := model.DefaultRates(1.0)

	// Get initial state
	state := net.SetState(nil)

	// Verify initial state: 9 positions available, X's turn
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			pos := fmt.Sprintf("p%d%d", i, j)
			if state[pos] != 1.0 {
				t.Errorf("expected %s = 1, got %f", pos, state[pos])
			}
		}
	}

	if state["next"] != 0 {
		t.Errorf("expected next = 0 (X's turn), got %f", state["next"])
	}

	// Run ODE
	prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
	opts := solver.DefaultOptions()
	opts.Abstol = 1e-4
	opts.Reltol = 1e-3
	opts.Dt = 0.1

	sol := solver.Solve(prob, solver.Tsit5(), opts)
	finalState := sol.GetFinalState()

	t.Logf("Final state - WinX: %.3f, WinO: %.3f", finalState["winX"], finalState["winO"])

	// With all rates equal and symmetric initial conditions,
	// some tokens should flow through the system
	totalMoved := 0.0
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			xHist := fmt.Sprintf("x%d%d", i, j)
			oHist := fmt.Sprintf("o%d%d", i, j)
			totalMoved += finalState[xHist] + finalState[oHist]
		}
	}

	if totalMoved == 0 {
		t.Error("expected some token flow through the system")
	}

	t.Logf("Total tokens moved to history: %.3f", totalMoved)
}

// TestMoveEvaluation demonstrates evaluating candidate moves via ODE
func TestMoveEvaluation(t *testing.T) {
	schema, _ := dsl.SchemaFromStruct(TicTacToe{})
	model := mpetri.FromSchema(schema)
	net := model.ToPetriNet()
	rates := model.DefaultRates(1.0)

	// Evaluate each possible first move
	initialState := net.SetState(nil)

	type moveScore struct {
		row, col int
		score    float64
	}
	var scores []moveScore

	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			// Create hypothetical state after this move
			hypState := make(map[string]float64)
			for k, v := range initialState {
				hypState[k] = v
			}

			// X plays at (row, col)
			pos := fmt.Sprintf("p%d%d", row, col)
			xHist := fmt.Sprintf("x%d%d", row, col)
			hypState[pos] = 0    // Position taken
			hypState[xHist] = 1  // X played here
			hypState["next"] = 1 // O's turn

			// Solve ODE
			prob := solver.NewProblem(net, hypState, [2]float64{0, 3.0}, rates)
			opts := solver.FastOptions()
			sol := solver.Solve(prob, solver.Tsit5(), opts)
			finalState := sol.GetFinalState()

			// Score: X wins - O wins
			score := finalState["winX"] - finalState["winO"]
			scores = append(scores, moveScore{row, col, score})
		}
	}

	// Log all scores
	for _, s := range scores {
		t.Logf("Move (%d,%d): score = %.4f", s.row, s.col, s.score)
	}

	// Find best move
	best := scores[0]
	for _, s := range scores[1:] {
		if s.score > best.score {
			best = s
		}
	}

	t.Logf("Best move: (%d,%d) with score %.4f", best.row, best.col, best.score)

	// Center (1,1) should be among the best moves for X
	centerScore := scores[4].score // (1,1) is index 4
	if centerScore < best.score-0.01 {
		t.Logf("Note: center is not the best move (score=%.4f vs best=%.4f)", centerScore, best.score)
	}
}

// BenchmarkODESingleEvaluation benchmarks a single ODE evaluation
func BenchmarkODESingleEvaluation(b *testing.B) {
	schema, _ := dsl.SchemaFromStruct(TicTacToe{})
	model := mpetri.FromSchema(schema)
	net := model.ToPetriNet()
	rates := model.DefaultRates(1.0)
	state := net.SetState(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
		opts := solver.FastOptions()
		_ = solver.Solve(prob, solver.Tsit5(), opts)
	}
}

// BenchmarkODEMoveEvaluation benchmarks evaluating all 9 possible first moves
func BenchmarkODEMoveEvaluation(b *testing.B) {
	schema, _ := dsl.SchemaFromStruct(TicTacToe{})
	model := mpetri.FromSchema(schema)
	net := model.ToPetriNet()
	rates := model.DefaultRates(1.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		initialState := net.SetState(nil)

		for row := 0; row < 3; row++ {
			for col := 0; col < 3; col++ {
				hypState := make(map[string]float64)
				for k, v := range initialState {
					hypState[k] = v
				}

				pos := fmt.Sprintf("p%d%d", row, col)
				xHist := fmt.Sprintf("x%d%d", row, col)
				hypState[pos] = 0
				hypState[xHist] = 1
				hypState["next"] = 1

				prob := solver.NewProblem(net, hypState, [2]float64{0, 3.0}, rates)
				opts := solver.FastOptions()
				_ = solver.Solve(prob, solver.Tsit5(), opts)
			}
		}
	}
}
