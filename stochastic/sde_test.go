package stochastic

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

func TestSDERefusesGatedModels(t *testing.T) {
	m := &metamodel.Model{
		Name: "gated",
		Places: []metamodel.Place{
			{ID: "a", Initial: 10, Capacity: 10},
			{ID: "licence", Initial: 1},
			{ID: "b"},
		},
		Transitions: []metamodel.Transition{{ID: "t", Rate: 1}},
		Arcs: []metamodel.Arc{
			{From: "a", To: "t"},
			{From: "licence", To: "t", Type: metamodel.ReadArc},
			{From: "t", To: "b"},
		},
	}
	res, err := SimulateSDE(m, nil, Options{Horizon: 1, Samples: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Diverged {
		t.Fatal("SDE did not refuse a model with a read arc")
	}
	if len(res.Caveats) == 0 {
		t.Error("Diverged with no Caveats naming why")
	}
}

func TestSDEDispatch(t *testing.T) {
	m := linearChain()
	res, err := Solve(m, nil, Options{Method: MethodSDE, Horizon: 1, Samples: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != string(MethodSDE) {
		t.Fatalf("Method = %q, want %q", res.Method, MethodSDE)
	}
	if len(res.Assumptions) != 1 || res.Assumptions[0] != ChemicalLangevinAssumption {
		t.Errorf("Assumptions = %v, want [ChemicalLangevinAssumption]", res.Assumptions)
	}
}

// TestSDEConsistencyLinearChain: the SDE mean must track the ODE reference —
// the same law-of-large-numbers relationship stochastic/consistency_test.go
// already holds SSA to, extended to the third engine. Weight-1 arcs only, so
// combinationsReal never needs its clamp: this case is about the SDE step
// itself, not the propensity's continuum generalization.
func TestSDEConsistencyLinearChain(t *testing.T) {
	m := linearChain()
	if !meanFieldExact(m) {
		t.Fatal("precondition: linear chain must be mean-field exact")
	}
	opts := Options{Horizon: 6, Samples: 61, Realizations: 400, Seed: consistencySeed, Method: MethodSDE}
	res, err := Solve(m, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Diverged {
		t.Fatalf("sde diverged: %s", res.Reason)
	}
	ref := odeReference(t, m, Options{Method: MethodODE, Horizon: opts.Horizon, Samples: opts.Samples})
	for _, s := range res.Series {
		// Same 1.0-token bound stochastic/consistency_test.go's SSA case
		// uses on this exact fixture (SD <= 5, SE over 400 realizations =
		// 0.25; 1.0 is 4 SE) — the SDE's diffusion approximates the same
		// CTMC, so its mean should be at least as close to the ODE as SSA's.
		if d := maxAbsDiff(t, s.Place, s.Values, ref[s.Place]); d > 1.0 {
			t.Errorf("%s: max|sde mean - ode| = %.3f (limit 1.0)", s.Place, d)
		}
	}

	// Conservation: a + b + c == 100 at every grid point. Unlike SSA (exact
	// integers), the SDE's clamp-at-zero and independent per-place noise
	// terms do NOT individually conserve token count within a single
	// realization — only stoichiometric coupling would guarantee that, and
	// this implementation gives each transition one shared noise draw
	// applied to every place its stoichiometry touches (matching the
	// standard chemical Langevin formulation), so conservation holds up to
	// the clamp's asymmetry. Check the ENSEMBLE MEAN instead, loosely.
	final := 0.0
	for _, s := range res.Series {
		final += s.Values[len(s.Values)-1]
	}
	if math.Abs(final-100) > 2.0 {
		t.Errorf("sum of means at horizon = %.3f, want ~100 (clamp-at-zero asymmetry bound)", final)
	}
}

// TestSDEVarianceMatchesSSAAtScale is the three-way half of the consistency
// gate: SDE's variance should approximate SSA's at large population, the
// same regime where both approximate the CTMC well. Uses the SIR fixture at
// x10 scale (sirModel(10)) — consistency_test.go's own Case 3 — for the
// least dispersion-heavy honest comparison available.
func TestSDEVarianceMatchesSSAAtScale(t *testing.T) {
	m := sirModel(10) // N = 10,000
	opts := Options{Horizon: 40, Samples: 81, Realizations: 100, Seed: consistencySeed}

	ssaOpts := opts
	ssaOpts.Method = MethodSSA
	ssa, err := Solve(m, nil, ssaOpts)
	if err != nil {
		t.Fatal(err)
	}

	sdeOpts := opts
	sdeOpts.Method = MethodSDE
	sde, err := Solve(m, nil, sdeOpts)
	if err != nil {
		t.Fatal(err)
	}
	if sde.Diverged {
		t.Fatalf("sde diverged: %s", sde.Reason)
	}

	ssaByPlace := seriesByPlace(ssa)
	for _, s := range sde.Series {
		want := ssaByPlace[s.Place]
		if len(want.StdDev) == 0 || len(s.StdDev) == 0 {
			t.Fatalf("%s: missing stdev on one side (ssa=%d sde=%d)", s.Place, len(want.StdDev), len(s.StdDev))
		}
		// Compare stdev at the grid point where SSA's own stdev peaks —
		// the informative moment, not an arbitrary one, and where relative
		// comparison is least sensitive to both being near zero.
		peak := 0
		for i, v := range want.StdDev {
			if v > want.StdDev[peak] {
				peak = i
			}
		}
		if want.StdDev[peak] < 1e-9 {
			continue // this place never varied; nothing to compare
		}
		rel := math.Abs(s.StdDev[peak]-want.StdDev[peak]) / want.StdDev[peak]
		if rel > 0.35 {
			t.Errorf("%s: stdev at peak (grid %d) sde=%.3f ssa=%.3f, relative diff %.2f (limit 0.35)",
				s.Place, peak, s.StdDev[peak], want.StdDev[peak], rel)
		}
	}
}

// TestSDEDimerisationDisagreesLikeSSA mirrors consistency_test.go's Case 4:
// combinationsReal generalizes SSA's exact combinatorics, so an SDE run on
// the same weight-2 dimerisation net should track SSA's mean, not the ODE's
// — proving the SDE propensity is really the continuum limit of C(m, w) and
// not accidentally computing solver's k*u^w instead.
func TestSDEDimerisationDisagreesFromODELikeSSA(t *testing.T) {
	m := dimerisation()
	if meanFieldExact(m) {
		t.Fatal("precondition: dimerisation must NOT be mean-field exact")
	}
	opts := Options{Horizon: 5, Samples: 51, Realizations: 300, Seed: consistencySeed}

	ssaOpts := opts
	ssaOpts.Method = MethodSSA
	ssa, err := Solve(m, nil, ssaOpts)
	if err != nil {
		t.Fatal(err)
	}
	sdeOpts := opts
	sdeOpts.Method = MethodSDE
	sde, err := Solve(m, nil, sdeOpts)
	if err != nil {
		t.Fatal(err)
	}
	if sde.Diverged {
		t.Fatalf("sde diverged: %s", sde.Reason)
	}
	ref := odeReference(t, m, Options{Method: MethodODE, Horizon: opts.Horizon, Samples: opts.Samples})

	ssaByPlace := seriesByPlace(ssa)
	for _, s := range sde.Series {
		wantSSA := ssaByPlace[s.Place].Values
		wantODE := ref[s.Place]
		dSDE := maxAbsDiff(t, s.Place, s.Values, wantSSA)
		dODE := maxAbsDiff(t, s.Place, s.Values, wantODE)
		// SDE should sit close to SSA (same propensity law) and clearly
		// farther from the ODE reference (a different rate law entirely),
		// the same disagreement consistency_test.go's Case 4 pins for SSA.
		if dSDE > 2.0 {
			t.Errorf("%s: max|sde - ssa| = %.3f, want close (both use C(m,w))", s.Place, dSDE)
		}
		if dODE < 1.0 {
			t.Errorf("%s: max|sde - ode| = %.3f, want > 1.0 (SDE should NOT agree with the ODE's k*u^w here)", s.Place, dODE)
		}
	}
}

func TestCombinationsRealAgreesWithCombinationsAtIntegers(t *testing.T) {
	for m := 0; m <= 10; m++ {
		for w := 0; w <= 4; w++ {
			want := combinations(m, w)
			got := combinationsReal(float64(m), w)
			if math.Abs(got-want) > 1e-9 {
				t.Errorf("combinationsReal(%d, %d) = %v, want %v (= combinations)", m, w, got, want)
			}
		}
	}
}

func TestCombinationsRealGoesNegativeBelowWMinus1(t *testing.T) {
	// The documented "wrong near zero" case combinationsReal's own comment
	// names: x=0.5, w=2 gives 0.5*(0.5-1)/2 = -0.125.
	if got := combinationsReal(0.5, 2); math.Abs(got-(-0.125)) > 1e-12 {
		t.Errorf("combinationsReal(0.5, 2) = %v, want -0.125", got)
	}
	// propensity() must clamp this to zero, not let a negative propensity
	// flip the sign of the noise term.
	tr := sdeTransition{rate: 1, terms: []sdeKineticTerm{{place: 0, weight: 2}}}
	if a := tr.propensity([]float64{0.5}); a != 0 {
		t.Errorf("propensity at x=0.5, w=2 = %v, want 0 (clamped)", a)
	}
}

func TestCombinationsRealWeight1IsIdentity(t *testing.T) {
	// The common case, and the one every unit-weight fixture in this
	// package relies on implicitly: weight 1 must reduce to x exactly, no
	// clamp ever engaging.
	for _, x := range []float64{0, 0.3, 1, 5.7, 100} {
		if got := combinationsReal(x, 1); got != x {
			t.Errorf("combinationsReal(%v, 1) = %v, want %v", x, got, x)
		}
	}
}
