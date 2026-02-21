package prover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
)

func TestCompiledCircuit_SaveAndLoad(t *testing.T) {
	// Compile and setup a circuit.
	p := NewProver()
	cc, err := p.CompileCircuit("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "simple")

	// Save to disk.
	if err := cc.SaveTo(dir); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify files exist.
	for _, name := range []string{"circuit.r1cs", "proving.key", "verifying.key", "circuit.hash"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected file %s: %v", name, err)
		}
	}

	// Load back from disk.
	loaded, err := LoadFrom(dir, ecc.BN254)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Prove with the loaded keys — this verifies the round-trip is correct.
	loaded.Name = "simple"
	p.StoreCircuit("simple", loaded)

	err = p.Verify("simple", &SimpleTestCircuit{X: 9, Y: 3})
	if err != nil {
		t.Fatalf("verify with loaded keys failed: %v", err)
	}
}

func TestLoadOrCompile_Fresh(t *testing.T) {
	dir := t.TempDir()
	p := NewProverWithKeyDir(dir)

	cc, err := p.LoadOrCompile("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("load or compile: %v", err)
	}

	if cc.Constraints == 0 {
		t.Error("expected non-zero constraints")
	}

	// Verify key files were created.
	for _, name := range []string{"circuit.r1cs", "proving.key", "verifying.key", "circuit.hash"} {
		path := filepath.Join(dir, "simple", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s: %v", path, err)
		}
	}
}

func TestLoadOrCompile_CacheHit(t *testing.T) {
	dir := t.TempDir()

	// First run — generates and saves keys.
	p1 := NewProverWithKeyDir(dir)
	cc1, err := p1.LoadOrCompile("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}

	// Export verifier from first run for comparison.
	sol1, err := p1.ExportVerifier("simple")
	if err != nil {
		t.Fatalf("export verifier 1: %v", err)
	}

	// Second run — should load from disk.
	p2 := NewProverWithKeyDir(dir)
	cc2, err := p2.LoadOrCompile("simple", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}

	// Same constraint count.
	if cc1.Constraints != cc2.Constraints {
		t.Errorf("constraint count mismatch: %d vs %d", cc1.Constraints, cc2.Constraints)
	}

	// Verifying keys produce identical Solidity verifiers.
	sol2, err := p2.ExportVerifier("simple")
	if err != nil {
		t.Fatalf("export verifier 2: %v", err)
	}
	if sol1 != sol2 {
		t.Error("exported Solidity verifiers differ — keys were not reused")
	}

	// Prove with the loaded keys.
	err = p2.Verify("simple", &SimpleTestCircuit{X: 25, Y: 5})
	if err != nil {
		t.Fatalf("verify with cached keys failed: %v", err)
	}
}

func TestLoadOrCompile_CircuitChanged(t *testing.T) {
	dir := t.TempDir()

	// First run with SimpleTestCircuit.
	p1 := NewProverWithKeyDir(dir)
	_, err := p1.LoadOrCompile("test", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	sol1, err := p1.ExportVerifier("test")
	if err != nil {
		t.Fatalf("export verifier 1: %v", err)
	}

	// Second run with a different circuit (BalanceCheckCircuit) under the same name.
	// The hash should differ, triggering regeneration.
	p2 := NewProverWithKeyDir(dir)
	_, err = p2.LoadOrCompile("test", &BalanceCheckCircuit{})
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}
	sol2, err := p2.ExportVerifier("test")
	if err != nil {
		t.Fatalf("export verifier 2: %v", err)
	}

	// Verifiers should differ because the circuit changed.
	if sol1 == sol2 {
		t.Error("expected different verifiers after circuit change")
	}
}
