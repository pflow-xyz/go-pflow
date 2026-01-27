package prover

import (
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
)

func TestCurveConfigs(t *testing.T) {
	// Verify curve configurations are set up correctly
	configs := []struct {
		name     string
		config   CurveConfig
		expected ecc.ID
		role     CurveRole
	}{
		{"BN254", BN254Config, ecc.BN254, RoleWrapper},
		{"BLS12-377", BLS12_377Config, ecc.BLS12_377, RoleInner},
		{"BW6-761", BW6_761Config, ecc.BW6_761, RoleAggregation},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			if tc.config.ID != tc.expected {
				t.Errorf("expected curve ID %v, got %v", tc.expected, tc.config.ID)
			}
			if tc.config.Role != tc.role {
				t.Errorf("expected role %v, got %v", tc.role, tc.config.Role)
			}
			if tc.config.Name == "" {
				t.Error("expected non-empty name")
			}
			if tc.config.FieldSize == 0 {
				t.Error("expected non-zero field size")
			}
		})
	}
}

func TestNewCurveProver(t *testing.T) {
	configs := []CurveConfig{BN254Config, BLS12_377Config, BW6_761Config}

	for _, config := range configs {
		t.Run(config.Name, func(t *testing.T) {
			prover := NewCurveProver(config)
			if prover == nil {
				t.Fatal("expected non-nil prover")
			}
			if prover.CurveID() != config.ID {
				t.Errorf("expected curve ID %v, got %v", config.ID, prover.CurveID())
			}
			if prover.Config().Name != config.Name {
				t.Errorf("expected config name %s, got %s", config.Name, prover.Config().Name)
			}
			if len(prover.ListCircuits()) != 0 {
				t.Error("expected empty circuit list for new prover")
			}
		})
	}
}

func TestRecursionStack(t *testing.T) {
	stack := NewRecursionStack()
	if stack == nil {
		t.Fatal("expected non-nil recursion stack")
	}

	// Verify each prover is configured correctly
	if stack.Inner.CurveID() != ecc.BLS12_377 {
		t.Errorf("expected inner prover on BLS12-377, got %v", stack.Inner.CurveID())
	}
	if stack.Aggregation.CurveID() != ecc.BW6_761 {
		t.Errorf("expected aggregation prover on BW6-761, got %v", stack.Aggregation.CurveID())
	}
	if stack.Wrapper.CurveID() != ecc.BN254 {
		t.Errorf("expected wrapper prover on BN254, got %v", stack.Wrapper.CurveID())
	}

	// Test GetProver
	if stack.GetProver(RoleInner) != stack.Inner {
		t.Error("GetProver(RoleInner) should return Inner prover")
	}
	if stack.GetProver(RoleAggregation) != stack.Aggregation {
		t.Error("GetProver(RoleAggregation) should return Aggregation prover")
	}
	if stack.GetProver(RoleWrapper) != stack.Wrapper {
		t.Error("GetProver(RoleWrapper) should return Wrapper prover")
	}
}

func TestCompileInnerPlaceholder(t *testing.T) {
	ccs, err := CompileInnerPlaceholder()
	if err != nil {
		t.Fatalf("failed to compile inner placeholder: %v", err)
	}

	// Verify constraint system was created
	if ccs == nil {
		t.Fatal("expected non-nil constraint system")
	}

	// Should have 5 public variables (4 public inputs + 1 constant)
	// gnark includes the constant "1" in the public variable count
	if ccs.GetNbPublicVariables() != 5 {
		t.Errorf("expected 5 public variables, got %d", ccs.GetNbPublicVariables())
	}
}

func TestCompileAggregationPlaceholder(t *testing.T) {
	ccs, err := CompileAggregationPlaceholder()
	if err != nil {
		t.Fatalf("failed to compile aggregation placeholder: %v", err)
	}

	if ccs == nil {
		t.Fatal("expected non-nil constraint system")
	}

	// Should have 5 public variables (4 public inputs + 1 constant)
	if ccs.GetNbPublicVariables() != 5 {
		t.Errorf("expected 5 public variables, got %d", ccs.GetNbPublicVariables())
	}
}

