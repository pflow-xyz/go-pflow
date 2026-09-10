package parser

import (
	"os"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/stochastic"
	"github.com/pflow-xyz/go-pflow/verify"
)

// The fixtures under testdata/pnml are real PNML files pulled from public
// GitHub repositories (GreatSPN Editor exports, an ePNK/pnml.org sample, an
// LTS-to-PNML conversion, and vanrein/perpetuum's inhibitor-arc traffic
// light), not hand-authored — so a parse or simulation failure here means
// FromPNML disagrees with what real tools actually write, not with an
// idealized shape this importer invented for itself.
func TestFromPNML(t *testing.T) {
	tests := []struct {
		file        string
		places      int
		transitions int
		arcs        int
		states      int
		bounded     bool
		deadlocks   int
		hasCycle    bool
	}{
		// GreatSPN "PT1": a two-place chain P0 -[T0]-> P1, one token at P0.
		// By hand: {P0:1,P1:0} -T0-> {P0:0,P1:1}, dead end. 2 states, 1 deadlock.
		{"pt1-chain.pnml", 2, 1, 2, 2, true, 1, false},

		// GreatSPN "PT3": a fork with NO initial marking at all (every place
		// starts at 0), so T2's sole input P2 is never present and nothing
		// ever fires. Exactly the initial marking, 1 reachable state.
		{"pt3-fork.pnml", 3, 1, 3, 1, true, 0, false},

		// The ePNK/pnml.org reference sample: one place "ready" (initial
		// marking 3) with a weight-2 arc into a transition with no outputs.
		// By hand: {ready:3} -t1-> {ready:1}, dead end (1 < 2). 2 states.
		{"sample-ptnet.pnml", 1, 1, 1, 2, true, 1, false},

		// vanrein/perpetuum's traffic light: a 3-place/3-transition cycle
		// (green->caution->yellow->stop->red->go->green) composed with an
		// independent 1-token night_service toggle gated by two inhibitor
		// arcs (id7, id10 — the real-world ad hoc `<type value="inhibitor"/>`
		// convention this importer specifically supports). By hand: the two
		// dimensions are independent and never block each other, so the
		// reachable set is their product: 3 traffic states x 2 night states
		// = 6. This is the strongest correctness check in this file, since
		// it was derived independently of running the code.
		{"traffic-light.pnml", 4, 5, 10, 6, true, 0, true},

		// A deliberately scrambled net (utwente-fmt/BW-NFM2016): place IDs
		// and their <name> text are intentionally mismatched (id "p1" is
		// named "p4" and vice versa) -- this is the regression check that
		// FromPNML keys everything off the PNML id, never the display name.
		// State count is a pinned regression golden, not hand-derived.
		{"scrambled-cycle.pnml", 5, 6, 14, 5, true, 0, true},

		// An LTS-to-PNML conversion (8 places, 7 transitions, all arcs
		// explicitly weighted via <inscription>1</inscription> rather than
		// left implicit) -- exercises explicit-weight parsing at scale.
		// State count is a pinned regression golden.
		{"dot-process.pnml", 8, 7, 24, 5, true, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile("testdata/pnml/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			m, err := FromPNML(data)
			if err != nil {
				t.Fatalf("FromPNML: %v", err)
			}
			if len(m.Places) != tc.places {
				t.Errorf("places: got %d, want %d", len(m.Places), tc.places)
			}
			if len(m.Transitions) != tc.transitions {
				t.Errorf("transitions: got %d, want %d", len(m.Transitions), tc.transitions)
			}
			if len(m.Arcs) != tc.arcs {
				t.Errorf("arcs: got %d, want %d", len(m.Arcs), tc.arcs)
			}
			if errs := metamodel.ValidateArcs(m); len(errs) > 0 {
				t.Fatalf("ValidateArcs: %v", errs)
			}

			res, err := metapetri.Convert(m, metapetri.Options{})
			if err != nil {
				t.Fatalf("metapetri.Convert: %v", err)
			}
			result := reachability.NewAnalyzer(res.Net).Analyze()
			if result.StateCount != tc.states {
				t.Errorf("states: got %d, want %d", result.StateCount, tc.states)
			}
			if result.Bounded != tc.bounded {
				t.Errorf("bounded: got %v, want %v", result.Bounded, tc.bounded)
			}
			if len(result.Deadlocks) != tc.deadlocks {
				t.Errorf("deadlocks: got %d, want %d", len(result.Deadlocks), tc.deadlocks)
			}
			if result.HasCycle != tc.hasCycle {
				t.Errorf("hasCycle: got %v, want %v", result.HasCycle, tc.hasCycle)
			}
		})
	}
}

