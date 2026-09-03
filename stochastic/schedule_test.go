package stochastic

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// TestScheduleMakesARush: a rate that varies over time is the one thing a
// constant rate cannot express, and averaging it away is exactly the smoothing
// that hides whether the queue recovers afterwards.
func TestScheduleMakesARush(t *testing.T) {
	m := staffedShop(2)

	// The same customers either way — about 254 over the day — but the rush
	// puts 240 of them in the first hour, well past what two baristas can
	// serve. Spread evenly they never queue at all.
	flat, err := Solve(m, nil, Options{
		Rates:   map[string]float64{"arrive": 254.0 / 8},
		Horizon: 8, Samples: 80, Realizations: 12, Seed: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	rush, err := Solve(m, nil, Options{
		Schedule: map[string][]metamodel.RateSegment{
			"arrive": {{Until: 1, Value: 240}, {Until: 8, Value: 2}},
		},
		Horizon: 8, Samples: 80, Realizations: 12, Seed: 4,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Peak queue, not P95: a one-hour rush is an eighth of the horizon, so a
	// percentile over the whole day averages the very thing being asked about
	// back out again. What the owner wants to know is how bad it got.
	if peak(rush, "queue") <= 5*peak(flat, "queue") {
		t.Errorf("peak queue was %.0f under a rush and %.0f spread evenly; the schedule barely mattered",
			peak(rush, "queue"), peak(flat, "queue"))
	}
	if len(rush.Times) < 2 || rush.Times[len(rush.Times)-1] < 7.5 {
		t.Errorf("the scheduled run covers %v, not the full horizon", rush.Times[len(rush.Times)-1])
	}
	// Segments are stitched, not restarted: time must never go backwards.
	for i := 1; i < len(rush.Times); i++ {
		if rush.Times[i] < rush.Times[i-1] {
			t.Fatalf("time went backwards at sample %d: %v then %v", i, rush.Times[i-1], rush.Times[i])
		}
	}
	t.Logf("flat peak queue %.0f, rush peak queue %.0f", peak(flat, "queue"), peak(rush, "queue"))
}

func peak(res *Result, place string) float64 {
	var top float64
	for _, s := range res.Series {
		if s.Place != place {
			continue
		}
		for _, v := range s.Values {
			if v > top {
				top = v
			}
		}
	}
	return top
}

// TestScheduleHoldsItsLastRate: a schedule that stops short of the horizon must
// hold, not silently revert to the model's rate — which would look like the
// rush ending a second time.
func TestScheduleHoldsItsLastRate(t *testing.T) {
	m := staffedShop(2)
	schedule := map[string][]metamodel.RateSegment{"arrive": {{Until: 1, Value: 100}}}
	base := Options{Horizon: 4}.withDefaults(m).Rates

	if got := scheduleRates(base, schedule, 0.5)["arrive"]; got != 100 {
		t.Errorf("rate at t=0.5 is %v, want 100", got)
	}
	if got := scheduleRates(base, schedule, 3)["arrive"]; got != 100 {
		t.Errorf("rate at t=3 is %v, want the last segment's 100 — not the model's %v",
			got, Rates(m)["arrive"])
	}
}

// TestSimulateHonoursRateOverrides was a silent hole: compile() read rates
// straight from the model and never saw Options.Rates, so the discrete engine
// ignored every override. `/api/simulate?rate.X=` appeared to work, returned a
// plausible trajectory, and answered the unmodified question.
func TestSimulateHonoursRateOverrides(t *testing.T) {
	m := staffedShop(3)

	slow, err := Simulate(m, nil, Options{
		Rates: map[string]float64{"arrive": 1}, Horizon: 8, Samples: 20, Realizations: 6, Seed: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	fast, err := Simulate(m, nil, Options{
		Rates: map[string]float64{"arrive": 60}, Horizon: 8, Samples: 20, Realizations: 6, Seed: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if fast.Metrics.Throughput["arrive"] <= 10*slow.Metrics.Throughput["arrive"] {
		t.Errorf("arrivals were %.1f at rate 60 and %.1f at rate 1: the override was ignored",
			fast.Metrics.Throughput["arrive"], slow.Metrics.Throughput["arrive"])
	}
	// And the model's own rate must still be the default for everything unset.
	if slow.Metrics.Throughput["finish"] == 0 {
		t.Error("nothing was served; an override of one rate should not silence the rest")
	}
}

// TestSolveDispatches pins the G2 contract: the zero Method is the SSA, "ode"
// is Forecast (which still refuses a gated net with Diverged and Gating() in
// Caveats), and anything else is an error rather than a silent default.
func TestSolveDispatches(t *testing.T) {
	m := staffedShop(2)

	ssa, err := Solve(m, nil, Options{Horizon: 1, Samples: 10, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if ssa.Method != "ssa" {
		t.Errorf("zero Method dispatched to %q, want ssa", ssa.Method)
	}
	if !strings.Contains(strings.Join(ssa.Assumptions, " "), "exponentially") {
		t.Errorf("the SSA path dropped its service-time assumption: %v", ssa.Assumptions)
	}

	ode, err := Solve(m, nil, Options{Horizon: 1, Samples: 10, Method: MethodODE})
	if err != nil {
		t.Fatal(err)
	}
	if ode.Method != "ode" || ode.Diverged {
		t.Errorf("MethodODE on an ungated net: method %q diverged=%v (%s)", ode.Method, ode.Diverged, ode.Reason)
	}

	gated := staffedShop(2)
	gated.Places = append(gated.Places, metamodel.Place{ID: "licence", Initial: 1})
	gated.Arcs = append(gated.Arcs, metamodel.Arc{From: "licence", To: "start", Type: metamodel.ReadArc})
	refused, err := Solve(gated, nil, Options{Horizon: 1, Samples: 10, Method: MethodODE})
	if err != nil {
		t.Fatal(err)
	}
	if !refused.Diverged || len(refused.Caveats) != len(gated.Gating()) {
		t.Errorf("MethodODE on a gated net: diverged=%v caveats=%v, want a refusal carrying Gating()",
			refused.Diverged, refused.Caveats)
	}

	if _, err := Solve(m, nil, Options{Horizon: 1, Method: "foo"}); err == nil {
		t.Error("an unknown method was accepted")
	}
}

// TestMarkingGuardIsEnforcedUnderSchedule: the guard evaluator must reach every
// segment of a scheduled run. Each segment compiles the model afresh, so
// dropping Guard from the per-segment Options would caveat every guard and
// fire serve where the application would refuse — on both the direct
// SimulateSchedule entry and the Solve dispatch that petri-pilot's Run uses.
func TestMarkingGuardIsEnforcedUnderSchedule(t *testing.T) {
	m := &metamodel.Model{
		Name: "guarded",
		Places: []metamodel.Place{
			{ID: "orders", Initial: 10},
			{ID: "done"},
			{ID: "reserve", Initial: 3},
		},
		Transitions: []metamodel.Transition{
			{ID: "serve", Rate: 40, Guard: `tokens("reserve") >= 5`},
		},
		Arcs: []metamodel.Arc{
			{From: "orders", To: "serve"},
			{From: "serve", To: "done"},
		},
	}
	opts := Options{
		Horizon: 5, Samples: 20, Realizations: 2, Seed: 1, Guard: stubGuard,
		Schedule: map[string][]metamodel.RateSegment{
			"serve": {{Until: 2, Value: 80}, {Until: 5, Value: 40}},
		},
	}
	for name, run := range map[string]func() (*Result, error){
		"SimulateSchedule": func() (*Result, error) { return SimulateSchedule(m, map[string]int{}, opts) },
		"Solve":            func() (*Result, error) { return Solve(m, map[string]int{}, opts) },
	} {
		res, err := run()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := res.Metrics.Throughput["serve"]; got != 0 {
			t.Errorf("%s: serve fired %.1f times with its guard false", name, got)
		}
		for _, c := range res.Caveats {
			if strings.Contains(c, "not enforced") {
				t.Errorf("%s: a marking-decidable guard should be enforced, not caveated: %q", name, c)
			}
		}
	}
}
