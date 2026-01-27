package prover

import (
	"testing"

	"github.com/consensys/gnark/frontend"
)

// SimpleTestCircuit for basic prover testing
type SimpleTestCircuit struct {
	X frontend.Variable `gnark:",public"`
	Y frontend.Variable
}

func (c *SimpleTestCircuit) Define(api frontend.API) error {
	// X == Y * Y
	api.AssertIsEqual(c.X, api.Mul(c.Y, c.Y))
	return nil
}

func TestProver_RegisterCircuit(t *testing.T) {
	p := NewProver()

	err := p.RegisterCircuit("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	cc, ok := p.GetCircuit("simple")
	if !ok {
		t.Fatal("circuit not found after registration")
	}

	t.Logf("Circuit registered:")
	t.Logf("  Name: %s", cc.Name)
	t.Logf("  Constraints: %d", cc.Constraints)
	t.Logf("  Public vars: %d", cc.PublicVars)
	t.Logf("  Private vars: %d", cc.PrivateVars)
}

func TestProver_ListCircuits(t *testing.T) {
	p := NewProver()

	_ = p.RegisterCircuit("c1", &SimpleTestCircuit{})
	_ = p.RegisterCircuit("c2", &SimpleTestCircuit{})
	_ = p.RegisterCircuit("c3", &SimpleTestCircuit{})

	circuits := p.ListCircuits()
	if len(circuits) != 3 {
		t.Errorf("expected 3 circuits, got %d", len(circuits))
	}
}

func TestProver_Prove(t *testing.T) {
	p := NewProver()

	err := p.RegisterCircuit("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Valid assignment: X=9, Y=3 (3*3=9)
	assignment := &SimpleTestCircuit{
		X: 9,
		Y: 3,
	}

	result, err := p.Prove("simple", assignment)
	if err != nil {
		t.Fatalf("prove failed: %v", err)
	}

	t.Logf("Proof generated:")
	t.Logf("  Circuit: %s", result.CircuitName)
	t.Logf("  Constraints: %d", result.Constraints)
	t.Logf("  Public inputs: %d", len(result.PublicInputs))

	// Verify proof points are initialized (not nil)
	if result.A[0] == nil || result.A[1] == nil {
		t.Error("proof point A not initialized")
	}
	if result.C[0] == nil || result.C[1] == nil {
		t.Error("proof point C not initialized")
	}
}

func TestProver_Verify(t *testing.T) {
	p := NewProver()

	err := p.RegisterCircuit("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Valid assignment
	assignment := &SimpleTestCircuit{
		X: 16,
		Y: 4,
	}

	err = p.Verify("simple", assignment)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	t.Log("Proof verified successfully")
}

func TestProver_VerifyFails(t *testing.T) {
	p := NewProver()

	err := p.RegisterCircuit("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Invalid assignment: X=16, Y=3 (3*3=9, not 16)
	assignment := &SimpleTestCircuit{
		X: 16,
		Y: 3,
	}

	err = p.Verify("simple", assignment)
	if err == nil {
		t.Error("expected verify to fail for invalid assignment")
	} else {
		t.Logf("Verify correctly failed: %v", err)
	}
}

func TestProver_CircuitNotFound(t *testing.T) {
	p := NewProver()

	_, err := p.Prove("nonexistent", &SimpleTestCircuit{})
	if err == nil {
		t.Error("expected error for nonexistent circuit")
	}
}

func TestProver_ExportVerifier(t *testing.T) {
	p := NewProver()

	err := p.RegisterCircuit("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	solidity, err := p.ExportVerifier("simple")
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	t.Logf("Exported Solidity verifier: %d bytes", len(solidity))

	// Verify it contains expected content
	if len(solidity) < 1000 {
		t.Errorf("exported Solidity too short: %d bytes", len(solidity))
	}
}

// Balance check circuit for more realistic testing
type BalanceCheckCircuit struct {
	Amount  frontend.Variable `gnark:",public"`
	Balance frontend.Variable
}

func (c *BalanceCheckCircuit) Define(api frontend.API) error {
	diff := api.Sub(c.Balance, c.Amount)
	api.ToBinary(diff, 64)
	return nil
}

func TestProver_BalanceCheck(t *testing.T) {
	p := NewProver()

	err := p.RegisterCircuit("balance", &BalanceCheckCircuit{})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Valid: balance=1000 >= amount=100
	assignment := &BalanceCheckCircuit{
		Amount:  100,
		Balance: 1000,
	}

	result, err := p.Prove("balance", assignment)
	if err != nil {
		t.Fatalf("prove failed: %v", err)
	}

	t.Logf("Balance check proof: %d constraints", result.Constraints)
	t.Logf("Public inputs: %v", result.PublicInputs)
}

func TestProver_BalanceCheckFails(t *testing.T) {
	p := NewProver()

	err := p.RegisterCircuit("balance", &BalanceCheckCircuit{})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Invalid: balance=50 < amount=100
	assignment := &BalanceCheckCircuit{
		Amount:  100,
		Balance: 50,
	}

	_, err = p.Prove("balance", assignment)
	if err == nil {
		t.Error("expected prove to fail for insufficient balance")
	} else {
		t.Logf("Prove correctly failed: %v", err)
	}
}
