package stochastic

import (
	"math"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// at returns the series value for place at model time t, which must be on the
// sample grid.
func at(t *testing.T, res *Result, place string, when float64) float64 {
	t.Helper()
	for _, s := range res.Series {
		if s.Place != place {
			continue
		}
		for i, tm := range res.Times {
			if math.Abs(tm-when) < 1e-9 {
				return s.Values[i]
			}
		}
	}
	t.Fatalf("no sample for %s at t=%v (times %v)", place, when, res.Times)
	return 0
}

// One barista, three orders, a brew that takes exactly two minutes: the
// deterministic schedule is the whole point, so the test checks the clock,
// not a distribution.
func TestDelayIsASingleServerClock(t *testing.T) {
	m := &metamodel.Model{
		Places: []metamodel.Place{{ID: "orders", Initial: 3}, {ID: "barista", Initial: 1}, {ID: "done"}},
		Transitions: []metamodel.Transition{
			{ID: "serve", Delay: 2},
		},
		Arcs: []metamodel.Arc{
			{From: "orders", To: "serve"}, {From: "barista", To: "serve"},
			{From: "serve", To: "done"}, {From: "serve", To: "barista"},
		},
	}
	res, err := Simulate(m, nil, Options{Horizon: 10, Samples: 11, Realizations: 1, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	// t=1: the first brew is in flight — barista and one order are in no place.
	if got := at(t, res, "barista", 1); got != 0 {
		t.Errorf("barista at t=1: got %v, want 0 (held for the brew)", got)
	}
	if got := at(t, res, "orders", 1); got != 2 {
		t.Errorf("orders at t=1: got %v, want 2", got)
	}
	// Completions land at exactly 2, 4 and 6. A sample on the instant reads
	// the marking before the event, as it does for an exponential firing,
	// so the grid is probed one unit after each.
	for when, want := range map[float64]float64{3: 1, 5: 2, 7: 3, 10: 3} {
		if got := at(t, res, "done", when); got != want {
			t.Errorf("done at t=%v: got %v, want %v", when, got, want)
		}
	}
	if got := at(t, res, "barista", 7); got != 1 {
		t.Errorf("barista at t=7: got %v, want 1 (idle after the last brew)", got)
	}
	if res.Metrics.Throughput["serve"] != 3 {
		t.Errorf("throughput: got %v, want 3", res.Metrics.Throughput["serve"])
	}
	if res.Metrics.InFlight["serve"] != 0 {
		t.Errorf("in flight at the horizon: got %v, want 0", res.Metrics.InFlight["serve"])
	}
	if len(res.Caveats) != 0 {
		t.Errorf("unexpected caveats: %v", res.Caveats)
	}
}

// Three loads and no shared resource: every enabling runs its own clock, so
// all three finish together at t=5, and until then they are in flight.
func TestDelayIsInfiniteServer(t *testing.T) {
	m := &metamodel.Model{
		Places:      []metamodel.Place{{ID: "loads", Initial: 3}, {ID: "clean"}},
		Transitions: []metamodel.Transition{{ID: "wash", Delay: 5}},
		Arcs:        []metamodel.Arc{{From: "loads", To: "wash"}, {From: "wash", To: "clean"}},
	}
	var completions []float64
	res, err := Simulate(m, nil, Options{Horizon: 8, Samples: 9, Realizations: 1,
		OnFire: func(_ int, at float64, id string, _ []int) {
			if id == "wash" {
				completions = append(completions, at)
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	if got := at(t, res, "loads", 4); got != 0 {
		t.Errorf("loads at t=4: got %v, want 0 (all three started at once)", got)
	}
	if got := at(t, res, "clean", 4); got != 0 {
		t.Errorf("clean at t=4: got %v, want 0 (still washing)", got)
	}
	if got := at(t, res, "clean", 6); got != 3 {
		t.Errorf("clean at t=6: got %v, want 3", got)
	}
	if len(completions) != 3 || completions[0] != 5 || completions[2] != 5 {
		t.Errorf("OnFire completions: got %v, want three at t=5", completions)
	}
}

// A run cut off mid-brew reports what is in flight rather than losing it.
func TestDelayReportsInFlightAtHorizon(t *testing.T) {
	m := &metamodel.Model{
		Places:      []metamodel.Place{{ID: "loads", Initial: 2}, {ID: "clean"}},
		Transitions: []metamodel.Transition{{ID: "wash", Delay: 5}},
		Arcs:        []metamodel.Arc{{From: "loads", To: "wash"}, {From: "wash", To: "clean"}},
	}
	res, err := Simulate(m, nil, Options{Horizon: 3, Samples: 4, Realizations: 4})
	if err != nil {
		t.Fatal(err)
	}
	if res.Final["loads"] != 0 || res.Final["clean"] != 0 {
		t.Errorf("final marking %v: want both empty, tokens in flight", res.Final)
	}
	if got := res.Metrics.InFlight["wash"]; got != 2 {
		t.Errorf("in flight: got %v, want 2", got)
	}
	if res.Metrics.Throughput["wash"] != 0 {
		t.Errorf("throughput counts completions only: got %v", res.Metrics.Throughput["wash"])
	}
}

// A brew that straddles a schedule boundary completes in the next segment.
func TestDelayCrossesScheduleSeam(t *testing.T) {
	m := &metamodel.Model{
		Places: []metamodel.Place{{ID: "loads", Initial: 1}, {ID: "clean"}, {ID: "never"}},
		Transitions: []metamodel.Transition{
			{ID: "wash", Delay: 3},
			{ID: "tick", Rate: 1, Schedule: []metamodel.RateSegment{{Until: 2, Value: 1}, {Value: 1}}},
		},
		Arcs: []metamodel.Arc{
			{From: "loads", To: "wash"}, {From: "wash", To: "clean"},
			{From: "never", To: "tick"}, {From: "tick", To: "never"},
		},
	}
	// Samples: 5 lands the sample grid on the seam and on both probe times:
	// segment [0,2) gets 2 samples ({0,2}), segment [2,6] gets 3 ({2,4,6}).
	res, err := SimulateSchedule(m, nil, Options{Horizon: 6, Samples: 5, Realizations: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := at(t, res, "clean", 2); got != 0 {
		t.Errorf("clean at the seam (t=2): got %v, want 0", got)
	}
	if got := at(t, res, "clean", 4); got != 1 {
		t.Errorf("clean at t=4: got %v, want 1 (completed at t=3, after the seam)", got)
	}
	if res.Metrics.Throughput["wash"] != 1 {
		t.Errorf("throughput across the seam: got %v, want 1", res.Metrics.Throughput["wash"])
	}
}

// A delayed transition beats the exponential race for a shared token, the
// way an immediate transition does: the exponential rival never gets it.
func TestDelayTakesSharedTokensFirst(t *testing.T) {
	m := &metamodel.Model{
		Places: []metamodel.Place{{ID: "job", Initial: 1}, {ID: "timed"}, {ID: "raced"}},
		Transitions: []metamodel.Transition{
			{ID: "race", Rate: 1000},
			{ID: "clock", Delay: 1},
		},
		Arcs: []metamodel.Arc{
			{From: "job", To: "race"}, {From: "race", To: "raced"},
			{From: "job", To: "clock"}, {From: "clock", To: "timed"},
		},
	}
	res, err := Simulate(m, nil, Options{Horizon: 2, Samples: 3, Realizations: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Final["timed"] != 1 || res.Final["raced"] != 0 {
		t.Errorf("final %v: want the timed transition to have taken the job", res.Final)
	}
}

func TestDelayRefusals(t *testing.T) {
	delayed := &metamodel.Model{
		Places:      []metamodel.Place{{ID: "a", Initial: 1}, {ID: "b"}},
		Transitions: []metamodel.Transition{{ID: "t", Delay: 1}},
		Arcs:        []metamodel.Arc{{From: "a", To: "t"}, {From: "t", To: "b"}},
	}

	// The continuous engines have no firing instant to run a timer from.
	fc, err := Forecast(delayed, nil, Options{Horizon: 2, Samples: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !fc.Diverged || !strings.Contains(fc.Reason, "delays on [t]") {
		t.Errorf("Forecast should refuse a delayed net: diverged=%v reason=%q", fc.Diverged, fc.Reason)
	}
	sde, err := SimulateSDE(delayed, nil, Options{Horizon: 2, Samples: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !sde.Diverged {
		t.Errorf("SimulateSDE should refuse a delayed net")
	}

	// The portable path carries delays too (the `timed` golden pins it across
	// the four languages), so it must run, not refuse.
	if _, err := Simulate(delayed, nil, Options{Horizon: 2, Samples: 3, Portable: true}); err != nil {
		t.Errorf("Portable run of a delayed net: %v", err)
	}

	// A timer with nothing to consume would restart forever in zero time.
	source := &metamodel.Model{
		Places:      []metamodel.Place{{ID: "b"}},
		Transitions: []metamodel.Transition{{ID: "t", Delay: 1}},
		Arcs:        []metamodel.Arc{{From: "t", To: "b"}},
	}
	if _, err := Simulate(source, nil, Options{Horizon: 2, Samples: 3}); err == nil || !strings.Contains(err.Error(), "no consuming input") {
		t.Errorf("delayed source transition: got %v, want a refusal", err)
	}

	// Rate alongside delay is accepted and caveated, never raced.
	both := &metamodel.Model{
		Places:      []metamodel.Place{{ID: "a", Initial: 1}, {ID: "b"}},
		Transitions: []metamodel.Transition{{ID: "t", Delay: 1, Rate: 50}},
		Arcs:        []metamodel.Arc{{From: "a", To: "t"}, {From: "t", To: "b"}},
	}
	res, err := Simulate(both, nil, Options{Horizon: 2, Samples: 5, Realizations: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Caveats) != 1 || !strings.Contains(res.Caveats[0], "rate is ignored") {
		t.Errorf("caveats %v: want one saying the rate is ignored", res.Caveats)
	}
	if got := at(t, res, "b", 0.5); got != 0 {
		t.Errorf("b at t=0.5: got %v, want 0 (the delay, not the rate, is the firing rule)", got)
	}
	if got := at(t, res, "b", 1.5); got != 1 {
		t.Errorf("b at t=1.5: got %v, want 1", got)
	}
}
