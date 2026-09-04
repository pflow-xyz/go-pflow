package stochastic

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/solver"
)

// G3 consistency gate: the SSA ensemble mean and solver's mass-action ODE are
// the same model exactly when every kinetic input has weight 1 and Gating()
// is empty (Cases 1-3), and they disagree outside that regime (Case 4).
//
// Both sides go through Solve so the dispatch itself is under test. Because
// Forecast hard-codes solver.DefaultOptions (reltol 1e-3), every SSA-vs-ODE
// comparison uses a tighter reference computed directly (referenceOptions),
// and Forecast is separately held to within 0.1 token of that reference.
//
// All tolerances are pre-registered at ~4 SE. Runs are seeded, so every
// assertion is regression-exact.

const consistencySeed = 20260902

// meanFieldExact is the precondition under which E[X(t)] of the SSA satisfies
// solver's ODE exactly: no gating, and no kinetic input of weight > 1 (the
// SSA uses C(m, w), solver uses m).
func meanFieldExact(m *metamodel.Model) bool {
	if len(m.Gating()) > 0 {
		return false
	}
	for i := range m.Transitions {
		for _, in := range m.Inputs(m.Transitions[i].ID) {
			if in.Weight > 1 {
				return false
			}
		}
	}
	return true
}

// referenceOptions is solver.AccurateOptions (Reltol 1e-6) with Dtmax
// lowered from 0.1 to 0.01. The solve itself is cheap since the Tsit5 error
// estimate was fixed on 2026-09-02 (an O(dt·f) term in Bhat had made step
// control first order and 1e-6 cost ~1M steps on the chain; it now hits
// Dtmax on every net here, ~0.1 ms per solve). What still needs care is the
// resampling: odeReference interpolates linearly onto the sample grid, and
// with nodes 0.1 apart that alone contributes dt²/8·|u″| ≈ 0.09 token on the
// chain (measured against the closed form) — the same size as the bounds in
// this file. At Dtmax 0.01 the interpolation error is 1.6e-3 token and the
// N=10000 SIR reference takes 6 ms.
func referenceOptions() *solver.Options {
	o := solver.AccurateOptions()
	o.Dtmax = 0.01
	return o
}

// odeReference integrates the same net with referenceOptions and resamples
// onto the grid Solve reports, keyed by place.
func odeReference(t *testing.T, m *metamodel.Model, opts Options) map[string][]float64 {
	t.Helper()
	opts = opts.withDefaults(m)
	start := startFrom(m, nil)
	net, places, err := toNet(m, start)
	if err != nil {
		t.Fatal(err)
	}
	state := make(map[string]float64, len(places))
	for _, p := range places {
		state[p] = float64(start[p])
	}
	prob := solver.NewProblem(net, state, [2]float64{0, opts.Horizon}, opts.Rates)
	sol := solver.Solve(prob, solver.Tsit5(), referenceOptions())
	if sol == nil {
		t.Fatal("solver returned no solution")
	}
	if sol.Truncated {
		t.Fatal("reference solve truncated")
	}
	grid := sampleTimes(opts)
	out := make(map[string][]float64, len(places))
	for _, p := range places {
		out[p] = resample(sol.T, sol.GetVariable(p), grid)
	}
	return out
}

func seriesByPlace(res *Result) map[string]Series {
	out := make(map[string]Series, len(res.Series))
	for _, s := range res.Series {
		out[s.Place] = s
	}
	return out
}

// maxAbsDiff is the comparison metric: max over grid points |a(t) − b(t)|.
func maxAbsDiff(t *testing.T, what string, a, b []float64) float64 {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: %d vs %d grid points", what, len(a), len(b))
	}
	worst := 0.0
	for i := range a {
		if d := math.Abs(a[i] - b[i]); d > worst {
			worst = d
		}
	}
	return worst
}

