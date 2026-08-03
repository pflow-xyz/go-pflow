package metamodel_test

import (
	"testing"

	. "github.com/pflow-xyz/go-pflow/metamodel"
)

// Transition.GuardUnrepresentable says "the source of this transition had a
// precondition that never made it into the model". It is the only signal that a
// net over-approximates its source when the lost precondition was a Go closure,
// because a closure leaves no guard TEXT behind for anything to notice. These
// tests pin the two ways it could silently evaporate: a copy, and a composition.

// TestCloneKeepsGuardUnrepresentable: Clone is on the path from a subnet into a
// flattened model. A copy that drops the marker launders an over-approximation
// into something that looks exact.
func TestCloneKeepsGuardUnrepresentable(t *testing.T) {
	tr := &Transition{ID: "t", GuardUnrepresentable: true}
	if !tr.Clone().GuardUnrepresentable {
		t.Error("Clone dropped GuardUnrepresentable: the copy claims a precondition the original admits it lost")
	}

	m := &Model{
		Name:        "m",
		Transitions: []Transition{{ID: "t", GuardUnrepresentable: true}},
	}
	if !m.Clone().Transitions[0].GuardUnrepresentable {
		t.Error("Model.Clone dropped GuardUnrepresentable")
	}
}

// unrepresentableBundle builds two subnets whose single transitions are fused by
// an EventLink. Only "b/step" lost a precondition.
func unrepresentableBundle() *Bundle {
	mk := func(id string, lost bool) Subnet {
		return Subnet{
			Type:    SubnetType,
			ID:      id,
			NetType: WorkflowNet,
			Model: &Model{
				Name:   id,
				Places: []Place{{ID: "in", Kind: TokenKind, Initial: 1, Exported: true}},
				Transitions: []Transition{
					{ID: "step", GuardUnrepresentable: lost},
				},
				Arcs: []Arc{{From: "in", To: "step", Weight: 1}},
			},
			Ports: []Port{{ID: "p", Kind: PortIn, Place: "in"}},
		}
	}
	b := NewBundle("fused")
	b.AddSubnet(mk("a", false))
	b.AddSubnet(mk("b", true))
	b.AddLink(Link{
		Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "step"},
		To:   Endpoint{Subnet: "b", Transition: "step"},
	})
	return b
}

// TestFusionInheritsGuardUnrepresentable: an EventLink fuses transitions into a
// rendezvous, enabled only when every participant is. Fusion cannot recover a
// precondition that was already lost, so if ANY member lost one the fused firing
// is still under-specified — the marker has to survive, or composition becomes a
// laundering step that hides the loss one level up.
func TestFusionInheritsGuardUnrepresentable(t *testing.T) {
	flat, err := unrepresentableBundle().Flatten()
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	if len(flat.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1 fused", len(flat.Transitions))
	}
	if !flat.Transitions[0].GuardUnrepresentable {
		t.Errorf("fused transition %q lost GuardUnrepresentable: one member's dropped precondition still gates the rendezvous",
			flat.Transitions[0].ID)
	}
}

// TestFlattenKeepsGuardUnrepresentableUnfused is the other half: a transition
// that is not part of any EventLink class travels the single-member path, which
// must carry the marker across too.
func TestFlattenKeepsGuardUnrepresentableUnfused(t *testing.T) {
	b := NewBundle("solo")
	b.AddSubnet(Subnet{
		Type: SubnetType, ID: "a", NetType: WorkflowNet,
		Model: &Model{
			Name:        "a",
			Places:      []Place{{ID: "in", Kind: TokenKind, Initial: 1}},
			Transitions: []Transition{{ID: "step", GuardUnrepresentable: true}},
			Arcs:        []Arc{{From: "in", To: "step", Weight: 1}},
		},
	})

	flat, err := b.Flatten()
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	if !flat.Transitions[0].GuardUnrepresentable {
		t.Error("unfused transition lost GuardUnrepresentable on the way through Flatten")
	}
}

// TestGenericToModelFlagsDroppedClosureGuard: PetriNet[S].ToModel drops Guard
// funcs because they cannot be serialised. When GuardExpr stands in, nothing is
// lost that a tool cannot see. When it does not, the emitted Model fires the
// transition whenever its inputs allow — strictly more often than the generic
// net — and only the marker records that.
func TestGenericToModelFlagsDroppedClosureGuard(t *testing.T) {
	build := func(expr string) Transition {
		n := NewPetriNet[TokenState[string]]("n")
		tr := NewGenericTransition[TokenState[string], TokenState[string]]("t")
		tr.Guard = func(TokenState[string]) bool { return false }
		tr.GuardExpr = expr
		n.AddTransition(tr)
		return n.ToModel().Transitions[0]
	}

	if got := build(""); !got.GuardUnrepresentable {
		t.Error("a closure guard with no GuardExpr vanished silently: the Model is more permissive than the net it came from")
	}
	if got := build("count > 0"); got.GuardUnrepresentable {
		t.Error("GuardExpr stands in for the closure, so nothing is unrepresentable; the marker would degrade verdicts for no reason")
	}
}
