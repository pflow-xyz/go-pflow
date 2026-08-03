package metamodel_test

import (
	"testing"

	. "github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
)

// There are THREE generic-net -> Model bridges, not one:
// (*PetriNet[S]).ToModel, ModelFromGenericToken and ModelFromGenericData. All
// three drop GenericTransition.Guard, because a Go closure has no expression
// form; all three must therefore say so with GuardUnrepresentable.
//
// Marking only the method left the two compat functions converting a net that
// lost a precondition with Overapproximates() == false — the exact hole the
// flag exists to close, reachable through the call the package doc recommends
// (`ModelFromGenericToken(net)`).
func TestCompatBridgesFlagDroppedClosureGuards(t *testing.T) {
	tokenNet := func(expr string) *PetriNet[TokenState[string]] {
		n := NewPetriNet[TokenState[string]]("n")
		n.AddPlace(NewGenericPlace("p", NewTokenState(1, "p")))
		tr := NewGenericTransition[TokenState[string], TokenState[string]]("t")
		tr.Guard = func(TokenState[string]) bool { return false }
		tr.GuardExpr = expr
		n.AddTransition(tr)
		n.AddArc(NewGenericArc[TokenState[string]]("p", "t"))
		return n
	}
	dataNet := func(expr string) *PetriNet[DataState[string]] {
		n := NewPetriNet[DataState[string]]("n")
		n.AddPlace(NewGenericPlace("p", NewDataState("v")))
		tr := NewGenericTransition[DataState[string], DataState[string]]("t")
		tr.Guard = func(DataState[string]) bool { return false }
		tr.GuardExpr = expr
		n.AddTransition(tr)
		n.AddArc(NewGenericArc[DataState[string]]("p", "t"))
		return n
	}

	cases := []struct {
		name string
		lost *Model // closure guard, no expression: the precondition is gone
		kept *Model // GuardExpr stands in: nothing is unrepresentable
	}{
		{"ToModel", tokenNet("").ToModel(), tokenNet("count > 0").ToModel()},
		{"ModelFromGenericToken", ModelFromGenericToken(tokenNet("")), ModelFromGenericToken(tokenNet("count > 0"))},
		{"ModelFromGenericData", ModelFromGenericData(dataNet("")), ModelFromGenericData(dataNet("count > 0"))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !c.lost.Transitions[0].GuardUnrepresentable {
				t.Error("a closure guard with no GuardExpr vanished silently: " +
					"the Model is more permissive than the net it came from, and nothing records it")
			}
			if c.kept.Transitions[0].GuardUnrepresentable {
				t.Error("GuardExpr stands in for the closure, so nothing is unrepresentable; " +
					"the marker would degrade verdicts for no reason")
			}

			// And the end of the chain this exists for: metapetri must see it.
			res, err := metapetri.Convert(c.lost, metapetri.Options{})
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			if !res.Diag.Overapproximates() {
				t.Error("conversion reports no over-approximation, but a precondition was dropped: " +
					"every existential verdict drawn from this net is about something the source does not implement")
			}
		})
	}
}
