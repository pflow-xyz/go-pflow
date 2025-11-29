package solver

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

func TestNewProblem(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("p1", 10.0, nil, 0, 0, nil)
	net.AddPlace("p2", 0.0, nil, 0, 0, nil)
	net.AddTransition("t1", "default", 0, 0, nil)

	initialState := map[string]float64{"p1": 10.0, "p2": 0.0}
	rates := map[string]float64{"t1": 1.0}
	tspan := [2]float64{0, 10}

	prob := NewProblem(net, initialState, tspan, rates)

	if prob.Net != net {
		t.Error("Net not set correctly")
	}
	if prob.U0["p1"] != 10.0 {
		t.Errorf("Expected U0[p1]=10.0, got %f", prob.U0["p1"])
	}
	if prob.Tspan[0] != 0 || prob.Tspan[1] != 10 {
		t.Errorf("Expected Tspan=[0, 10], got %v", prob.Tspan)
	}
	if prob.Rates["t1"] != 1.0 {
		t.Errorf("Expected Rates[t1]=1.0, got %f", prob.Rates["t1"])
	}
	if prob.F == nil {
		t.Error("ODE function not initialized")
	}
	if len(prob.stateLabels) != 2 {
		t.Errorf("Expected 2 state labels, got %d", len(prob.stateLabels))
	}
}

func TestSolutionGetVariable(t *testing.T) {
	sol := &Solution{
		T: []float64{0, 1, 2},
		U: []map[string]float64{
			{"p1": 10.0, "p2": 0.0},
			{"p1": 5.0, "p2": 5.0},
			{"p1": 0.0, "p2": 10.0},
		},
		StateLabels: []string{"p1", "p2"},
	}

	// Test by string
	p1 := sol.GetVariable("p1")
	if len(p1) != 3 {
		t.Errorf("Expected 3 values, got %d", len(p1))
	}
	if p1[0] != 10.0 || p1[1] != 5.0 || p1[2] != 0.0 {
		t.Errorf("Expected [10, 5, 0], got %v", p1)
	}

	// Test by index
	p2 := sol.GetVariable(1)
	if len(p2) != 3 {
		t.Errorf("Expected 3 values, got %d", len(p2))
	}
	if p2[0] != 0.0 || p2[1] != 5.0 || p2[2] != 10.0 {
		t.Errorf("Expected [0, 5, 10], got %v", p2)
	}

	// Test invalid - nonexistent variables should return a slice with zeros
	invalid := sol.GetVariable("nonexistent")
	if invalid == nil {
		t.Error("Expected non-nil slice for nonexistent variable")
	}
	// The slice will have values (zeros from map lookup), so check they're all zero
	for i, v := range invalid {
		if v != 0.0 {
			t.Errorf("Expected 0.0 for nonexistent variable at index %d, got %f", i, v)
		}
	}
}

func TestSolutionGetFinalState(t *testing.T) {
	sol := &Solution{
		T: []float64{0, 1, 2},
		U: []map[string]float64{
			{"p1": 10.0},
			{"p1": 5.0},
			{"p1": 0.0},
		},
		StateLabels: []string{"p1"},
	}

	finalState := sol.GetFinalState()
	if finalState["p1"] != 0.0 {
		t.Errorf("Expected final p1=0.0, got %f", finalState["p1"])
	}

	// Test empty solution
	emptySol := &Solution{U: []map[string]float64{}}
	if emptySol.GetFinalState() != nil {
		t.Error("Expected nil for empty solution")
	}
}

