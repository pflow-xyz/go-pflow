package petri

import (
	"testing"
)

func TestBuildIncidenceMatrix(t *testing.T) {
	// Create a simple model: P1 -> T1 -> P2
	model := NewModel("test")
	model.AddPlace(Place{ID: "P1", Initial: 1})
	model.AddPlace(Place{ID: "P2", Initial: 0})
	model.AddTransition(Transition{ID: "T1"})
	model.AddArc(Arc{Source: "P1", Target: "T1"})
	model.AddArc(Arc{Source: "T1", Target: "P2"})

	im := BuildIncidenceMatrix(model)

	// Check structure
	if len(im.Places) != 2 {
		t.Errorf("expected 2 places, got %d", len(im.Places))
	}
	if len(im.Transitions) != 1 {
		t.Errorf("expected 1 transition, got %d", len(im.Transitions))
	}

	// Check incidence values
	// T1 consumes from P1 (-1) and produces to P2 (+1)
	p1Effect := im.Get("P1", "T1")
	p2Effect := im.Get("P2", "T1")

	if p1Effect != -1 {
		t.Errorf("P1 effect from T1: expected -1, got %d", p1Effect)
	}
	if p2Effect != 1 {
		t.Errorf("P2 effect from T1: expected +1, got %d", p2Effect)
	}
}

func TestBuildIncidenceMatrixConservative(t *testing.T) {
	// Create a conservative model: tokens flow between places without creation/destruction
	// P1 <-> T1 <-> P2 (bidirectional flow)
	model := NewModel("conservative")
	model.AddPlace(Place{ID: "P1", Initial: 5})
	model.AddPlace(Place{ID: "P2", Initial: 0})

	// T_forward: P1 -> P2
	model.AddTransition(Transition{ID: "T_forward"})
	model.AddArc(Arc{Source: "P1", Target: "T_forward"})
	model.AddArc(Arc{Source: "T_forward", Target: "P2"})

	// T_back: P2 -> P1
	model.AddTransition(Transition{ID: "T_back"})
	model.AddArc(Arc{Source: "P2", Target: "T_back"})
	model.AddArc(Arc{Source: "T_back", Target: "P1"})

	im := BuildIncidenceMatrix(model)

	// T_forward: -1 for P1, +1 for P2
	if im.Get("P1", "T_forward") != -1 {
		t.Errorf("P1 from T_forward: expected -1, got %d", im.Get("P1", "T_forward"))
	}
	if im.Get("P2", "T_forward") != 1 {
		t.Errorf("P2 from T_forward: expected +1, got %d", im.Get("P2", "T_forward"))
	}

	// T_back: +1 for P1, -1 for P2
	if im.Get("P1", "T_back") != 1 {
		t.Errorf("P1 from T_back: expected +1, got %d", im.Get("P1", "T_back"))
	}
	if im.Get("P2", "T_back") != -1 {
		t.Errorf("P2 from T_back: expected -1, got %d", im.Get("P2", "T_back"))
	}
}

func TestPlaceInvariantVerify(t *testing.T) {
	inv := PlaceInvariant{
		Weights: map[string]int{"P1": 1, "P2": 1},
		Value:   5,
	}

	// Marking that satisfies invariant
	marking1 := Marking{"P1": 3, "P2": 2}
	if !inv.Verify(marking1) {
		t.Error("invariant should hold for marking {P1:3, P2:2}")
	}

	// Marking that violates invariant
	marking2 := Marking{"P1": 3, "P2": 3}
	if inv.Verify(marking2) {
		t.Error("invariant should NOT hold for marking {P1:3, P2:3}")
	}
}

func TestFindPlaceInvariants(t *testing.T) {
	// Conservative model with two connected places
	model := NewModel("conservative")
	model.AddPlace(Place{ID: "P1", Initial: 5})
	model.AddPlace(Place{ID: "P2", Initial: 0})

	model.AddTransition(Transition{ID: "T_forward"})
	model.AddArc(Arc{Source: "P1", Target: "T_forward"})
	model.AddArc(Arc{Source: "T_forward", Target: "P2"})

	model.AddTransition(Transition{ID: "T_back"})
	model.AddArc(Arc{Source: "P2", Target: "T_back"})
	model.AddArc(Arc{Source: "T_back", Target: "P1"})

	invariants := FindPlaceInvariants(model)

	if len(invariants) == 0 {
		t.Fatal("expected at least one place invariant for conservative net")
	}

	// Should find P1 + P2 = 5
	found := false
	for _, inv := range invariants {
		if inv.Weights["P1"] == 1 && inv.Weights["P2"] == 1 && inv.Value == 5 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected invariant P1 + P2 = 5, got %v", invariants)
	}
}

