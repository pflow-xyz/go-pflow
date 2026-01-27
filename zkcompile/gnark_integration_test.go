package zkcompile

import (
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/std/hash/mimc"
)

// mimcHash computes MiMC hash of two inputs in a gnark circuit
func mimcHash(api frontend.API, left, right frontend.Variable) frontend.Variable {
	h, _ := mimc.NewMiMC(api)
	h.Write(left)
	h.Write(right)
	return h.Sum()
}

// SimpleTransferCircuit is a manually-written circuit that matches
// what our codegen produces for: balances[from] >= amount
// This tests that our constraint structure is correct.
type SimpleTransferCircuit struct {
	// Public inputs
	PreStateRoot frontend.Variable `gnark:",public"`
	Amount       frontend.Variable `gnark:",public"`
	From         frontend.Variable `gnark:",public"`

	// Private inputs (state value proven via Merkle proof)
	BalancesFrom frontend.Variable

	// Merkle proof (simplified: 3 levels for testing)
	PathElement0 frontend.Variable
	PathIndex0   frontend.Variable
	PathElement1 frontend.Variable
	PathIndex1   frontend.Variable
	PathElement2 frontend.Variable
	PathIndex2   frontend.Variable
}

func (c *SimpleTransferCircuit) Define(api frontend.API) error {
	// Guard: balances[from] >= amount
	// Constraint: balances_from - amount >= 0
	diff := api.Sub(c.BalancesFrom, c.Amount)
	api.AssertIsLessOrEqual(diff, big.NewInt(1<<62)) // range check (positive)

	// Merkle proof verification using MiMC
	// leaf = MiMC(from, balancesFrom)
	leaf := mimcHash(api, c.From, c.BalancesFrom)

	// Level 0
	api.AssertIsBoolean(c.PathIndex0)
	left0 := api.Select(c.PathIndex0, c.PathElement0, leaf)
	right0 := api.Select(c.PathIndex0, leaf, c.PathElement0)
	h0 := mimcHash(api, left0, right0)

	// Level 1
	api.AssertIsBoolean(c.PathIndex1)
	left1 := api.Select(c.PathIndex1, c.PathElement1, h0)
	right1 := api.Select(c.PathIndex1, h0, c.PathElement1)
	h1 := mimcHash(api, left1, right1)

	// Level 2
	api.AssertIsBoolean(c.PathIndex2)
	left2 := api.Select(c.PathIndex2, c.PathElement2, h1)
	right2 := api.Select(c.PathIndex2, h1, c.PathElement2)
	computedRoot := mimcHash(api, left2, right2)

	// Root check
	api.AssertIsEqual(computedRoot, c.PreStateRoot)

	return nil
}

func TestGnarkIntegration_SimpleCircuit(t *testing.T) {
	// This tests that gnark can compile a simple circuit
	var circuit SimpleTransferCircuit

	// Compile the circuit
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		t.Fatalf("circuit compilation failed: %v", err)
	}

	t.Logf("Circuit compiled successfully!")
	t.Logf("  Constraints: %d", cs.GetNbConstraints())
	t.Logf("  Public inputs: %d", cs.GetNbPublicVariables())
	t.Logf("  Private inputs: %d", cs.GetNbSecretVariables())
}

// MinimalCircuit tests the absolute basics
type MinimalCircuit struct {
	X frontend.Variable `gnark:",public"`
	Y frontend.Variable
}

func (c *MinimalCircuit) Define(api frontend.API) error {
	// Simple constraint: X == Y * Y (Y is square root of X)
	api.AssertIsEqual(c.X, api.Mul(c.Y, c.Y))
	return nil
}

func TestGnarkIntegration_MinimalProof(t *testing.T) {
	var circuit MinimalCircuit

	// Compile
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	t.Logf("Compiled: %d constraints", cs.GetNbConstraints())

	// Setup
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Logf("Setup complete")

	// Valid witness: X=9, Y=3 (3*3=9)
	assignment := &MinimalCircuit{
		X: 9,
		Y: 3,
	}

	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("witness failed: %v", err)
	}

	// Prove
	proof, err := groth16.Prove(cs, pk, witness)
	if err != nil {
		t.Fatalf("prove failed: %v", err)
	}
	t.Logf("Proof generated")

	// Verify
	publicWitness, _ := witness.Public()
	err = groth16.Verify(proof, vk, publicWitness)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	t.Logf("Proof VERIFIED!")
}

// BalanceCheckCircuit tests the core guard logic without Merkle proofs
type BalanceCheckCircuit struct {
	// Public
	Amount frontend.Variable `gnark:",public"`

	// Private
	Balance frontend.Variable
}

func (c *BalanceCheckCircuit) Define(api frontend.API) error {
	// Guard: balance >= amount
	diff := api.Sub(c.Balance, c.Amount)
	// Prove diff is non-negative by showing it fits in 64 bits
	api.ToBinary(diff, 64)
	return nil
}

func TestGnarkIntegration_BalanceCheck(t *testing.T) {
	var circuit BalanceCheckCircuit

	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	t.Logf("Balance check circuit: %d constraints", cs.GetNbConstraints())

	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Valid: balance=1000, amount=100
	assignment := &BalanceCheckCircuit{
		Amount:  100,
		Balance: 1000,
	}

	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("witness failed: %v", err)
	}

	proof, err := groth16.Prove(cs, pk, witness)
	if err != nil {
		t.Fatalf("prove failed: %v", err)
	}

	publicWitness, _ := witness.Public()
	err = groth16.Verify(proof, vk, publicWitness)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	t.Logf("Balance check proof VERIFIED! (balance=%d >= amount=%d)", 1000, 100)
}

