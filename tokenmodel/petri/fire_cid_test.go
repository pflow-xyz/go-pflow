package petri

import (
	"strings"
	"testing"

	mainpetri "github.com/pflow-xyz/go-pflow/petri"
)

func swapModel() *Model {
	m := NewModel("swap")
	m.AddPlace(Place{ID: "a", Initial: 2})
	m.AddPlace(Place{ID: "b", Initial: 0})
	m.AddTransition(Transition{ID: "fwd"})
	m.AddTransition(Transition{ID: "back"})
	m.AddArc(Arc{Source: "a", Target: "fwd"})
	m.AddArc(Arc{Source: "fwd", Target: "b"})
	m.AddArc(Arc{Source: "b", Target: "back"})
	m.AddArc(Arc{Source: "back", Target: "a"})
	return m
}

// --- fire.go -----------------------------------------------------------------

func TestFireMovesTokens(t *testing.T) {
	s := NewState(swapModel())

	if !s.Enabled("fwd") {
		t.Fatal("fwd should be enabled with a=2")
	}
	if s.Enabled("back") {
		t.Fatal("back should be disabled with b=0")
	}

	if err := s.Fire("fwd"); err != nil {
		t.Fatalf("Fire(fwd): %v", err)
	}
	if s.Tokens("a") != 1 || s.Tokens("b") != 1 {
		t.Errorf("after fwd: a=%d b=%d, want 1/1", s.Tokens("a"), s.Tokens("b"))
	}
	if s.Sequence != 1 {
		t.Errorf("sequence = %d, want 1", s.Sequence)
	}
}

func TestFireDisabledTransitionFails(t *testing.T) {
	s := NewState(swapModel())
	if err := s.Fire("back"); err == nil {
		t.Error("firing a disabled transition must fail")
	}
	if err := s.Fire("no-such-transition"); err == nil {
		t.Error("firing an unknown transition must fail")
	}
	// Failed fires must not corrupt state.
	if s.Tokens("a") != 2 || s.Tokens("b") != 0 || s.Sequence != 0 {
		t.Errorf("state mutated by failed fire: a=%d b=%d seq=%d", s.Tokens("a"), s.Tokens("b"), s.Sequence)
	}
}

func TestStateCloneIsDeep(t *testing.T) {
	s := NewState(swapModel())
	c := s.Clone()

	if err := c.Fire("fwd"); err != nil {
		t.Fatal(err)
	}
	if s.Tokens("a") != 2 || s.Tokens("b") != 0 {
		t.Errorf("firing on a clone mutated the original: a=%d b=%d", s.Tokens("a"), s.Tokens("b"))
	}
	if s.Sequence == c.Sequence {
		t.Error("clone shares sequence state with original")
	}
}

func TestEnabledTransitions(t *testing.T) {
	s := NewState(swapModel())
	enabled := s.EnabledTransitions()
	if len(enabled) != 1 || enabled[0] != "fwd" {
		t.Errorf("enabled = %v, want [fwd]", enabled)
	}
}

func TestFireInvariantViolationDetected(t *testing.T) {
	m := NewModel("guarded")
	m.AddPlace(Place{ID: "a", Initial: 1})
	m.AddPlace(Place{ID: "b", Initial: 0})
	m.AddTransition(Transition{ID: "t"})
	m.AddArc(Arc{Source: "a", Target: "t"})
	m.AddArc(Arc{Source: "t", Target: "b"})
	// An invariant that the firing violates: a must stay at 1.
	m.AddInvariant(Invariant{ID: "a-pinned", Expr: "a == 1"})

	s := NewState(m)
	s.CheckInvariants = true

	err := s.Fire("t")
	if err == nil {
		t.Fatal("expected an invariant violation error")
	}
	if !strings.Contains(err.Error(), "a-pinned") {
		t.Errorf("error %q should name the violated invariant", err)
	}
}

func TestMarkingClone(t *testing.T) {
	m := Marking{"a": 1}
	c := m.Clone()
	c["a"] = 99
	if m["a"] != 1 {
		t.Error("Marking.Clone is not deep")
	}
}

// --- cid.go --------------------------------------------------------------------