// TestFromPNML_IDsNotNames pins the scrambled-cycle regression concretely:
// the place named "p4" in its PNML <name> label actually has id "p1" and
// carries the net's only initial token, and every arc must resolve against
// that id.
func TestFromPNML_IDsNotNames(t *testing.T) {
	data, err := os.ReadFile("testdata/pnml/scrambled-cycle.pnml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := FromPNML(data)
	if err != nil {
		t.Fatal(err)
	}
	p := m.PlaceByID("p1")
	if p == nil {
		t.Fatal("place p1 not found")
	}
	if p.Description != "p4" {
		t.Errorf("place p1's <name>: got %q, want %q", p.Description, "p4")
	}
	if p.Initial != 1 {
		t.Errorf("place p1's initial marking: got %d, want 1", p.Initial)
	}
	for _, other := range m.Places {
		if other.ID != "p1" && other.Initial != 0 {
			t.Errorf("place %s: got initial %d, want 0 (only p1 should carry a token)", other.ID, other.Initial)
		}
	}
}

// TestFromPNML_InhibitorArc pins the ad hoc <type value="inhibitor"/>
// convention (not the formal inhibitorptnet.pntd extension) that real files
// like vanrein/perpetuum's actually use.
func TestFromPNML_InhibitorArc(t *testing.T) {
	data, err := os.ReadFile("testdata/pnml/traffic-light.pnml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := FromPNML(data)
	if err != nil {
		t.Fatal(err)
	}
	var inhibitors int
	for _, a := range m.Arcs {
		if a.Type == metamodel.InhibitorArc {
			inhibitors++
			if a.From != "night_service" {
				t.Errorf("inhibitor arc from %q, want night_service", a.From)
			}
		}
	}
	if inhibitors != 2 {
		t.Errorf("inhibitor arcs: got %d, want 2", inhibitors)
	}
}

// TestFromPNML_RejectsHighLevelNet confirms the importer refuses a
// Symmetric-Net/HLPNG-typed document rather than silently misreading
// colored-token content as a plain P/T-net.
func TestFromPNML_RejectsHighLevelNet(t *testing.T) {
	doc := `<?xml version="1.0"?>
<pnml xmlns="http://www.pnml.org/version-2009/grammar/pnml">
	<net id="n1" type="http://www.pnml.org/version-2009/grammar/symmetricnet">
		<place id="p1"/>
	</net>
</pnml>`
	if _, err := FromPNML([]byte(doc)); err == nil {
		t.Fatal("expected an error for a symmetricnet-typed document")
	}
}

// TestFromPNML_Simulate runs the imported traffic-light net through the SSA
// engine (not just reachability) and checks the two conservation laws the
// net's structure guarantees at EVERY sampled instant, not merely at the
// end: exactly one token cycles among {green, yellow, red}, and
// night_service never holds more than one token. A weight/direction mistake
// in the importer (e.g. an inhibitor arc's threshold silently becoming a
// consuming arc) would show up here as a violated invariant, not just a
// different final state.
func TestFromPNML_Simulate(t *testing.T) {
	data, err := os.ReadFile("testdata/pnml/traffic-light.pnml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := FromPNML(data)
	if err != nil {
		t.Fatal(err)
	}
	res, err := stochastic.Simulate(m, nil, stochastic.Options{Horizon: 50, Samples: 50, Realizations: 1, Seed: 1})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	byPlace := map[string][]float64{}
	for _, s := range res.Series {
		byPlace[s.Place] = s.Values
	}
	for _, p := range []string{"green", "yellow", "red", "night_service"} {
		if byPlace[p] == nil {
			t.Fatalf("no series for place %q", p)
		}
	}
	for i := range res.Times {
		traffic := byPlace["green"][i] + byPlace["yellow"][i] + byPlace["red"][i]
		if traffic != 1 {
			t.Errorf("t=%v: green+yellow+red = %v, want 1", res.Times[i], traffic)
		}
		if n := byPlace["night_service"][i]; n != 0 && n != 1 {
			t.Errorf("t=%v: night_service = %v, want 0 or 1", res.Times[i], n)
		}
	}
}

// TestFromPNML_RejectsResetArc confirms the importer refuses a reset arc
// rather than silently dropping the semantics it can't express.
func TestFromPNML_RejectsResetArc(t *testing.T) {
	doc := `<?xml version="1.0"?>
<pnml xmlns="http://www.pnml.org/version-2009/grammar/pnml">
	<net id="n1" type="http://www.pnml.org/version-2009/grammar/ptnet">
		<place id="p1"><initialMarking><text>5</text></initialMarking></place>
		<transition id="t1"/>
		<arc id="a1" source="p1" target="t1"><type value="reset"/></arc>
	</net>
</pnml>`
	if _, err := FromPNML([]byte(doc)); err == nil {
		t.Fatal("expected an error for a reset arc")
	}
}

// TestFromPNML_TAPAALGroundTruth checks imported models against ground
// truth from TAPAAL's own verifypn test suite (github.com/TAPAAL/verifypn,
// test_models/<name>/{model.pnml,query.xml}) — CTL/reachability properties
// with an independently-known TRUE/FALSE verdict baked into TAPAAL's own
// query id, not derived from go-pflow. A mismatch here means either the
// importer or go-pflow's verify package disagrees with an established
// model-checker's confirmed answer, not merely with a hand-authored
// expectation.
//
// These files also confirmed a real third arc-type convention: TAPAAL
// writes type="inhibitor" as an XML ATTRIBUTE on <arc>, distinct from both
// PNML's silence-means-normal default and vanrein/perpetuum's <type
// value="inhibitor"/> CHILD ELEMENT (see pnml_test.go's other cases) —
// arcType() in pnml.go recognizes both shapes.
func TestFromPNML_TAPAALGroundTruth(t *testing.T) {
	tests := []struct {
		name string
		file string
		// verdict runs the actual verify.Property check(s) that mirror
		// TAPAAL's query.xml verdict(s) for this model and fails the test if
		// the status disagrees.
		verdict func(t *testing.T, v *verify.Verifier)
	}{
		// query id deadlock-test018.TRUE-1: EF(deadlock). Structure: P0=1,
		// P0 -(inhibitor)-> T0 -> P0. The inhibitor blocks T0 whenever P0>=1,
		// so with P0=1 at t=0 the initial marking is already a deadlock.
		{"deadlock-test018", "tapaal-deadlock-test018.pnml", func(t *testing.T, v *verify.Verifier) {
			r := v.Check(verify.Property{Kind: verify.KindDeadlockFree})
			if r.Verdicts[0].Status != verify.Refuted {
				t.Errorf("deadlock-free: got %s, want refuted (TAPAAL: EF(deadlock) is TRUE)", r.Verdicts[0].Status)
			}
		}},
		// query id reduction-023-deadlock.TRUE-1: EF(deadlock), no inhibitor
		// arcs at all. P16=1, P18=0; P18->T10->P16. T10 needs P18>=1, never
		// available, so again the initial marking is the deadlock.
		{"reduction-023-deadlock", "tapaal-reduction-023-deadlock.pnml", func(t *testing.T, v *verify.Verifier) {
			r := v.Check(verify.Property{Kind: verify.KindDeadlockFree})
			if r.Verdicts[0].Status != verify.Refuted {
				t.Errorf("deadlock-free: got %s, want refuted (TAPAAL: EF(deadlock) is TRUE)", r.Verdicts[0].Status)
			}
		}},
		// query id deadlock-test020.FALSE-7 (AG!deadlock, i.e. deadlock IS
		// reachable): P0=1, rest 0; P0->T0->{P1,P3}; P1->T1->P2. Firing
		// T0 then T1 reaches P0=0,P1=0,P2=1,P3=1 with neither transition
		// enabled — a deadlock two firings deep, not at t=0.
		{"deadlock-test020", "tapaal-deadlock-test020.pnml", func(t *testing.T, v *verify.Verifier) {
			r := v.Check(verify.Property{Kind: verify.KindDeadlockFree})
			if r.Verdicts[0].Status != verify.Refuted {
				t.Errorf("deadlock-free: got %s, want refuted (TAPAAL: a deadlock is reachable)", r.Verdicts[0].Status)
			}
		}},
		// query id (unnamed).TRUE: EF(p=5). p=4; p-4->T0-3->p (net -1) and
		// p-2->T1-3->p (net +1). Firing T1 once reaches p=5.
		{"ruleN-001", "tapaal-ruleN-001.pnml", func(t *testing.T, v *verify.Verifier) {
			r := v.Check(verify.Property{Kind: verify.KindReachable, Target: map[string]int{"p": 5}})
			if r.Verdicts[0].Status != verify.Proved {
				t.Errorf("reachable p=5: got %s, want proved (TAPAAL: TRUE)", r.Verdicts[0].Status)
			}
		}},
		// query-test005.FALSE-1: EF(Sum=10, P0=P1=P2=0) is FALSE — max
		// possible Sum is 2+3+4=9. query-test005.TRUE-2: EF(Sum=9,
		// P0=P1=P2=0) is TRUE — drain every counter into Sum.
		{"query-test005", "tapaal-query-test005.pnml", func(t *testing.T, v *verify.Verifier) {
			r := v.Check(
				verify.Property{Kind: verify.KindUnreachable, Target: map[string]int{"Sum": 10, "P0": 0, "P1": 0, "P2": 0}},
				verify.Property{Kind: verify.KindReachable, Target: map[string]int{"Sum": 9, "P0": 0, "P1": 0, "P2": 0}},
			)
			if r.Verdicts[0].Status != verify.Proved {
				t.Errorf("unreachable Sum=10: got %s, want proved (TAPAAL: FALSE-1, Sum=10 is unreachable)", r.Verdicts[0].Status)
			}
			if r.Verdicts[1].Status != verify.Proved {
				t.Errorf("reachable Sum=9: got %s, want proved (TAPAAL: TRUE-2)", r.Verdicts[1].Status)
			}
		}},
		// reduction-050.FALSE-1: EF(P1=1) is FALSE. P0=1,P2=1; P0->T0->P1,
		// and P2 -(inhibitor)-> T0. P2 never changes (nothing consumes or
		// produces it), so T0 is permanently disabled and P1 stays 0 forever.
		{"reduction-050", "tapaal-reduction-050.pnml", func(t *testing.T, v *verify.Verifier) {
			r := v.Check(verify.Property{Kind: verify.KindUnreachable, Target: map[string]int{"P1": 1}})
			if r.Verdicts[0].Status != verify.Proved {
				t.Errorf("unreachable P1=1: got %s, want proved (TAPAAL: FALSE-1)", r.Verdicts[0].Status)
			}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/pnml/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			m, err := FromPNML(data)
			if err != nil {
				t.Fatalf("FromPNML: %v", err)
			}
			res, err := metapetri.Convert(m, metapetri.Options{})
			if err != nil {
				t.Fatalf("metapetri.Convert: %v", err)
			}
			tc.verdict(t, verify.New(res.Net))
		})
	}
}
