package prover

import (
	"fmt"
	"sync"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// CurveConfig holds configuration for a specific elliptic curve.
type CurveConfig struct {
	ID        ecc.ID
	Name      string
	FieldSize int
	Role      CurveRole
}

// CurveRole identifies the purpose of a curve in the recursion stack.
type CurveRole int

const (
	// RoleInner is for inner proofs (BLS12-377).
	RoleInner CurveRole = iota
	// RoleAggregation is for aggregating inner proofs (BW6-761).
	RoleAggregation
	// RoleWrapper is for Ethereum-compatible final proof (BN254).
	RoleWrapper
)

// Standard curve configurations for recursive proof aggregation.
var (
	// BN254Config is for Ethereum-compatible proofs (wrapper/L1 submission).
	// Used as the outermost layer since Ethereum has precompiles for BN254.
	BN254Config = CurveConfig{
		ID:        ecc.BN254,
		Name:      "bn254",
		FieldSize: 254,
		Role:      RoleWrapper,
	}

	// BLS12_377Config is for inner proofs (batch proofs).
	// Chosen because BW6-761's base field equals BLS12-377's scalar field,
	// making verification "native" (no field emulation needed).
	BLS12_377Config = CurveConfig{
		ID:        ecc.BLS12_377,
		Name:      "bls12-377",
		FieldSize: 253,
		Role:      RoleInner,
	}

	// BW6_761Config is for aggregation proofs.
	// Efficiently verifies BLS12-377 proofs (native field arithmetic).
	BW6_761Config = CurveConfig{
		ID:        ecc.BW6_761,
		Name:      "bw6-761",
		FieldSize: 377,
		Role:      RoleAggregation,
	}
)

// CurveProver is a prover bound to a specific curve.
// It wraps the base Prover functionality with curve-specific operations.
type CurveProver struct {
	config   CurveConfig
	mu       sync.RWMutex
	circuits map[string]*CurveCompiledCircuit
}

// CurveCompiledCircuit holds a compiled circuit with curve-specific keys.
type CurveCompiledCircuit struct {
	Name         string
	CS           constraint.ConstraintSystem
	ProvingKey   groth16.ProvingKey
	VerifyingKey groth16.VerifyingKey
	Constraints  int
	PublicVars   int
	PrivateVars  int
	Curve        ecc.ID
}

// NewCurveProver creates a new prover for the specified curve.
func NewCurveProver(config CurveConfig) *CurveProver {
	return &CurveProver{
		config:   config,
		circuits: make(map[string]*CurveCompiledCircuit),
	}
}

// Config returns the curve configuration.
func (cp *CurveProver) Config() CurveConfig {
	return cp.config
}

// CurveID returns the curve ID.
func (cp *CurveProver) CurveID() ecc.ID {
	return cp.config.ID
}

// RegisterCircuit compiles a circuit for this curve and runs trusted setup.
func (cp *CurveProver) RegisterCircuit(name string, circuit frontend.Circuit) error {
	cc, err := cp.CompileCircuit(name, circuit)
	if err != nil {
		return err
	}
	cp.StoreCircuit(name, cc)
	return nil
}

// CompileCircuit compiles a circuit for this curve without storing it.
func (cp *CurveProver) CompileCircuit(name string, circuit frontend.Circuit) (*CurveCompiledCircuit, error) {
	// Compile to R1CS using this curve's scalar field
	cs, err := frontend.Compile(cp.config.ID.ScalarField(), r1cs.NewBuilder, circuit)
	if err != nil {
		return nil, fmt.Errorf("circuit compilation failed for %s: %w", cp.config.Name, err)
	}

	// Trusted setup (in production, use MPC ceremony or universal setup)
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		return nil, fmt.Errorf("setup failed for %s: %w", cp.config.Name, err)
	}

	return &CurveCompiledCircuit{
		Name:         name,
		CS:           cs,
		ProvingKey:   pk,
		VerifyingKey: vk,
		Constraints:  cs.GetNbConstraints(),
		PublicVars:   cs.GetNbPublicVariables(),
		PrivateVars:  cs.GetNbSecretVariables(),
		Curve:        cp.config.ID,
	}, nil
}

