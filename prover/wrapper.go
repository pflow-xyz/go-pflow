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
	"github.com/consensys/gnark/std/algebra/emulated/sw_bw6761"
	stdgroth16 "github.com/consensys/gnark/std/recursion/groth16"
)

// WrapperCircuit verifies a BW6-761 aggregation proof in BN254.
// This is the final layer that produces an Ethereum-compatible proof.
//
// BN254 has precompile support on Ethereum (EIP-196/197), making
// on-chain verification gas-efficient (~250k gas).
//
// Public inputs (passed through to L1):
//   - PrevStateRoot: State root before the first batch in the aggregation
//   - FinalStateRoot: State root after the last batch
//   - BatchStart: First batch number covered
//   - BatchEnd: Last batch number covered (inclusive)
//
// Private inputs:
//   - AggregationProof: The BW6-761 aggregation proof to verify
//   - AggregationWitness: Public witness from the aggregation circuit
//   - AggregationVK: Verifying key for the aggregation circuit
type WrapperCircuit struct {
	// Public inputs (passed to L1 contract)
	PrevStateRoot  frontend.Variable `gnark:",public"`
	FinalStateRoot frontend.Variable `gnark:",public"`
	BatchStart     frontend.Variable `gnark:",public"`
	BatchEnd       frontend.Variable `gnark:",public"`

	// Aggregation proof and witness (BW6-761)
	AggregationProof   stdgroth16.Proof[sw_bw6761.G1Affine, sw_bw6761.G2Affine]
	AggregationWitness stdgroth16.Witness[sw_bw6761.ScalarField]
	AggregationVK      stdgroth16.VerifyingKey[sw_bw6761.G1Affine, sw_bw6761.G2Affine, sw_bw6761.GTEl]
}

// Define implements the wrapper circuit constraints.
func (c *WrapperCircuit) Define(api frontend.API) error {
	// Create recursive verifier for BW6-761 proofs
	verifier, err := stdgroth16.NewVerifier[sw_bw6761.ScalarField, sw_bw6761.G1Affine, sw_bw6761.G2Affine, sw_bw6761.GTEl](api)
	if err != nil {
		return fmt.Errorf("failed to create verifier: %w", err)
	}

	// Verify the aggregation proof
	if err := verifier.AssertProof(c.AggregationVK, c.AggregationProof, c.AggregationWitness, stdgroth16.WithCompleteArithmetic()); err != nil {
		return fmt.Errorf("failed to verify aggregation proof: %w", err)
	}

	// Verify public inputs match
	// Aggregation circuit public inputs: [prevStateRoot, finalStateRoot, batchStart, batchEnd]
	api.AssertIsEqual(c.AggregationWitness.Public[0].Limbs[0], c.PrevStateRoot)
	api.AssertIsEqual(c.AggregationWitness.Public[1].Limbs[0], c.FinalStateRoot)
	api.AssertIsEqual(c.AggregationWitness.Public[2].Limbs[0], c.BatchStart)
	api.AssertIsEqual(c.AggregationWitness.Public[3].Limbs[0], c.BatchEnd)

	return nil
}

// AggregationPlaceholderCircuit is a minimal representation of the aggregation circuit
// for placeholder generation.
type AggregationPlaceholderCircuit struct {
	PrevStateRoot  frontend.Variable `gnark:",public"`
	FinalStateRoot frontend.Variable `gnark:",public"`
	BatchStart     frontend.Variable `gnark:",public"`
	BatchEnd       frontend.Variable `gnark:",public"`
	Dummy          frontend.Variable
}

func (c *AggregationPlaceholderCircuit) Define(api frontend.API) error {
	api.AssertIsDifferent(c.Dummy, 0)
	return nil
}

// CompileAggregationPlaceholder compiles a minimal aggregation circuit to get the structure
// needed for placeholder VK, proof, and witness generation.
func CompileAggregationPlaceholder() (constraint.ConstraintSystem, error) {
	circuit := &AggregationPlaceholderCircuit{}
	return frontend.Compile(ecc.BW6_761.ScalarField(), r1cs.NewBuilder, circuit)
}

