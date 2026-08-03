package metapetri_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/verify"
)

// TestPermissiveIsMonotoneWhenCapacityIsInPlay defends the claim the whole
// direction model rests on: dropping a guard only ever ADDS behaviour.
//
// It is not obviously true once capacity exists, because a newly-enabled
// transition can fill a place to its bound and thereby DISABLE another one —
// so dropping a constraint appears to remove a firing. The net below is
// exactly that shape: T is gated, U and T both produce into a capacity-1
// place, and once the gate is dropped T can fire first and lock U out.
//
// It still holds, because a Petri net's semantics is a SET of runs, not one
// run: the analysed net keeps the run in which T does not fire. Inclusion is
// checked over reachable markings, which is what every verdict quantifies over.
// If this test ever fails, Direction needs an Incomparable value and Verify has
// to cap the Permissive column as hard as it caps Restrictive.
func TestPermissiveIsMonotoneWhenCapacityIsInPlay(t *testing.T) {
	// gate=true expresses the guard as an inhibitor arc, which reachability
	// evaluates; gate=false states the same condition as guard text, which it
	// does not. The two nets are otherwise identical.
	build := func(structural bool) *metamodel.Model {
		m := &metamodel.Model{
			Name: "capgate",
			Places: []metamodel.Place{
				{ID: "a", Initial: 1},
				{ID: "b", Initial: 0, Capacity: 1},
				{ID: "c", Initial: 1},
				{ID: "g", Initial: 1},
			},
			Transitions: []metamodel.Transition{{ID: "T"}, {ID: "U"}},
			Arcs: []metamodel.Arc{
				{From: "a", To: "T", Weight: 1},
				{From: "T", To: "b", Weight: 1},
				{From: "c", To: "U", Weight: 1},
				{From: "U", To: "b", Weight: 1},
			},
		}
		if structural {
			m.Arcs = append(m.Arcs, metamodel.Arc{From: "g", To: "T", Type: metamodel.InhibitorArc, Weight: 1})
			return m
		}
		m.Transitions[0].Guard = `tokens("g") == 0`
		return m
	}

	model, err := metapetri.Convert(build(true), metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert(structural): %v", err)
	}
	analysed, err := metapetri.Convert(build(false), metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert(guard text): %v", err)
	}
	if model.Diag.Overapproximates() {
		t.Fatalf("the structural net loses nothing; notes:\n%s", model.Diag)
	}
	if !analysed.Diag.Overapproximates() {
		t.Fatalf("the guard-text net drops a constraint and must say so; notes:\n%s", analysed.Diag)
	}

	strict, loose := reachableMarkings(t, model), reachableMarkings(t, analysed)
	for m := range strict {
		if !loose[m] {
			t.Errorf("marking %s is reachable in the model but NOT in the net called permissive: "+
				"the capacity interaction breaks monotonicity and Direction needs an Incomparable value", m)
		}
	}
	if len(loose) <= len(strict) {
		t.Errorf("the guard-dropped net should admit strictly more markings (%d) than the model (%d); "+
			"if it does not, this test no longer exercises the interaction it was written for",
			len(loose), len(strict))
	}
}