func TestCIDDeterministicAndOrderInsensitive(t *testing.T) {
	m1 := swapModel()

	// Same net, elements declared in a different order.
	m2 := NewModel("swap")
	m2.AddTransition(Transition{ID: "back"})
	m2.AddTransition(Transition{ID: "fwd"})
	m2.AddPlace(Place{ID: "b", Initial: 0})
	m2.AddPlace(Place{ID: "a", Initial: 2})
	m2.AddArc(Arc{Source: "back", Target: "a"})
	m2.AddArc(Arc{Source: "b", Target: "back"})
	m2.AddArc(Arc{Source: "fwd", Target: "b"})
	m2.AddArc(Arc{Source: "a", Target: "fwd"})

	if m1.CID() == "" {
		t.Fatal("empty CID")
	}
	if m1.CID() != m2.CID() {
		t.Errorf("CID depends on declaration order:\n %s\n %s", m1.CID(), m2.CID())
	}
	if m1.CID() != m1.CID() {
		t.Error("CID not deterministic")
	}
	if !m1.Equal(m2) {
		t.Error("Equal should hold for reordered declarations")
	}
}

func TestCIDChangesWithStructure(t *testing.T) {
	m1 := swapModel()
	m2 := swapModel()
	m2.AddPlace(Place{ID: "extra", Initial: 0})

	if m1.CID() == m2.CID() {
		t.Error("CID must change when structure changes")
	}
	if m1.Equal(m2) {
		t.Error("Equal must be false for different structures")
	}
}

func TestStructurallyEqualIgnoresInitialMarking(t *testing.T) {
	m1 := swapModel()
	m2 := swapModel()
	m2.Places[0].Initial = 7 // same shape, different marking

	if m1.Equal(m2) {
		t.Error("Equal should be sensitive to the initial marking")
	}
	if !m1.StructurallyEqual(m2) {
		t.Error("StructurallyEqual should ignore the initial marking")
	}
}

func TestIdentityHashStableAcrossNames(t *testing.T) {
	m1 := swapModel()
	if m1.IdentityHash() == "" {
		t.Fatal("empty identity hash")
	}
	if m1.IdentityHash() != m1.IdentityHash() {
		t.Error("IdentityHash not deterministic")
	}
}

// --- equivalence.go -------------------------------------------------------------

func TestSemanticEquivalenceSelf(t *testing.T) {
	m := swapModel()
	res := m.IsSemanticEquivalent(swapModel())
	if res == nil {
		t.Fatal("nil result")
	}
	if !res.Equivalent {
		t.Errorf("a model must be semantically equivalent to itself: %+v", res)
	}
}

func TestSemanticEquivalenceRenamedNodes(t *testing.T) {
	// Same structure with all IDs renamed: signatures should still match,
	// because semantic equivalence is name-independent.
	m2 := NewModel("renamed")
	m2.AddPlace(Place{ID: "x", Initial: 2})
	m2.AddPlace(Place{ID: "y", Initial: 0})
	m2.AddTransition(Transition{ID: "go"})
	m2.AddTransition(Transition{ID: "undo"})
	m2.AddArc(Arc{Source: "x", Target: "go"})
	m2.AddArc(Arc{Source: "go", Target: "y"})
	m2.AddArc(Arc{Source: "y", Target: "undo"})
	m2.AddArc(Arc{Source: "undo", Target: "x"})

	res := swapModel().IsSemanticEquivalent(m2)
	if !res.Equivalent {
		t.Errorf("renamed model should be equivalent: %+v", res)
	}
}

func TestSemanticEquivalenceDetectsDifference(t *testing.T) {
	other := NewModel("different")
	other.AddPlace(Place{ID: "only", Initial: 1})
	other.AddTransition(Transition{ID: "sink"})
	other.AddArc(Arc{Source: "only", Target: "sink"})

	res := swapModel().IsSemanticEquivalent(other)
	if res.Equivalent {
		t.Error("structurally different models must not be equivalent")
	}
}

func TestVerifyIsomorphism(t *testing.T) {
	m1 := swapModel()

	m2 := NewModel("renamed")
	m2.AddPlace(Place{ID: "x", Initial: 2})
	m2.AddPlace(Place{ID: "y", Initial: 0})
	m2.AddTransition(Transition{ID: "go"})
	m2.AddTransition(Transition{ID: "undo"})
	m2.AddArc(Arc{Source: "x", Target: "go"})
	m2.AddArc(Arc{Source: "go", Target: "y"})
	m2.AddArc(Arc{Source: "y", Target: "undo"})
	m2.AddArc(Arc{Source: "undo", Target: "x"})

	good := &NodeMapping{
		Places:      map[string]string{"a": "x", "b": "y"},
		Transitions: map[string]string{"fwd": "go", "back": "undo"},
	}
	res := m1.VerifyIsomorphism(m2, good)
	if !res.Isomorphic {
		t.Errorf("correct mapping rejected: %+v", res.Errors)
	}

	// A wrong mapping (swapped places) must be rejected with a reason.
	bad := &NodeMapping{
		Places:      map[string]string{"a": "y", "b": "x"},
		Transitions: map[string]string{"fwd": "go", "back": "undo"},
	}
	res = m1.VerifyIsomorphism(m2, bad)
	if res.Isomorphic {
		t.Error("swapped-place mapping accepted as isomorphism")
	}
	if len(res.Errors) == 0 {
		t.Error("rejection must name at least one failure")
	}
}