// StoreCircuit stores a pre-compiled circuit.
func (cp *CurveProver) StoreCircuit(name string, cc *CurveCompiledCircuit) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.circuits[name] = cc
}

// GetCircuit returns a compiled circuit by name.
func (cp *CurveProver) GetCircuit(name string) (*CurveCompiledCircuit, bool) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	cc, ok := cp.circuits[name]
	return cc, ok
}

// ListCircuits returns all registered circuit names.
func (cp *CurveProver) ListCircuits() []string {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	names := make([]string, 0, len(cp.circuits))
	for name := range cp.circuits {
		names = append(names, name)
	}
	return names
}

// Prove generates a Groth16 proof for the given circuit and witness.
func (cp *CurveProver) Prove(circuitName string, assignment frontend.Circuit) (*CurveProofResult, error) {
	cp.mu.RLock()
	cc, ok := cp.circuits[circuitName]
	cp.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("circuit %q not registered on curve %s", circuitName, cp.config.Name)
	}

	// Create witness from assignment
	fullWitness, err := frontend.NewWitness(assignment, cp.config.ID.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("witness creation failed: %w", err)
	}

	// Generate proof
	proof, err := groth16.Prove(cc.CS, cc.ProvingKey, fullWitness)
	if err != nil {
		return nil, fmt.Errorf("proof generation failed: %w", err)
	}

	// Extract public witness
	publicWitness, err := fullWitness.Public()
	if err != nil {
		return nil, fmt.Errorf("public witness extraction failed: %w", err)
	}

	return &CurveProofResult{
		Proof:         proof,
		PublicWitness: publicWitness,
		VerifyingKey:  cc.VerifyingKey,
		CircuitName:   cc.Name,
		Constraints:   cc.Constraints,
		Curve:         cp.config.ID,
	}, nil
}

// Verify verifies a proof against the stored verifying key.
func (cp *CurveProver) Verify(circuitName string, proof *CurveProofResult) error {
	cp.mu.RLock()
	cc, ok := cp.circuits[circuitName]
	cp.mu.RUnlock()

	if !ok {
		return fmt.Errorf("circuit %q not registered on curve %s", circuitName, cp.config.Name)
	}

	return groth16.Verify(proof.Proof, cc.VerifyingKey, proof.PublicWitness)
}

// CurveProofResult contains the proof and related data for a specific curve.
type CurveProofResult struct {
	Proof         groth16.Proof
	PublicWitness witness.Witness
	VerifyingKey  groth16.VerifyingKey
	CircuitName   string
	Constraints   int
	Curve         ecc.ID
}

// RecursionStack manages the three provers needed for recursive aggregation.
// Inner proofs are generated on BLS12-377, aggregated on BW6-761,
// and wrapped for Ethereum on BN254.
type RecursionStack struct {
	Inner       *CurveProver // BLS12-377 for batch proofs
	Aggregation *CurveProver // BW6-761 for aggregation
	Wrapper     *CurveProver // BN254 for Ethereum
}

// NewRecursionStack creates a new recursion stack with provers for all three curves.
func NewRecursionStack() *RecursionStack {
	return &RecursionStack{
		Inner:       NewCurveProver(BLS12_377Config),
		Aggregation: NewCurveProver(BW6_761Config),
		Wrapper:     NewCurveProver(BN254Config),
	}
}

// GetProver returns the prover for the given role.
func (rs *RecursionStack) GetProver(role CurveRole) *CurveProver {
	switch role {
	case RoleInner:
		return rs.Inner
	case RoleAggregation:
		return rs.Aggregation
	case RoleWrapper:
		return rs.Wrapper
	default:
		return nil
	}
}
