package stochastic

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// fixture loads a single-net model from testdata/, a byte copy of the
// petri-pilot services/ file of the same name.
func fixture(t *testing.T, name string) *metamodel.Model {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var m metamodel.Model
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

// coffeeshop is the real model: beans/milk/cups drawn down by weighted arcs,
// restocked in bulk. It is the resource-forecast case in miniature.
func coffeeshop(t *testing.T) *metamodel.Model {
	t.Helper()
	return fixture(t, "coffeeshop.json")
}

func marking(m *metamodel.Model) map[string]int {
	out := map[string]int{}
	for _, p := range m.Places {
		out[p.ID] = p.Initial
	}
	return out
}

// TestForecastIsPure is the property that separates this from the dashboard's
// approach: asking what happens next must not change anything.
func TestForecastIsPure(t *testing.T) {
	m := coffeeshop(t)
	start := marking(m)

	before := map[string]int{}
	for k, v := range start {
		before[k] = v
	}
	if _, err := Forecast(m, start, Options{Horizon: 8, Samples: 20}); err != nil {
		t.Fatalf("forecast: %v", err)
	}
	for k, v := range before {
		if start[k] != v {
			t.Errorf("forecast mutated the marking: %s went %d -> %d", k, v, start[k])
		}
	}
	// And the model itself must be untouched.
	if m.Places[0].Initial != before[m.Places[0].ID] {
		t.Error("forecast mutated the model")
	}
}

// stableNet is a net whose mass-action ODE does not blow up: unit arc weights,
// so no place is raised to a large power.
func stableNet() *metamodel.Model {
	return &metamodel.Model{
		Name: "drain",
		Places: []metamodel.Place{
			{ID: "full", Kind: metamodel.TokenKind, Initial: 100},
			{ID: "empty", Kind: metamodel.TokenKind},
		},
		Transitions: []metamodel.Transition{{ID: "flow", Rate: 0.5}},
		Arcs: []metamodel.Arc{
			{From: "full", To: "flow", Weight: 1},
			{From: "flow", To: "empty", Weight: 1},
		},
	}
}

// TestForecastDetectsDivergence is the coffee shop's real answer: mass action
// with a weight-20 arc means beans^20, and the ODE runs away. Reporting that is
// the feature — the alternative is a dashboard plotting minus two trillion cups.
func TestForecastDetectsDivergence(t *testing.T) {
	m := coffeeshop(t)
	// Strip the declared capacities so the run reaches the solver at all: the
	// shop caps its stock places, and a capacity is refused up front for a
	// different and equally good reason. What is under test here is the
	// numerical failure, not the structural one.
	for i := range m.Places {
		m.Places[i].Capacity = 0
	}
	if gating := m.Gating(); len(gating) > 0 {
		t.Fatalf("this test needs an ungated model to reach the solver: %v", gating)
	}

	res, err := Forecast(m, marking(m), Options{Horizon: 8, Samples: 30})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Diverged {
		t.Fatalf("expected divergence on a model with weight-20 arcs; final = %v", res.Final)
	}
	if res.Reason == "" {
		t.Error("divergence must come with an explanation the caller can act on")
	}
	t.Logf("reported: %s", res.Reason)
}

func TestForecastIsDeterministic(t *testing.T) {
	m := stableNet()
	a, err := Forecast(m, marking(m), Options{Horizon: 8, Samples: 30})
	if err != nil {
		t.Fatal(err)
	}
	if a.Diverged {
		t.Fatalf("the stable net should not diverge: %s", a.Reason)
	}
	for i := 0; i < 5; i++ {
		b, err := Forecast(m, marking(m), Options{Horizon: 8, Samples: 30})
		if err != nil {
			t.Fatal(err)
		}
		for j := range a.Final {
			// Compared with a tolerance rather than for bit equality: the solver
			// accumulates over map iteration in places, so the last couple of
			// digits are not reproducible. That is worth knowing but does not
			// affect a forecast anyone reads.
			if diff := math.Abs(a.Final[j] - b.Final[j]); diff > 1e-9*math.Abs(a.Final[j])+1e-12 {
				t.Fatalf("run %d differs at %s: %v vs %v", i, j, a.Final[j], b.Final[j])
			}
		}
	}
}

// TestDepletesStock: the coffee shop's whole point is "when do I run out".
// Beans are consumed 20 at a time from 1000, so over a long enough horizon with
// no restocking they must deplete.
//
// Asked of the discrete engine, because the shop declares capacities on its
// stock places and Forecast now refuses a model whose constraints it cannot
// honour — see TestForecastRefusesGatedModels.
func TestDepletesStock(t *testing.T) {
	m := coffeeshop(t)
	// Silence restocking so the draw-down is visible.
	rates := Rates(m)
	for id := range rates {
		if len(id) > 8 && id[:8] == "restock_" {
			rates[id] = 0
		}
	}

	res, err := Simulate(m, marking(m), Options{Horizon: 40, Samples: 80, Rates: rates})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Depleted) == 0 {
		t.Errorf("nothing depleted over 40h with restocking off; final = %v", res.Final)
	}
	t.Logf("depletion order: %+v", res.Depleted)
}

// TestSimulateIsReproducible: a stochastic forecast that changes on refresh is
// indistinguishable from a bug, so an unseeded call is still repeatable.
func TestSimulateIsReproducible(t *testing.T) {
	m := coffeeshop(t)
	a, err := Simulate(m, marking(m), Options{Horizon: 5, Samples: 20})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Simulate(m, marking(m), Options{Horizon: 5, Samples: 20})
	if err != nil {
		t.Fatal(err)
	}
	for k := range a.Final {
		if a.Final[k] != b.Final[k] {
			t.Errorf("unseeded runs differ at %s: %v vs %v", k, a.Final[k], b.Final[k])
		}
	}
}

