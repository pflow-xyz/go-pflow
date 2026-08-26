package solver

import (
	"math"
	"testing"
)

func TestNewVectorProblemExponentialDecay(t *testing.T) {
	k := 0.7
	f := func(_ float64, u []float64) []float64 {
		return []float64{-k * u[0]}
	}
	prob := NewVectorProblem([]string{"x"}, []float64{2.0}, [2]float64{0, 5}, f)
	sol := Solve(prob, nil, AccurateOptions())

	if sol.Truncated {
		t.Fatal("solve unexpectedly truncated")
	}
	series := sol.GetVariable("x")
	if len(series) != len(sol.T) {
		t.Fatalf("series length %d != %d time points", len(series), len(sol.T))
	}
	for i, tt := range sol.T {
		want := 2.0 * math.Exp(-k*tt)
		got := series[i]
		if math.Abs(got-want) > 1e-6*(1+math.Abs(want)) {
			t.Fatalf("at t=%.4f: got %.10f, want %.10f", tt, got, want)
		}
	}
	final := sol.GetFinalState()["x"]
	want := 2.0 * math.Exp(-k*5)
	if math.Abs(final-want) > 1e-6 {
		t.Errorf("final state: got %.10f, want %.10f", final, want)
	}
}

func TestNewVectorProblemTruncated(t *testing.T) {
	f := func(_ float64, u []float64) []float64 {
		return []float64{-u[0]}
	}
	prob := NewVectorProblem([]string{"x"}, []float64{1.0}, [2]float64{0, 100}, f)
	opts := DefaultOptions()
	opts.Maxiters = 5
	sol := Solve(prob, nil, opts)
	if !sol.Truncated {
		t.Error("expected Truncated after Maxiters exhaustion")
	}
}

func TestNewSolutionRoundTrip(t *testing.T) {
	times := []float64{0, 1, 2}
	u := []map[string]float64{
		{"a": 1.0, "b": 0.0},
		{"a": 0.5, "b": 0.5},
		{"a": 0.25, "b": 0.75},
	}
	sol := NewSolution(times, u, []string{"a", "b"}, true, nil)

	if !sol.Truncated {
		t.Error("Truncated not propagated")
	}
	a := sol.GetVariable("a")
	wantA := []float64{1.0, 0.5, 0.25}
	for i := range wantA {
		if a[i] != wantA[i] {
			t.Errorf("GetVariable(a)[%d] = %v, want %v", i, a[i], wantA[i])
		}
	}
	final := sol.GetFinalState()
	if final["a"] != 0.25 || final["b"] != 0.75 {
		t.Errorf("GetFinalState = %v", final)
	}
	if sol.ColorMap() != nil {
		t.Error("expected nil ColorMap")
	}
}

// TestNewVectorProblemImplicitEuler verifies that a vector problem carries a
// working hashmap F, so the F-consuming entry points (ImplicitEuler,
// SolveImplicit, FindEquilibrium) work instead of nil-func panicking.
func TestNewVectorProblemImplicitEuler(t *testing.T) {
	k := 0.7
	f := func(_ float64, u []float64) []float64 {
		return []float64{-k * u[0]}
	}
	prob := NewVectorProblem([]string{"x"}, []float64{2.0}, [2]float64{0, 5}, f)

	sol := ImplicitEuler(prob, DefaultOptions())
	if sol.Truncated {
		t.Fatal("implicit solve unexpectedly truncated")
	}
	final := sol.GetFinalState()["x"]
	want := 2.0 * math.Exp(-k*5)
	// Implicit Euler is first order: loose tolerance.
	if math.Abs(final-want) > 1e-2 {
		t.Errorf("implicit final state: got %.10f, want %.10f", final, want)
	}

	// FindEquilibrium also consumes F; give it a horizon long enough for the
	// decay to settle.
	longProb := NewVectorProblem([]string{"x"}, []float64{2.0}, [2]float64{0, 60}, f)
	if eq, ok := FindEquilibrium(longProb); !ok || math.Abs(eq["x"]) > 1e-2 {
		t.Errorf("FindEquilibrium: got %v (ok=%v), want x near 0", eq, ok)
	}
}
