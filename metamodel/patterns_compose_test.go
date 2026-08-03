package metamodel

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/reachability"
)

func TestResourcePoolToSubnet(t *testing.T) {
	pool := NewResourcePool("db", 3, "connection")
	sub := pool.ToSubnet("db")

	if sub.NetType != ResourceNet {
		t.Errorf("NetType = %q, want ResourceNet", sub.NetType)
	}

	var expr string
	for _, c := range sub.Model.Constraints {
		if c.ID == "pool_conservation" {
			expr = c.Expr
		}
	}
	if expr == "" {
		t.Fatal("a pool must carry its conservation law; that law is what makes it safe to share")
	}
	for _, want := range []string{`tokens("available")`, `tokens("in_use")`, "== 3"} {
		if !strings.Contains(expr, want) {
			t.Errorf("constraint %q missing %q", expr, want)
		}
	}

	b := NewBundle("p")
	b.AddSubnet(*sub)
	if res := b.Validate(); !res.Valid {
		t.Errorf("pool subnet does not validate: %+v", res.Errors)
	}
}

// TestResourcePoolConservationIsProvable: the emitted law must be a real
// P-invariant, not just a string on the model.
func TestResourcePoolConservationIsProvable(t *testing.T) {
	sub := NewResourcePool("db", 3, "connection").ToSubnet("db")

	b := NewBundle("p")
	b.AddSubnet(*sub)
	flat, err := b.Flatten()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}

	net := toPetriNet(t, flat)
	invariants := reachability.NewInvariantAnalyzer(net).FindPInvariants(markingOf(flat))
	if !coversPlaces(invariants, "available", "in_use") {
		var got []string
		for _, inv := range invariants {
			got = append(got, inv.String())
		}
		t.Errorf("pool conservation is not derivable; invariants were:\n%s", strings.Join(got, "\n"))
	}
}

func TestStateMachineToSubnet(t *testing.T) {
	sm := NewStateMachine("door", "closed")
	sub := sm.ToSubnet("door")

	if sub.NetType != WorkflowNet {
		t.Errorf("NetType = %q, want WorkflowNet", sub.NetType)
	}
	var found bool
	for _, c := range sub.Model.Constraints {
		if c.ID == "state_mutex" && strings.Contains(c.Expr, "== 1") {
			found = true
		}
	}
	if !found {
		t.Error("a state machine must carry its mutual-exclusion constraint")
	}
}

// TestGenericToModelRecoversKinds checks the bridge reads token vs data state
// off the generic type parameter.
func TestGenericToModelRecoversKinds(t *testing.T) {
	tokenNet := NewPetriNet[TokenState[string]]("counters")
	tokenNet.AddPlace(NewGenericPlace("count", NewTokenState(4, "unit")))

	m := tokenNet.ToModel()
	if len(m.Places) != 1 {
		t.Fatalf("want 1 place, got %d", len(m.Places))
	}
	if m.Places[0].Kind != TokenKind {
		t.Errorf("Kind = %q, want token", m.Places[0].Kind)
	}
	if m.Places[0].Initial != 4 {
		t.Errorf("Initial = %d, want 4", m.Places[0].Initial)
	}

	dataNet := NewPetriNet[DataState[string]]("docs")
	dataNet.AddPlace(NewGenericPlace("doc", NewDataState("hello")))

	dm := dataNet.ToModel()
	if dm.Places[0].Kind != DataKind {
		t.Errorf("Kind = %q, want data", dm.Places[0].Kind)
	}
	if dm.Places[0].InitialValue != "hello" {
		t.Errorf("InitialValue = %v, want hello", dm.Places[0].InitialValue)
	}
}

// TestGenericToModelDropsClosures documents the deliberate loss: Guard/Action
// funcs cannot be serialised, so only GuardExpr survives.
func TestGenericToModelDropsClosures(t *testing.T) {
	n := NewPetriNet[TokenState[string]]("n")
	tr := NewGenericTransition[TokenState[string], TokenState[string]]("t")
	tr.Guard = func(TokenState[string]) bool { return false }
	tr.GuardExpr = "count > 0"
	n.AddTransition(tr)

	m := n.ToModel()
	if m.Transitions[0].Guard != "count > 0" {
		t.Errorf("Guard = %q, want the declared expression", m.Transitions[0].Guard)
	}
}
