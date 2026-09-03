package stochastic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// The parity goldens in testdata/ were produced by the PRE-MOVE petri-pilot
// engine (pkg/runtime/sim.Simulate before the SSA was promoted here), by
// pkg/runtime/sim/internal/paritygolden at petri-pilot commit
//
//	67538d70c977fa7d6de9e0ac1f247da900b64600
//
// Every float is compared with ==. A difference is a finding about the move —
// the sample path changed — and is never fixed by regenerating the golden.

// parityOptions are the Options each golden was generated with.
var parityOptions = map[string]Options{
	"parity_coffeeshop_seed42.json": {Horizon: 8, Samples: 60, Realizations: 5, Seed: 42},
	"parity_sir_seed11.json":        {Horizon: 40, Samples: 81, Realizations: 8, Seed: 11},
}

// paritySIR is the SIR net of consistency_test.go Case 2, exactly as
// paritygolden declares it (arc order and explicit weights included).
func paritySIR() *metamodel.Model {
	return &metamodel.Model{
		Name: "sir",
		Places: []metamodel.Place{
			{ID: "S", Initial: 990},
			{ID: "I", Initial: 10},
			{ID: "R", Initial: 0},
		},
		Transitions: []metamodel.Transition{
			{ID: "infect", Rate: 0.0005},
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

func loadGolden(t *testing.T, name string) *Result {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var want Result
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return &want
}

func TestParityCoffeeshopSeed42(t *testing.T) {
	assertParity(t, "parity_coffeeshop_seed42.json", fixture(t, "coffeeshop.json"))
}

func TestParitySIRSeed11(t *testing.T) {
	assertParity(t, "parity_sir_seed11.json", paritySIR())
}

func assertParity(t *testing.T, golden string, m *metamodel.Model) {
	t.Helper()
	want := loadGolden(t, golden)
	got, err := Simulate(m, nil, parityOptions[golden])
	if err != nil {
		t.Fatal(err)
	}

	if got.Method != want.Method {
		t.Errorf("method: got %q, want %q", got.Method, want.Method)
	}
	exactFloats(t, "times", got.Times, want.Times)

	if len(got.Series) != len(want.Series) {
		t.Fatalf("series: got %d, want %d", len(got.Series), len(want.Series))
	}
	for i := range want.Series {
		if got.Series[i].Place != want.Series[i].Place {
			t.Fatalf("series[%d]: got place %q, want %q", i, got.Series[i].Place, want.Series[i].Place)
		}
		exactFloats(t, "series."+want.Series[i].Place+".values", got.Series[i].Values, want.Series[i].Values)
		exactFloats(t, "series."+want.Series[i].Place+".std_dev", got.Series[i].StdDev, want.Series[i].StdDev)
	}
	exactMap(t, "final", got.Final, want.Final)

	if want.Metrics == nil || got.Metrics == nil {
		t.Fatalf("metrics: got %v, want %v", got.Metrics != nil, want.Metrics != nil)
	}
	exactMap(t, "metrics.throughput", got.Metrics.Throughput, want.Metrics.Throughput)
	exactMap(t, "metrics.mean", got.Metrics.Mean, want.Metrics.Mean)
	exactMap(t, "metrics.p95", got.Metrics.P95, want.Metrics.P95)
	exactMap(t, "metrics.utilization", got.Metrics.Utilization, want.Metrics.Utilization)
}

// exactFloats asserts == on every element; no tolerance, by design.
func exactFloats(t *testing.T, what string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %d values, want %d", what, len(got), len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d]: got %v, want %v", what, i, got[i], want[i])
		}
	}
}

func exactMap(t *testing.T, what string, got, want map[string]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %d keys, want %d", what, len(got), len(want))
	}
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("%s[%s]: missing", what, k)
			continue
		}
		if g != w {
			t.Errorf("%s[%s]: got %v, want %v", what, k, g, w)
		}
	}
}