func TestNewAggregatorCircuit(t *testing.T) {
	numProofs := 4 // Use smaller number for faster test

	circuit, err := NewAggregatorCircuit(numProofs)
	if err != nil {
		t.Fatalf("failed to create aggregator circuit: %v", err)
	}

	if circuit == nil {
		t.Fatal("expected non-nil circuit")
	}
	if circuit.NumProofs != numProofs {
		t.Errorf("expected NumProofs=%d, got %d", numProofs, circuit.NumProofs)
	}
	if len(circuit.InnerProofs) != numProofs {
		t.Errorf("expected %d inner proofs, got %d", numProofs, len(circuit.InnerProofs))
	}
	if len(circuit.InnerWitnesses) != numProofs {
		t.Errorf("expected %d inner witnesses, got %d", numProofs, len(circuit.InnerWitnesses))
	}
}

func TestNewWrapperCircuit(t *testing.T) {
	circuit, err := NewWrapperCircuit()
	if err != nil {
		t.Fatalf("failed to create wrapper circuit: %v", err)
	}

	if circuit == nil {
		t.Fatal("expected non-nil circuit")
	}
}

func TestAggregatorWitnessValidation(t *testing.T) {
	// Test empty witness
	emptyWitness := &AggregatorWitness{
		InnerProofs: []groth16.Proof{},
	}

	_, err := emptyWitness.ToAssignment()
	if err == nil {
		t.Error("expected error for empty witness")
	}

	// Test mismatched counts
	mismatchedWitness := &AggregatorWitness{
		InnerProofs:    make([]groth16.Proof, 4),
		InnerWitnesses: make([]witness.Witness, 3), // Different count
	}

	_, err = mismatchedWitness.ToAssignment()
	if err == nil {
		t.Error("expected error for mismatched witness counts")
	}
}

func TestPipelineConfig(t *testing.T) {
	config := DefaultPipelineConfig()

	if config.BatchSize != DefaultAggregationSize {
		t.Errorf("expected default batch size %d, got %d",
			DefaultAggregationSize, config.BatchSize)
	}
	if config.InnerCircuitName != "batch8" {
		t.Errorf("expected inner circuit name 'batch8', got '%s'", config.InnerCircuitName)
	}
}

func TestAggregatedBatchProof(t *testing.T) {
	proof := AggregatedBatchProof{
		PrevStateRoot: [32]byte{1, 2, 3},
		NewStateRoot:  [32]byte{4, 5, 6},
		BatchStart:    1,
		BatchEnd:      8,
		NumBatches:    8,
		PublicInputs: []*big.Int{
			big.NewInt(100),
			big.NewInt(200),
		},
	}

	if proof.BatchStart != 1 {
		t.Error("unexpected batch start")
	}
	if proof.BatchEnd != 8 {
		t.Error("unexpected batch end")
	}
	if proof.NumBatches != 8 {
		t.Error("unexpected num batches")
	}
}

func TestWrappedProof(t *testing.T) {
	wrapped := WrappedProof{
		A:             [2]*big.Int{big.NewInt(1), big.NewInt(2)},
		B:             [2][2]*big.Int{{big.NewInt(3), big.NewInt(4)}, {big.NewInt(5), big.NewInt(6)}},
		C:             [2]*big.Int{big.NewInt(7), big.NewInt(8)},
		RawProof:      [8]*big.Int{},
		PublicInputs:  []string{"0x1", "0x2", "0x3", "0x4"},
		PrevStateRoot: [32]byte{1},
		NewStateRoot:  [32]byte{2},
		BatchStart:    1,
		BatchEnd:      8,
		NumBatches:    8,
	}

	if len(wrapped.PublicInputs) != 4 {
		t.Errorf("expected 4 public inputs, got %d", len(wrapped.PublicInputs))
	}
	if wrapped.NumBatches != 8 {
		t.Error("unexpected num batches")
	}
}