// NewWrapperCircuit creates a new wrapper circuit with placeholder values.
// Used for circuit compilation (trusted setup).
func NewWrapperCircuit() (*WrapperCircuit, error) {
	// Compile a minimal aggregation circuit to get the structure
	aggCCS, err := CompileAggregationPlaceholder()
	if err != nil {
		return nil, fmt.Errorf("failed to compile aggregation placeholder: %w", err)
	}

	circuit := &WrapperCircuit{}

	// Create placeholder VK, proof, and witness
	circuit.AggregationVK = stdgroth16.PlaceholderVerifyingKey[sw_bw6761.G1Affine, sw_bw6761.G2Affine, sw_bw6761.GTEl](aggCCS)
	circuit.AggregationProof = stdgroth16.PlaceholderProof[sw_bw6761.G1Affine, sw_bw6761.G2Affine](aggCCS)
	circuit.AggregationWitness = stdgroth16.PlaceholderWitness[sw_bw6761.ScalarField](aggCCS)

	return circuit, nil
}

// WrapperWitness holds the witness values for the wrapper circuit.
type WrapperWitness struct {
	PrevStateRoot  *big.Int
	FinalStateRoot *big.Int
	BatchStart     uint64
	BatchEnd       uint64
	// The aggregation proof and its public witness
	AggregationProof   groth16.Proof
	AggregationWitness witness.Witness
	AggregationVK      groth16.VerifyingKey
}

// ToAssignment converts a WrapperWitness to a circuit assignment.
func (w *WrapperWitness) ToAssignment() (*WrapperCircuit, error) {
	circuit := &WrapperCircuit{
		PrevStateRoot:  w.PrevStateRoot,
		FinalStateRoot: w.FinalStateRoot,
		BatchStart:     w.BatchStart,
		BatchEnd:       w.BatchEnd,
	}

	// Convert verifying key
	vk, err := stdgroth16.ValueOfVerifyingKey[sw_bw6761.G1Affine, sw_bw6761.G2Affine, sw_bw6761.GTEl](w.AggregationVK)
	if err != nil {
		return nil, fmt.Errorf("failed to convert aggregation VK: %w", err)
	}
	circuit.AggregationVK = vk

	// Convert proof
	proof, err := stdgroth16.ValueOfProof[sw_bw6761.G1Affine, sw_bw6761.G2Affine](w.AggregationProof)
	if err != nil {
		return nil, fmt.Errorf("failed to convert aggregation proof: %w", err)
	}
	circuit.AggregationProof = proof

	// Convert witness
	wit, err := stdgroth16.ValueOfWitness[sw_bw6761.ScalarField](w.AggregationWitness)
	if err != nil {
		return nil, fmt.Errorf("failed to convert aggregation witness: %w", err)
	}
	circuit.AggregationWitness = wit

	return circuit, nil
}

// RegisterWrapperCircuit registers the wrapper circuit with a CurveProver.
// The prover must be configured for BN254.
func RegisterWrapperCircuit(prover *CurveProver) error {
	if prover.CurveID() != ecc.BN254 {
		return fmt.Errorf("wrapper circuit requires BN254 prover, got %s", prover.Config().Name)
	}

	circuit, err := NewWrapperCircuit()
	if err != nil {
		return fmt.Errorf("failed to create wrapper circuit: %w", err)
	}

	return prover.RegisterCircuit("wrapper", circuit)
}

// WrappedProof represents the final Ethereum-compatible proof.
type WrappedProof struct {
	// Proof points for Solidity verification (same format as ProofResult)
	A [2]*big.Int
	B [2][2]*big.Int
	C [2]*big.Int

	// Raw proof as flat array: [A.X, A.Y, B.X[0], B.X[1], B.Y[0], B.Y[1], C.X, C.Y]
	RawProof [8]*big.Int

	// Public inputs for L1 contract
	PublicInputs []string

	// Metadata
	PrevStateRoot [32]byte
	NewStateRoot  [32]byte
	BatchStart    uint64
	BatchEnd      uint64
	NumBatches    int
}

// GetWrapperProverOptions returns the prover options needed for generating
// wrapper proofs that verify aggregation proofs.
func GetWrapperProverOptions() backend.ProverOption {
	return stdgroth16.GetNativeProverOptions(ecc.BN254.ScalarField(), ecc.BW6_761.ScalarField())
}

// GetWrapperVerifierOptions returns the verifier options for verifying
// wrapper proofs.
func GetWrapperVerifierOptions() backend.VerifierOption {
	return stdgroth16.GetNativeVerifierOptions(ecc.BN254.ScalarField(), ecc.BW6_761.ScalarField())
}