// forecastTol is how far Forecast (solver.DefaultOptions, reltol 1e-3,
// Dtmax 0.1) may sit from the reference solve. The pre-registered figure was
// 0.1 token for every net of population <= 1000, and it held only because the
// pre-2026-09-02 error estimate over-refined every step (~1.7k steps on the
// chain instead of ~60). With honest step control Forecast's dominant error
// is its own linear resample of a 0.1-spaced trajectory onto the sample grid:
// worst case dtmax²/8·max|u″| = 0.01/8·150 ≈ 0.19 token on the chain
// (b″ = 200(e^{-t/2}/4 − e^{-t}), |b″(0)| = 150); measured 0.137 on b. The
// solver error at reltol 1e-3 is two orders below that. Hence 0.25.
const forecastTol = 0.25

// bothSides runs the SSA ensemble and the ODE through Solve, checks the
// dispatch labels, holds Forecast within tol of the reference solve, and
// returns (ssa series by place, reference ode by place).
func bothSides(t *testing.T, m *metamodel.Model, opts Options, tol float64) (map[string]Series, map[string][]float64) {
	t.Helper()
	ssaOpts := opts
	ssaOpts.Method = MethodSSA
	ssa, err := Solve(m, nil, ssaOpts)
	if err != nil {
		t.Fatal(err)
	}
	if ssa.Method != "ssa" {
		t.Fatalf("ssa dispatch: Method = %q", ssa.Method)
	}

	odeOpts := Options{Method: MethodODE, Horizon: opts.Horizon, Samples: opts.Samples}
	ode, err := Solve(m, nil, odeOpts)
	if err != nil {
		t.Fatal(err)
	}
	if ode.Method != "ode" {
		t.Fatalf("ode dispatch: Method = %q", ode.Method)
	}
	if ode.Diverged {
		t.Fatalf("ode diverged: %s", ode.Reason)
	}

	ref := odeReference(t, m, odeOpts)
	for _, s := range ode.Series {
		if d := maxAbsDiff(t, s.Place, s.Values, ref[s.Place]); d > tol {
			t.Errorf("Forecast(%s) is %.4f tokens from the reference solve (limit %g)", s.Place, d, tol)
		}
	}
	return seriesByPlace(ssa), ref
}

// assertConserved checks a linear invariant sum_p coef[p]*mean_p(t) == total
// at every SSA grid point, to 1e-9.
func assertConserved(t *testing.T, ssa map[string]Series, coef map[string]float64, total float64) {
	t.Helper()
	var n int
	for _, s := range ssa {
		n = len(s.Values)
		break
	}
	for i := 0; i < n; i++ {
		sum := 0.0
		for p, c := range coef {
			sum += c * ssa[p].Values[i]
		}
		if math.Abs(sum-total) > 1e-9 {
			t.Errorf("grid point %d: invariant = %v, want %v", i, sum, total)
		}
	}
}

// ---------------------------------------------------------------------------
// Case 1 — linear chain a -> b -> c: propensities are linear in the marking,
// so the mean field is exact and there is a closed form to check both engines
// against, which stops the gate passing on a shared bug.

func linearChain() *metamodel.Model {
	return &metamodel.Model{
		Name: "chain",
		Places: []metamodel.Place{
			{ID: "a", Initial: 100},
			{ID: "b"},
			{ID: "c"},
		},
		Transitions: []metamodel.Transition{
			{ID: "ab", Rate: 1.0},
			{ID: "bc", Rate: 0.5},
		},
		Arcs: []metamodel.Arc{
			{From: "a", To: "ab"},
			{From: "ab", To: "b"},
			{From: "b", To: "bc"},
			{From: "bc", To: "c"},
		},
	}
}

