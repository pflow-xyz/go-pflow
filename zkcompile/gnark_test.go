package zkcompile

import (
	"strings"
	"testing"
)

func TestGnarkCodegen_SimpleTransfer(t *testing.T) {
	// Compile a simple transfer guard
	guardCompiler := NewGuardCompiler()
	guardResult, err := guardCompiler.Compile("balances[from] >= amount")
	if err != nil {
		t.Fatalf("guard compile error: %v", err)
	}

	// Compile Merkle proofs
	merkleCompiler := NewMerkleProofCompiler(guardResult.Witnesses)
	stateRoot := guardResult.Witnesses.AddBinding("preStateRoot")
	_, merkleConstraints := merkleCompiler.CompileAllStateAccesses(guardResult.StateReads, stateRoot.Name)

	// Generate gnark code
	codegen := NewGnarkCodegen("circuit", "TransferCircuit")
	code := codegen.GenerateCircuit(guardResult, nil, merkleConstraints, nil)

	t.Logf("=== Generated gnark Circuit ===\n%s", code)

	// Verify code structure
	if !strings.Contains(code, "package circuit") {
		t.Error("expected package declaration")
	}
	if !strings.Contains(code, "type TransferCircuit struct") {
		t.Error("expected circuit struct")
	}
	if !strings.Contains(code, "func (c *TransferCircuit) Define(api frontend.API) error") {
		t.Error("expected Define method")
	}
	if !strings.Contains(code, "gnark/frontend") {
		t.Error("expected gnark frontend import")
	}
	if !strings.Contains(code, "mimc") {
		t.Error("expected mimc import")
	}
}

func TestGnarkCodegen_ERC20TransferFrom(t *testing.T) {
	// Compile the full ERC-20 transferFrom guard
	guardCompiler := NewGuardCompiler()
	guardResult, err := guardCompiler.Compile("balances[from] >= amount && allowances[from][caller] >= amount")
	if err != nil {
		t.Fatalf("guard compile error: %v", err)
	}

	// Compile Merkle proofs
	merkleCompiler := NewMerkleProofCompiler(guardResult.Witnesses)
	stateRoot := guardResult.Witnesses.AddBinding("preStateRoot")
	proofs, merkleConstraints := merkleCompiler.CompileAllStateAccesses(guardResult.StateReads, stateRoot.Name)

	// Add invariant constraints (conservation law)
	invariantCompiler := NewInvariantCompiler(guardResult.Witnesses)
	transitions := []StateTransition{
		{
			Place:   "balances",
			Keys:    []string{"from"},
			PreVar:  "balances_from_pre",
			PostVar: "balances_from_post",
		},
		{
			Place:   "balances",
			Keys:    []string{"to"},
			PreVar:  "balances_to_pre",
			PostVar: "balances_to_post",
		},
	}
	invariantConstraints := invariantCompiler.CompileConservation("balances", "totalSupply", transitions)

	// Generate gnark code
	codegen := NewGnarkCodegen("circuit", "TransferFromCircuit")
	code := codegen.GenerateCircuit(guardResult, proofs, merkleConstraints, invariantConstraints)

	t.Logf("=== Generated gnark Circuit (ERC-20 TransferFrom) ===")
	t.Logf("Code length: %d bytes", len(code))
	t.Logf("")

	// Just show first 100 lines
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if i >= 100 {
			t.Logf("... (%d more lines)", len(lines)-100)
			break
		}
		t.Logf("%s", line)
	}

	// Verify key components
	if !strings.Contains(code, "PreStateRoot") {
		t.Error("expected PreStateRoot public input")
	}
	if !strings.Contains(code, "PostStateRoot") {
		t.Error("expected PostStateRoot public input")
	}
	if !strings.Contains(code, "Amount") || !strings.Contains(code, "From") {
		t.Error("expected transaction binding fields")
	}
	if !strings.Contains(code, "api.AssertIsEqual") {
		t.Error("expected equality constraints")
	}
	if !strings.Contains(code, "mimcHash") {
		t.Error("expected mimcHash calls")
	}
}

func TestGnarkCodegen_CircuitStats(t *testing.T) {
	// Compile the full ERC-20 transferFrom guard
	guardCompiler := NewGuardCompiler()
	guardResult, err := guardCompiler.Compile("balances[from] >= amount && allowances[from][caller] >= amount")
	if err != nil {
		t.Fatalf("guard compile error: %v", err)
	}

	// Compile Merkle proofs
	merkleCompiler := NewMerkleProofCompiler(guardResult.Witnesses)
	stateRoot := guardResult.Witnesses.AddBinding("preStateRoot")
	_, merkleConstraints := merkleCompiler.CompileAllStateAccesses(guardResult.StateReads, stateRoot.Name)

	// Compute stats
	stats := ComputeStats(guardResult, merkleConstraints, nil)

	t.Logf("=== Circuit Statistics ===")
	t.Logf("%s", stats)
	t.Logf("")

	// Verify reasonable numbers
	if stats.TotalConstraints < 100 {
		t.Errorf("expected at least 100 constraints for transferFrom, got %d", stats.TotalConstraints)
	}
	if stats.PoseidonHashCount < 40 {
		t.Errorf("expected at least 40 Poseidon hashes (2 proofs * 20 levels), got %d", stats.PoseidonHashCount)
	}
	if stats.PublicInputCount < 3 {
		t.Errorf("expected at least 3 public inputs (from, caller, amount), got %d", stats.PublicInputCount)
	}
}

func TestToGoName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"balances_from", "BalancesFrom"},
		{"amount", "Amount"},
		{"gte_diff_0", "GteDiff0"},
		{"merkle_h0_7", "MerkleH07"},
		{"const_0x0000", "Addr0000"},
		{"pre_state_root", "PreStateRoot"},
	}

	for _, tt := range tests {
		result := toGoName(tt.input)
		if result != tt.expected {
			t.Errorf("toGoName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGnarkCodegen_ExpressionGeneration(t *testing.T) {
	codegen := NewGnarkCodegen("test", "Test")

	tests := []struct {
		expr     *Expr
		expected string
	}{
		{VarExpr("amount"), "c.Amount"},
		{ConstInt(42), "frontend.Variable(42)"},
		{AddExpr(VarExpr("a"), VarExpr("b")), "api.Add(c.A, c.B)"},
		{SubExpr(VarExpr("a"), VarExpr("b")), "api.Sub(c.A, c.B)"},
		{MulExpr(VarExpr("a"), VarExpr("b")), "api.Mul(c.A, c.B)"},
		{NegExpr(VarExpr("a")), "api.Neg(c.A)"},
	}

	for _, tt := range tests {
		result := codegen.generateExpr(tt.expr)
		if result != tt.expected {
			t.Errorf("generateExpr(%v) = %q, want %q", tt.expr, result, tt.expected)
		}
	}
}
