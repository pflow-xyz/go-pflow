package metapetri_test

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/verify"
)

// TestArcOrderIsIndependentOfAuthoringOrder: petri.PetriNet keeps arcs in a
// slice, so anything that hashes or renders a converted net would otherwise
// depend on the order the model happened to list them in.
func TestArcOrderIsIndependentOfAuthoringOrder(t *testing.T) {
	places := []metamodel.Place{
		{ID: "a", Kind: metamodel.TokenKind, Initial: 1},
		{ID: "b", Kind: metamodel.TokenKind},
		{ID: "c", Kind: metamodel.TokenKind},
	}
	trans := []metamodel.Transition{{ID: "t"}}
	forward := []metamodel.Arc{
		{From: "a", To: "t", Weight: 1},
		{From: "b", To: "t", Weight: 2, Type: metamodel.InhibitorArc},
		{From: "t", To: "c", Weight: 3},
	}
	reversed := []metamodel.Arc{forward[2], forward[1], forward[0]}

	key := func(arcs []metamodel.Arc) []string {
		res, err := metapetri.Convert(&metamodel.Model{
			Name: "m", Places: places, Transitions: trans, Arcs: arcs,
		}, metapetri.Options{})
		if err != nil {
			t.Fatalf("Convert: %v", err)
		}
		out := make([]string, 0, len(res.Net.Arcs))
		for _, a := range res.Net.Arcs {
			out = append(out, a.Source+"->"+a.Target)
		}
		return out
	}

	got, want := key(reversed), key(forward)
	if len(got) != len(want) {
		t.Fatalf("arc counts differ: %v vs %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arc order depends on authoring order:\n got %v\nwant %v", got, want)
		}
	}
}

// TestCapacityIsHonoured pins the difference between the two test-local bridges
// this package replaced: metamodel's honoured Place.Capacity, statemachine's
// did not. Ignoring it analyses a strictly larger state space than the model
// has, which is the permissive direction — silently.
func TestCapacityIsHonoured(t *testing.T) {
	m := &metamodel.Model{
		Name: "capped",
		Places: []metamodel.Place{
			{ID: "src", Kind: metamodel.TokenKind, Initial: 5},
			{ID: "sink", Kind: metamodel.TokenKind, Capacity: 2},
		},
		Transitions: []metamodel.Transition{{ID: "move"}},
		Arcs: []metamodel.Arc{
			{From: "src", To: "move", Weight: 1},
			{From: "move", To: "sink", Weight: 1},
		},
	}

	res, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if cap := res.Net.Places["sink"].Capacity; len(cap) != 1 || cap[0] != 2 {
		t.Fatalf("sink capacity = %v, want [2]", cap)
	}
	if unbounded := res.Net.Places["src"].Capacity; len(unbounded) != 0 {
		t.Errorf("src capacity = %v; capacity 0 means unbounded and must not be emitted as a bound", unbounded)
	}

	// Without the capacity, all six markings of src+sink are reachable.
	states := reachability.NewAnalyzer(res.Net).WithMaxStates(1000).Analyze().StateCount
	if states != 3 {
		t.Errorf("explored %d states, want 3 (sink stops at its capacity of 2)", states)
	}
}

// TestUnknownArcEndpointIsAnError: silently skipping an arc that names nothing
// would be another invisible over-approximation.
func TestUnknownArcEndpointIsAnError(t *testing.T) {
	_, err := metapetri.Convert(&metamodel.Model{
		Name:        "broken",
		Places:      []metamodel.Place{{ID: "a", Kind: metamodel.TokenKind}},
		Transitions: []metamodel.Transition{{ID: "t"}},
		Arcs:        []metamodel.Arc{{From: "a", To: "nope"}},
	}, metapetri.Options{})
	if err == nil {
		t.Fatal("an arc naming an element that does not exist must be an error, not a dropped constraint")
	}
}

