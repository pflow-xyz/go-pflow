package stochastic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// spinner is a two-place cycle fast enough that every realization runs into
// the 1000000-step limit long before its horizon: tens of milliseconds a
// realization, so a few dozen realizations would take seconds.
func spinner() *metamodel.Model {
	return &metamodel.Model{
		Name:        "spinner",
		Places:      []metamodel.Place{{ID: "a", Initial: 3}, {ID: "b"}},
		Transitions: []metamodel.Transition{{ID: "ab", Rate: 1e6}, {ID: "ba", Rate: 1e6}},
		Arcs: []metamodel.Arc{
			{From: "a", To: "ab"}, {From: "ab", To: "b"},
			{From: "b", To: "ba"}, {From: "ba", To: "a"},
		},
	}
}

// source is a pure source into one place: nothing drains q, so its count —
// and the time-weighted ledger sized by that count — grows without limit.
func source() *metamodel.Model {
	return &metamodel.Model{
		Name:        "source",
		Places:      []metamodel.Place{{ID: "q"}},
		Transitions: []metamodel.Transition{{ID: "arrive", Rate: 1e6}},
		Arcs:        []metamodel.Arc{{From: "arrive", To: "q"}},
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestContextCancelsMidRun: a run that would take seconds returns promptly
// once its context is done, with an error that errors.Is recognises and no
// Result — on the flat path, the scheduled path, and for both kinds of done.
func TestContextCancelsMidRun(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{"flat", Options{Horizon: 1e3, Realizations: 40}},
		{"scheduled", Options{Horizon: 1e3, Realizations: 40, Schedule: map[string][]metamodel.RateSegment{
			"ab": {{Until: 250, Value: 1e6}, {Until: 500, Value: 2e6}, {Until: 1e3, Value: 1e6}},
		}}},
		{"portable", Options{Horizon: 1e3, Realizations: 40, Portable: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/deadline", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			opts := tc.opts
			opts.Context = ctx
			began := time.Now()
			res, err := Solve(spinner(), nil, opts)
			elapsed := time.Since(began)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v, want one wrapping context.DeadlineExceeded", err)
			}
			if res != nil {
				t.Errorf("a cancelled call returned a Result: %+v", res)
			}
			// Unbounded, this is 40 realizations x 1M steps: seconds. The
			// check runs every 1024 steps, so the overshoot is milliseconds.
			if elapsed > 500*time.Millisecond {
				t.Errorf("returned %v after the call began; want promptly after the 30ms deadline", elapsed)
			}
		})
		t.Run(tc.name+"/cancel", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			opts := tc.opts
			opts.Context = ctx
			time.AfterFunc(30*time.Millisecond, cancel)
			began := time.Now()
			_, err := Solve(spinner(), nil, opts)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want one wrapping context.Canceled", err)
			}
			if elapsed := time.Since(began); elapsed > 500*time.Millisecond {
				t.Errorf("returned %v after the call began; want promptly after the 30ms cancel", elapsed)
			}
		})
	}
}

// TestContextAlreadyDoneRefusesEveryMethod: the continuous engines honour a
// done context too, and nothing is computed for it.
func TestContextAlreadyDoneRefusesEveryMethod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, method := range []Method{MethodSSA, MethodODE, MethodSDE} {
		res, err := Solve(spinner(), nil, Options{Method: method, Context: ctx})
		if !errors.Is(err, context.Canceled) || res != nil {
			t.Errorf("%s: (%v, %v), want (nil, context.Canceled)", method, res, err)
		}
	}
}

// TestContextNotDoneChangesNothing: a live context is a bound that did not
// bite, so the run is the run it was without one.
func TestContextNotDoneChangesNothing(t *testing.T) {
	for _, method := range []Method{MethodSSA, MethodODE, MethodSDE} {
		base := Options{Method: method, Horizon: 4, Realizations: 3, Seed: 9}
		plain, err := Solve(source(), nil, Options{Method: method, Horizon: 4, Realizations: 3, Seed: 9,
			Rates: map[string]float64{"arrive": 5}})
		if err != nil {
			t.Fatal(err)
		}
		base.Rates = map[string]float64{"arrive": 5}
		base.Context = context.Background()
		withCtx, err := Solve(source(), nil, base)
		if err != nil {
			t.Fatal(err)
		}
		if mustJSON(t, plain) != mustJSON(t, withCtx) {
			t.Errorf("%s: a live context changed the result", method)
		}
	}
}