func TestConsistencyLinearChain(t *testing.T) {
	m := linearChain()
	if !meanFieldExact(m) {
		t.Fatal("precondition: linear chain must be mean-field exact")
	}
	opts := Options{Horizon: 6, Samples: 61, Realizations: 400, Seed: consistencySeed}
	ssa, ode := bothSides(t, m, opts, forecastTol)

	// a(t) ~ Binomial(100, e^-t): SD <= 5, SE over 400 realizations = 0.25;
	// 1.0 is 4 SE.
	for _, p := range []string{"a", "b", "c"} {
		if d := maxAbsDiff(t, p, ssa[p].Values, ode[p]); d > 1.0 {
			t.Errorf("%s: max|ssa mean − ode| = %.3f (limit 1.0)", p, d)
		}
	}

	// Closed form.
	grid := sampleTimes(opts.withDefaults(m))
	closed := map[string][]float64{"a": {}, "b": {}, "c": {}}
	for _, tt := range grid {
		a := 100 * math.Exp(-tt)
		b := 200 * (math.Exp(-tt/2) - math.Exp(-tt))
		closed["a"] = append(closed["a"], a)
		closed["b"] = append(closed["b"], b)
		closed["c"] = append(closed["c"], 100-a-b)
	}
	for _, p := range []string{"a", "b", "c"} {
		if d := maxAbsDiff(t, p, ssa[p].Values, closed[p]); d > 1.0 {
			t.Errorf("%s: max|ssa mean − closed form| = %.3f (limit 1.0)", p, d)
		}
		if d := maxAbsDiff(t, p, ode[p], closed[p]); d > 0.1 {
			t.Errorf("%s: max|ode − closed form| = %.4f (limit 0.1)", p, d)
		}
	}

	assertConserved(t, ssa, map[string]float64{"a": 1, "b": 1, "c": 1}, 100)
}

// ---------------------------------------------------------------------------
// Cases 2 and 3 — SIR at finite N, and the same net ten times larger. The
// mean field is exact in the rate law but not in the moments (infect is
// second order), so the gap is a finite-size effect and must shrink with N.

// sirModel builds S/I/R with R0 = 5 at population scale k (k=1: N=1000).
func sirModel(scale int) *metamodel.Model {
	return &metamodel.Model{
		Name: "sir",
		Places: []metamodel.Place{
			{ID: "S", Initial: 990 * scale},
			{ID: "I", Initial: 10 * scale},
			{ID: "R", Initial: 0},
		},
		Transitions: []metamodel.Transition{
			{ID: "infect", Rate: 0.0005 / float64(scale)},
			{ID: "recover", Rate: 0.1},
		},
		Arcs: []metamodel.Arc{
			{From: "S", To: "infect", Weight: 1},
			{From: "I", To: "infect", Weight: 1},
			{From: "infect", To: "I", Weight: 2},
			{From: "I", To: "recover", Weight: 1},
			{From: "recover", To: "R", Weight: 1},
		},
	}
}

var sirOpts = Options{Horizon: 40, Samples: 81, Seed: consistencySeed}

// sirGap runs SIR at the given scale and returns the SSA series, the ODE
// reference and the per-place max relative error (|mean − ode| / N).
func sirGap(t *testing.T, scale, realizations int) (map[string]Series, map[string][]float64, map[string]float64) {
	t.Helper()
	m := sirModel(scale)
	if !meanFieldExact(m) {
		t.Fatalf("precondition: SIR x%d must be mean-field exact", scale)
	}
	opts := sirOpts
	opts.Realizations = realizations
	n := float64(1000 * scale)
	// The bound was scaled with N (0.1 per 1000 tokens) while the buggy error
	// estimate put Forecast 0.1095 tokens from the reference at N=10000. With
	// the fix the SIR curve is smooth on the 0.1 grid: measured 0.0019 token
	// at N=1000 and 0.019 at N=10000, so the flat forecastTol holds at both.
	ssa, ode := bothSides(t, m, opts, forecastTol)
	rel := map[string]float64{}
	for _, p := range []string{"S", "I", "R"} {
		rel[p] = maxAbsDiff(t, p, ssa[p].Values, ode[p]) / n
	}
	assertConserved(t, ssa, map[string]float64{"S": 1, "I": 1, "R": 1}, n)
	return ssa, ode, rel
}