func TestGnarkIntegration_BalanceCheckFails(t *testing.T) {
	var circuit BalanceCheckCircuit

	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	pk, _, err := groth16.Setup(cs)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Invalid: balance=50, amount=100 (insufficient balance)
	assignment := &BalanceCheckCircuit{
		Amount:  100,
		Balance: 50,
	}

	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatalf("witness failed: %v", err)
	}

	// This should fail - can't prove negative number fits in 64 bits
	_, err = groth16.Prove(cs, pk, witness)
	if err != nil {
		t.Logf("Proof correctly FAILED for insufficient balance: %v", err)
		return
	}

	t.Error("Expected proof to fail for insufficient balance, but it succeeded")
}

// MiMCHashCircuit tests MiMC hashing
type MiMCHashCircuit struct {
	Left  frontend.Variable `gnark:",public"`
	Right frontend.Variable `gnark:",public"`
	Hash  frontend.Variable `gnark:",public"`
}

func (c *MiMCHashCircuit) Define(api frontend.API) error {
	computed := mimcHash(api, c.Left, c.Right)
	api.AssertIsEqual(c.Hash, computed)
	return nil
}

func TestGnarkIntegration_MiMCHash(t *testing.T) {
	var circuit MiMCHashCircuit

	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	t.Logf("MiMC circuit: %d constraints", cs.GetNbConstraints())

	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Logf("MiMC hash circuit compiled successfully")
	t.Logf("  Proving key ready: %v", pk != nil)
	t.Logf("  Verification key ready: %v", vk != nil)
}

// TransferFromCircuit tests the full ERC-20 transferFrom guard
// balances[from] >= amount && allowances[from][caller] >= amount
type TransferFromCircuit struct {
	// Public inputs
	PreStateRoot frontend.Variable `gnark:",public"`
	Amount       frontend.Variable `gnark:",public"`
	From         frontend.Variable `gnark:",public"`
	Caller       frontend.Variable `gnark:",public"`

	// Private: state values
	BalancesFrom         frontend.Variable
	AllowancesFromCaller frontend.Variable

	// Merkle proofs (simplified: 2 levels each)
	BalancePath0 frontend.Variable
	BalanceIdx0  frontend.Variable
	BalancePath1 frontend.Variable
	BalanceIdx1  frontend.Variable

	AllowancePath0 frontend.Variable
	AllowanceIdx0  frontend.Variable
	AllowancePath1 frontend.Variable
	AllowanceIdx1  frontend.Variable

	// Intermediate root for two-tree structure
	BalanceSubRoot   frontend.Variable
	AllowanceSubRoot frontend.Variable
}

func (c *TransferFromCircuit) Define(api frontend.API) error {
	// Guard 1: balances[from] >= amount
	diff1 := api.Sub(c.BalancesFrom, c.Amount)
	api.ToBinary(diff1, 64)

	// Guard 2: allowances[from][caller] >= amount
	diff2 := api.Sub(c.AllowancesFromCaller, c.Amount)
	api.ToBinary(diff2, 64)

	// Merkle proof for balances[from]
	balanceLeaf := mimcHash(api, c.From, c.BalancesFrom)

	api.AssertIsBoolean(c.BalanceIdx0)
	left0 := api.Select(c.BalanceIdx0, c.BalancePath0, balanceLeaf)
	right0 := api.Select(c.BalanceIdx0, balanceLeaf, c.BalancePath0)
	h0 := mimcHash(api, left0, right0)

	api.AssertIsBoolean(c.BalanceIdx1)
	left1 := api.Select(c.BalanceIdx1, c.BalancePath1, h0)
	right1 := api.Select(c.BalanceIdx1, h0, c.BalancePath1)
	balanceRoot := mimcHash(api, left1, right1)

	api.AssertIsEqual(balanceRoot, c.BalanceSubRoot)

	// Merkle proof for allowances[from][caller]
	// Key is hash of (from, caller)
	allowanceKey := mimcHash(api, c.From, c.Caller)
	allowanceLeaf := mimcHash(api, allowanceKey, c.AllowancesFromCaller)

	api.AssertIsBoolean(c.AllowanceIdx0)
	aLeft0 := api.Select(c.AllowanceIdx0, c.AllowancePath0, allowanceLeaf)
	aRight0 := api.Select(c.AllowanceIdx0, allowanceLeaf, c.AllowancePath0)
	aH0 := mimcHash(api, aLeft0, aRight0)

	api.AssertIsBoolean(c.AllowanceIdx1)
	aLeft1 := api.Select(c.AllowanceIdx1, c.AllowancePath1, aH0)
	aRight1 := api.Select(c.AllowanceIdx1, aH0, c.AllowancePath1)
	allowanceRoot := mimcHash(api, aLeft1, aRight1)

	api.AssertIsEqual(allowanceRoot, c.AllowanceSubRoot)

	// Final state root combines both sub-roots
	computedRoot := mimcHash(api, c.BalanceSubRoot, c.AllowanceSubRoot)
	api.AssertIsEqual(computedRoot, c.PreStateRoot)

	return nil
}

func TestGnarkIntegration_TransferFromCircuit(t *testing.T) {
	var circuit TransferFromCircuit

	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	t.Logf("=== TransferFrom Circuit ===")
	t.Logf("  Constraints: %d", cs.GetNbConstraints())
	t.Logf("  Public inputs: %d", cs.GetNbPublicVariables())
	t.Logf("  Private inputs: %d", cs.GetNbSecretVariables())

	// Setup
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Logf("  Setup complete")
	t.Logf("  Proving key G1 points: %d", pk.NbG1())

	_ = vk // verification key ready for export to Solidity
}
