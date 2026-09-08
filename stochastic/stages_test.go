package stochastic

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// washModel is a single-bay wash: cars arrive, queue, get washed for ~15 min,
// and impatient drivers leave. k > 1 declares the wash cycle predictable.
func washModel(k int) *metamodel.Model {
	kf := false
	return &metamodel.Model{
		Name: "wash",
		Places: []metamodel.Place{
			{ID: "queue"},
			{ID: "washing"},
			{ID: "bay_free", Initial: 1},
			{ID: "washed", Tags: map[string]string{"outcome": "served"}},
			{ID: "left", Tags: map[string]string{"outcome": "loss"}},
		},
		Transitions: []metamodel.Transition{
			{ID: "arrive", Rate: 3.5},
			{ID: "pull_in", Rate: 720},
			{ID: "finish", Rate: 4, Stages: k},
			{ID: "leave", Rate: 2},
		},
		Arcs: []metamodel.Arc{
			{From: "arrive", To: "queue"},
			{From: "queue", To: "pull_in", Kinetic: &kf},
			{From: "bay_free", To: "pull_in", Kinetic: &kf},
			{From: "pull_in", To: "washing"},
			{From: "washing", To: "finish"},
			{From: "finish", To: "washed"},
			{From: "finish", To: "bay_free"},
			{From: "queue", To: "leave"},
			{From: "leave", To: "left"},
		},
	}
}

// TestStagesKeepTheVocabulary: a staged run reports the original places and
// transitions only — no stage internals anywhere in the result.
func TestStagesKeepTheVocabulary(t *testing.T) {
	if testing.Short() {
		t.Skip("runs SSA realizations")
	}
	res, err := Simulate(washModel(4), nil, Options{Horizon: 8, Realizations: 8, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	leak := func(name string) bool { return strings.Contains(name, "@") }
	for _, s := range res.Series {
		if leak(s.Place) {
			t.Errorf("stage place %q leaked into Series", s.Place)
		}
	}
	for p := range res.Final {
		if leak(p) {
			t.Errorf("stage place %q leaked into Final", p)
		}
	}
	for id := range res.Metrics.Throughput {
		if leak(id) {
			t.Errorf("stage transition %q leaked into Throughput", id)
		}
	}
	for _, c := range res.Contended {
		if leak(c.Place) {
			t.Errorf("stage place %q leaked into Contended", c.Place)
		}
		for _, id := range c.Blocking {
			if leak(id) {
				t.Errorf("stage transition %q leaked into Contended blocking list", id)
			}
		}
	}
	if _, ok := res.Metrics.Throughput["finish"]; !ok {
		t.Error("staged transition's throughput missing under its own name")
	}
	// The assumption note names the exception instead of repeating the
	// exponential claim wholesale.
	found := false
	for _, a := range res.Assumptions {
		if strings.Contains(a, "Erlang-4") {
			found = true
		}
	}
	if !found {
		t.Errorf("assumption note does not name the staged transition: %v", res.Assumptions)
	}
}

// TestStagesPreserveTheMean: Erlang-k keeps the mean service time, so
// long-run throughput must match the exponential model within noise.
func TestStagesPreserveTheMean(t *testing.T) {
	if testing.Short() {
		t.Skip("runs SSA realizations")
	}
	expo, err := Simulate(washModel(0), nil, Options{Horizon: 24, Realizations: 16, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	erlang, err := Simulate(washModel(4), nil, Options{Horizon: 24, Realizations: 16, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	e, g := expo.Metrics.Throughput["finish"], erlang.Metrics.Throughput["finish"]
	if g < e*0.85 || g > e*1.15 {
		t.Errorf("staging moved the mean: exponential served %.1f, Erlang-4 %.1f", e, g)
	}
}

// TestStagesCutTheQueue: the whole point. A predictable wash queues less than
// an erratic one at the same mean — M/E4/1 waits roughly (1+1/4)/2 = 62.5%%
// of M/M/1's — so fewer drivers give up. This is the claim the engine's
// exponential assumption always warned about, now testable model-side.
func TestStagesCutTheQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("runs SSA realizations")
	}
	expo, err := Simulate(washModel(0), nil, Options{Horizon: 24, Realizations: 96, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	erlang, err := Simulate(washModel(4), nil, Options{Horizon: 24, Realizations: 96, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	if erlang.Final["left"] >= expo.Final["left"] {
		t.Errorf("Erlang-4 service did not cut abandonment: %.1f left vs exponential's %.1f",
			erlang.Final["left"], expo.Final["left"])
	}
	if erlang.Metrics.Mean["queue"] >= expo.Metrics.Mean["queue"] {
		t.Errorf("Erlang-4 service did not shorten the queue: mean %.2f vs %.2f",
			erlang.Metrics.Mean["queue"], expo.Metrics.Mean["queue"])
	}
}

// TestStagesUnderSchedule: a scheduled staged run keeps the vocabulary and
// conserves cars — the boundary carry is in expanded space, so mid-wash cars
// are neither lost nor restarted into thin air at segment boundaries.
func TestStagesUnderSchedule(t *testing.T) {
	if testing.Short() {
		t.Skip("runs SSA realizations")
	}
	res, err := Simulate(washModel(3), nil, Options{
		Horizon: 6, Realizations: 8, Seed: 7,
		Schedule: map[string][]metamodel.RateSegment{
			"arrive": {{Until: 2, Value: 1}, {Until: 4, Value: 8}, {Until: 6, Value: 2}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for p := range res.Final {
		if strings.Contains(p, "@") {
			t.Errorf("stage place %q leaked from a scheduled run", p)
		}
	}
	arrived := res.Metrics.Throughput["arrive"]
	accounted := res.Final["washed"] + res.Final["left"] + res.Final["queue"] + res.Final["washing"]
	if arrived == 0 {
		t.Fatal("no arrivals; fixture broken")
	}
	drift := (arrived - accounted) / arrived
	if drift > 0.05 || drift < -0.05 {
		t.Errorf("cars unaccounted for across segment boundaries: %.1f arrived, %.1f accounted (%.0f%%)",
			arrived, accounted, 100*drift)
	}
}
