package zkcompile

import (
	"bytes"
	"fmt"
	"io"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// SolidityExporter generates Solidity verifier contracts from gnark circuits.
type SolidityExporter struct {
	curve ecc.ID
}

// NewSolidityExporter creates a new Solidity exporter for the BN254 curve.
// BN254 is the standard curve for Ethereum (also called alt_bn128).
func NewSolidityExporter() *SolidityExporter {
	return &SolidityExporter{
		curve: ecc.BN254,
	}
}

// ExportVerifier compiles a circuit and exports the Groth16 verifier to Solidity.
// Returns the verifier contract source code.
func (e *SolidityExporter) ExportVerifier(circuit frontend.Circuit) (string, error) {
	// Compile the circuit
	cs, err := frontend.Compile(e.curve.ScalarField(), r1cs.NewBuilder, circuit)
	if err != nil {
		return "", fmt.Errorf("circuit compilation failed: %w", err)
	}

	// Setup (generates proving and verification keys)
	_, vk, err := groth16.Setup(cs)
	if err != nil {
		return "", fmt.Errorf("setup failed: %w", err)
	}

	// Export verifier to Solidity
	var buf bytes.Buffer
	err = vk.ExportSolidity(&buf)
	if err != nil {
		return "", fmt.Errorf("solidity export failed: %w", err)
	}

	return buf.String(), nil
}

// ExportVerifierWithKeys compiles a circuit and returns both the Solidity verifier
// and the proving/verification keys for later use.
func (e *SolidityExporter) ExportVerifierWithKeys(circuit frontend.Circuit) (
	solidityCode string,
	pk groth16.ProvingKey,
	vk groth16.VerifyingKey,
	err error,
) {
	// Compile the circuit
	cs, err := frontend.Compile(e.curve.ScalarField(), r1cs.NewBuilder, circuit)
	if err != nil {
		return "", nil, nil, fmt.Errorf("circuit compilation failed: %w", err)
	}

	// Setup
	pk, vk, err = groth16.Setup(cs)
	if err != nil {
		return "", nil, nil, fmt.Errorf("setup failed: %w", err)
	}

	// Export verifier
	var buf bytes.Buffer
	err = vk.ExportSolidity(&buf)
	if err != nil {
		return "", nil, nil, fmt.Errorf("solidity export failed: %w", err)
	}

	return buf.String(), pk, vk, nil
}

// ExportVerifierToWriter exports the Solidity verifier to the provided writer.
func (e *SolidityExporter) ExportVerifierToWriter(circuit frontend.Circuit, w io.Writer) error {
	// Compile the circuit
	cs, err := frontend.Compile(e.curve.ScalarField(), r1cs.NewBuilder, circuit)
	if err != nil {
		return fmt.Errorf("circuit compilation failed: %w", err)
	}

	// Setup
	_, vk, err := groth16.Setup(cs)
	if err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	// Export verifier
	return vk.ExportSolidity(w)
}

// GenerateZKWrapper generates a generic ZK-verified state transition wrapper contract.
// The contractName parameter sets the contract name in the generated Solidity code.
func GenerateZKWrapper(contractName string) string {
	return fmt.Sprintf(`// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./Verifier.sol";

/// @title %sZK
/// @notice ZK-verified state transitions
/// @dev Wraps the Groth16 verifier for state transition proofs
contract %sZK {
    // The Groth16 verifier contract
    Verifier public immutable verifier;

    // Current state root (Merkle root of all state)
    bytes32 public stateRoot;

    // Epoch counter for ordering
    uint256 public epoch;

    // Events
    event StateTransition(
        bytes32 indexed oldRoot,
        bytes32 indexed newRoot,
        uint256 indexed epoch,
        bytes32 actionHash
    );

    event ProofVerified(
        address indexed caller,
        bytes32 indexed actionHash,
        uint256 gasUsed
    );

    constructor(address _verifier, bytes32 _initialRoot) {
        verifier = Verifier(_verifier);
        stateRoot = _initialRoot;
        epoch = 0;
    }

    /// @notice Execute a state transition with ZK proof
    /// @param proof The Groth16 proof (a, b, c points)
    /// @param publicInputs The public inputs to the circuit
    /// @param newStateRoot The new state root after transition
    /// @param actionData Encoded action data for logging
    function verifyAndExecute(
        uint256[8] calldata proof,
        uint256[] calldata publicInputs,
        bytes32 newStateRoot,
        bytes calldata actionData
    ) external {
        uint256 startGas = gasleft();

        // Verify the proof includes correct state roots
        require(publicInputs.length >= 2, "Invalid public inputs");
        require(bytes32(publicInputs[0]) == stateRoot, "Pre-state root mismatch");
        require(bytes32(publicInputs[1]) == newStateRoot, "Post-state root mismatch");

        // Verify the ZK proof
        bool valid = verifier.verifyProof(
            [proof[0], proof[1]],           // a
            [[proof[2], proof[3]], [proof[4], proof[5]]], // b
            [proof[6], proof[7]],           // c
            publicInputs
        );
        require(valid, "Invalid proof");

        // Update state
        bytes32 oldRoot = stateRoot;
        stateRoot = newStateRoot;
        epoch++;

        bytes32 actionHash = keccak256(actionData);

        emit StateTransition(oldRoot, newStateRoot, epoch, actionHash);
        emit ProofVerified(msg.sender, actionHash, startGas - gasleft());
    }

    /// @notice Verify a proof without executing (for testing/validation)
    /// @param proof The Groth16 proof
    /// @param publicInputs The public inputs
    /// @return valid Whether the proof is valid
    function verifyOnly(
        uint256[8] calldata proof,
        uint256[] calldata publicInputs
    ) external view returns (bool valid) {
        return verifier.verifyProof(
            [proof[0], proof[1]],
            [[proof[2], proof[3]], [proof[4], proof[5]]],
            [proof[6], proof[7]],
            publicInputs
        );
    }

    /// @notice Get current state info
    /// @return root Current state root
    /// @return currentEpoch Current epoch number
    function getState() external view returns (bytes32 root, uint256 currentEpoch) {
        return (stateRoot, epoch);
    }
}
`, contractName, contractName)
}

// GenerateProofHelper generates a Go helper for creating proofs.
func GenerateProofHelper(packageName, circuitType string) string {
	return fmt.Sprintf(`package %s

import (
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// ProofData contains the proof and public inputs for Solidity verification
type ProofData struct {
	A            [2]string   // G1 point
	B            [2][2]string // G2 point
	C            [2]string   // G1 point
	PublicInputs []string
}

// Prover generates proofs for %s circuits
type Prover struct {
	cs frontend.CompiledConstraintSystem
	pk groth16.ProvingKey
	vk groth16.VerifyingKey
}

// NewProver creates a new prover with setup
func NewProver() (*Prover, error) {
	var circuit %s

	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return nil, err
	}

	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		return nil, err
	}

	return &Prover{cs: cs, pk: pk, vk: vk}, nil
}

// Prove generates a proof for the given witness assignment
func (p *Prover) Prove(assignment *%s) (*ProofData, error) {
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return nil, err
	}

	proof, err := groth16.Prove(p.cs, p.pk, witness)
	if err != nil {
		return nil, err
	}

	// Extract proof components for Solidity
	// Note: actual extraction requires proof serialization
	// This is a placeholder structure
	_ = proof
	return &ProofData{
		// Proof components would be extracted here
	}, nil
}

// Verify verifies a proof locally
func (p *Prover) Verify(assignment *%s) error {
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return err
	}

	proof, err := groth16.Prove(p.cs, p.pk, witness)
	if err != nil {
		return err
	}

	publicWitness, err := witness.Public()
	if err != nil {
		return err
	}

	return groth16.Verify(proof, p.vk, publicWitness)
}
`, packageName, circuitType, circuitType, circuitType, circuitType)
}
