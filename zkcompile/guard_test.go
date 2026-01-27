package zkcompile

import (
	"strings"
	"testing"
)

func TestGuardCompiler_SimpleComparison(t *testing.T) {
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("amount >= 0")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Should have binding for 'amount'
	if _, ok := result.Witnesses.Get("amount"); !ok {
		t.Error("expected 'amount' witness")
	}

	// Should have constraints for >= (diff = left - right, range check)
	if len(result.Constraints) < 2 {
		t.Errorf("expected at least 2 constraints, got %d", len(result.Constraints))
	}

	t.Logf("Constraints:")
	for _, c := range result.Constraints {
		t.Logf("  %s [%s]", c, c.Tag)
	}
}

func TestGuardCompiler_StateAccess(t *testing.T) {
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("balances[from] >= amount")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Should have state read for balances[from]
	if len(result.StateReads) != 1 {
		t.Errorf("expected 1 state read, got %d", len(result.StateReads))
	}

	sr := result.StateReads[0]
	if sr.PlaceID != "balances" {
		t.Errorf("expected place 'balances', got '%s'", sr.PlaceID)
	}
	if len(sr.KeyBindings) != 1 || sr.KeyBindings[0] != "from" {
		t.Errorf("expected key ['from'], got %v", sr.KeyBindings)
	}

	// Should have witnesses: balances_from, from, amount
	if _, ok := result.Witnesses.Get("balances_from"); !ok {
		t.Error("expected 'balances_from' witness")
	}
	if _, ok := result.Witnesses.Get("amount"); !ok {
		t.Error("expected 'amount' witness")
	}

	t.Logf("State reads: %v", result.StateReads)
	t.Logf("Witnesses: %v", result.Witnesses.Variables)
}

func TestGuardCompiler_NestedMapAccess(t *testing.T) {
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("allowances[from][caller] >= amount")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Should have nested state read
	if len(result.StateReads) != 1 {
		t.Errorf("expected 1 state read, got %d", len(result.StateReads))
	}

	sr := result.StateReads[0]
	if sr.PlaceID != "allowances" {
		t.Errorf("expected place 'allowances', got '%s'", sr.PlaceID)
	}
	if !sr.IsNested {
		t.Error("expected nested access")
	}
	if len(sr.KeyBindings) != 2 {
		t.Errorf("expected 2 keys, got %d", len(sr.KeyBindings))
	}
	if sr.KeyBindings[0] != "from" || sr.KeyBindings[1] != "caller" {
		t.Errorf("expected keys ['from', 'caller'], got %v", sr.KeyBindings)
	}

	t.Logf("State read: %s", sr)
}

func TestGuardCompiler_LogicalAnd(t *testing.T) {
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("balances[from] >= amount && to != address(0)")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Should have state read for balances[from]
	foundBalances := false
	for _, sr := range result.StateReads {
		if sr.PlaceID == "balances" {
			foundBalances = true
		}
	}
	if !foundBalances {
		t.Error("expected state read for balances")
	}

	// Should have constraint for != (diff * inv = 1)
	foundNEQ := false
	for _, c := range result.Constraints {
		if strings.Contains(c.Tag, "!=") {
			foundNEQ = true
		}
	}
	if !foundNEQ {
		t.Error("expected constraint for != comparison")
	}

	t.Logf("Constraints:")
	for _, c := range result.Constraints {
		t.Logf("  %s [%s]", c, c.Tag)
	}
}

func TestGuardCompiler_ERC20TransferGuard(t *testing.T) {
	// Real ERC-20 transfer guard
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("balances[from] >= amount && to != address(0)")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	t.Logf("=== ERC-20 Transfer Guard Compilation ===")
	t.Logf("Expression: balances[from] >= amount && to != address(0)")
	t.Logf("")

	t.Logf("State Reads (require Merkle proofs):")
	for _, sr := range result.StateReads {
		t.Logf("  - %s", sr)
	}
	t.Logf("")

	t.Logf("Witnesses:")
	for name, w := range result.Witnesses.Variables {
		t.Logf("  - %s: %s", name, w.Source)
	}
	t.Logf("")

	t.Logf("Constraints:")
	for i, c := range result.Constraints {
		t.Logf("  %d. %s", i+1, c)
		if c.Tag != "" {
			t.Logf("     [%s]", c.Tag)
		}
	}
}

