package zkcompile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPipeline_Transfer(t *testing.T) {
	pipeline := NewPipeline("", "transfer")

	result, err := pipeline.Compile("balances[from] >= amount", "TransferCircuit")
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	t.Logf("\n%s", result.Summary())

	// Verify reasonable results
	if result.TotalConstraints < 80 {
		t.Errorf("expected at least 80 constraints, got %d", result.TotalConstraints)
	}
	if len(result.GnarkCircuitCode) < 1000 {
		t.Errorf("expected gnark code, got %d bytes", len(result.GnarkCircuitCode))
	}
}

func TestPipeline_TransferFrom(t *testing.T) {
	pipeline := NewPipeline("", "transfer")

	result, err := pipeline.Compile(
		"balances[from] >= amount && allowances[from][caller] >= amount",
		"TransferFromCircuit",
	)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	t.Logf("\n%s", result.Summary())

	// TransferFrom should have more constraints (2 state accesses)
	if result.TotalConstraints < 160 {
		t.Errorf("expected at least 160 constraints, got %d", result.TotalConstraints)
	}
}

func TestPipeline_WriteFiles(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "zkcompile-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pipeline := NewPipeline(tmpDir, "transfer")

	result, err := pipeline.Compile("balances[from] >= amount", "TransferCircuit")
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	err = pipeline.WriteFiles(result, "TransferCircuit")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Verify files exist
	files := []string{"circuit.go", "TransferCircuitZK.sol", "prover.go"}
	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", f)
		} else {
			info, _ := os.Stat(path)
			t.Logf("Created: %s (%d bytes)", f, info.Size())
		}
	}
}

func TestQuickCompile(t *testing.T) {
	result, err := QuickCompile("balances[from] >= amount && to != address(0)")
	if err != nil {
		t.Fatalf("quick compile failed: %v", err)
	}

	t.Logf("Quick compile: %d constraints, %d public inputs",
		result.TotalConstraints, result.PublicInputs)
}

func TestPipeline_VestingGuard(t *testing.T) {
	pipeline := NewPipeline("", "vesting")

	// ERC-5725 vesting claim guard
	result, err := pipeline.Compile(
		"vestSchedules[tokenId] >= 0 && vestOwners[tokenId] == caller",
		"VestingClaimCircuit",
	)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	t.Logf("=== Vesting Claim Circuit ===")
	t.Logf("Total constraints: %d", result.TotalConstraints)
	t.Logf("Public inputs: %d", result.PublicInputs)
	t.Logf("Private inputs: %d", result.PrivateInputs)
}

func TestPipeline_AllERC20Operations(t *testing.T) {
	tests := []struct {
		name  string
		guard string
	}{
		{"transfer", "balances[from] >= amount && to != address(0)"},
		{"transferFrom", "balances[from] >= amount && allowances[from][caller] >= amount"},
		{"approve", "owner == caller"}, // simplified
		{"mint", "caller == minter"},   // simplified
		{"burn", "balances[from] >= amount"},
	}

	t.Logf("=== ERC-20 Operation Circuits ===\n")
	t.Logf("%-15s %12s %8s %8s", "Operation", "Constraints", "Public", "Private")
	t.Logf("%-15s %12s %8s %8s", "---------", "-----------", "------", "-------")

	for _, tt := range tests {
		result, err := QuickCompile(tt.guard)
		if err != nil {
			t.Errorf("%s failed: %v", tt.name, err)
			continue
		}
		t.Logf("%-15s %12d %8d %8d",
			tt.name, result.TotalConstraints, result.PublicInputs, result.PrivateInputs)
	}
}
