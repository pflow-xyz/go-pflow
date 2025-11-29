package sensitivity

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// createGameNet creates a net with competing win/lose outcomes
func createGameNet() (*petri.PetriNet, map[string]float64, map[string]float64) {
	net := petri.Build().
		Place("tokens", 10).
		Place("win", 0).
		Place("lose", 0).
		Transition("to_win").
		Transition("to_lose").
		Arc("tokens", "to_win", 1).
		Arc("to_win", "win", 1).
		Arc("tokens", "to_lose", 1).
		Arc("to_lose", "lose", 1).
		Done()

	state := net.SetState(nil)
	rates := map[string]float64{"to_win": 0.6, "to_lose": 0.4}

	return net, state, rates
}

func TestPlaceScorer(t *testing.T) {
	net, state, rates := createGameNet()

	prob := solver.NewProblem(net, state, [2]float64{0, 10}, rates)
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

	scorer := PlaceScorer("win")
	score := scorer(sol)

	if score <= 0 {
		t.Errorf("Expected positive score for wins, got %f", score)
	}
}

func TestDiffScorer(t *testing.T) {
	net, state, rates := createGameNet()

	prob := solver.NewProblem(net, state, [2]float64{0, 10}, rates)
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

	scorer := DiffScorer("win", "lose")
	score := scorer(sol)

	// With higher win rate, should be positive
	if score <= 0 {
		t.Errorf("Expected positive diff (win > lose), got %f", score)
	}
}

func TestFinalStateScorer(t *testing.T) {
	net, state, rates := createGameNet()

	prob := solver.NewProblem(net, state, [2]float64{0, 10}, rates)
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

	scorer := FinalStateScorer(func(final map[string]float64) float64 {
		return final["win"]*2 - final["lose"]
	})
	score := scorer(sol)

	if score <= 0 {
		t.Errorf("Expected positive custom score, got %f", score)
	}
}

func TestAnalyzeRates(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).
		WithTimeSpan(0, 10).
		WithOptions(solver.FastOptions())

	result := analyzer.AnalyzeRates()

	// Should have baseline
	if result.Baseline == 0 {
		t.Error("Baseline should not be zero")
	}

	// Should have scores for both transitions
	if _, ok := result.Scores["to_win"]; !ok {
		t.Error("Missing score for to_win")
	}
	if _, ok := result.Scores["to_lose"]; !ok {
		t.Error("Missing score for to_lose")
	}

	// Disabling to_win should hurt (negative impact)
	if result.Impact["to_win"] >= 0 {
		t.Errorf("Disabling to_win should have negative impact, got %f", result.Impact["to_win"])
	}

	// Disabling to_lose should help (positive impact)
	if result.Impact["to_lose"] <= 0 {
		t.Errorf("Disabling to_lose should have positive impact, got %f", result.Impact["to_lose"])
	}

	// Should have ranking
	if len(result.Ranking) != 2 {
		t.Errorf("Expected 2 ranked params, got %d", len(result.Ranking))
	}
}

func TestAnalyzeRatesParallel(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 10)

	result := analyzer.AnalyzeRatesParallel()

	// Should match sequential results
	seqResult := analyzer.AnalyzeRates()

	if math.Abs(result.Baseline-seqResult.Baseline) > 0.001 {
		t.Error("Parallel baseline doesn't match sequential")
	}
}

func TestSweepRate(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 10)

	values := []float64{0, 0.25, 0.5, 0.75, 1.0}
	result := analyzer.SweepRate("to_win", values)

	if len(result.Scores) != 5 {
		t.Errorf("Expected 5 scores, got %d", len(result.Scores))
	}

	// Higher to_win rate should give higher score
	if result.Scores[4] <= result.Scores[0] {
		t.Error("Higher to_win rate should improve score")
	}

	// Best should be highest rate
	if result.Best.Value != 1.0 {
		t.Errorf("Best value should be 1.0, got %f", result.Best.Value)
	}
}

func TestSweepRateRange(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 10)

	result := analyzer.SweepRateRange("to_win", 0, 1, 5)

	if len(result.Scores) != 5 {
		t.Errorf("Expected 5 scores, got %d", len(result.Scores))
	}

	// Check values are evenly spaced
	expected := []float64{0, 0.25, 0.5, 0.75, 1.0}
	for i, v := range result.Values {
		if math.Abs(v-expected[i]) > 0.001 {
			t.Errorf("Value %d: expected %f, got %f", i, expected[i], v)
		}
	}
}

func TestGradient(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 10)

	// Gradient of to_win should be positive (increasing rate improves score)
	gradWin := analyzer.Gradient("to_win", 0.01)
	if gradWin <= 0 {
		t.Errorf("Gradient of to_win should be positive, got %f", gradWin)
	}

	// Gradient of to_lose should be negative (increasing rate hurts score)
	gradLose := analyzer.Gradient("to_lose", 0.01)
	if gradLose >= 0 {
		t.Errorf("Gradient of to_lose should be negative, got %f", gradLose)
	}
}

func TestAllGradients(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 10)

	gradients := analyzer.AllGradients(0.01)

	if len(gradients) != 2 {
		t.Errorf("Expected 2 gradients, got %d", len(gradients))
	}

	if _, ok := gradients["to_win"]; !ok {
		t.Error("Missing gradient for to_win")
	}
}

func TestAllGradientsParallel(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 10)

	gradients := analyzer.AllGradientsParallel(0.01)

	if len(gradients) != 2 {
		t.Errorf("Expected 2 gradients, got %d", len(gradients))
	}
}

func TestGridSearch(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 5)

	grid := NewGridSearch(analyzer).
		AddParameter("to_win", []float64{0.3, 0.6, 0.9}).
		AddParameter("to_lose", []float64{0.2, 0.4})

	result := grid.Run()

	// Should have 3 * 2 = 6 combinations
	if len(result.Combinations) != 6 {
		t.Errorf("Expected 6 combinations, got %d", len(result.Combinations))
	}

	if len(result.Scores) != 6 {
		t.Errorf("Expected 6 scores, got %d", len(result.Scores))
	}

	// Best should have high to_win, low to_lose
	if result.Best.Parameters["to_win"] != 0.9 {
		t.Errorf("Best to_win should be 0.9, got %f", result.Best.Parameters["to_win"])
	}
	if result.Best.Parameters["to_lose"] != 0.2 {
		t.Errorf("Best to_lose should be 0.2, got %f", result.Best.Parameters["to_lose"])
	}
}

func TestGridSearchAddParameterRange(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := PlaceScorer("win")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 5)

	grid := NewGridSearch(analyzer).
		AddParameterRange("to_win", 0, 1, 3) // 0, 0.5, 1.0

	result := grid.Run()

	if len(result.Combinations) != 3 {
		t.Errorf("Expected 3 combinations, got %d", len(result.Combinations))
	}
}

func TestRanking(t *testing.T) {
	net, state, rates := createGameNet()

	scorer := DiffScorer("win", "lose")
	analyzer := NewAnalyzer(net, state, rates, scorer).WithTimeSpan(0, 10)

	result := analyzer.AnalyzeRates()

	// Ranking should be sorted by absolute impact
	if len(result.Ranking) < 2 {
		t.Fatal("Need at least 2 ranked params")
	}

	// First should have higher absolute impact than second
	if math.Abs(result.Ranking[0].Impact) < math.Abs(result.Ranking[1].Impact) {
		t.Error("Ranking not sorted by absolute impact")
	}
}
