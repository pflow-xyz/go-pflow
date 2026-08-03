package petrigen

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// gatedModel is a model whose "step" transition is gated on "gate" by an arc of
// the given type, and consumes only from "ready".
func gatedModel(typ metamodel.ArcType) *metamodel.Model {
	return &metamodel.Model{
		Name: "gated",
		Places: []metamodel.Place{
			{ID: "ready", Kind: metamodel.TokenKind, Initial: 1},
			{ID: "gate", Kind: metamodel.TokenKind, Initial: 2},
			{ID: "done", Kind: metamodel.TokenKind},
		},
		Transitions: []metamodel.Transition{{ID: "step"}},
		Arcs: []metamodel.Arc{
			{From: "ready", To: "step", Weight: 1},
			{From: "gate", To: "step", Weight: 2, Type: typ},
			{From: "step", To: "done", Weight: 1},
		},
	}
}

// TestBuildContextRefusesTokenlessArcs pins the one thing the generated
// topology cannot say.
//
// Topology[t].Inputs is a list of place indices that Fire decrements. Read and
// inhibitor arcs move no tokens, so folding either into Inputs emits a circuit
// that consumes from a place the model only tests — and because the circuit is
// then *proved*, the wrong semantics carries a proof. Compiling it into a
// normal arc is the single worst outcome available here, so BuildContext
// refuses rather than guessing.
func TestBuildContextRefusesTokenlessArcs(t *testing.T) {
	for _, typ := range []metamodel.ArcType{metamodel.ReadArc, metamodel.InhibitorArc} {
		t.Run(string(typ), func(t *testing.T) {
			_, err := BuildContext(gatedModel(typ), "gated")
			if err == nil {
				t.Fatalf("a %q arc has no representation in Topology; BuildContext must refuse it "+
					"instead of encoding it as a consuming input", typ)
			}
			if !strings.Contains(err.Error(), "gate -> step") {
				t.Errorf("error should name the offending arc, got %q", err)
			}
		})
	}
}

// TestBuildContextRefusesUnknownArcType: the same rule as everywhere else in
// the ecosystem — an ArcType this build does not understand is an error, never
// a normal arc. Silence here would turn a constraint into token theft inside a
// proof.
func TestBuildContextRefusesUnknownArcType(t *testing.T) {
	_, err := BuildContext(gatedModel(metamodel.ArcType("reset")), "gated")
	if err == nil {
		t.Fatal("an unknown arc type must be refused")
	}
	if !strings.Contains(err.Error(), "unknown type") {
		t.Errorf("error should say the type is unknown, got %q", err)
	}
}

// TestBuildContextAcceptsNormalArcs guards the refusal from over-reaching: an
// ordinary net must still compile, and the read place must not have been
// silently dropped from a model that never had one.
func TestBuildContextAcceptsNormalArcs(t *testing.T) {
	ctx, err := BuildContext(gatedModel(metamodel.NormalArc), "gated")
	if err != nil {
		t.Fatalf("a normal net must still build: %v", err)
	}
	if len(ctx.Transitions) != 1 || len(ctx.Transitions[0].Inputs) != 2 {
		t.Fatalf("step should consume from both places, got %+v", ctx.Transitions)
	}
}
