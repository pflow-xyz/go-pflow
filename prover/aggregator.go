package prover

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/algebra/native/sw_bls12377"
	stdgroth16 "github.com/consensys/gnark/std/recursion/groth16"
)

// DefaultAggregationSize is the default number of inner proofs to aggregate.
const DefaultAggregationSize = 8

// AggregatorCircuit verifies N BLS12-377 batch proofs and aggregates them.
// This circuit runs on BW6-761, where BLS12-377 verification is "native"
// (BW6-761's base field equals BLS12-377's scalar field).
//
// Public inputs:
//   - PrevStateRoot: State root before the first batch
//   - FinalStateRoot: State root after the last batch
//   - BatchStart: First batch number in the range
//   - BatchEnd: Last batch number in the range (inclusive)
//
// Private inputs:
//   - InnerProofs: The N BLS12-377 Groth16 proofs to verify
//   - InnerWitnesses: Public witnesses for each inner proof
//   - InnerVK: Verifying key for the inner circuit (shared for all)
type AggregatorCircuit struct {
	// Public inputs
	PrevStateRoot  frontend.Variable `gnark:",public"`
	FinalStateRoot frontend.Variable `gnark:",public"`
	BatchStart     frontend.Variable `gnark:",public"`
	BatchEnd       frontend.Variable `gnark:",public"`

	// Number of proofs to aggregate (fixed at compile time)
	NumProofs int

	// Inner proofs and witnesses (BLS12-377)
	InnerProofs    []stdgroth16.Proof[sw_bls12377.G1Affine, sw_bls12377.G2Affine]
	InnerWitnesses []stdgroth16.Witness[sw_bls12377.ScalarField]
	InnerVK        stdgroth16.VerifyingKey[sw_bls12377.G1Affine, sw_bls12377.G2Affine, sw_bls12377.GT]
}

// Define implements the aggregation circuit constraints.
func (c *AggregatorCircuit) Define(api frontend.API) error {
	if c.NumProofs < 1 {
		return fmt.Errorf("NumProofs must be at least 1")
	}

	// Create recursive verifier for BLS12-377 proofs
	verifier, err := stdgroth16.NewVerifier[sw_bls12377.ScalarField, sw_bls12377.G1Affine, sw_bls12377.G2Affine, sw_bls12377.GT](api)
	if err != nil {
		return fmt.Errorf("failed to create verifier: %w", err)
	}

	// Track state root chain
	currentRoot := c.PrevStateRoot

	// Verify each inner proof and chain state roots
	for i := 0; i < c.NumProofs; i++ {
		// Verify the proof
		if err := verifier.AssertProof(c.InnerVK, c.InnerProofs[i], c.InnerWitnesses[i], stdgroth16.WithCompleteArithmetic()); err != nil {
			return fmt.Errorf("failed to verify inner proof %d: %w", i, err)
		}

		// Extract state roots from witness
		// Inner batch circuit public inputs: [prevRoot, newRoot, txRoot, batchNum]
		innerPrevRoot := c.InnerWitnesses[i].Public[0]
		innerNewRoot := c.InnerWitnesses[i].Public[1]
		batchNum := c.InnerWitnesses[i].Public[3]

		// Chain: current proof's prevRoot must equal last proof's newRoot
		api.AssertIsEqual(innerPrevRoot.Limbs[0], currentRoot)
		currentRoot = innerNewRoot.Limbs[0]

		// Verify batch numbers are sequential
		if i == 0 {
			api.AssertIsEqual(batchNum.Limbs[0], c.BatchStart)
		} else {
			// Each batch number should increment
			expectedBatch := api.Add(c.BatchStart, i)
			api.AssertIsEqual(batchNum.Limbs[0], expectedBatch)
		}
	}

	// Final state root must match claimed FinalStateRoot
	api.AssertIsEqual(currentRoot, c.FinalStateRoot)

	// Verify batch range
	expectedEnd := api.Add(c.BatchStart, c.NumProofs-1)
	api.AssertIsEqual(c.BatchEnd, expectedEnd)

	return nil
}

// InnerBatchCircuit is a minimal representation of the batch circuit for placeholder generation.
// Used only to get the correct constraint system structure for placeholder VK/proof/witness.
type InnerBatchCircuit struct {
	PrevStateRoot frontend.Variable `gnark:",public"`
	NewStateRoot  frontend.Variable `gnark:",public"`
	TxRoot        frontend.Variable `gnark:",public"`
	BatchNum      frontend.Variable `gnark:",public"`
	// Private inputs omitted for placeholder
	Dummy frontend.Variable
}

func (c *InnerBatchCircuit) Define(api frontend.API) error {
	api.AssertIsDifferent(c.Dummy, 0)
	return nil
}

// CompileInnerPlaceholder compiles a minimal inner circuit to get the structure
// needed for placeholder VK, proof, and witness generation.
func CompileInnerPlaceholder() (constraint.ConstraintSystem, error) {
	circuit := &InnerBatchCircuit{}
	return frontend.Compile(ecc.BLS12_377.ScalarField(), r1cs.NewBuilder, circuit)
}

