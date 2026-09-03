package stochastic

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// suppliedMachine is a machine that is never short of work and is fed by one
// supplier: `make` needs a job and ten units of stock, and `deliver` brings
// fifty units at a time.
//
// The point of the fixture is the state Depletion cannot see. With deliveries
// worth less than the machine can consume, the stock is refilled and drawn down
// all run — never empty for long, its mean well above the ten units a firing
// takes — and yet it is what the run is waiting for. That was the
// shipped café's milk exactly: 990 units an hour of demand against 1000 of
// supply, an empty Depleted list, and an operator told their idle baristas were
// losing half the trade with no way to reach "buy more milk" from the output.
//
// jobs is stocked far beyond what the horizon can consume and its arc is
// non-kinetic, so the machine's pace is its own rate and the only thing that
// can hold it up is stock.
func suppliedMachine(deliveryRate float64) *metamodel.Model {
	no := false
	return &metamodel.Model{
		Name: "supplied_machine",
		Places: []metamodel.Place{
			{ID: "jobs", Initial: 100000},
			{ID: "stock", Initial: 100},
			{ID: "made"},
		},
		Transitions: []metamodel.Transition{
			{ID: "make", Rate: 60},
			{ID: "deliver", Rate: deliveryRate},
		},
		Arcs: []metamodel.Arc{
			{From: "jobs", To: "make", Kinetic: &no},
			{From: "stock", To: "make", Weight: 10, Kinetic: &no},
			{From: "make", To: "made"},
			{From: "deliver", To: "stock", Weight: 50},
		},
	}
}

var contentionOpts = Options{Horizon: 8, Samples: 480, Realizations: 24, Seed: 20260807}

func contentionOf(t *testing.T, res *Result, place string) Contention {
	t.Helper()
	for _, c := range res.Contended {
		if c.Place == place {
			return c
		}
	}
	return Contention{Place: place}
}

// TestContentionNamesAFullySubscribedResource is the regression test for a run
// that reports a shortage nowhere.
func TestContentionNamesAFullySubscribedResource(t *testing.T) {
	// Deliveries of 50 at rate 9 are 450 units an hour against a machine that
	// would take 600, so the stock is short a quarter of the time and never
	// stays empty — the supply keeps arriving.
	res, err := Simulate(suppliedMachine(9), nil, contentionOpts)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range res.Depleted {
		if d.Place == "stock" {
			t.Fatalf("stock reported as depleted at %.2f — pick numbers that keep it moving, "+
				"this test is about the shortage Depletion cannot see", d.At)
		}
	}

	// A machine that can take 600 units an hour fed 450 of them is idle for the
	// difference: a quarter of the run, give or take the buffer it started with
	// and the lumpiness of a fifty-unit delivery. Both sides matter — under-
	// reporting hides the constraint, over-reporting would mean the measure is
	// counting time nothing was waiting on.
	got := contentionOf(t, res, "stock")
	const want = 1 - 450.0/600
	if got.Fraction < want*0.6 || got.Fraction > want*1.6 {
		t.Errorf("stock held up the machine for %.1f%% of the run; a supply covering %.0f%% of demand "+
			"idles it about %.0f%% of the time", got.Fraction*100, 450.0/600*100, want*100)
	}
	if len(got.Blocking) != 1 || got.Blocking[0] != "make" {
		t.Errorf("stock reported as blocking %v, want [make]", got.Blocking)
	}
	t.Logf("stock: mean %.1f, never depleted, blocking %v for %.0f%% of the run",
		res.Metrics.Mean["stock"], got.Blocking, got.Fraction*100)

	// A place is short of something at some point in almost any run, so the
	// fraction has to be a share of the horizon and not a count of the
	// transitions it inconvenienced: one empty place commonly blocks several at
	// once, and crediting each of them separately reports a place as short for
	// more of the run than the run lasted.
	for _, c := range res.Contended {
		if c.Fraction > 1 {
			t.Errorf("%s was short for %.2f of the horizon, which is more horizon than there is", c.Place, c.Fraction)
		}
	}
}

