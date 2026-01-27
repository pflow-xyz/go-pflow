package zkcompile

import (
	"strings"
	"testing"
)

func TestMerkleProofCompiler_SimpleProof(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewMerkleProofCompiler(witnesses)

	// Manually create a simple proof for testing
	witnesses.AddBinding("key")
	witnesses.AddBinding("value")
	witnesses.AddBinding("root")

	proof := &MerkleProof{
		Key:          "key",
		Value:        "value",
		PathElements: []string{"sibling0", "sibling1", "sibling2"},
		PathIndices:  []string{"idx0", "idx1", "idx2"},
		Root:         "root",
	}

	// Add path witnesses
	for _, s := range proof.PathElements {
		witnesses.AddComputed(s)
	}
	for _, s := range proof.PathIndices {
		witnesses.AddComputed(s)
	}

	constraints := compiler.CompileProof(proof)

	// Should have:
	// - 1 leaf hash
	// - 3 levels * (1 boolean + 2 select + 1 hash) = 12
	// - 1 root equality
	// Total: 14 constraints
	t.Logf("Generated %d constraints", len(constraints))

	// Check for leaf hash constraint
	foundLeaf := false
	for _, c := range constraints {
		if c.Type == Poseidon && strings.Contains(c.Tag, "leaf") {
			foundLeaf = true
			t.Logf("Leaf: %s [%s]", c, c.Tag)
		}
	}
	if !foundLeaf {
		t.Error("expected leaf hash constraint")
	}

	// Check for root equality constraint
	foundRoot := false
	for _, c := range constraints {
		if c.Type == Equal && strings.Contains(c.Tag, "root") {
			foundRoot = true
			t.Logf("Root: %s [%s]", c, c.Tag)
		}
	}
	if !foundRoot {
		t.Error("expected root equality constraint")
	}

	// Check for boolean constraints on path indices
	boolCount := 0
	for _, c := range constraints {
		if c.Type == Boolean {
			boolCount++
		}
	}
	if boolCount != 3 {
		t.Errorf("expected 3 boolean constraints for path indices, got %d", boolCount)
	}

	t.Logf("\nAll constraints:")
	for i, c := range constraints {
		t.Logf("  %d. %s", i+1, c)
	}
}

func TestMerkleProofCompiler_StateAccess(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewMerkleProofCompiler(witnesses)

	// Simulate a state access from guard compilation
	access := &StateAccess{
		WitnessName: "balances_alice",
		PlaceID:     "balances",
		KeyBindings: []string{"alice"},
		IsNested:    false,
	}

	// Add the value witness (normally done by guard compiler)
	witnesses.AddStateRead("balances", []string{"alice"})

	// Add state root witness
	stateRoot := witnesses.AddBinding("stateRoot")

	proof, constraints := compiler.CompileStateAccess(access, stateRoot.Name)

	t.Logf("=== State Access Merkle Proof ===")
	t.Logf("Access: %s", access)
	t.Logf("Proof key witness: %s", proof.Key)
	t.Logf("Proof value witness: %s", proof.Value)
	t.Logf("Path depth: %d", len(proof.PathElements))
	t.Logf("")

	// Should have MerkleProofDepth levels
	if len(proof.PathElements) != MerkleProofDepth {
		t.Errorf("expected %d path elements, got %d", MerkleProofDepth, len(proof.PathElements))
	}

	t.Logf("Constraints (%d total):", len(constraints))
	for i, c := range constraints[:min(10, len(constraints))] {
		t.Logf("  %d. %s", i+1, c)
	}
	if len(constraints) > 10 {
		t.Logf("  ... and %d more", len(constraints)-10)
	}
}