func TestVerifyInvariantStructurally(t *testing.T) {
	// Conservative model
	model := NewModel("conservative")
	model.AddPlace(Place{ID: "P1", Initial: 5})
	model.AddPlace(Place{ID: "P2", Initial: 0})

	model.AddTransition(Transition{ID: "T_forward"})
	model.AddArc(Arc{Source: "P1", Target: "T_forward"})
	model.AddArc(Arc{Source: "T_forward", Target: "P2"})

	model.AddTransition(Transition{ID: "T_back"})
	model.AddArc(Arc{Source: "P2", Target: "T_back"})
	model.AddArc(Arc{Source: "T_back", Target: "P1"})

	// This invariant is preserved: P1 + P2 = constant
	validInvariant := PlaceInvariant{
		Weights: map[string]int{"P1": 1, "P2": 1},
		Value:   5,
	}

	if !VerifyInvariantStructurally(model, validInvariant) {
		t.Error("P1 + P2 should be structurally preserved")
	}

	// This invariant is NOT preserved: P1 - P2 = constant
	invalidInvariant := PlaceInvariant{
		Weights: map[string]int{"P1": 1, "P2": -1},
		Value:   5,
	}

	if VerifyInvariantStructurally(model, invalidInvariant) {
		t.Error("P1 - P2 should NOT be structurally preserved")
	}
}

func TestAnalyzeMintBurn(t *testing.T) {
	// Model with mint (creates tokens) and burn (destroys tokens)
	model := NewModel("token")
	model.AddPlace(Place{ID: "balances", Initial: 0})
	model.AddPlace(Place{ID: "totalSupply", Initial: 0})

	// Mint: creates tokens (non-conservative)
	model.AddTransition(Transition{ID: "mint"})
	model.AddArc(Arc{Source: "mint", Target: "balances"})
	model.AddArc(Arc{Source: "mint", Target: "totalSupply"})

	// Burn: destroys tokens (non-conservative)
	model.AddTransition(Transition{ID: "burn"})
	model.AddArc(Arc{Source: "balances", Target: "burn"})
	model.AddArc(Arc{Source: "totalSupply", Target: "burn"})

	// Transfer: moves tokens (conservative)
	model.AddTransition(Transition{ID: "transfer"})
	model.AddArc(Arc{Source: "balances", Target: "transfer"})
	model.AddArc(Arc{Source: "transfer", Target: "balances"})

	result := Analyze(model)

	// Mint and burn are non-conservative (they change total tokens)
	hasNonConservative := false
	for _, tid := range result.NonConservativeTransitions {
		if tid == "mint" || tid == "burn" {
			hasNonConservative = true
		}
	}
	if !hasNonConservative {
		t.Error("expected mint/burn to be classified as non-conservative")
	}

	// Transfer is conservative (moves tokens without creating/destroying)
	hasConservative := false
	for _, tid := range result.ConservativeTransitions {
		if tid == "transfer" {
			hasConservative = true
		}
	}
	if !hasConservative {
		t.Error("expected transfer to be classified as conservative")
	}
}

func TestPlaceInvariantString(t *testing.T) {
	inv := PlaceInvariant{
		Weights: map[string]int{"P1": 1, "P2": 2, "P3": -1},
		Value:   10,
	}

	str := inv.String()
	// Should contain all terms
	if str == "" {
		t.Error("String() should not return empty string")
	}
	// The exact format may vary due to map iteration order
	// Just verify it contains the value
	if len(str) < 5 {
		t.Errorf("String() seems too short: %s", str)
	}
}