// TestMaxStepsIsSharedAcrossRealizations: the budget is the call's. The first
// realization spends all of it, the rest do not advance, and the Result says
// so in the budget's own words — deterministically.
func TestMaxStepsIsSharedAcrossRealizations(t *testing.T) {
	perReal := make([]int, 3)
	opts := Options{Horizon: 1e3, Realizations: 3, MaxSteps: 1500,
		OnFire: func(r int, _ float64, _ string, _ []int) { perReal[r]++ }}
	res, err := Simulate(spinner(), nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if perReal[0] != 1500 || perReal[1] != 0 || perReal[2] != 0 {
		t.Errorf("firings per realization = %v, want [1500 0 0]", perReal)
	}
	if !res.Truncated || !res.Diverged {
		t.Errorf("Truncated=%v Diverged=%v, want both set", res.Truncated, res.Diverged)
	}
	if !strings.Contains(res.Reason, "1500-step budget (Options.MaxSteps)") {
		t.Errorf("Reason = %q, want it to name the 1500-step budget", res.Reason)
	}
	if strings.Contains(res.Reason, "1000000-step limit") {
		t.Errorf("Reason = %q names the per-realization limit, which never bit", res.Reason)
	}

	opts.OnFire = nil
	again, err := Simulate(spinner(), nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if mustJSON(t, res) != mustJSON(t, again) {
		t.Error("the same budgeted call gave two different results")
	}
}

// TestMaxStepsIsSharedAcrossScheduleSegments: a scheduled run is one call, so
// ten segments do not get ten budgets — which is exactly how the engine's own
// per-realization limit behaves, and why a caller could not bound a request.
func TestMaxStepsIsSharedAcrossScheduleSegments(t *testing.T) {
	var segs []metamodel.RateSegment
	for i := 1; i <= 10; i++ {
		segs = append(segs, metamodel.RateSegment{Until: float64(i), Value: 1e6})
	}
	fired := 0
	res, err := SimulateSchedule(spinner(), nil, Options{
		Horizon: 10, Realizations: 2, MaxSteps: 700,
		Schedule: map[string][]metamodel.RateSegment{"ab": segs},
		OnFire:   func(int, float64, string, []int) { fired++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if fired != 700 {
		t.Errorf("fired %d times across 10 segments x 2 realizations, want the call's budget of 700", fired)
	}
	if !res.Truncated || !strings.Contains(res.Reason, "700-step budget") {
		t.Errorf("Truncated=%v Reason=%q, want the budget named", res.Truncated, res.Reason)
	}
	if len(res.Times) == 0 || res.Times[len(res.Times)-1] != 10 {
		t.Errorf("a budgeted scheduled run should still report every segment's grid, got %d times", len(res.Times))
	}
}

// TestBoundsThatDoNotBiteChangeNothing: a budget and a cap the run never
// reaches, on both sampler paths, flat and scheduled, leave the Result byte
// for byte what it was — the checks draw nothing from the random stream.
func TestBoundsThatDoNotBiteChangeNothing(t *testing.T) {
	m := staffedShop(2)
	for _, portable := range []bool{false, true} {
		for _, sched := range []map[string][]metamodel.RateSegment{nil, {"arrive": {{Until: 1, Value: 40}, {Until: 8, Value: 4}}}} {
			base := Options{Horizon: 8, Samples: 40, Realizations: 6, Seed: 5, Portable: portable, Schedule: sched}
			plain, err := Simulate(m, nil, base)
			if err != nil {
				t.Fatal(err)
			}
			bounded := base
			bounded.MaxSteps = 1 << 40
			bounded.MaxPlaceTokens = 1 << 20
			bounded.Context = context.Background()
			got, err := Simulate(m, nil, bounded)
			if err != nil {
				t.Fatal(err)
			}
			if mustJSON(t, plain) != mustJSON(t, got) {
				t.Errorf("portable=%v scheduled=%v: bounds that never bit changed the result", portable, sched != nil)
			}
		}
	}
}

// TestMaxPlaceTokensStopsAPureSource: q would reach about a million; the cap
// stops the realization at 100, names the place, and the time-weighted
// ledger — sized by the largest count it has seen — stays that small.
func TestMaxPlaceTokensStopsAPureSource(t *testing.T) {
	const limit = 100
	fired := 0
	opts := Options{Horizon: 1e3, Realizations: 2, MaxPlaceTokens: limit,
		OnFire: func(int, float64, string, []int) { fired++ }}
	lim, err := newLimits(opts)
	if err != nil {
		t.Fatal(err)
	}
	res, stats, _, err := simulate(source(), nil, opts, nil, lim)
	if err != nil {
		t.Fatal(err)
	}
	if res.Final["q"] != limit {
		t.Errorf("final q = %v, want the cap %d", res.Final["q"], limit)
	}
	if fired != 2*limit {
		t.Errorf("fired %d times, want %d (the refused firing is not reported)", fired, 2*limit)
	}
	if !res.Truncated || !strings.Contains(res.Reason, "raised q to 101 tokens, above the 100-token cap (Options.MaxPlaceTokens)") {
		t.Errorf("Truncated=%v Reason=%q, want the place and the cap named", res.Truncated, res.Reason)
	}
	for i, d := range stats.times.dwell {
		if len(d) > 2*(limit+1) {
			t.Errorf("dwell ledger for place %d has %d slots; the cap should hold it to %d", i, len(d), 2*(limit+1))
		}
	}

	// And through the public entry point, which must agree.
	pub, err := Simulate(source(), nil, Options{Horizon: 1e3, Realizations: 2, MaxPlaceTokens: limit})
	if err != nil {
		t.Fatal(err)
	}
	if pub.Final["q"] != limit || pub.Reason != res.Reason {
		t.Errorf("Simulate: final q=%v reason=%q", pub.Final["q"], pub.Reason)
	}
}

// TestMaxPlaceTokensRefusesAStartOverTheCap: a marking already over the cap
// never runs, and the Reason says it was the start, not a firing.
func TestMaxPlaceTokensRefusesAStartOverTheCap(t *testing.T) {
	res, err := Simulate(source(), map[string]int{"q": 50}, Options{Horizon: 1, MaxPlaceTokens: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || !strings.Contains(res.Reason, "starting marking holds 50 tokens in q, above the 10-token cap") {
		t.Errorf("Truncated=%v Reason=%q", res.Truncated, res.Reason)
	}
	if res.Metrics.Throughput["arrive"] != 0 {
		t.Errorf("arrive fired %v times from a refused start", res.Metrics.Throughput["arrive"])
	}
}

// TestMaxPlaceTokensRefusesADelayedCompletion: a timed transition's
// completion adds tokens too, and is held to the same cap; the refused one
// stays in flight rather than vanishing.
func TestMaxPlaceTokensRefusesADelayedCompletion(t *testing.T) {
	m := &metamodel.Model{
		Name:        "wash",
		Places:      []metamodel.Place{{ID: "dirty", Initial: 5}, {ID: "clean"}},
		Transitions: []metamodel.Transition{{ID: "wash", Delay: 1}},
		Arcs:        []metamodel.Arc{{From: "dirty", To: "wash"}, {From: "wash", To: "clean", Weight: 2}},
	}
	res, err := Simulate(m, nil, Options{Horizon: 5, MaxPlaceTokens: 6})
	if err != nil {
		t.Fatal(err)
	}
	if res.Final["clean"] != 6 {
		t.Errorf("final clean = %v, want the cap 6", res.Final["clean"])
	}
	if !res.Truncated || !strings.Contains(res.Reason, "raised clean to 8 tokens") {
		t.Errorf("Truncated=%v Reason=%q", res.Truncated, res.Reason)
	}
	if got := res.Metrics.InFlight["wash"]; got != 2 {
		t.Errorf("in flight = %v, want the 2 completions never applied", got)
	}
}

// TestBothLimitsAreReported: when the budget and the cap each stop some
// realization, the Reason carries both, in the order they happened.
func TestBothLimitsAreReported(t *testing.T) {
	res, err := Simulate(source(), nil, Options{Horizon: 1e3, Realizations: 3, MaxPlaceTokens: 100, MaxSteps: 150})
	if err != nil {
		t.Fatal(err)
	}
	capAt := strings.Index(res.Reason, "100-token cap")
	budgetAt := strings.Index(res.Reason, "150-step budget")
	if capAt < 0 || budgetAt < 0 || capAt > budgetAt {
		t.Errorf("Reason = %q, want the cap (realization 0) then the budget (realization 1)", res.Reason)
	}
}

// TestNegativeBoundsAreRefused on every method, so a malformed Options fails
// the same way whichever engine it reaches.
func TestNegativeBoundsAreRefused(t *testing.T) {
	for _, method := range []Method{MethodSSA, MethodODE, MethodSDE} {
		if _, err := Solve(source(), nil, Options{Method: method, MaxSteps: -1}); err == nil || !strings.Contains(err.Error(), "MaxSteps") {
			t.Errorf("%s: MaxSteps=-1 gave %v, want a refusal naming it", method, err)
		}
		if _, err := Solve(source(), nil, Options{Method: method, MaxPlaceTokens: -1}); err == nil || !strings.Contains(err.Error(), "MaxPlaceTokens") {
			t.Errorf("%s: MaxPlaceTokens=-1 gave %v, want a refusal naming it", method, err)
		}
	}
	if _, err := SimulateSchedule(source(), nil, Options{MaxSteps: -1}); err == nil {
		t.Error("SimulateSchedule accepted MaxSteps=-1")
	}
}
