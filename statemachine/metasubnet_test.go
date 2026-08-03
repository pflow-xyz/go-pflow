package statemachine

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/verify"
)

func lightChart() *Chart {
	return NewChart("light").
		Region("state").
		State("red").Initial().
		State("green").
		State("yellow").
		EndRegion().
		When("timer").In("state:red").GoTo("state:green").
		When("timer").In("state:green").GoTo("state:yellow").
		When("timer").In("state:yellow").GoTo("state:red").
		Build()
}

func TestChartMetaSubnetShape(t *testing.T) {
	sub := lightChart().ToMetaSubnet()

	if sub.NetType != metamodel.WorkflowNet {
		t.Errorf("NetType = %q, want WorkflowNet: a chart marking is a cursor, not a resource pool", sub.NetType)
	}
	for _, id := range []string{"state_red", "state_green", "state_yellow", "event:timer"} {
		if sub.Model.PlaceByID(id) == nil {
			t.Errorf("missing place %q", id)
		}
	}
	if p := sub.Model.PlaceByID("state_red"); p != nil && p.Initial != 1 {
		t.Errorf("initial state red has %d tokens, want 1", p.Initial)
	}

	var hasEventPort bool
	for _, p := range sub.Ports {
		if p.ID == "evt:timer" && p.Kind == metamodel.PortIn {
			hasEventPort = true
		}
	}
	if !hasEventPort {
		t.Error("missing evt:timer in-port; an upstream subnet needs it to drive the chart")
	}
}

// TestRegionMutexIsProvable: ch04 says the mutual-exclusion constraint "is
// enforced by the Petri net structure". Emitting it as a Constraint is what lets
// a tool actually check that, here and after composition.
func TestRegionMutexIsProvable(t *testing.T) {
	sub := lightChart().ToMetaSubnet()

	var expr string
	for _, c := range sub.Model.Constraints {
		if c.ID == "region_mutex_state" {
			expr = c.Expr
		}
	}
	if expr == "" {
		t.Fatal("no mutex constraint emitted for region \"state\"")
	}
	for _, want := range []string{`tokens("state_red")`, `tokens("state_green")`, `tokens("state_yellow")`, "== 1"} {
		if !strings.Contains(expr, want) {
			t.Errorf("constraint %q missing %q", expr, want)
		}
	}

	// And it must be a real P-invariant of the emitted net, not just a string.
	b := metamodel.NewBundle("light")
	b.AddSubnet(*sub)
	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		t.Fatalf("convert bundle: %v", err)
	}
	invariants := reachability.NewInvariantAnalyzer(res.Net).FindPInvariants(res.Marking)

	for _, inv := range invariants {
		got := map[string]bool{}
		for p := range inv.Coefficients {
			got[p] = true
		}
		if got["state_red"] && got["state_green"] && got["state_yellow"] {
			return // the mutex is derivable
		}
	}
	t.Error("the region mutex is not derivable as a P-invariant of the emitted net")
}

// TestGuardLossIsRecorded: chart guards are Go closures with no expression form,
// so the emitted net over-approximates. That must be visible in the output, not
// silently dropped.
func TestGuardLossIsRecorded(t *testing.T) {
	chart := NewChart("gated").
		Region("s").
		State("a").Initial().
		State("b").
		EndRegion().
		When("go").In("s:a").GoTo("s:b").
		If(func(state map[string]float64) bool { return state["s_a"] > 0 }).
		Build()

	sub := chart.ToMetaSubnet()

	var noted, flagged bool
	for _, tr := range sub.Model.Transitions {
		if strings.Contains(tr.Description, "guard not represented") {
			noted = true
		}
		if tr.GuardUnrepresentable {
			flagged = true
		}
	}
	if !noted {
		t.Error("a dropped closure guard makes the net more permissive than the chart; it must be recorded")
	}
	// The prose is for a human reading generated output; the flag is the
	// machine-readable half, and it is the one metapetri keys on. Losing
	// either one is a regression: prose alone is what let a dropped guard
	// convert as lossless, and a flag alone would make the loss invisible in
	// the artefact a modeller actually reads.
	if !flagged {
		t.Error("GuardUnrepresentable is unset: the loss is prose-only again, so no tool can see it")
	}
}

// TestClosureGuardReachesTheSoundnessBridge is the end-to-end claim, and the
// reason GuardUnrepresentable exists. metamodel/metapetri classifies a
// conversion Permissive when a transition carries a guard it cannot evaluate.
// A chart guard is a Go closure, so it never becomes guard TEXT — which used to
// mean a chart that lost a precondition converted with Overapproximates() ==
// false, and verify reported Proved for an existential property on a net
// strictly more permissive than the chart it came from. Exactly the hole
// metapetri was built to close, one level up.
func TestClosureGuardReachesTheSoundnessBridge(t *testing.T) {
	chart := NewChart("gated").
		Region("s").
		State("a").Initial().
		State("b").
		EndRegion().
		When("go").In("s:a").GoTo("s:b").
		If(func(state map[string]float64) bool { return state["s_a"] > 1 }).
		Build()

	sub := chart.ToMetaSubnet()

	// Stand in for the upstream subnet that would deliver the event: without a
	// token in the delivery slot nothing fires at all and liveness is Refuted,
	// which transfers under over-approximation and would prove nothing here.
	if ep := sub.Model.PlaceByID("event:go"); ep != nil {
		ep.Initial = 1
	} else {
		t.Fatal("missing event delivery place")
	}

	res, err := metapetri.ConvertBundle(metamodel.NewBundle("gatedbundle").AddSubnet(*sub), metapetri.Options{})
	if err != nil {
		t.Fatalf("ConvertBundle: %v", err)
	}

	if !res.Diag.Overapproximates() {
		t.Fatal("conversion reports no over-approximation, but the chart's precondition was dropped: " +
			"every existential verdict below is about a net the chart does not implement")
	}

	var found bool
	for _, n := range res.Diag.Notes {
		if n.Code == metapetri.CodeGuardUnrepresentable && n.Dir == metapetri.Permissive {
			found = true
		}
	}
	if !found {
		t.Errorf("no %s note; got %v", metapetri.CodeGuardUnrepresentable, res.Diag.Notes)
	}

	rep, err := metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(rep.Verdicts) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(rep.Verdicts))
	}
	// Quasi-liveness is existential: the witness firing sequence uses the very
	// firing the closure forbids, so Proved must not survive the trip.
	if got := rep.Verdicts[0].Status; got != verify.Unknown {
		t.Errorf("live verdict = %q, want %q: a witness found under a dropped precondition is not a witness for the chart",
			got, verify.Unknown)
	}
}