func TestSolutionGetState(t *testing.T) {
	sol := &Solution{
		T: []float64{0, 1, 2},
		U: []map[string]float64{
			{"p1": 10.0},
			{"p1": 5.0},
			{"p1": 0.0},
		},
		StateLabels: []string{"p1"},
	}

	state := sol.GetState(1)
	if state["p1"] != 5.0 {
		t.Errorf("Expected p1=5.0 at index 1, got %f", state["p1"])
	}

	// Test invalid indices
	if sol.GetState(-1) != nil {
		t.Error("Expected nil for negative index")
	}
	if sol.GetState(10) != nil {
		t.Error("Expected nil for out of bounds index")
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Dt != 0.01 {
		t.Errorf("Expected Dt=0.01, got %f", opts.Dt)
	}
	if opts.Dtmin != 1e-6 {
		t.Errorf("Expected Dtmin=1e-6, got %f", opts.Dtmin)
	}
	if opts.Dtmax != 0.1 {
		t.Errorf("Expected Dtmax=0.1, got %f", opts.Dtmax)
	}
	if opts.Abstol != 1e-6 {
		t.Errorf("Expected Abstol=1e-6, got %f", opts.Abstol)
	}
	if opts.Reltol != 1e-3 {
		t.Errorf("Expected Reltol=1e-3, got %f", opts.Reltol)
	}
	if opts.Maxiters != 100000 {
		t.Errorf("Expected Maxiters=100000, got %d", opts.Maxiters)
	}
	if !opts.Adaptive {
		t.Error("Expected Adaptive=true")
	}
}

func TestTsit5(t *testing.T) {
	solver := Tsit5()

	if solver.Name != "Tsit5" {
		t.Errorf("Expected name 'Tsit5', got '%s'", solver.Name)
	}
	if solver.Order != 5 {
		t.Errorf("Expected order 5, got %d", solver.Order)
	}
	if len(solver.C) != 7 {
		t.Errorf("Expected 7 nodes, got %d", len(solver.C))
	}
	if len(solver.A) != 7 {
		t.Errorf("Expected 7 rows in A matrix, got %d", len(solver.A))
	}
	if len(solver.B) != 7 {
		t.Errorf("Expected 7 solution weights, got %d", len(solver.B))
	}
	if len(solver.Bhat) != 7 {
		t.Errorf("Expected 7 error weights, got %d", len(solver.Bhat))
	}
}

func TestSolveSimpleDecay(t *testing.T) {
	// Simple decay: A -> (no output)
	// dA/dt = -k*A
	// Solution: A(t) = A0 * exp(-k*t)
	net := petri.NewPetriNet()
	net.AddPlace("A", 100.0, nil, 0, 0, nil)
	net.AddTransition("decay", "default", 0, 0, nil)
	net.AddArc("A", "decay", 1.0, false)
	// No output arc - tokens are consumed

	initialState := map[string]float64{"A": 100.0}
	rates := map[string]float64{"decay": 0.1}
	tspan := [2]float64{0, 10}

	prob := NewProblem(net, initialState, tspan, rates)
	sol := Solve(prob, Tsit5(), DefaultOptions())

	// Check that we have a solution
	if len(sol.T) == 0 {
		t.Fatal("Solution has no time points")
	}
	if len(sol.U) == 0 {
		t.Fatal("Solution has no states")
	}

	// Check initial state
	if sol.U[0]["A"] != 100.0 {
		t.Errorf("Expected initial A=100.0, got %f", sol.U[0]["A"])
	}

	// Check that A is decreasing
	for i := 1; i < len(sol.U); i++ {
		if sol.U[i]["A"] > sol.U[i-1]["A"] {
			t.Errorf("A should be decreasing, but increased at step %d", i)
		}
	}

	// Check approximate exponential decay
	// A(10) ≈ 100 * exp(-0.1 * 10) ≈ 100 * exp(-1) ≈ 36.79
	finalA := sol.GetFinalState()["A"]
	expected := 100.0 * math.Exp(-1.0)
	relError := math.Abs(finalA-expected) / expected
	if relError > 0.01 { // 1% tolerance
		t.Errorf("Expected final A≈%.2f, got %.2f (rel error %.2f%%)",
			expected, finalA, relError*100)
	}
}

func TestSolveConservation(t *testing.T) {
	// Test conservation: A -> B
	// Total should be conserved
	net := petri.NewPetriNet()
	net.AddPlace("A", 100.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	initialState := map[string]float64{"A": 100.0, "B": 0.0}
	rates := map[string]float64{"convert": 0.1}
	tspan := [2]float64{0, 50} // Longer time to allow more complete conversion

	prob := NewProblem(net, initialState, tspan, rates)
	sol := Solve(prob, Tsit5(), DefaultOptions())

	// Check conservation at each step
	tolerance := 0.01
	for i, state := range sol.U {
		total := state["A"] + state["B"]
		if math.Abs(total-100.0) > tolerance {
			t.Errorf("Conservation violated at step %d: total=%.2f", i, total)
		}
	}

	// Check final state - most of A should have converted
	finalState := sol.GetFinalState()
	if finalState["A"] > 10.0 { // Most should be converted
		t.Errorf("Expected A to be mostly depleted, got %.2f", finalState["A"])
	}
	if finalState["B"] < 90.0 {
		t.Errorf("Expected B≈90+, got %.2f", finalState["B"])
	}
}

func TestSolveNonAdaptive(t *testing.T) {
	// Test non-adaptive stepping
	net := petri.NewPetriNet()
	net.AddPlace("A", 10.0, nil, 0, 0, nil)
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("A", "t", 1.0, false)

	initialState := map[string]float64{"A": 10.0}
	rates := map[string]float64{"t": 0.1}
	tspan := [2]float64{0, 1}

	prob := NewProblem(net, initialState, tspan, rates)
	opts := &Options{
		Dt:       0.1,
		Dtmin:    0.1,
		Dtmax:    0.1,
		Abstol:   1e-6,
		Reltol:   1e-3,
		Maxiters: 1000,
		Adaptive: false,
	}
	sol := Solve(prob, Tsit5(), opts)

	// With fixed dt=0.1 and tspan=[0,1], we expect ~11 points (0, 0.1, 0.2, ..., 1.0)
	if len(sol.T) < 10 || len(sol.T) > 12 {
		t.Errorf("Expected ~11 time points with fixed dt, got %d", len(sol.T))
	}
}

func TestSolveCatalysis(t *testing.T) {
	// Test A + B -> 2B (B catalyzes conversion of A)
	net := petri.NewPetriNet()
	net.AddPlace("A", 100.0, nil, 0, 0, nil)
	net.AddPlace("B", 1.0, nil, 0, 0, nil)
	net.AddTransition("catalyze", "default", 0, 0, nil)
	net.AddArc("A", "catalyze", 1.0, false)
	net.AddArc("B", "catalyze", 1.0, false)
	net.AddArc("catalyze", "B", 2.0, false)

	initialState := map[string]float64{"A": 100.0, "B": 1.0}
	rates := map[string]float64{"catalyze": 0.01}
	tspan := [2]float64{0, 50}

	prob := NewProblem(net, initialState, tspan, rates)
	sol := Solve(prob, Tsit5(), DefaultOptions())

	// Check that B increases (autocatalytic)
	if sol.U[0]["B"] >= sol.GetFinalState()["B"] {
		t.Error("B should increase over time (autocatalytic)")
	}

	// Check conservation: A + B should equal initial sum
	initialSum := 101.0
	finalState := sol.GetFinalState()
	finalSum := finalState["A"] + finalState["B"]
	if math.Abs(finalSum-initialSum) > 1.0 {
		t.Errorf("Conservation violated: initial sum=%.2f, final sum=%.2f",
			initialSum, finalSum)
	}
}

func TestCopyState(t *testing.T) {
	original := map[string]float64{"A": 1.0, "B": 2.0}
	copied := CopyState(original)

	// Check values match
	if copied["A"] != 1.0 || copied["B"] != 2.0 {
		t.Error("Copied state values don't match")
	}

	// Verify it's a deep copy
	copied["A"] = 999.0
	if original["A"] != 1.0 {
		t.Error("Modifying copy affected original - not a deep copy")
	}
}

func TestBuildODEFunction(t *testing.T) {
	// Simple test: A -> B with rate 1.0
	net := petri.NewPetriNet()
	net.AddPlace("A", 10.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	rates := map[string]float64{"convert": 1.0}
	f := buildODEFunction(net, rates)

	// At state A=10, B=0, flux should be 1.0*10=10
	// dA/dt = -10, dB/dt = +10
	state := map[string]float64{"A": 10.0, "B": 0.0}
	du := f(0, state)

	if math.Abs(du["A"]+10.0) > 0.001 {
		t.Errorf("Expected dA/dt=-10, got %f", du["A"])
	}
	if math.Abs(du["B"]-10.0) > 0.001 {
		t.Errorf("Expected dB/dt=+10, got %f", du["B"])
	}

	// Test with zero concentration - flux should be zero
	state = map[string]float64{"A": 0.0, "B": 0.0}
	du = f(0, state)
	if du["A"] != 0.0 || du["B"] != 0.0 {
		t.Errorf("Expected zero derivatives when A=0, got dA=%f, dB=%f", du["A"], du["B"])
	}
}