// TestInhibitorWeightIsThreshold: the weight is a threshold, so a weight-2
// inhibitor still permits one token in the observed place.
func TestInhibitorWeightIsThreshold(t *testing.T) {
	m := &metamodel.Model{
		Name: "inhib",
		Places: []metamodel.Place{
			{ID: "guardp", Kind: metamodel.TokenKind, Initial: 1},
			{ID: "src", Kind: metamodel.TokenKind, Initial: 1},
			{ID: "dst", Kind: metamodel.TokenKind},
		},
		Transitions: []metamodel.Transition{{ID: "t"}},
		Arcs: []metamodel.Arc{
			{From: "guardp", To: "t", Weight: 2, Type: metamodel.InhibitorArc},
			{From: "src", To: "t", Weight: 1},
			{From: "t", To: "dst", Weight: 1},
		},
	}
	res, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Diag.Overapproximates() || res.Diag.Underapproximates() {
		t.Fatalf("an inhibitor weight is representable, so nothing is lost; notes:\n%s", res.Diag)
	}

	report, err := metapetri.Verify(res, verify.Property{
		Kind:   verify.KindReachable,
		Target: map[string]int{"dst": 1},
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := report.Verdicts[0].Status; got != verify.Proved {
		t.Errorf("dst == 1 is %s; one token does not reach the weight-2 inhibitor threshold, so t stays enabled", got)
	}
}

// TestReadArcBecomesReversedInhibitor pins the bridge rule. petri.Arc must NOT
// grow a Read field — it is the wire format shared with parser/json.go and
// every JS consumer — and it does not need one: an inhibitor arc pointing
// transition -> place is already enabled only while the place holds at least
// the weight, which is exactly read-arc semantics. So the encoding is a
// reversal, and the conversion is lossless.
func TestReadArcBecomesReversedInhibitor(t *testing.T) {
	m := &metamodel.Model{
		Name: "reads",
		Places: []metamodel.Place{
			{ID: "in", Kind: metamodel.TokenKind, Initial: 1},
			{ID: "gate", Kind: metamodel.TokenKind, Initial: 2},
		},
		Transitions: []metamodel.Transition{{ID: "t"}},
		Arcs: []metamodel.Arc{
			{From: "in", To: "t", Weight: 1},
			{From: "gate", To: "t", Weight: 2, Type: metamodel.ReadArc},
		},
	}

	res, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Diag.Overapproximates() || res.Diag.Underapproximates() {
		t.Fatalf("a read arc is carried across exactly; notes:\n%s", res.Diag)
	}

	var found bool
	for _, a := range res.Net.Arcs {
		if a.Source == "t" && a.Target == "gate" {
			found = true
			if !a.InhibitTransition || a.GetWeightSum() != 2 {
				t.Errorf("read arc encoded as %+v, want a weight-2 inhibitor from t to gate", a)
			}
		}
		if a.Source == "gate" && a.Target == "t" {
			t.Errorf("a read arc must not survive as a forward arc: %+v", a)
		}
	}
	if !found {
		t.Fatalf("read arc was dropped; arcs = %+v", res.Net.Arcs)
	}

	// The encoding has to gate the analysis, not merely appear in it.
	report, err := metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := report.Verdicts[0].Status; got != verify.Proved {
		t.Errorf("KindLive = %s, want proved: gate holds the 2 tokens t reads", got)
	}

	m.Places[1].Initial = 1 // one short of the read weight
	res, err = metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	report, err = metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := report.Verdicts[0].Status; got != verify.Refuted {
		t.Errorf("KindLive = %s, want refuted: the read arc is one token short", got)
	}
}

// TestUnknownArcTypeIsRefused: an arc type this build cannot execute must stop
// the conversion. Analysing it as a normal arc would consume tokens the arc
// never meant to move, and every verdict downstream would be about a different
// net than the one on disk.
func TestUnknownArcTypeIsRefused(t *testing.T) {
	m := &metamodel.Model{
		Name:        "future",
		Places:      []metamodel.Place{{ID: "p", Kind: metamodel.TokenKind, Initial: 1}},
		Transitions: []metamodel.Transition{{ID: "t"}},
		Arcs:        []metamodel.Arc{{From: "p", To: "t", Weight: 1, Type: metamodel.ArcType("reset")}},
	}
	if _, err := metapetri.Convert(m, metapetri.Options{}); err == nil {
		t.Fatal("Convert accepted an arc type it cannot execute")
	}
}
