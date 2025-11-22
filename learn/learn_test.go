package learn

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

func TestConstantRateFunc(t *testing.T) {
	rf := NewConstantRateFunc(0.5)

	state := map[string]float64{"A": 10.0, "B": 5.0}

	// Should always return the constant rate
	if rate := rf.Eval(state, 0.0); rate != 0.5 {
		t.Errorf("Expected rate 0.5, got %f", rate)
	}

	if rate := rf.Eval(state, 100.0); rate != 0.5 {
		t.Errorf("Expected rate 0.5, got %f", rate)
	}

	// Should have no parameters
	if rf.NumParams() != 0 {
		t.Errorf("Expected 0 params, got %d", rf.NumParams())
	}
}

func TestLinearRateFunc(t *testing.T) {
	// Linear model: k = θ₀ + θ₁*A + θ₂*B
	places := []string{"A", "B"}
	params := []float64{1.0, 0.1, 0.2} // bias=1, weight_A=0.1, weight_B=0.2

	rf := NewLinearRateFunc(places, params, false, false)

	if rf.NumParams() != 3 {
		t.Errorf("Expected 3 params, got %d", rf.NumParams())
	}

	state := map[string]float64{"A": 10.0, "B": 5.0}

	// Expected: 1.0 + 0.1*10 + 0.2*5 = 1 + 1 + 1 = 3.0
	rate := rf.Eval(state, 0.0)
	if math.Abs(rate-3.0) > 1e-6 {
		t.Errorf("Expected rate 3.0, got %f", rate)
	}
}

func TestLinearRateFuncWithReLU(t *testing.T) {
	places := []string{"A"}
	params := []float64{-1.0, 0.5} // bias=-1, weight=0.5

	rf := NewLinearRateFunc(places, params, true, false)

	// With A=1: k = -1 + 0.5*1 = -0.5, ReLU -> 0
	state := map[string]float64{"A": 1.0}
	rate := rf.Eval(state, 0.0)
	if rate != 0.0 {
		t.Errorf("Expected rate 0.0 (ReLU), got %f", rate)
	}

	// With A=5: k = -1 + 0.5*5 = 1.5, ReLU -> 1.5
	state["A"] = 5.0
	rate = rf.Eval(state, 0.0)
	if math.Abs(rate-1.5) > 1e-6 {
		t.Errorf("Expected rate 1.5, got %f", rate)
	}
}

func TestLinearRateFuncTimeDependent(t *testing.T) {
	places := []string{"A"}
	params := []float64{1.0, 0.1, 0.05} // bias=1, weight_A=0.1, weight_t=0.05

	rf := NewLinearRateFunc(places, params, false, true)

	if rf.NumParams() != 3 {
		t.Errorf("Expected 3 params (including time), got %d", rf.NumParams())
	}

	state := map[string]float64{"A": 10.0}

	// At t=0: k = 1.0 + 0.1*10 + 0.05*0 = 2.0
	rate := rf.Eval(state, 0.0)
	if math.Abs(rate-2.0) > 1e-6 {
		t.Errorf("Expected rate 2.0 at t=0, got %f", rate)
	}

	// At t=10: k = 1.0 + 0.1*10 + 0.05*10 = 2.5
	rate = rf.Eval(state, 10.0)
	if math.Abs(rate-2.5) > 1e-6 {
		t.Errorf("Expected rate 2.5 at t=10, got %f", rate)
	}
}

func TestLinearRateFuncSetParams(t *testing.T) {
	places := []string{"A"}
	params := []float64{1.0, 0.1}

	rf := NewLinearRateFunc(places, params, false, false)

	// Change parameters
	newParams := []float64{2.0, 0.5}
	rf.SetParams(newParams)

	gotParams := rf.GetParams()
	if len(gotParams) != 2 {
		t.Errorf("Expected 2 params, got %d", len(gotParams))
	}
	if gotParams[0] != 2.0 || gotParams[1] != 0.5 {
		t.Errorf("Expected params [2.0, 0.5], got %v", gotParams)
	}

	// Verify new parameters are used
	state := map[string]float64{"A": 10.0}
	rate := rf.Eval(state, 0.0)
	// Expected: 2.0 + 0.5*10 = 7.0
	if math.Abs(rate-7.0) > 1e-6 {
		t.Errorf("Expected rate 7.0 with new params, got %f", rate)
	}
}