// NewAggregatorCircuit creates a new aggregator circuit with placeholder values.
// Used for circuit compilation (trusted setup).
func NewAggregatorCircuit(numProofs int) (*AggregatorCircuit, error) {
	// Compile a minimal inner circuit to get the structure
	innerCCS, err := CompileInnerPlaceholder()
	if err != nil {
		return nil, fmt.Errorf("failed to compile inner placeholder: %w", err)
	}

	circuit := &AggregatorCircuit{
		NumProofs:      numProofs,
		InnerProofs:    make([]stdgroth16.Proof[sw_bls12377.G1Affine, sw_bls12377.G2Affine], numProofs),
		InnerWitnesses: make([]stdgroth16.Witness[sw_bls12377.ScalarField], numProofs),
	}

	// Create placeholder VK
	circuit.InnerVK = stdgroth16.PlaceholderVerifyingKey[sw_bls12377.G1Affine, sw_bls12377.G2Affine, sw_bls12377.GT](innerCCS)

	// Create placeholder proofs and witnesses
	for i := 0; i < numProofs; i++ {
		circuit.InnerProofs[i] = stdgroth16.PlaceholderProof[sw_bls12377.G1Affine, sw_bls12377.G2Affine](innerCCS)
		circuit.InnerWitnesses[i] = stdgroth16.PlaceholderWitness[sw_bls12377.ScalarField](innerCCS)
	}

	return circuit, nil
}

// AggregatorWitness holds the witness values for aggregation.
type AggregatorWitness struct {
	PrevStateRoot  *big.Int
	FinalStateRoot *big.Int
	BatchStart     uint64
	BatchEnd       uint64
	InnerProofs    []groth16.Proof
	InnerWitnesses []witness.Witness
	InnerVK        groth16.VerifyingKey
}

// ToAssignment converts an AggregatorWitness to a circuit assignment.
func (w *AggregatorWitness) ToAssignment() (*AggregatorCircuit, error) {
	numProofs := len(w.InnerProofs)
	if numProofs == 0 {
		return nil, fmt.Errorf("no inner proofs provided")
	}
	if len(w.InnerWitnesses) != numProofs {
		return nil, fmt.Errorf("witness count mismatch: %d proofs, %d witnesses", numProofs, len(w.InnerWitnesses))
	}

	circuit := &AggregatorCircuit{
		PrevStateRoot:  w.PrevStateRoot,
		FinalStateRoot: w.FinalStateRoot,
		BatchStart:     w.BatchStart,
		BatchEnd:       w.BatchEnd,
		NumProofs:      numProofs,
		InnerProofs:    make([]stdgroth16.Proof[sw_bls12377.G1Affine, sw_bls12377.G2Affine], numProofs),
		InnerWitnesses: make([]stdgroth16.Witness[sw_bls12377.ScalarField], numProofs),
	}

	// Convert verifying key
	vk, err := stdgroth16.ValueOfVerifyingKey[sw_bls12377.G1Affine, sw_bls12377.G2Affine, sw_bls12377.GT](w.InnerVK)
	if err != nil {
		return nil, fmt.Errorf("failed to convert VK: %w", err)
	}
	circuit.InnerVK = vk

	// Convert proofs and witnesses
	for i := 0; i < numProofs; i++ {
		proof, err := stdgroth16.ValueOfProof[sw_bls12377.G1Affine, sw_bls12377.G2Affine](w.InnerProofs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert proof %d: %w", i, err)
		}
		circuit.InnerProofs[i] = proof

		wit, err := stdgroth16.ValueOfWitness[sw_bls12377.ScalarField](w.InnerWitnesses[i])
		if err != nil {
			return nil, fmt.Errorf("failed to convert witness %d: %w", i, err)
		}
		circuit.InnerWitnesses[i] = wit
	}

	return circuit, nil
}

// RegisterAggregatorCircuit registers the aggregator circuit with a CurveProver.
// The prover must be configured for BW6-761.
func RegisterAggregatorCircuit(prover *CurveProver, numProofs int) error {
	if prover.CurveID() != ecc.BW6_761 {
		return fmt.Errorf("aggregator circuit requires BW6-761 prover, got %s", prover.Config().Name)
	}

	circuit, err := NewAggregatorCircuit(numProofs)
	if err != nil {
		return fmt.Errorf("failed to create aggregator circuit: %w", err)
	}

	name := fmt.Sprintf("aggregator%d", numProofs)
	return prover.RegisterCircuit(name, circuit)
}

// AggregatedBatchProof represents the output of proof aggregation.
type AggregatedBatchProof struct {
	// Final proof (BW6-761) ready for wrapper or direct verification
	Proof         groth16.Proof
	VerifyingKey  groth16.VerifyingKey
	PublicInputs  []*big.Int
	PrevStateRoot [32]byte
	NewStateRoot  [32]byte
	BatchStart    uint64
	BatchEnd      uint64
	NumBatches    int
}

// GetNativeProverOptions returns the prover options needed for generating
// inner proofs that will be verified in an outer circuit.
func GetNativeProverOptions() backend.ProverOption {
	return stdgroth16.GetNativeProverOptions(ecc.BW6_761.ScalarField(), ecc.BLS12_377.ScalarField())
}

// GetNativeVerifierOptions returns the verifier options for verifying
// proofs that were generated with GetNativeProverOptions.
func GetNativeVerifierOptions() backend.VerifierOption {
	return stdgroth16.GetNativeVerifierOptions(ecc.BW6_761.ScalarField(), ecc.BLS12_377.ScalarField())
}
