package statemachine

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
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
	flat, err := b.Flatten()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}

	net := metaToPetri(t, flat)
	marking := reachability.Marking{}
	for _, p := range flat.Places {
		marking[p.ID] = p.Initial
	}
	invariants := reachability.NewInvariantAnalyzer(net).FindPInvariants(marking)

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

	var noted bool
	for _, tr := range sub.Model.Transitions {
		if strings.Contains(tr.Description, "guard not represented") {
			noted = true
		}
	}
	if !noted {
		t.Error("a dropped closure guard makes the net more permissive than the chart; it must be recorded")
	}
}

// metaToPetri converts token places and weighted arcs into a petri.PetriNet.
//
// Test-local on purpose: a public metamodel -> petri bridge has to decide what
// happens to data places, colored tokens and inhibitor weights, and that is a
// design question this test has no business settling.
func metaToPetri(t *testing.T, m *metamodel.Model) *petri.PetriNet {
	t.Helper()
	b := petri.Build()

	token := map[string]bool{}
	for _, p := range m.Places {
		if p.IsToken() {
			token[p.ID] = true
			b.Place(p.ID, float64(p.Initial))
		}
	}
	for _, tr := range m.Transitions {
		b.Transition(tr.ID)
	}
	for _, a := range m.Arcs {
		if m.PlaceByID(a.From) != nil && !token[a.From] {
			continue
		}
		if m.PlaceByID(a.To) != nil && !token[a.To] {
			continue
		}
		w := a.Weight
		if w == 0 {
			w = 1
		}
		if a.Type == metamodel.InhibitorArc {
			b.InhibitorArc(a.From, a.To, float64(w))
		} else {
			b.Arc(a.From, a.To, float64(w))
		}
	}
	return b.Done()
}