func TestSimulateReportsSpread(t *testing.T) {
	m := coffeeshop(t)
	res, err := Simulate(m, marking(m), Options{Horizon: 5, Samples: 20, Realizations: 8})
	if err != nil {
		t.Fatal(err)
	}
	var sawSpread bool
	for _, s := range res.Series {
		if s.StdDev == nil {
			t.Fatalf("%s has no stddev with 8 realizations", s.Place)
		}
		for _, v := range s.StdDev {
			if v > 0 {
				sawSpread = true
			}
		}
	}
	if !sawSpread {
		t.Error("8 stochastic realizations produced zero variance everywhere")
	}
}

// TestRatesComeFromTheModel pins the single-source-of-truth property: the
// coffee-shop dashboard keeps its own rate table, and that is the drift.
func TestRatesComeFromTheModel(t *testing.T) {
	m := coffeeshop(t)
	r := Rates(m)
	for _, want := range []struct {
		id   string
		rate float64
	}{{"make_espresso", 20}, {"make_latte", 12}, {"make_cappuccino", 10}} {
		if r[want.id] != want.rate {
			t.Errorf("rate[%s] = %v, want %v (declared in the model)", want.id, r[want.id], want.rate)
		}
	}
}

// TestReadArcsDoNotConsume: a read arc gates a firing but moves nothing, so a
// simulation that treated it as an input would drain a place the model only
// tests.
func TestReadArcsDoNotConsume(t *testing.T) {
	m := &metamodel.Model{
		Name: "gated",
		Places: []metamodel.Place{
			{ID: "ready", Kind: metamodel.TokenKind, Initial: 10},
			{ID: "gate", Kind: metamodel.TokenKind, Initial: 5},
			{ID: "done", Kind: metamodel.TokenKind},
		},
		Transitions: []metamodel.Transition{{ID: "go", Rate: 5}},
		Arcs: []metamodel.Arc{
			{From: "ready", To: "go", Weight: 1},
			{From: "gate", To: "go", Weight: 1, Type: metamodel.ReadArc},
			{From: "go", To: "done", Weight: 1},
		},
	}
	res, err := Simulate(m, marking(m), Options{Horizon: 10, Samples: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Final["gate"] != 5 {
		t.Errorf("gate = %v after firing, want 5 — a read arc must not be consumed", res.Final["gate"])
	}
}

// TestForecastResampleErrorIsSmall pins the fix for item 1b: with the Tsit5
// estimator honest, DefaultOptions' Dtmax could leave the linear resample in
// Forecast as the dominant error on a coarse sample grid. forecastSolverOptions
// caps Dtmax to a quarter of the grid spacing so that error stays well under
// the solver's own Reltol.
func TestForecastResampleErrorIsSmall(t *testing.T) {
	m := linearChain()
	opts := Options{Horizon: 6, Samples: 61}
	res, err := Forecast(m, marking(m), opts)
	if err != nil {
		t.Fatal(err)
	}
	grid := sampleTimes(opts.withDefaults(m))
	byPlace := map[string][]float64{}
	for _, s := range res.Series {
		byPlace[s.Place] = s.Values
	}
	closed := map[string][]float64{"a": {}, "b": {}, "c": {}}
	for _, tt := range grid {
		a := 100 * math.Exp(-tt)
		b := 200 * (math.Exp(-tt/2) - math.Exp(-tt))
		closed["a"] = append(closed["a"], a)
		closed["b"] = append(closed["b"], b)
		closed["c"] = append(closed["c"], 100-a-b)
	}
	for _, p := range []string{"a", "b", "c"} {
		worst := 0.0
		for i := range grid {
			if d := math.Abs(byPlace[p][i] - closed[p][i]); d > worst {
				worst = d
			}
		}
		// Before the grid cap this was ~0.16 (dominated by a Dtmax=0.1
		// resample); with it, the observed worst case is ~0.01, close to
		// DefaultOptions' own Reltol=1e-3 floor on a place near 50-100
		// tokens. 0.05 is comfortably above that floor and well below the
		// pre-fix number, so a regression back to resample-dominated error
		// still trips this.
		if worst > 0.05 {
			t.Errorf("%s: max|forecast − closed form| = %.4f, want <= 0.05 (resample error should not dominate)", p, worst)
		}
	}
}

// TestForecastSolverOptionsCapsDtmaxToGrid documents the cap directly, not
// just its downstream effect on accuracy.
func TestForecastSolverOptionsCapsDtmaxToGrid(t *testing.T) {
	opts := Options{Horizon: 6, Samples: 61} // spacing = 0.1, cap = 0.025
	so := forecastSolverOptions(opts.withDefaults(&metamodel.Model{}))
	if so.Dtmax != 0.025 {
		t.Errorf("Dtmax = %v, want 0.025 (a quarter of the 0.1 grid spacing)", so.Dtmax)
	}
	if so.Dt > so.Dtmax {
		t.Errorf("Dt = %v exceeds capped Dtmax = %v", so.Dt, so.Dtmax)
	}

	// A grid coarser than the default Dtmax must not raise it.
	coarse := Options{Horizon: 100, Samples: 3} // spacing = 50
	soCoarse := forecastSolverOptions(coarse.withDefaults(&metamodel.Model{}))
	if soCoarse.Dtmax != 0.1 {
		t.Errorf("Dtmax = %v, want unchanged default 0.1 on a coarse grid", soCoarse.Dtmax)
	}
}
