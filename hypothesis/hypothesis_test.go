package hypothesis

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Helper to create a simple test net: A -> B with configurable rate
func createSimpleNet() (*petri.PetriNet, map[string]float64) {
	net := petri.NewPetriNet()
	net.AddPlace("A", 10.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	rates := map[string]float64{"convert": 1.0}
	return net, rates
}

// Helper to create a game-like net with two competing outcomes
func createGameNet() (*petri.PetriNet, map[string]float64) {
	net := petri.NewPetriNet()
	// Resources
	net.AddPlace("tokens", 10.0, nil, 0, 0, nil)
	// Outcomes
	net.AddPlace("win", 0.0, nil, 0, 0, nil)
	net.AddPlace("lose", 0.0, nil, 0, 0, nil)
	// Transitions
	net.AddTransition("to_win", "default", 0, 0, nil)
	net.AddTransition("to_lose", "default", 0, 0, nil)

	net.AddArc("tokens", "to_win", 1.0, false)
	net.AddArc("to_win", "win", 1.0, false)
	net.AddArc("tokens", "to_lose", 1.0, false)
	net.AddArc("to_lose", "lose", 1.0, false)

	rates := map[string]float64{"to_win": 0.6, "to_lose": 0.4}
	return net, rates
}

func TestNewEvaluator(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer)

	if eval.net != net {
		t.Error("Net not set")
	}
	if eval.scorer == nil {
		t.Error("Scorer not set")
	}
}

func TestEvaluate(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 10).
		WithOptions(solver.DefaultOptions())

	base := map[string]float64{"A": 10, "B": 0}
	score := eval.Evaluate(base, map[string]float64{"A": 20}) // Double the A tokens

	// With more A tokens, more should convert to B
	if score <= 0 {
		t.Errorf("Expected positive score, got %f", score)
	}
}

func TestEvaluateState(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 10)

	state := map[string]float64{"A": 10, "B": 0}
	score := eval.EvaluateState(state)

	if score <= 0 {
		t.Errorf("Expected positive score, got %f", score)
	}
}

func TestEarlyTermination(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).
		WithEarlyTermination(func(state map[string]float64) bool {
			return state["A"] < 0 // Negative tokens = infeasible
		}).
		WithInfeasibleScore(-999)

	// Feasible state
	feasibleScore := eval.EvaluateState(map[string]float64{"A": 10, "B": 0})
	if feasibleScore == -999 {
		t.Error("Feasible state should not return infeasible score")
	}

	// Infeasible state
	infeasibleScore := eval.EvaluateState(map[string]float64{"A": -1, "B": 0})
	if infeasibleScore != -999 {
		t.Errorf("Infeasible state should return -999, got %f", infeasibleScore)
	}
}

func TestEvaluateMany(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},  // Less A
		{"A": 10}, // Same A
		{"A": 20}, // More A
	}

	results := eval.EvaluateMany(base, updates)

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// More A should lead to higher score
	if results[2].Score <= results[0].Score {
		t.Errorf("More A should score higher: A=5 score=%f, A=20 score=%f",
			results[0].Score, results[2].Score)
	}
}

func TestEvaluateManyParallel(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},
		{"A": 10},
		{"A": 20},
	}

	results := eval.EvaluateManyParallel(base, updates)

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Verify indices are correct
	for i, r := range results {
		if r.Index != i {
			t.Errorf("Result %d has wrong index %d", i, r.Index)
		}
	}
}

func TestFindBest(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},  // Index 0: Less A
		{"A": 20}, // Index 1: More A (should be best)
		{"A": 10}, // Index 2: Same A
	}

	bestIdx, bestScore := eval.FindBest(base, updates)

	if bestIdx != 1 {
		t.Errorf("Expected best index 1, got %d", bestIdx)
	}
	if bestScore <= 0 {
		t.Errorf("Expected positive best score, got %f", bestScore)
	}

	// Empty updates
	emptyIdx, emptyScore := eval.FindBest(base, []map[string]float64{})
	if emptyIdx != -1 {
		t.Errorf("Empty updates should return -1, got %d", emptyIdx)
	}
	if !math.IsInf(emptyScore, -1) {
		t.Errorf("Empty updates should return -Inf, got %f", emptyScore)
	}
}

func TestFindBestParallel(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},
		{"A": 20}, // Should be best
		{"A": 10},
	}

	bestIdx, _ := eval.FindBestParallel(base, updates)

	if bestIdx != 1 {
		t.Errorf("Expected best index 1, got %d", bestIdx)
	}
}

func TestCompare(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	stateMore := map[string]float64{"A": 20, "B": 0}
	stateLess := map[string]float64{"A": 5, "B": 0}

	cmp := eval.Compare(stateMore, stateLess)
	if cmp != 1 {
		t.Errorf("More A should be better, got comparison %d", cmp)
	}

	cmp = eval.Compare(stateLess, stateMore)
	if cmp != -1 {
		t.Errorf("Less A should be worse, got comparison %d", cmp)
	}
}

func TestSensitivityAnalysis(t *testing.T) {
	net, rates := createGameNet()

	// Score = win - lose (higher is better)
	scorer := func(final map[string]float64) float64 {
		return final["win"] - final["lose"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 10)

	state := map[string]float64{"tokens": 10, "win": 0, "lose": 0}
	analysis := eval.SensitivityAnalysis(state)

	// Should have baseline and both transitions
	if _, ok := analysis["_baseline"]; !ok {
		t.Error("Missing baseline in sensitivity analysis")
	}
	if _, ok := analysis["to_win"]; !ok {
		t.Error("Missing to_win in sensitivity analysis")
	}
	if _, ok := analysis["to_lose"]; !ok {
		t.Error("Missing to_lose in sensitivity analysis")
	}

	// Disabling to_win should make score worse (more negative)
	// Disabling to_lose should make score better (more positive)
	if analysis["to_win"] >= analysis["_baseline"] {
		t.Error("Disabling to_win should worsen score")
	}
	if analysis["to_lose"] <= analysis["_baseline"] {
		t.Error("Disabling to_lose should improve score")
	}
}

func TestSensitivityImpact(t *testing.T) {
	net, rates := createGameNet()

	scorer := func(final map[string]float64) float64 {
		return final["win"] - final["lose"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 10)

	state := map[string]float64{"tokens": 10, "win": 0, "lose": 0}
	impact := eval.SensitivityImpact(state)

	// Should not have baseline
	if _, ok := impact["_baseline"]; ok {
		t.Error("Impact should not include baseline")
	}

	// Disabling to_win should have negative impact
	if impact["to_win"] >= 0 {
		t.Errorf("Disabling to_win should have negative impact, got %f", impact["to_win"])
	}

	// Disabling to_lose should have positive impact
	if impact["to_lose"] <= 0 {
		t.Errorf("Disabling to_lose should have positive impact, got %f", impact["to_lose"])
	}
}

func TestWithOptions(t *testing.T) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).
		WithOptions(solver.AccurateOptions()).
		WithTimeSpan(0, 1).
		WithInfeasibleScore(-1000)

	if eval.opts != solver.AccurateOptions() {
		// Note: This compares pointers, which won't match since AccurateOptions() creates new
		// Just verify opts is not nil
		if eval.opts == nil {
			t.Error("Options should not be nil")
		}
	}

	if eval.tspan[1] != 1 {
		t.Errorf("TimeSpan end should be 1, got %f", eval.tspan[1])
	}

	if eval.infeasibleScore != -1000 {
		t.Errorf("InfeasibleScore should be -1000, got %f", eval.infeasibleScore)
	}
}