// TestCapacityBoundedDataPlaceDropIsPermissive: a dropped data place gates
// firings through its CAPACITY as well as through its outgoing arcs.
// reachability disables a firing that would push an output place past its
// bound, so dropping a capacity-1 data place that a transition writes to lets
// that transition fire forever. Keying "does this gate anything?" on outgoing
// arcs alone reported the drop as Lossless, and every verdict then went
// uncapped.
func TestCapacityBoundedDataPlaceDropIsPermissive(t *testing.T) {
	dataPlace := func(capacity int) *metamodel.Model {
		return &metamodel.Model{
			Name: "datacap",
			Places: []metamodel.Place{
				{ID: "src", Initial: 3},
				{ID: "log", Kind: metamodel.DataKind, Type: "map[string]int", Capacity: capacity},
			},
			Transitions: []metamodel.Transition{{ID: "write"}},
			Arcs: []metamodel.Arc{
				{From: "src", To: "write", Weight: 1},
				{From: "write", To: "log", Weight: 1},
			},
		}
	}

	bounded, err := metapetri.Convert(dataPlace(1), metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !bounded.Diag.Overapproximates() {
		t.Errorf("dropping a capacity-1 data place removes the bound that stops `write` firing three times, "+
			"which is a lost constraint; notes:\n%s", bounded.Diag)
	}

	// The mirror case keeps the note honest: an unbounded data place absorbs
	// any production, so losing it changes no enablement.
	unbounded, err := metapetri.Convert(dataPlace(0), metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if unbounded.Diag.Overapproximates() {
		t.Errorf("an unbounded write-only data place gates nothing, so dropping it is lossless; notes:\n%s",
			unbounded.Diag)
	}
}

// TestPropertyNamingDroppedPlaceIsNotProved: a dropped place reads as zero
// tokens, so "the marking log=7 is unreachable" came back proved/exhaustive —
// a confident answer about a place the analysed net does not contain. The drop
// itself is genuinely lossless (the place gates nothing), so the direction
// table never gets a chance to cap it; the refusal has to come from the
// property naming a place the net has not got.
func TestPropertyNamingDroppedPlaceIsNotProved(t *testing.T) {
	m := &metamodel.Model{
		Name: "vacuous",
		Places: []metamodel.Place{
			{ID: "p", Initial: 1},
			{ID: "cfg", Kind: metamodel.DataKind, Type: "map[string]int", Initial: 7},
		},
		Transitions: []metamodel.Transition{{ID: "t"}},
		Arcs: []metamodel.Arc{
			{From: "p", To: "t", Weight: 1},
			{From: "t", To: "cfg", Weight: 1},
		},
	}
	res, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Diag.Overapproximates() || res.Diag.Underapproximates() {
		t.Fatalf("this drop is lossless, so no direction capping applies; notes:\n%s", res.Diag)
	}

	report, err := metapetri.Verify(res,
		verify.Property{Kind: verify.KindUnreachable, Target: map[string]int{"cfg": 7}},
		verify.Property{Kind: verify.KindReachable, Target: map[string]int{"cfg": 1}},
	)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	for _, vd := range report.Verdicts {
		if vd.Status != verify.Unknown {
			t.Errorf("%s over dropped place cfg = %s (%s): %s", vd.Property.Kind, vd.Status, vd.Method, vd.Detail)
		}
		if !strings.Contains(vd.Detail, "cfg") {
			t.Errorf("%s verdict does not name the missing place: %s", vd.Property.Kind, vd.Detail)
		}
	}
}

// TestTokenizedDataPlaceCannotRefuteWholeNetProperties covers the case the
// Direction model structurally cannot: a Direction compares firing SEQUENCES,
// but TokenizeData changes the MARKING VECTOR by adding a coordinate the model
// does not have. "bounded" and "conserves" quantify over every place, so a data
// place that is merely written repeatedly refutes both — and Restrictive trusts
// refutations, so the wrong answer was reported with confidence.
func TestTokenizedDataPlaceCannotRefuteWholeNetProperties(t *testing.T) {
	m := &metamodel.Model{
		Name: "audit",
		Places: []metamodel.Place{
			{ID: "p", Initial: 1},
			{ID: "log", Kind: metamodel.DataKind, Type: "map[string]int"},
		},
		Transitions: []metamodel.Transition{{ID: "tick"}},
		Arcs: []metamodel.Arc{
			{From: "p", To: "tick", Weight: 1},
			{From: "tick", To: "p", Weight: 1},
			{From: "tick", To: "log", Weight: 1},
		},
	}

	res, err := metapetri.Convert(m, metapetri.Options{TokenizeData: true})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(res.Tokenized) != 1 || res.Tokenized[0] != "log" {
		t.Fatalf("Tokenized = %v, want [log]", res.Tokenized)
	}

	report, err := metapetri.Verify(res,
		verify.Property{Kind: verify.KindBounded},
		verify.Property{Kind: verify.KindConserves},
		verify.Property{Kind: verify.KindInvariant, Expr: "log <= 1"},
	)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	for _, vd := range report.Verdicts {
		if vd.Status != verify.Unknown {
			t.Errorf("%s = %s on the tokenized net; the token place p is bounded and conserved, and the only "+
				"counterexample is the fabricated count in log: %s", vd.Property.Kind, vd.Status, vd.Detail)
		}
		if !strings.Contains(vd.Detail, "log") {
			t.Errorf("%s verdict does not name the tokenized place: %s", vd.Property.Kind, vd.Detail)
		}
	}

	// The cap is scoped to the fabricated dimension, not applied to the whole
	// report: this target names only a token place, and a witness found in a
	// net that merely added constraints is a real firing sequence of the
	// model, so the verdict stands.
	report, err = metapetri.Verify(res, verify.Property{Kind: verify.KindReachable, Target: map[string]int{"p": 1}})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := report.Verdicts[0].Status; got != verify.Proved {
		t.Errorf("reachability of a token-place-only target = %s, want proved — the tokenized-place cap must "+
			"not swallow verdicts whose scope never touches it: %s", got, report.Verdicts[0].Detail)
	}
}

// reachableMarkings renders the reachable markings of a converted net as a set
// of stable strings, so two nets can be compared for inclusion.
func reachableMarkings(t *testing.T, res *metapetri.Result) map[string]bool {
	t.Helper()
	r := reachability.NewAnalyzer(res.Net).
		WithInitialMarking(res.Marking).
		WithMaxStates(10000).
		BuildGraph()
	if r.Truncated {
		t.Fatalf("state space truncated at %d states; the fixture is meant to be tiny", r.StateCount)
	}

	out := make(map[string]bool, len(r.Graph.StatesList()))
	for _, st := range r.Graph.StatesList() {
		parts := make([]string, 0, len(st.Marking))
		for place, n := range st.Marking {
			parts = append(parts, fmt.Sprintf("%s=%d", place, n))
		}
		sort.Strings(parts)
		out["{"+strings.Join(parts, " ")+"}"] = true
	}
	return out
}