func TestMLPRateFunc(t *testing.T) {
	places := []string{"A"}
	hiddenSize := 2

	rf := NewMLPRateFunc(places, hiddenSize, "relu", true, false)

	// Should have parameters for W1, b1, W2, b2
	// W1: 2x1=2, b1: 2, W2: 2, b2: 1 -> total: 7
	expectedParams := hiddenSize*1 + hiddenSize + hiddenSize + 1
	if rf.NumParams() != expectedParams {
		t.Errorf("Expected %d params, got %d", expectedParams, rf.NumParams())
	}

	// Just verify it runs without error
	state := map[string]float64{"A": 10.0}
	rate := rf.Eval(state, 0.0)

	// Should be non-negative due to ReLU on output
	if rate < 0 {
		t.Errorf("Expected non-negative rate with output ReLU, got %f", rate)
	}
}

func TestMLPRateFuncTanh(t *testing.T) {
	places := []string{"A", "B"}
	hiddenSize := 3

	rf := NewMLPRateFunc(places, hiddenSize, "tanh", false, false)

	// W1: 3x2=6, b1: 3, W2: 3, b2: 1 -> total: 13
	expectedParams := hiddenSize*2 + hiddenSize + hiddenSize + 1
	if rf.NumParams() != expectedParams {
		t.Errorf("Expected %d params, got %d", expectedParams, rf.NumParams())
	}

	// Verify it runs
	state := map[string]float64{"A": 5.0, "B": 3.0}
	_ = rf.Eval(state, 0.0)
}