func TestSignatureFromPetriNetAgreesWithModel(t *testing.T) {
	// The same structure via the core petri package must produce an
	// equivalent signature — this bridge is what pflow-xyz parity rests on.
	net := mainpetri.Build().
		Place("a", 2).Place("b", 0).
		Transition("fwd").Transition("back").
		Arc("a", "fwd", 1).Arc("fwd", "b", 1).
		Arc("b", "back", 1).Arc("back", "a", 1).
		Done()

	res := swapModel().IsSemanticEquivalentToPetriNet(net)
	if !res.Equivalent {
		t.Errorf("model and core-net of identical structure must be equivalent: %+v", res)
	}
}

// --- bridge.go -----------------------------------------------------------------

func TestBridgeRoundTripPetriNet(t *testing.T) {
	m := swapModel()

	net := m.ToPetriNet()
	if len(net.Places) != 2 || len(net.Transitions) != 2 || len(net.Arcs) != 4 {
		t.Fatalf("ToPetriNet shape: %d/%d/%d", len(net.Places), len(net.Transitions), len(net.Arcs))
	}
	if net.Places["a"].GetTokenCount() != 2 {
		t.Errorf("initial marking lost: a=%f", net.Places["a"].GetTokenCount())
	}

	back := FromPetriNet(net)
	if !m.StructurallyEqual(back) {
		t.Error("Model -> PetriNet -> Model changed the structure")
	}
	// Marking survives the round trip too.
	if got := NewState(back).Tokens("a"); got != 2 {
		t.Errorf("round trip lost the marking: a=%d", got)
	}
}

func TestBridgeRoundTripSchema(t *testing.T) {
	m := swapModel()
	m.AddInvariant(Invariant{ID: "conserve", Expr: "a + b == 2"})

	s := m.ToSchema()
	if s == nil {
		t.Fatal("nil schema")
	}
	back := FromSchema(s)
	if !m.StructurallyEqual(back) {
		t.Error("Model -> Schema -> Model changed the structure")
	}
	if len(back.Invariants) != 1 || back.Invariants[0].ID != "conserve" {
		t.Errorf("invariants lost in round trip: %v", back.Invariants)
	}
}

func TestDefaultRatesAndODEProblem(t *testing.T) {
	m := swapModel()
	rates := m.DefaultRates(2.5)
	if len(rates) != 2 || rates["fwd"] != 2.5 || rates["back"] != 2.5 {
		t.Errorf("DefaultRates = %v", rates)
	}

	prob := m.ToODEProblem(func() map[string]float64 { return m.DefaultRates(1.0) }, [2]float64{0, 10})
	if prob == nil {
		t.Fatal("nil ODE problem")
	}
}

// --- trajectory-based mapping discovery ------------------------------------------

func TestDiscoverMappingByTrajectory(t *testing.T) {
	m1 := swapModel()

	// The same dynamics under renamed places, as a core petri net.
	net := mainpetri.Build().
		Place("x", 2).Place("y", 0).
		Transition("go").Transition("undo").
		Arc("x", "go", 1).Arc("go", "y", 1).
		Arc("y", "undo", 1).Arc("undo", "x", 1).
		Done()
	rates := map[string]float64{"go": 1.0, "undo": 1.0}

	res := m1.DiscoverMappingByTrajectory(net, rates, [2]float64{0, 5})
	if res == nil {
		t.Fatal("nil result")
	}

	mapping := res.ToNodeMapping()

	// a and x share the same trajectory (both start at 2 and follow identical
	// dynamics), likewise b and y. Discovery may leave a place ambiguous but
	// must never map it to the wrong partner.
	if got, ok := mapping.Places["a"]; ok && got != "x" {
		t.Errorf("a mapped to %q, want x", got)
	}
	if got, ok := mapping.Places["b"]; ok && got != "y" {
		t.Errorf("b mapped to %q, want y", got)
	}
	if len(res.PlaceMappings) == 0 {
		t.Error("no place mappings discovered at all")
	}
}
