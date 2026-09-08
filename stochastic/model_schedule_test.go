package stochastic

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// truckModel carries its own day shape: a dead morning, a lunch rush, a
// quiet afternoon. The declaration lives on the source transition, not in
// any scenario.
func truckModel() *metamodel.Model {
	kf := false
	return &metamodel.Model{
		Name: "truck",
		Places: []metamodel.Place{
			{ID: "queue"},
			{ID: "cooks", Initial: 2},
			{ID: "cooking"},
			{ID: "fed", Tags: map[string]string{"outcome": "served"}},
			{ID: "walked", Tags: map[string]string{"outcome": "loss"}},
		},
		Transitions: []metamodel.Transition{
			{ID: "arrive", Rate: 18, Schedule: []metamodel.RateSegment{
				{Until: 2, Value: 4}, {Until: 4, Value: 45}, {Until: 8, Value: 8},
			}},
			{ID: "take_order", Rate: 720},
			{ID: "serve", Rate: 20},
			{ID: "walk", Rate: 4},
		},
		Arcs: []metamodel.Arc{
			{From: "arrive", To: "queue"},
			{From: "queue", To: "take_order", Kinetic: &kf},
			{From: "cooks", To: "take_order", Kinetic: &kf},
			{From: "take_order", To: "cooking"},
			{From: "cooking", To: "serve"},
			{From: "serve", To: "fed"},
			{From: "serve", To: "cooks"},
			{From: "queue", To: "walk"},
			{From: "walk", To: "walked"},
		},
	}
}

// TestModelScheduleShapesTheRun: the declared rush shows up in the
// trajectory with no scenario schedule anywhere — the queue at the end of
// the rush dwarfs the morning's.
func TestModelScheduleShapesTheRun(t *testing.T) {
	if testing.Short() {
		t.Skip("runs SSA realizations")
	}
	res, err := Simulate(truckModel(), nil, Options{Horizon: 8, Realizations: 12, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	var queue []float64
	for _, s := range res.Series {
		if s.Place == "queue" {
			queue = s.Values
		}
	}
	if queue == nil {
		t.Fatal("no queue series")
	}
	at := func(hour float64) float64 {
		best, bv := 0, 1e9
		for i, tm := range res.Times {
			if d := tm - hour; d*d < bv {
				best, bv = i, d*d
			}
		}
		return queue[best]
	}
	if rush, morning := at(3.9), at(1.9); rush <= morning+1 {
		t.Errorf("declared rush invisible: queue %.1f at end of rush vs %.1f end of morning", rush, morning)
	}

	// The decisive check: a morning-only horizon sees the declared 4/h, not
	// the nominal 18/h flat — ~8 arrivals, not ~36.
	morning, err := Simulate(truckModel(), nil, Options{Horizon: 2, Realizations: 12, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	if arrived := morning.Metrics.Throughput["arrive"]; arrived > 20 {
		t.Errorf("morning ran at the flat nominal rate: %.0f arrivals in 2h, want ~8", arrived)
	}
}

// TestScenarioOverridesModelSchedule: the scenario is the question being
// asked, so its constant rate beats the declared shape for that transition.
func TestScenarioOverridesModelSchedule(t *testing.T) {
	if testing.Short() {
		t.Skip("runs SSA realizations")
	}
	res, err := Simulate(truckModel(), nil, Options{
		Horizon: 8, Realizations: 12, Seed: 7,
		Rates: map[string]float64{"arrive": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	arrived := res.Metrics.Throughput["arrive"]
	if arrived > 30 {
		t.Errorf("scenario rate override lost to the model schedule: %.0f arrivals, want ~16", arrived)
	}
}

// TestModelScheduleRefusedByODE: the continuous engine cannot vary a rate
// over time, and must say so rather than run the day shape flat.
func TestModelScheduleRefusedByODE(t *testing.T) {
	res, err := Forecast(truckModel(), nil, Options{Horizon: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Diverged {
		t.Error("Forecast ran a scheduled model without refusing")
	}
}
