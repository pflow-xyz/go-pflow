package compat

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
)

// tinyLegacyNet builds a 2-place / 1-transition net with integer-valued
// initials and a unit-weight arc — should round-trip with no Diagnostics.
func tinyLegacyNet() *petri.PetriNet {
	n := petri.NewPetriNet()
	n.AddPlace("a", []float64{3}, nil, 0, 0, nil)
	n.AddPlace("b", nil, nil, 0, 0, nil)
	n.AddTransition("t", "", 0, 0, nil)
	n.AddArc("a", "t", []float64{1}, false)
	n.AddArc("t", "b", []float64{1}, false)
	return n
}

func TestToModelCleanCase(t *testing.T) {
	n := tinyLegacyNet()
	m, diag, err := ToModel(n)
	if err != nil {
		t.Fatal(err)
	}
	if diag.HasLoss() {
		t.Errorf("expected lossless translation, got notes:\n%s", strings.Join(diag.Notes, "\n"))
	}
	if len(m.Places) != 2 {
		t.Errorf("places = %d, want 2", len(m.Places))
	}
	if len(m.Transitions) != 1 {
		t.Errorf("transitions = %d, want 1", len(m.Transitions))
	}
	if len(m.Arcs) != 2 {
		t.Errorf("arcs = %d, want 2", len(m.Arcs))
	}
	// Check initial marking.
	if m.Places[0].Initial != 3 {
		t.Errorf("place a initial = %d, want 3", m.Places[0].Initial)
	}
}

func TestToModelInhibitorLowersToGuard(t *testing.T) {
	n := petri.NewPetriNet()
	n.AddPlace("buffer", []float64{0}, nil, 0, 0, nil)
	n.AddTransition("produce", "", 0, 0, nil)
	n.AddArc("buffer", "produce", []float64{5}, true) // inhibitor: produce blocks when buffer >= 5
	m, diag, err := ToModel(n)
	if err != nil {
		t.Fatal(err)
	}
	if !diag.HasLoss() {
		t.Error("expected note about inhibitor->guard lowering")
	}
	tr := m.TransitionByID("produce")
	if tr == nil {
		t.Fatal("produce transition missing")
	}
	if tr.Guard != `tokens("buffer") < 5` {
		t.Errorf("guard = %q, want tokens(\"buffer\") < 5", tr.Guard)
	}
	// Inhibitor arc itself must NOT have been added as a normal arc.
	for _, a := range m.Arcs {
		if a.Source == "buffer" && a.Target == "produce" {
			t.Errorf("inhibitor arc leaked into model: %+v", a)
		}
	}
}

func TestToModelFloorsFractionalInitial(t *testing.T) {
	n := petri.NewPetriNet()
	n.AddPlace("p", []float64{3.7}, nil, 0, 0, nil)
	_, diag, err := ToModel(n)
	if err != nil {
		t.Fatal(err)
	}
	if !diag.HasLoss() {
		t.Fatal("expected diagnostic on fractional initial")
	}
	joined := strings.Join(diag.Notes, "\n")
	if !strings.Contains(joined, "floored to 3") {
		t.Errorf("expected floor diagnostic, got:\n%s", joined)
	}
}

func TestToModelCapacityBecomesInvariant(t *testing.T) {
	n := petri.NewPetriNet()
	n.AddPlace("p", nil, []float64{10}, 0, 0, nil)
	m, _, err := ToModel(n)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Invariants) != 1 {
		t.Fatalf("expected 1 invariant from capacity, got %d", len(m.Invariants))
	}
	if !strings.Contains(m.Invariants[0].Expr, `tokens("p") <= 10`) {
		t.Errorf("invariant expr = %q, want tokens(\"p\") <= 10", m.Invariants[0].Expr)
	}
}

func TestFromModelDropsGuardsWithDiagnostic(t *testing.T) {
	m := tmpetri.NewModel("guarded")
	m.AddPlace(tmpetri.Place{ID: "p"})
	m.AddTransition(tmpetri.Transition{ID: "t", Guard: `tokens("p") > 0`})
	m.AddArc(tmpetri.Arc{Source: "p", Target: "t"})
	n, diag, err := FromModel(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := n.Transitions["t"]; !ok {
		t.Error("transition t missing in legacy net")
	}
	joined := strings.Join(diag.Notes, "\n")
	if !strings.Contains(joined, `guard "tokens(\"p\") > 0" dropped`) {
		t.Errorf("expected guard-dropped diagnostic, got:\n%s", joined)
	}
}

func TestRoundTripPreservesStructure(t *testing.T) {
	// Old → new → old. Structure (places, transitions, arcs counts) survives;
	// visualization fields don't, but we don't compare those.
	orig := tinyLegacyNet()
	m, _, err := ToModel(orig)
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := FromModel(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Places) != len(orig.Places) {
		t.Errorf("place count drift: %d -> %d", len(orig.Places), len(back.Places))
	}
	if len(back.Transitions) != len(orig.Transitions) {
		t.Errorf("transition count drift: %d -> %d", len(orig.Transitions), len(back.Transitions))
	}
	if len(back.Arcs) != len(orig.Arcs) {
		t.Errorf("arc count drift: %d -> %d", len(orig.Arcs), len(back.Arcs))
	}
}

func TestNilInputsErrorCleanly(t *testing.T) {
	if _, _, err := ToModel(nil); err == nil {
		t.Error("expected error on nil PetriNet")
	}
	if _, _, err := FromModel(nil); err == nil {
		t.Error("expected error on nil Model")
	}
}
