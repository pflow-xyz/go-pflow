package learn

import (
	"math"
	"testing"
)

func TestMinimizeQuadratic(t *testing.T) {
	f := func(x []float64) float64 {
		return (x[0]-3)*(x[0]-3) + 2*(x[1]+1)*(x[1]+1)
	}
	res, err := Minimize(f, []float64{0, 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Params[0]-3) > 1e-2 || math.Abs(res.Params[1]+1) > 1e-2 {
		t.Fatalf("minimum not found: %v (loss %.6f)", res.Params, res.FinalLoss)
	}
	if res.FinalLoss > res.InitialLoss {
		t.Fatalf("loss increased: %.6f -> %.6f", res.InitialLoss, res.FinalLoss)
	}
}

func TestMinimizeCoordinateDescent(t *testing.T) {
	f := func(x []float64) float64 { return math.Abs(x[0] - 1) }
	opts := DefaultFitOptions()
	opts.Method = "coordinate-descent"
	res, err := Minimize(f, []float64{5}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Params[0]-1) > 0.1 {
		t.Fatalf("minimum not found: %v", res.Params)
	}
}

func TestMinimizeRejectsEmpty(t *testing.T) {
	if _, err := Minimize(func([]float64) float64 { return 0 }, nil, nil); err == nil {
		t.Fatal("empty parameter vector not rejected")
	}
}

func TestHingeRankLoss(t *testing.T) {
	dec := func(scores []float64, pref []bool) RankedDecision {
		return RankedDecision{Scores: scores, Preferred: pref}
	}
	// preferred wins by more than margin: zero
	if l := HingeRankLoss([]RankedDecision{dec([]float64{1, 0}, []bool{true, false})}, 0.1); l != 0 {
		t.Fatalf("clear win scored %.6f, want 0", l)
	}
	// preferred loses: violation = margin + gap
	if l := HingeRankLoss([]RankedDecision{dec([]float64{0, 1}, []bool{true, false})}, 0.1); math.Abs(l-1.1) > 1e-12 {
		t.Fatalf("loss = %.6f, want 1.1", l)
	}
	// only the BEST preferred needs to win
	if l := HingeRankLoss([]RankedDecision{dec([]float64{0, 5, 1}, []bool{true, true, false})}, 0.1); l != 0 {
		t.Fatalf("best-preferred rule violated: %.6f", l)
	}
	// nothing to rank: zero, not a crash
	all := dec([]float64{1, 2}, []bool{true, true})
	none := dec([]float64{1, 2}, []bool{false, false})
	if l := HingeRankLoss([]RankedDecision{all, none}, 0.1); l != 0 {
		t.Fatalf("degenerate decisions scored %.6f", l)
	}
	// decisions sum
	two := []RankedDecision{
		dec([]float64{0, 1}, []bool{true, false}),
		dec([]float64{0, 2}, []bool{true, false}),
	}
	if l := HingeRankLoss(two, 0); math.Abs(l-3) > 1e-12 {
		t.Fatalf("sum = %.6f, want 3", l)
	}
}