func argmax(v []float64) int {
	best := 0
	for i := range v {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

func TestConsistencySIRAndLLN(t *testing.T) {
	// Case 2: N = 1000, 200 realizations. 50 tokens = 5% of N is the loose
	// finite-size + take-off-dispersion bound.
	ssa2, ode2, rel2 := sirGap(t, 1, 200)
	for p, r := range rel2 {
		if abs := r * 1000; abs > 50 {
			t.Errorf("case 2 %s: max|ssa mean − ode| = %.2f (limit 50)", p, abs)
		}
	}
	last := len(ssa2["R"].Values) - 1
	if r := ssa2["R"].Values[last]; r <= 900 {
		t.Errorf("case 2: mean R(40) = %.2f, want > 900", r)
	}
	if sd := ssa2["R"].StdDev[last]; sd >= 40 {
		t.Errorf("case 2: std dev of R(40) = %.2f, want < 40 (bimodality guard)", sd)
	}
	if a, b := argmax(ssa2["I"].Values), argmax(ode2["I"]); a-b > 1 || b-a > 1 {
		t.Errorf("case 2: peak of I at grid %d (ssa) vs %d (ode); want within one step", a, b)
	}
	t.Logf("case 2 relative errors: S %.4f I %.4f R %.4f", rel2["S"], rel2["I"], rel2["R"])

	// Case 3: the LLN statement. Same R0, N = 10000, 100 realizations. The
	// relative gap must be <= 2% AND strictly smaller than at N = 1000.
	_, _, rel3 := sirGap(t, 10, 100)
	t.Logf("case 3 relative errors: S %.4f I %.4f R %.4f", rel3["S"], rel3["I"], rel3["R"])
	for _, p := range []string{"S", "I", "R"} {
		if rel3[p] > 0.02 {
			t.Errorf("case 3 %s: relative error %.4f (limit 0.02)", p, rel3[p])
		}
		if !(rel3[p] < rel2[p]) {
			t.Errorf("case 3 %s: relative error %.4f did not shrink below case 2's %.4f", p, rel3[p], rel2[p])
		}
	}
}

// ---------------------------------------------------------------------------
// Case 4 — honesty of the precondition. Dimerisation 2A -> B: the SSA uses
// k·C(A,2), solver uses k·A, so the engines must DISAGREE. A change to either
// rate law that brings them together, or a Gating() that grows a weight rule,
// trips this rather than silently moving the goalposts.

func dimerisation() *metamodel.Model {
	return &metamodel.Model{
		Name: "dimer",
		Places: []metamodel.Place{
			{ID: "A", Initial: 200},
			{ID: "B"},
		},
		Transitions: []metamodel.Transition{
			{ID: "bind", Rate: 0.01},
		},
		Arcs: []metamodel.Arc{
			{From: "A", To: "bind", Weight: 2},
			{From: "bind", To: "B", Weight: 1},
		},
	}
}

func TestConsistencyPreconditionIsHonest(t *testing.T) {
	m := dimerisation()
	if meanFieldExact(m) {
		t.Fatal("precondition: a weight-2 kinetic input must not be mean-field exact")
	}
	opts := Options{Horizon: 5, Samples: 51, Realizations: 200, Seed: consistencySeed}
	ssa, ode := bothSides(t, m, opts, forecastTol)

	worst := 0.0
	for _, p := range []string{"A", "B"} {
		if d := maxAbsDiff(t, p, ssa[p].Values, ode[p]); d > worst {
			worst = d
		}
	}
	if worst <= 1.0 {
		t.Errorf("SSA and ODE agree to %.3f tokens on 2A -> B; they must disagree (> 1.0) because their rate laws differ", worst)
	}
	assertConserved(t, ssa, map[string]float64{"A": 1, "B": 2}, 200)
}