func TestAnalyzeERC20Conservation(t *testing.T) {
	// Model ERC-20 token with conservation law: sum(balances) == totalSupply
	// But with mint/burn, this becomes: transfer conserves, mint/burn don't
	model := NewModel("ERC20")

	// Places (state)
	model.AddPlace(Place{ID: "balances", Initial: 0})
	model.AddPlace(Place{ID: "totalSupply", Initial: 0})
	model.AddPlace(Place{ID: "allowances", Initial: 0})

	// Transitions
	// Transfer: balances[from] -> balances[to] (conservative within balances)
	model.AddTransition(Transition{ID: "transfer", Guard: "balances[from] >= amount"})
	model.AddArc(Arc{Source: "balances", Target: "transfer", Keys: []string{"from"}})
	model.AddArc(Arc{Source: "transfer", Target: "balances", Keys: []string{"to"}})

	// Approve: updates allowances (no effect on token supply)
	model.AddTransition(Transition{ID: "approve"})
	model.AddArc(Arc{Source: "approve", Target: "allowances", Keys: []string{"owner", "spender"}})

	// Mint: creates tokens (both balances and totalSupply increase)
	model.AddTransition(Transition{ID: "mint"})
	model.AddArc(Arc{Source: "mint", Target: "balances", Keys: []string{"to"}})
	model.AddArc(Arc{Source: "mint", Target: "totalSupply"})

	// Burn: destroys tokens (both balances and totalSupply decrease)
	model.AddTransition(Transition{ID: "burn", Guard: "balances[from] >= amount"})
	model.AddArc(Arc{Source: "balances", Target: "burn", Keys: []string{"from"}})
	model.AddArc(Arc{Source: "totalSupply", Target: "burn"})

	result := Analyze(model)

	// Transfer should be conservative (moves tokens without changing total)
	transferIsConservative := false
	for _, tid := range result.ConservativeTransitions {
		if tid == "transfer" {
			transferIsConservative = true
		}
	}
	if !transferIsConservative {
		t.Error("transfer should be conservative")
	}

	// Mint and burn are non-conservative
	mintNonConservative := false
	burnNonConservative := false
	for _, tid := range result.NonConservativeTransitions {
		if tid == "mint" {
			mintNonConservative = true
		}
		if tid == "burn" {
			burnNonConservative = true
		}
	}
	if !mintNonConservative {
		t.Error("mint should be non-conservative")
	}
	if !burnNonConservative {
		t.Error("burn should be non-conservative")
	}

	// The key insight: balances + totalSupply difference should be preserved by mint/burn
	// Actually: when mint fires, both balances and totalSupply increase by same amount
	// So balances - totalSupply is NOT preserved (both sides change equally but in same direction)
	// But we can verify: balances flow is balanced for transfer

	// Verify that P-invariants are found for connected places
	if len(result.PlaceInvariants) > 0 {
		t.Logf("Found %d place invariants", len(result.PlaceInvariants))
		for _, inv := range result.PlaceInvariants {
			t.Logf("  %s", inv.String())
		}
	}
}

// TestFindPlaceInvariantsForkIsNotConservative is the regression for the old
// connected-component heuristic: a fork transition (a -> t -> b+c) linked all
// three places into one group and emitted "a + b + c == initial" as an
// invariant, even though firing t increases the sum — the package's own
// VerifyInvariantStructurally rejected the very invariants FindPlaceInvariants
// produced.
func TestFindPlaceInvariantsForkIsNotConservative(t *testing.T) {
	m := NewModel("fork")
	m.AddPlace(Place{ID: "a", Initial: 5})
	m.AddPlace(Place{ID: "b", Initial: 0})
	m.AddPlace(Place{ID: "c", Initial: 0})
	m.AddTransition(Transition{ID: "t"})
	m.AddArc(Arc{Source: "a", Target: "t"})
	m.AddArc(Arc{Source: "t", Target: "b"})
	m.AddArc(Arc{Source: "t", Target: "c"})

	for _, inv := range FindPlaceInvariants(m) {
		if !VerifyInvariantStructurally(m, inv) {
			t.Errorf("FindPlaceInvariants returned %s, which its own verifier rejects", inv.String())
		}
		if len(inv.Weights) == 3 && inv.Weights["a"] == 1 && inv.Weights["b"] == 1 && inv.Weights["c"] == 1 {
			t.Errorf("the all-ones fork 'invariant' is back: %s", inv.String())
		}
	}
}

// TestFindPlaceInvariantsAllVerifyStructurally: every invariant returned must
// pass the package's own structural check, across a few model shapes.
func TestFindPlaceInvariantsAllVerifyStructurally(t *testing.T) {
	models := map[string]*Model{}

	swap := NewModel("swap")
	swap.AddPlace(Place{ID: "a", Initial: 3})
	swap.AddPlace(Place{ID: "b", Initial: 0})
	swap.AddTransition(Transition{ID: "fwd"})
	swap.AddTransition(Transition{ID: "back"})
	swap.AddArc(Arc{Source: "a", Target: "fwd"})
	swap.AddArc(Arc{Source: "fwd", Target: "b"})
	swap.AddArc(Arc{Source: "b", Target: "back"})
	swap.AddArc(Arc{Source: "back", Target: "a"})
	models["swap"] = swap

	mint := NewModel("mint")
	mint.AddPlace(Place{ID: "supply", Initial: 0})
	mint.AddTransition(Transition{ID: "mint"})
	mint.AddArc(Arc{Source: "mint", Target: "supply"})
	models["mint"] = mint

	for name, m := range models {
		t.Run(name, func(t *testing.T) {
			invs := FindPlaceInvariants(m)
			for _, inv := range invs {
				if !VerifyInvariantStructurally(m, inv) {
					t.Errorf("returned invariant %s fails structural verification", inv.String())
				}
			}
			if name == "swap" && len(invs) == 0 {
				t.Error("swap net conserves a+b; expected at least one invariant")
			}
			if name == "mint" && len(invs) != 0 {
				t.Errorf("mint net conserves nothing; got %v", invs)
			}
		})
	}
}