func TestMerkleProofCompiler_NestedAccess(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewMerkleProofCompiler(witnesses)

	// Simulate a nested state access: allowances[owner][spender]
	access := &StateAccess{
		WitnessName: "allowances_owner_spender",
		PlaceID:     "allowances",
		KeyBindings: []string{"owner", "spender"},
		IsNested:    true,
	}

	// Add the value witness
	witnesses.AddStateRead("allowances", []string{"owner", "spender"})

	stateRoot := witnesses.AddBinding("stateRoot")

	proof, constraints := compiler.CompileStateAccess(access, stateRoot.Name)

	t.Logf("=== Nested State Access Merkle Proof ===")
	t.Logf("Access: %s", access)
	t.Logf("Proof key witness: %s (composite)", proof.Key)
	t.Logf("")

	// Check for composite key hash
	foundComposite := false
	for _, c := range constraints {
		if c.Type == Poseidon && strings.Contains(c.Tag, "compositeKey") {
			foundComposite = true
			t.Logf("Composite key: %s [%s]", c, c.Tag)
			break
		}
	}
	if !foundComposite {
		t.Error("expected composite key hash for nested access")
	}

	t.Logf("Total constraints: %d", len(constraints))
}

func TestMerkleProofCompiler_AllStateAccesses(t *testing.T) {
	witnesses := NewWitnessTable()

	// Simulate guard compiler output for: balances[from] >= amount && allowances[from][caller] >= amount
	witnesses.AddStateRead("balances", []string{"from"})
	witnesses.AddStateRead("allowances", []string{"from", "caller"})

	stateAccesses := witnesses.StateReads
	stateRoot := witnesses.AddBinding("preStateRoot")

	compiler := NewMerkleProofCompiler(witnesses)
	proofs, constraints := compiler.CompileAllStateAccesses(stateAccesses, stateRoot.Name)

	t.Logf("=== ERC-20 TransferFrom State Proofs ===")
	t.Logf("State accesses: %d", len(stateAccesses))
	for _, access := range stateAccesses {
		t.Logf("  - %s", access)
	}
	t.Logf("")

	t.Logf("Generated proofs: %d", len(proofs))
	for i, proof := range proofs {
		t.Logf("  %d. key=%s value=%s", i+1, proof.Key, proof.Value)
	}
	t.Logf("")

	t.Logf("Total constraints: %d", len(constraints))

	// Each state access generates ~(1 + MerkleProofDepth*4 + 1) constraints
	// Plus nested access gets an extra composite key hash
	expectedMin := len(stateAccesses) * (MerkleProofDepth*4 + 2)
	if len(constraints) < expectedMin {
		t.Errorf("expected at least %d constraints, got %d", expectedMin, len(constraints))
	}
}

func TestMerkleProofCompiler_IntegrationWithGuard(t *testing.T) {
	// Full integration: guard compilation → Merkle proof compilation
	guardCompiler := NewGuardCompiler()
	result, err := guardCompiler.Compile("balances[from] >= amount")
	if err != nil {
		t.Fatalf("guard compile error: %v", err)
	}

	t.Logf("=== Full Integration Test ===")
	t.Logf("Guard: balances[from] >= amount")
	t.Logf("")

	t.Logf("Guard constraints (%d):", len(result.Constraints))
	for i, c := range result.Constraints {
		t.Logf("  %d. %s [%s]", i+1, c, c.Tag)
	}
	t.Logf("")

	t.Logf("State reads requiring Merkle proofs: %d", len(result.StateReads))
	for _, sr := range result.StateReads {
		t.Logf("  - %s (witness: %s)", sr, sr.WitnessName)
	}
	t.Logf("")

	// Now compile Merkle proofs for state reads
	merkleCompiler := NewMerkleProofCompiler(result.Witnesses)
	stateRoot := result.Witnesses.AddBinding("stateRoot")
	proofs, merkleConstraints := merkleCompiler.CompileAllStateAccesses(result.StateReads, stateRoot.Name)

	t.Logf("Merkle proof constraints: %d", len(merkleConstraints))
	t.Logf("")

	// Total constraint count
	totalConstraints := len(result.Constraints) + len(merkleConstraints)
	t.Logf("TOTAL CONSTRAINTS: %d", totalConstraints)
	t.Logf("  - Guard logic: %d", len(result.Constraints))
	t.Logf("  - Merkle proofs: %d", len(merkleConstraints))
	t.Logf("  - Proofs generated: %d", len(proofs))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
