package solver

import (
	"math"
	"testing"
)

// The Tsit5 tableau's Bhat is the difference b - b̂ between the 5th-order
// solution weights and the embedded 4th-order weights (OrdinaryDiffEq.jl's
// btilde convention; Tsitouras 2011 tabulates the same differences). Two
// facts pin it: the difference of two consistent weight vectors sums to
// zero, and b - Bhat must itself be a 4th-order quadrature rule on the nodes
// C, i.e. sum_i b̂_i c_i^p = 1/(p+1) for p = 0..3. Until 2026-09-02 the last
// entry was +1/66 instead of -1/66, which violated both.

func TestTsit5BhatSumsToZero(t *testing.T) {
	s := Tsit5()
	sum := 0.0
	for _, w := range s.Bhat {
		sum += w
	}
	if math.Abs(sum) > 1e-15 {
		t.Fatalf("sum(Bhat) = %g, want 0 (|sum| <= 1e-15)", sum)
	}
}

func TestTsit5EmbeddedWeightsAreFourthOrder(t *testing.T) {
	s := Tsit5()
	if len(s.B) != len(s.Bhat) || len(s.C) != len(s.B) {
		t.Fatalf("tableau lengths B=%d Bhat=%d C=%d", len(s.B), len(s.Bhat), len(s.C))
	}
	// b̂ = b - Bhat: the embedded 4th-order weights, recovered from the
	// difference vector. Source: Tsitouras, CAMWA 62 (2011) 770-775, Table 1,
	// as transcribed by OrdinaryDiffEq.jl (tsit_tableaus.jl, btilde = b - b̂).
	bhat := make([]float64, len(s.B))
	for i := range s.B {
		bhat[i] = s.B[i] - s.Bhat[i]
	}
	for p := 0; p <= 3; p++ {
		got := 0.0
		for i := range bhat {
			got += bhat[i] * math.Pow(s.C[i], float64(p))
		}
		want := 1.0 / float64(p+1)
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("4th-order quadrature condition p=%d: sum b̂_i c_i^p = %.15g, want %.15g", p, got, want)
		}
	}
	// And it must NOT satisfy the 5th-order condition: if it did, the pair
	// would carry no error information at all.
	got := 0.0
	for i := range bhat {
		got += bhat[i] * math.Pow(s.C[i], 4)
	}
	if math.Abs(got-0.2) < 1e-6 {
		t.Errorf("b̂ satisfies the p=4 condition (%.15g); embedded method is not distinct from the 5th-order one", got)
	}
}

// TestTsit5ErrorEstimateVanishesForConstantRHS: for u' = c every stage
// derivative equals c, so the estimate dt * sum(Bhat_j) * c must be ~0. A
// Bhat that does not sum to zero reports an O(dt) error on a problem every
// RK method integrates exactly.
func TestTsit5ErrorEstimateVanishesForConstantRHS(t *testing.T) {
	s := Tsit5()
	const c, dt = 3.0, 0.5
	est := 0.0
	for _, w := range s.Bhat {
		est += dt * w * c
	}
	if math.Abs(est) > 1e-14 {
		t.Fatalf("error estimate for constant RHS = %g, want ~0", est)
	}

	// Same fact end to end: with a huge Dt the controller must accept every
	// step at Dtmax for u' = 3, since there is no error to reject.
	f := func(_ float64, u []float64) []float64 { return []float64{c} }
	prob := NewVectorProblem([]string{"x"}, []float64{0}, [2]float64{0, 10}, f)
	opts := &Options{Dt: 1, Dtmin: 1e-6, Dtmax: 1, Abstol: 1e-12, Reltol: 1e-12, Maxiters: 1000, Adaptive: true}
	sol := Solve(prob, s, opts)
	if sol.Truncated {
		t.Fatal("constant-RHS solve truncated")
	}
	if steps := len(sol.T) - 1; steps != 10 {
		t.Errorf("constant RHS took %d steps at Dtmax=1 over [0,10], want 10", steps)
	}
	if got := sol.GetFinalState()["x"]; math.Abs(got-30) > 1e-9 {
		t.Errorf("x(10) = %g, want 30", got)
	}
}

// TestTsit5StepControlIsFifthOrder solves u' = -k u on [0,10] (k = 0.7,
// u(0) = 2) at reltol 1e-3 .. 1e-6 and checks that the accepted-step count
// grows by well under 3x per decade of tolerance. A 5th-order estimate
// gives ~10^(1/5) = 1.58x; the first-order estimate the +1/66 bug produced
// gave ~10x (measured 1.7k / 14k / 119k / 994k on a linear chain). The
// global error is also held to a small multiple of the tolerance.
func TestTsit5StepControlIsFifthOrder(t *testing.T) {
	const k, u0, tf = 0.7, 2.0, 10.0
	f := func(_ float64, u []float64) []float64 { return []float64{-k * u[0]} }
	want := u0 * math.Exp(-k*tf)

	tols := []float64{1e-3, 1e-4, 1e-5, 1e-6}
	steps := make([]int, len(tols))
	for i, tol := range tols {
		prob := NewVectorProblem([]string{"x"}, []float64{u0}, [2]float64{0, tf}, f)
		opts := &Options{Dt: 0.01, Dtmin: 1e-10, Dtmax: 100, Abstol: tol * 1e-3, Reltol: tol, Maxiters: 1000000, Adaptive: true}
		sol := Solve(prob, Tsit5(), opts)
		if sol.Truncated {
			t.Fatalf("reltol %g: truncated after %d steps", tol, len(sol.T)-1)
		}
		steps[i] = len(sol.T) - 1
		got := sol.GetFinalState()["x"]
		relErr := math.Abs(got-want) / want
		t.Logf("reltol %g: %d steps, final rel error %.3g", tol, steps[i], relErr)
		// Local tolerance tol per step over ~steps steps; global error is
		// bounded well within 10*tol for this contractive problem.
		if relErr > 10*tol {
			t.Errorf("reltol %g: global rel error %.3g exceeds 10*tol", tol, relErr)
		}
	}
	for i := 1; i < len(tols); i++ {
		ratio := float64(steps[i]) / float64(steps[i-1])
		if ratio > 3 {
			t.Errorf("step count grew %.2fx from reltol %g to %g (%d -> %d); first-order controller signature (5th order gives ~1.6x)",
				ratio, tols[i-1], tols[i], steps[i-1], steps[i])
		}
	}
}
