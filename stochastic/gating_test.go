package stochastic

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// stubGuard is the hermetic stand-in for a guard language. It decides exactly
// one expression — the marking guard these tests use — and errors on every
// other, which is the contract GuardFunc promises for an expression the
// marking cannot supply: the parameter guard below must still be caveated
// because the stub cannot decide it.
func stubGuard(expr string, mk metamodel.Marking) (bool, error) {
	if expr == `tokens("reserve") >= 5` {
		return mk["reserve"] >= 5, nil
	}
	return false, fmt.Errorf("stubGuard: cannot evaluate %q", expr)
}

// staffedShop is the smallest model in which "add a barista" is a question with
// an answer: orders arrive, a barista is held for the whole brew, and only then
// is a drink served.
//
// The staffing split is the point. An atomic make_drink that seized and
// released a barista in one firing would leave the pool untouched at every
// observable instant, so headcount could not possibly change the outcome.
func staffedShop(baristas int) *metamodel.Model {
	return &metamodel.Model{
		Name: "staffed",
		Places: []metamodel.Place{
			{ID: "queue"},
			{ID: "brewing"},
			{ID: "served"},
			{ID: "available", Initial: baristas},
			{ID: "busy"},
		},
		Transitions: []metamodel.Transition{
			{ID: "arrive", Rate: 12},
			{ID: "start", Rate: 60},
			{ID: "finish", Rate: 20},
		},
		Arcs: []metamodel.Arc{
			{From: "arrive", To: "queue"},
			{From: "queue", To: "start"},
			{From: "available", To: "start"},
			{From: "start", To: "brewing"},
			{From: "start", To: "busy"},
			{From: "brewing", To: "finish"},
			{From: "busy", To: "finish"},
			{From: "finish", To: "served"},
			{From: "finish", To: "available"},
		},
	}
}