// TestContentionIsQuietWhenSupplyIsAmple is the other half: the report has to
// be an answer, not an inventory. A shop that is never waiting on its stock
// must not have its stock named.
func TestContentionIsQuietWhenSupplyIsAmple(t *testing.T) {
	res, err := Simulate(suppliedMachine(60), nil, contentionOpts)
	if err != nil {
		t.Fatal(err)
	}
	if got := contentionOf(t, res, "stock"); got.Fraction > 0.01 {
		t.Errorf("stock held up the machine for %.1f%% of the run on 3000 units an hour against 600 of demand",
			got.Fraction*100)
	}

	lean, err := Simulate(suppliedMachine(9), nil, contentionOpts)
	if err != nil {
		t.Fatal(err)
	}
	// And the report has to be about the thing that actually costs throughput:
	// the ample shop makes more.
	if res.Metrics.Throughput["make"] <= lean.Metrics.Throughput["make"] {
		t.Errorf("ample supply made %.0f against the short supply's %.0f — then the shortage cost nothing "+
			"and reporting it is noise", res.Metrics.Throughput["make"], lean.Metrics.Throughput["make"])
	}
}

// TestAScheduledRunStillNamesWhatItWaitedOn is the regression test for a
// diagnostic that vanished the moment a caller asked the question it was built
// for. A schedule exists to describe a rush, and a rush is precisely the
// interval where capacity binds — but the scheduled driver assembled its Result
// field by field and never set Contended, so every scheduled scenario reported
// waiting on nothing. The café console's Rush box showed an empty list for a
// shop running at 87% utilization: an answer of "nothing was short" that no
// input could ever falsify.
//
// Splitting a run at boundaries the rates do not change at must not change what
// it reports, so the schedule here is four segments of one rate: the same run,
// merely observed in pieces. A per-segment fraction cannot be averaged into
// that — it is a share of its own segment — which is why the ledger merges raw
// blocked time and contentions divides once, by the whole horizon.
func TestAScheduledRunStillNamesWhatItWaitedOn(t *testing.T) {
	model := suppliedMachine(9)
	base := contentionOpts

	whole, err := Simulate(model, nil, base)
	if err != nil {
		t.Fatal(err)
	}
	split := base
	split.Schedule = map[string][]metamodel.RateSegment{
		"deliver": {{Until: 2, Value: 9}, {Until: 4, Value: 9}, {Until: 6, Value: 9}, {Until: 8, Value: 9}},
	}
	pieces, err := SimulateSchedule(model, nil, split)
	if err != nil {
		t.Fatal(err)
	}

	if len(pieces.Contended) == 0 {
		t.Fatal("a scheduled run reported waiting on nothing; the machine is fed 450 units an hour " +
			"and can take 600, so there is something to name")
	}
	got := contentionOf(t, pieces, "stock")
	want := contentionOf(t, whole, "stock").Fraction
	if want <= 0 {
		t.Fatalf("the unscheduled run found no contention on stock; the fixture, not the schedule, is wrong")
	}
	if got.Fraction < want*0.6 || got.Fraction > want*1.4 {
		t.Errorf("stock held up the machine for %.1f%% of the scheduled run against %.1f%% of the same run "+
			"unsplit; cutting a run into segments at rates that never change must not move what it waited on",
			got.Fraction*100, want*100)
	}
	if len(got.Blocking) != 1 || got.Blocking[0] != "make" {
		t.Errorf("stock reported as blocking %v, want [make]", got.Blocking)
	}
	for _, c := range pieces.Contended {
		if c.Fraction > 1 {
			t.Errorf("%s was short for %.2f of the horizon, which is more horizon than there is — "+
				"a merged ledger divided by a segment's span rather than the run's", c.Place, c.Fraction)
		}
	}
}