func TestGuardCompiler_ERC20TransferFromGuard(t *testing.T) {
	// Real ERC-20 transferFrom guard
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("balances[from] >= amount && allowances[from][caller] >= amount")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	t.Logf("=== ERC-20 TransferFrom Guard Compilation ===")
	t.Logf("Expression: balances[from] >= amount && allowances[from][caller] >= amount")
	t.Logf("")

	// Should have 2 state reads
	if len(result.StateReads) != 2 {
		t.Errorf("expected 2 state reads, got %d", len(result.StateReads))
	}

	t.Logf("State Reads (require Merkle proofs):")
	for _, sr := range result.StateReads {
		t.Logf("  - %s (witness: %s)", sr, sr.WitnessName)
	}
	t.Logf("")

	t.Logf("Constraints (%d total):", len(result.Constraints))
	for i, c := range result.Constraints {
		t.Logf("  %d. %s", i+1, c)
	}
}

func TestGuardCompiler_NotEqual(t *testing.T) {
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("a != b")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Should have 2 constraints: diff = a - b, diff * inv = 1
	if len(result.Constraints) != 2 {
		t.Errorf("expected 2 constraints for !=, got %d", len(result.Constraints))
	}

	// Check for the inverse multiplication constraint
	foundInverse := false
	for _, c := range result.Constraints {
		if c.Type == Equal && strings.Contains(c.Left.String(), "neq") {
			foundInverse = true
		}
	}
	if !foundInverse {
		t.Error("expected inverse constraint for != proof")
	}

	t.Logf("!= Constraints:")
	for _, c := range result.Constraints {
		t.Logf("  %s [%s]", c, c.Tag)
	}
}

func TestGuardCompiler_Arithmetic(t *testing.T) {
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("a + b * c >= d")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Should have witnesses for a, b, c, d
	for _, name := range []string{"a", "b", "c", "d"} {
		if _, ok := result.Witnesses.Get(name); !ok {
			t.Errorf("expected witness '%s'", name)
		}
	}

	t.Logf("Arithmetic expression constraints:")
	for _, c := range result.Constraints {
		t.Logf("  %s", c)
	}
}

func TestGuardCompiler_BooleanNot(t *testing.T) {
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("!revoked")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	// Should have boolean constraint on 'revoked'
	foundBool := false
	for _, c := range result.Constraints {
		if c.Type == Boolean {
			foundBool = true
		}
	}
	if !foundBool {
		t.Error("expected boolean constraint for NOT operand")
	}

	t.Logf("NOT constraints:")
	for _, c := range result.Constraints {
		t.Logf("  %s", c)
	}
}

func TestGuardCompiler_EmptyGuard(t *testing.T) {
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	if len(result.Constraints) != 0 {
		t.Errorf("empty guard should have no constraints, got %d", len(result.Constraints))
	}
}

func TestGuardCompiler_VestingGuard(t *testing.T) {
	// Vesting claim guard from ERC-5725
	compiler := NewGuardCompiler()
	result, err := compiler.Compile("vestSchedules[tokenId] >= 0 && vestOwners[tokenId] == caller")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	t.Logf("=== Vesting Claim Guard ===")
	t.Logf("State Reads:")
	for _, sr := range result.StateReads {
		t.Logf("  - %s", sr)
	}
	t.Logf("")
	t.Logf("Constraints:")
	for _, c := range result.Constraints {
		t.Logf("  %s [%s]", c, c.Tag)
	}
}

// Benchmark constraint generation
func BenchmarkGuardCompiler_ERC20Transfer(b *testing.B) {
	expr := "balances[from] >= amount && to != address(0)"
	for i := 0; i < b.N; i++ {
		compiler := NewGuardCompiler()
		_, err := compiler.Compile(expr)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGuardCompiler_ERC20TransferFrom(b *testing.B) {
	expr := "balances[from] >= amount && allowances[from][caller] >= amount"
	for i := 0; i < b.N; i++ {
		compiler := NewGuardCompiler()
		_, err := compiler.Compile(expr)
		if err != nil {
			b.Fatal(err)
		}
	}
}