// TestStaffingChangesTheOutcome is the regression test for the whole firing-rule
// fix. Before it, the SSA engine ignored every constraint that was not a
// consuming arc, so two runs differing only in headcount were identical.
func TestStaffingChangesTheOutcome(t *testing.T) {
	opts := Options{Horizon: 8, Samples: 60, Realizations: 12, Seed: 7}

	one, err := Simulate(staffedShop(1), map[string]int{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	three, err := Simulate(staffedShop(3), map[string]int{}, opts)
	if err != nil {
		t.Fatal(err)
	}

	// The marking is read from the model's own initial values when the caller
	// passes nothing, so these two runs differ only in headcount.
	oneOut := one.Metrics.Throughput["finish"]
	threeOut := three.Metrics.Throughput["finish"]
	if threeOut <= oneOut {
		t.Errorf("three baristas served %.1f drinks, one served %.1f: headcount did not help", threeOut, oneOut)
	}

	oneQueue := one.Metrics.P95["queue"]
	threeQueue := three.Metrics.P95["queue"]
	if threeQueue >= oneQueue {
		t.Errorf("P95 queue was %.1f with three baristas and %.1f with one: staffing did not relieve the queue",
			threeQueue, oneQueue)
	}
	t.Logf("1 barista: %.1f served, P95 queue %.1f, utilization %.2f",
		oneOut, oneQueue, one.Metrics.Utilization["pool"])
	t.Logf("3 baristas: %.1f served, P95 queue %.1f, utilization %.2f",
		threeOut, threeQueue, three.Metrics.Utilization["pool"])
}

// TestPoolIsConserved: a barista never disappears and is never duplicated. If
// the engine let `start` fire with no one available, this is what would break.
func TestPoolIsConserved(t *testing.T) {
	res, err := Simulate(staffedShop(2), map[string]int{}, Options{Horizon: 12, Samples: 40, Realizations: 5, Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	total := res.Final["available"] + res.Final["busy"]
	if math := total - 2.0; math > 1e-9 || math < -1e-9 {
		t.Errorf("available + busy = %.6f, want 2 — the pool leaked", total)
	}
	if res.Metrics.Utilization["pool"] <= 0 {
		t.Errorf("utilization was %v; with a queue this deep the baristas should be working",
			res.Metrics.Utilization)
	}
}

// TestInhibitorArcBlocks: an inhibitor is a hard stop, not a slowdown.
func TestInhibitorArcBlocks(t *testing.T) {
	m := &metamodel.Model{
		Name: "closed",
		Places: []metamodel.Place{
			{ID: "orders", Initial: 5},
			{ID: "done"},
			{ID: "closed_sign", Initial: 1},
		},
		Transitions: []metamodel.Transition{{ID: "serve", Rate: 50}},
		Arcs: []metamodel.Arc{
			{From: "orders", To: "serve"},
			{From: "serve", To: "done"},
			{From: "closed_sign", To: "serve", Type: metamodel.InhibitorArc},
		},
	}

	res, err := Simulate(m, map[string]int{}, Options{Horizon: 10, Samples: 20, Realizations: 3, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Metrics.Throughput["serve"]; got != 0 {
		t.Errorf("serve fired %.1f times while inhibited", got)
	}
	if res.Final["orders"] != 5 {
		t.Errorf("orders = %.1f, want 5: tokens moved through an inhibited transition", res.Final["orders"])
	}
}

// TestReadArcGatesWithoutConsuming: a read arc must block below its threshold
// and still be intact afterwards. Consuming it — which the ODE builder did —
// disables the transition after one firing and looks like a throughput bug.
func TestReadArcGatesWithoutConsuming(t *testing.T) {
	m := func(licences int) *metamodel.Model {
		return &metamodel.Model{
			Name: "licensed",
			Places: []metamodel.Place{
				{ID: "orders", Initial: 20},
				{ID: "done"},
				{ID: "licence", Initial: licences},
			},
			Transitions: []metamodel.Transition{{ID: "serve", Rate: 30}},
			Arcs: []metamodel.Arc{
				{From: "orders", To: "serve"},
				{From: "serve", To: "done"},
				{From: "licence", To: "serve", Type: metamodel.ReadArc},
			},
		}
	}
	opts := Options{Horizon: 5, Samples: 20, Realizations: 3, Seed: 2}

	without, err := Simulate(m(0), map[string]int{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got := without.Metrics.Throughput["serve"]; got != 0 {
		t.Errorf("serve fired %.1f times with no licence", got)
	}

	with, err := Simulate(m(1), map[string]int{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if with.Metrics.Throughput["serve"] == 0 {
		t.Fatal("serve never fired with a licence present")
	}
	if with.Final["licence"] != 1 {
		t.Errorf("licence = %.1f, want 1: a read arc consumed the thing it only tests", with.Final["licence"])
	}
}

// TestCapacityIsAPostFiringBound: restocking must not overfill the hopper.
func TestCapacityIsAPostFiringBound(t *testing.T) {
	m := &metamodel.Model{
		Name: "hopper",
		Places: []metamodel.Place{
			{ID: "delivery", Initial: 100},
			{ID: "beans", Capacity: 10},
		},
		Transitions: []metamodel.Transition{{ID: "restock", Rate: 40}},
		Arcs: []metamodel.Arc{
			{From: "delivery", To: "restock"},
			{From: "restock", To: "beans", Weight: 4},
		},
	}
	res, err := Simulate(m, map[string]int{}, Options{Horizon: 10, Samples: 40, Realizations: 3, Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Series {
		if s.Place != "beans" {
			continue
		}
		for i, v := range s.Values {
			if v > 10 {
				t.Fatalf("beans reached %.1f at t=%.2f, over the declared capacity of 10", v, res.Times[i])
			}
		}
	}
	// And the bound must bind rather than merely not be exceeded by luck: 4 at a
	// time into a 10-capacity place stops at 8.
	if res.Final["beans"] != 8 {
		t.Errorf("beans settled at %.1f, want 8 (the last firing that fits)", res.Final["beans"])
	}
}

// TestMarkingGuardIsEnforced: a guard decidable from token counts alone is part
// of the firing rule, not decoration.
func TestMarkingGuardIsEnforced(t *testing.T) {
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
	res, err := Simulate(m, map[string]int{}, Options{Horizon: 5, Samples: 20, Realizations: 2, Seed: 1, Guard: stubGuard})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Metrics.Throughput["serve"]; got != 0 {
		t.Errorf("serve fired %.1f times with its guard false", got)
	}
	if len(res.Caveats) != 0 {
		t.Errorf("a marking-decidable guard should be enforced, not caveated: %v", res.Caveats)
	}
}

// TestParameterGuardIsCaveatedNotGuessed: a guard needing an action parameter
// cannot be settled from the marking. Saying so beats both silently enforcing
// it and silently ignoring it.
func TestParameterGuardIsCaveatedNotGuessed(t *testing.T) {
	m := &metamodel.Model{
		Name:        "parameterised",
		Places:      []metamodel.Place{{ID: "orders", Initial: 5}, {ID: "done"}},
		Transitions: []metamodel.Transition{{ID: "serve", Rate: 20, Guard: "amount > 0"}},
		Arcs:        []metamodel.Arc{{From: "orders", To: "serve"}, {From: "serve", To: "done"}},
	}
	res, err := Simulate(m, map[string]int{}, Options{Horizon: 5, Samples: 10, Seed: 1, Guard: stubGuard})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Caveats) == 0 {
		t.Fatal("no caveat for a guard this engine cannot evaluate")
	}
	if !strings.Contains(strings.Join(res.Caveats, " "), "serve") {
		t.Errorf("the caveat does not name the transition: %v", res.Caveats)
	}
}

// TestForecastRefusesGatedModels: the continuous engine has no firing instant,
// so it cannot test a read arc, an inhibitor, a capacity or a guard. Returning
// a smooth wrong curve is worse than returning nothing, because a dashboard
// will plot it either way.
func TestForecastRefusesGatedModels(t *testing.T) {
	m := staffedShop(2)
	m.Places = append(m.Places, metamodel.Place{ID: "licence", Initial: 1})
	m.Arcs = append(m.Arcs, metamodel.Arc{From: "licence", To: "start", Type: metamodel.ReadArc})

	res, err := Forecast(m, map[string]int{}, Options{Horizon: 8, Samples: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Diverged {
		t.Fatal("forecast answered a model whose constraints it cannot honour")
	}
	if !strings.Contains(res.Reason, "Simulate") {
		t.Errorf("the refusal does not point at the engine that can answer: %q", res.Reason)
	}
	if len(res.Caveats) == 0 {
		t.Error("refused without naming what it could not honour")
	}
}

// TestForecastStillAnswersUngatedModels: the refusal must be narrow, or it is
// just a broken forecast endpoint. A plain net has no gating — and neither does
// a resource pool built from ordinary consuming arcs, which is why the café's
// staffing is modelled that way rather than with a read arc.
func TestForecastStillAnswersUngatedModels(t *testing.T) {
	for _, tc := range []struct {
		name  string
		model *metamodel.Model
		grew  string
	}{
		{
			name: "cascade",
			model: &metamodel.Model{
				Name:        "cascade",
				Places:      []metamodel.Place{{ID: "a", Initial: 100}, {ID: "b"}},
				Transitions: []metamodel.Transition{{ID: "flow", Rate: 1}},
				Arcs:        []metamodel.Arc{{From: "a", To: "flow"}, {From: "flow", To: "b"}},
			},
			grew: "b",
		},
		{name: "resource pool", model: staffedShop(2), grew: "served"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Forecast(tc.model, map[string]int{}, Options{Horizon: 5, Samples: 20})
			if err != nil {
				t.Fatal(err)
			}
			if res.Diverged {
				t.Fatalf("refused an ungated net: %s", res.Reason)
			}
			if res.Final[tc.grew] <= 0 {
				t.Errorf("nothing flowed into %s: %v", tc.grew, res.Final)
			}
		})
	}
}

// TestSSAAgreesWithTheSharedRule pins the engine's index-addressed gating to
// metamodel's firing rule. They are two implementations of one definition; the
// day they disagree is the day this whole consolidation stops being worth
// anything.
func TestSSAAgreesWithTheSharedRule(t *testing.T) {
	m := staffedShop(2)
	m.Places = append(m.Places, metamodel.Place{ID: "hopper", Capacity: 3})
	m.Arcs = append(m.Arcs,
		metamodel.Arc{From: "finish", To: "hopper"},
		metamodel.Arc{From: "hopper", To: "start", Type: metamodel.ReadArc})

	trs, places, _, err := compile(m, nil, stubGuard)
	if err != nil {
		t.Fatal(err)
	}
	index := map[string]int{}
	for i, p := range places {
		index[p] = i
	}

	// Walk a spread of markings rather than a single one: the disagreements
	// worth catching are at the boundaries.
	for _, mk := range []metamodel.Marking{
		{},
		{"queue": 1, "available": 2},
		{"queue": 1, "available": 0, "busy": 2},
		{"queue": 3, "available": 1, "hopper": 0},
		{"queue": 3, "available": 1, "hopper": 3},
		{"brewing": 1, "busy": 1, "hopper": 3},
	} {
		vec := make([]int, len(places))
		for p, n := range mk {
			if i, ok := index[p]; ok {
				vec[i] = n
			}
		}
		for i := range trs {
			id := trs[i].id
			want := m.Enabled(id, mk)

			enough := true
			for _, in := range trs[i].inputs {
				if vec[in.place] < in.weight {
					enough = false
					break
				}
			}
			got := enough && trs[i].gated(vec) && trs[i].allows(places, vec)

			if got != want {
				t.Errorf("at %v, engine says %s enabled=%v but metamodel says %v (%v)",
					mk, id, got, want, m.EnabledWhyNot(id, mk))
			}
		}
	}
}

// TestDepletionDistinguishesAPoolFromStock.
//
// Both hit zero, and until Recovered existed both were reported identically —
// which told a café owner their staff had run out when what had actually
// happened was that every barista was busy for a moment. A pool refills itself
// by construction; stock does not.
func TestDepletionDistinguishesAPoolFromStock(t *testing.T) {
	m := staffedShop(1)
	// Give the shop a finite stock that nothing replaces.
	m.Places = append(m.Places, metamodel.Place{ID: "beans", Initial: 20})
	m.Arcs = append(m.Arcs, metamodel.Arc{From: "beans", To: "start", Weight: 2})

	res, err := Simulate(m, map[string]int{}, Options{Horizon: 8, Samples: 60, Realizations: 1, Seed: 4})
	if err != nil {
		t.Fatal(err)
	}

	byPlace := map[string]Depletion{}
	for _, d := range res.Depleted {
		byPlace[d.Place] = d
	}

	pool, sawPool := byPlace["available"]
	if !sawPool {
		t.Fatal("the barista pool never emptied; this fixture is not exercising the case")
	}
	if !pool.Recovered {
		t.Error("the barista pool was reported as gone for good; a pool refills itself")
	}

	stock, sawStock := byPlace["beans"]
	if !sawStock {
		t.Fatalf("beans never ran out over 8h from 20 with a weight-2 arc; final = %v", res.Final)
	}
	if stock.Recovered {
		t.Error("beans were reported as recovered, but nothing restocks them")
	}
	t.Logf("pool empty at %.2fh (recovered=%v); beans out at %.2fh (recovered=%v)",
		pool.At, pool.Recovered, stock.At, stock.Recovered)
}

// TestNilGuardCaveatsLikeAParameterGuard pins the injection seam: with no
// evaluator at all, a marking-decidable guard gets the same caveat a parameter
// guard gets, rather than being silently enforced or silently dropped.
func TestNilGuardCaveatsLikeAParameterGuard(t *testing.T) {
	m := &metamodel.Model{
		Name:        "guarded",
		Places:      []metamodel.Place{{ID: "orders", Initial: 10}, {ID: "done"}, {ID: "reserve", Initial: 3}},
		Transitions: []metamodel.Transition{{ID: "serve", Rate: 40, Guard: `tokens("reserve") >= 5`}},
		Arcs:        []metamodel.Arc{{From: "orders", To: "serve"}, {From: "serve", To: "done"}},
	}
	withNil, err := Simulate(m, map[string]int{}, Options{Horizon: 5, Samples: 10, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := &metamodel.Model{
		Name:        "parameterised",
		Places:      []metamodel.Place{{ID: "orders", Initial: 5}, {ID: "done"}},
		Transitions: []metamodel.Transition{{ID: "serve", Rate: 20, Guard: "amount > 0"}},
		Arcs:        []metamodel.Arc{{From: "orders", To: "serve"}, {From: "serve", To: "done"}},
	}
	withStub, err := Simulate(p, map[string]int{}, Options{Horizon: 5, Samples: 10, Seed: 1, Guard: stubGuard})
	if err != nil {
		t.Fatal(err)
	}
	if len(withNil.Caveats) != 1 || len(withStub.Caveats) != 1 || withNil.Caveats[0] != withStub.Caveats[0] {
		t.Errorf("nil guard caveats %v, stub-on-parameter-guard caveats %v; want the same single string",
			withNil.Caveats, withStub.Caveats)
	}
	if withNil.Metrics.Throughput["serve"] == 0 {
		t.Error("with no evaluator the guard must not be enforced; serve never fired")
	}
}
