package zkcompile

import (
	"strings"
	"testing"

	"github.com/consensys/gnark/frontend"
)

// TestBalanceCircuit for Solidity export testing
type TestBalanceCircuit struct {
	Amount  frontend.Variable `gnark:",public"`
	Balance frontend.Variable
}

func (c *TestBalanceCircuit) Define(api frontend.API) error {
	diff := api.Sub(c.Balance, c.Amount)
	api.ToBinary(diff, 64)
	return nil
}

func TestSolidityExporter_ExportVerifier(t *testing.T) {
	exporter := NewSolidityExporter()

	var circuit TestBalanceCircuit
	solidityCode, err := exporter.ExportVerifier(&circuit)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	t.Logf("Generated Solidity verifier: %d bytes", len(solidityCode))

	// Verify it contains expected components
	if !strings.Contains(solidityCode, "pragma solidity") {
		t.Error("expected pragma solidity")
	}
	if !strings.Contains(solidityCode, "verifyProof") {
		t.Error("expected verifyProof function")
	}
	if !strings.Contains(solidityCode, "PRECOMPILE_VERIFY") {
		t.Error("expected precompile constants")
	}

	// Show first 50 lines
	lines := strings.Split(solidityCode, "\n")
	t.Logf("\n=== Solidity Verifier (first 50 lines) ===")
	for i, line := range lines {
		if i >= 50 {
			t.Logf("... (%d more lines)", len(lines)-50)
			break
		}
		t.Logf("%s", line)
	}
}

func TestSolidityExporter_ExportWithKeys(t *testing.T) {
	exporter := NewSolidityExporter()

	var circuit TestBalanceCircuit
	solidityCode, pk, vk, err := exporter.ExportVerifierWithKeys(&circuit)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	t.Logf("=== Export Results ===")
	t.Logf("Solidity code: %d bytes", len(solidityCode))
	t.Logf("Proving key G1 points: %d", pk.NbG1())
	t.Logf("Proving key G2 points: %d", pk.NbG2())
	t.Logf("Verification key ready: %v", vk != nil)
}

func TestGenerateZKWrapper(t *testing.T) {
	code := GenerateZKWrapper("TestToken")

	t.Logf("=== TestTokenZK Contract ===\n%s", code)

	// Verify structure
	if !strings.Contains(code, "contract TestTokenZK") {
		t.Error("expected TestTokenZK contract")
	}
	if !strings.Contains(code, "verifyAndExecute") {
		t.Error("expected verifyAndExecute function")
	}
	if !strings.Contains(code, "stateRoot") {
		t.Error("expected stateRoot storage")
	}
	if !strings.Contains(code, "StateTransition") {
		t.Error("expected StateTransition event")
	}
}

func TestGenerateProofHelper(t *testing.T) {
	code := GenerateProofHelper("transfer", "TransferCircuit")

	t.Logf("=== Proof Helper (first 30 lines) ===")
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if i >= 30 {
			break
		}
		t.Logf("%s", line)
	}

	// Verify structure
	if !strings.Contains(code, "package transfer") {
		t.Error("expected package declaration")
	}
	if !strings.Contains(code, "type Prover struct") {
		t.Error("expected Prover struct")
	}
	if !strings.Contains(code, "func NewProver") {
		t.Error("expected NewProver function")
	}
}

// Full integration: compile circuit, export verifier, show stats
func TestSolidityExporter_FullPipeline(t *testing.T) {
	t.Logf("=== Full ZK Pipeline Test ===\n")

	// Step 1: Define circuit (balance check)
	var circuit TestBalanceCircuit
	t.Logf("1. Circuit defined: TestBalanceCircuit")

	// Step 2: Export Solidity verifier
	exporter := NewSolidityExporter()
	solidityCode, pk, vk, err := exporter.ExportVerifierWithKeys(&circuit)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	t.Logf("2. Solidity verifier exported: %d bytes", len(solidityCode))
	t.Logf("   - Proving key: %d G1 + %d G2 points", pk.NbG1(), pk.NbG2())

	// Step 3: Generate wrapper contract
	wrapperCode := GenerateZKWrapper("TestBalance")
	t.Logf("3. ZK wrapper generated: %d bytes", len(wrapperCode))

	// Step 4: Show deployment plan
	t.Logf("\n=== Deployment Plan ===")
	t.Logf("1. Deploy Verifier.sol (the exported Groth16 verifier)")
	t.Logf("2. Deploy ArcTokenZK.sol with Verifier address + initial state root")
	t.Logf("3. Users call verifyAndExecute() with ZK proofs")
	t.Logf("")
	t.Logf("Gas estimates:")
	t.Logf("  - Verifier deployment: ~2M gas")
	t.Logf("  - ArcTokenZK deployment: ~500K gas")
	t.Logf("  - verifyAndExecute call: ~300K gas (proof verification)")

	_ = vk // verification key can be used for off-chain verification
}