func TestNewLearnableProblem(t *testing.T) {
	// Simple A -> B network
	net := petri.NewPetriNet()
	net.AddPlace("A", 100.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	initialState := map[string]float64{"A": 100.0, "B": 0.0}

	// Create learnable rate
	rf := NewConstantRateFunc(0.1)
	rateFuncs := map[string]RateFunc{"convert": rf}

	prob := NewLearnableProblem(net, initialState, [2]float64{0, 10}, rateFuncs)

	if prob.Net != net {
		t.Error("Net not set correctly")
	}
	if prob.NumParams() != 0 {
		t.Errorf("Expected 0 params (constant rate), got %d", prob.NumParams())
	}
}

func TestLearnableProblemSolve(t *testing.T) {
	// Simple A -> B with learnable linear rate
	net := petri.NewPetriNet()
	net.AddPlace("A", 100.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	initialState := map[string]float64{"A": 100.0, "B": 0.0}

	// Create learnable rate: k = 0.1 (bias-only model, state-independent)
	rf := NewLinearRateFunc([]string{}, []float64{0.1}, false, false)
	rateFuncs := map[string]RateFunc{"convert": rf}

	prob := NewLearnableProblem(net, initialState, [2]float64{0, 50}, rateFuncs)

	// Solve
	sol := prob.Solve(solver.Tsit5(), solver.DefaultOptions())

	if len(sol.T) == 0 {
		t.Fatal("Solution has no time points")
	}

	// Check conservation
	finalState := sol.GetFinalState()
	total := finalState["A"] + finalState["B"]
	if math.Abs(total-100.0) > 0.1 {
		t.Errorf("Conservation violated: total=%.2f", total)
	}

	// Check that conversion happened
	if finalState["B"] < 50.0 {
		t.Errorf("Expected significant conversion, got B=%.2f", finalState["B"])
	}
}

func TestLearnableProblemGetSetParams(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("A", 100.0, nil, 0, 0, nil)
	net.AddTransition("t1", "default", 0, 0, nil)
	net.AddTransition("t2", "default", 0, 0, nil)

	initialState := map[string]float64{"A": 100.0}

	// Two learnable rates
	rf1 := NewLinearRateFunc([]string{"A"}, []float64{1.0, 0.1}, false, false)
	rf2 := NewLinearRateFunc([]string{"A"}, []float64{2.0, 0.2}, false, false)
	rateFuncs := map[string]RateFunc{"t1": rf1, "t2": rf2}

	prob := NewLearnableProblem(net, initialState, [2]float64{0, 10}, rateFuncs)

	// Should have 4 total params
	if prob.NumParams() != 4 {
		t.Errorf("Expected 4 params, got %d", prob.NumParams())
	}

	// Get all params
	params, indices := prob.GetAllParams()
	if len(params) != 4 {
		t.Errorf("Expected 4 params, got %d", len(params))
	}

	// Modify and set
	newParams := []float64{5.0, 0.5, 3.0, 0.3}
	prob.SetAllParams(newParams, indices)

	// Verify changes
	params2, _ := prob.GetAllParams()
	for i, v := range newParams {
		if math.Abs(params2[i]-v) > 1e-6 {
			t.Errorf("Param %d: expected %.2f, got %.2f", i, v, params2[i])
		}
	}
}

func TestDatasetCreation(t *testing.T) {
	times := []float64{0, 1, 2, 3}
	obs := map[string][]float64{
		"A": {100, 90, 80, 70},
		"B": {0, 10, 20, 30},
	}

	data, err := NewDataset(times, obs)
	if err != nil {
		t.Fatalf("Error creating dataset: %v", err)
	}

	if len(data.Times) != 4 {
		t.Errorf("Expected 4 time points, got %d", len(data.Times))
	}

	if len(data.Observations) != 2 {
		t.Errorf("Expected 2 observed variables, got %d", len(data.Observations))
	}
}

func TestDatasetValidation(t *testing.T) {
	// Mismatched lengths should error
	times := []float64{0, 1, 2}
	obs := map[string][]float64{
		"A": {100, 90}, // Too short
	}

	_, err := NewDataset(times, obs)
	if err == nil {
		t.Error("Expected error for mismatched lengths")
	}

	// Empty times should error
	_, err = NewDataset([]float64{}, obs)
	if err == nil {
		t.Error("Expected error for empty times")
	}
}

func TestMSELoss(t *testing.T) {
	// Create a simple solution
	sol := &solver.Solution{
		T: []float64{0, 1, 2},
		U: []map[string]float64{
			{"A": 100.0, "B": 0.0},
			{"A": 50.0, "B": 50.0},
			{"A": 0.0, "B": 100.0},
		},
		StateLabels: []string{"A", "B"},
	}

	// Perfect match
	times := []float64{0, 1, 2}
	obs := map[string][]float64{
		"A": {100, 50, 0},
		"B": {0, 50, 100},
	}
	data, _ := NewDataset(times, obs)

	loss := MSELoss(sol, data)
	if loss > 1e-6 {
		t.Errorf("Expected near-zero loss for perfect match, got %f", loss)
	}

	// With error
	obs2 := map[string][]float64{
		"A": {100, 60, 10}, // Error: [0, 10, 10]
		"B": {0, 40, 90},   // Error: [0, 10, 10]
	}
	data2, _ := NewDataset(times, obs2)

	loss2 := MSELoss(sol, data2)
	// MSE = (0^2 + 10^2 + 10^2 + 0^2 + 10^2 + 10^2) / 6 = 400/6 = 66.67
	expected := 400.0 / 6.0
	if math.Abs(loss2-expected) > 0.01 {
		t.Errorf("Expected MSE=%.2f, got %.2f", expected, loss2)
	}
}

func TestInterpolateSolution(t *testing.T) {
	sol := &solver.Solution{
		T: []float64{0, 1, 2},
		U: []map[string]float64{
			{"A": 0.0},
			{"A": 10.0},
			{"A": 20.0},
		},
		StateLabels: []string{"A"},
	}

	// Test at exact points
	times := []float64{0, 1, 2}
	values := InterpolateSolution(sol, times, "A")

	expected := []float64{0, 10, 20}
	for i, v := range expected {
		if math.Abs(values[i]-v) > 1e-6 {
			t.Errorf("At t=%f: expected %f, got %f", times[i], v, values[i])
		}
	}

	// Test interpolation
	times = []float64{0.5, 1.5}
	values = InterpolateSolution(sol, times, "A")

	expected = []float64{5.0, 15.0}
	for i, v := range expected {
		if math.Abs(values[i]-v) > 1e-6 {
			t.Errorf("At t=%f: expected %f, got %f", times[i], v, values[i])
		}
	}
}

func TestGenerateUniformTimes(t *testing.T) {
	times := GenerateUniformTimes(0, 10, 11)

	if len(times) != 11 {
		t.Errorf("Expected 11 points, got %d", len(times))
	}

	// Check endpoints
	if times[0] != 0 {
		t.Errorf("Expected first time=0, got %f", times[0])
	}
	if times[10] != 10 {
		t.Errorf("Expected last time=10, got %f", times[10])
	}

	// Check spacing
	for i := 1; i < len(times); i++ {
		dt := times[i] - times[i-1]
		if math.Abs(dt-1.0) > 1e-6 {
			t.Errorf("Expected dt=1.0, got %f", dt)
		}
	}
}

func TestFitSimpleRecovery(t *testing.T) {
	// Test that we can recover a known rate from synthetic data
	// A -> B with true rate k=0.2

	// Generate synthetic data
	net := petri.NewPetriNet()
	net.AddPlace("A", 100.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	initialState := map[string]float64{"A": 100.0, "B": 0.0}
	trueRates := map[string]float64{"convert": 0.2}

	// Generate true solution
	trueProb := solver.NewProblem(net, initialState, [2]float64{0, 20}, trueRates)
	trueSol := solver.Solve(trueProb, solver.Tsit5(), solver.DefaultOptions())

	// Create dataset from true solution
	times := GenerateUniformTimes(0, 20, 11)
	obsA := InterpolateSolution(trueSol, times, "A")
	obsB := InterpolateSolution(trueSol, times, "B")
	data, _ := NewDataset(times, map[string][]float64{"A": obsA, "B": obsB})

	// Create learnable problem with initial guess
	rf := NewLinearRateFunc([]string{}, []float64{0.1}, false, false) // Start with k=0.1
	learnProb := NewLearnableProblem(net, initialState, [2]float64{0, 20},
		map[string]RateFunc{"convert": rf})

	// Fit
	opts := &FitOptions{
		MaxIters:      500,
		Tolerance:     1e-4,
		Method:        "nelder-mead",
		Verbose:       false,
		SolverMethod:  solver.Tsit5(),
		SolverOptions: solver.DefaultOptions(),
	}

	result, err := Fit(learnProb, data, MSELoss, opts)
	if err != nil {
		t.Fatalf("Fit error: %v", err)
	}

	// Check that we recovered approximately the true rate
	recoveredRate := result.Params[0]
	if math.Abs(recoveredRate-0.2) > 0.05 {
		t.Errorf("Expected to recover rate≈0.2, got %.4f", recoveredRate)
	}

	// Check that loss decreased
	if result.FinalLoss >= result.InitialLoss {
		t.Errorf("Loss did not decrease: initial=%.4f, final=%.4f",
			result.InitialLoss, result.FinalLoss)
	}

	// Final loss should be very small (near zero for perfect fit)
	if result.FinalLoss > 1.0 {
		t.Errorf("Final loss too high: %.4f", result.FinalLoss)
	}
}
