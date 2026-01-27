package zkcompile

import (
	"fmt"
)

// MerkleProofDepth is the maximum tree depth supported.
// 20 levels supports 2^20 (~1M) leaves.
const MerkleProofDepth = 20

// MerkleProofCompiler generates constraints for Merkle proof verification.
// Uses Poseidon hash which is ZK-friendly (much cheaper than keccak256 in circuits).
type MerkleProofCompiler struct {
	witnesses   *WitnessTable
	constraints []*Constraint
}

// NewMerkleProofCompiler creates a new Merkle proof compiler.
func NewMerkleProofCompiler(witnesses *WitnessTable) *MerkleProofCompiler {
	return &MerkleProofCompiler{
		witnesses:   witnesses,
		constraints: make([]*Constraint, 0),
	}
}

// MerkleProof represents a proof path for a single state access.
type MerkleProof struct {
	// Leaf data
	Key   string // Key being proven (e.g., "alice" for balances[alice])
	Value string // Witness name holding the value

	// Path witnesses (sibling hashes at each level)
	PathElements []string // Witness names for sibling hashes
	PathIndices  []string // Witness names for path direction (0=left, 1=right)

	// Root
	Root string // Witness name for expected state root
}

// CompileProof generates constraints to verify a Merkle proof.
//
// The verification logic:
//   1. Compute leaf hash: leaf = Poseidon(key, value)
//   2. Walk up the tree: for each level i,
//      - if pathIndex[i] == 0: hash = Poseidon(current, sibling)
//      - if pathIndex[i] == 1: hash = Poseidon(sibling, current)
//   3. Final hash must equal the committed root
//
// In ZK, we use arithmetic to select left/right without branching:
//   hash = Poseidon(
//       pathIndex * sibling + (1 - pathIndex) * current,
//       pathIndex * current + (1 - pathIndex) * sibling
//   )
func (c *MerkleProofCompiler) CompileProof(proof *MerkleProof) []*Constraint {
	var constraints []*Constraint

	// Step 1: Compute leaf hash
	// leaf = Poseidon(key, value)
	leafHash := c.witnesses.AddComputed("merkle_leaf")
	constraints = append(constraints,
		PoseidonConstraint(
			VarExpr(leafHash.Name),
			VarExpr(proof.Key),
			VarExpr(proof.Value),
			fmt.Sprintf("leaf = Poseidon(%s, %s)", proof.Key, proof.Value),
		),
	)

	// Step 2: Walk up the tree
	currentHash := leafHash.Name
	for i := 0; i < len(proof.PathElements); i++ {
		pathIdx := proof.PathIndices[i]
		sibling := proof.PathElements[i]
		nextHash := c.witnesses.AddComputed(fmt.Sprintf("merkle_h%d", i))

		// Ensure pathIndex is boolean (0 or 1)
		constraints = append(constraints,
			BooleanConstraint(VarExpr(pathIdx), fmt.Sprintf("pathIndex[%d] is boolean", i)),
		)

		// Compute left and right inputs based on path index
		// left  = pathIndex * sibling + (1 - pathIndex) * current
		// right = pathIndex * current + (1 - pathIndex) * sibling
		leftWitness := c.witnesses.AddComputed(fmt.Sprintf("merkle_left%d", i))
		rightWitness := c.witnesses.AddComputed(fmt.Sprintf("merkle_right%d", i))

		// left = pathIndex * sibling + (1 - pathIndex) * current
		// left = pathIndex * (sibling - current) + current
		constraints = append(constraints,
			EqualConstraint(
				VarExpr(leftWitness.Name),
				AddExpr(
					MulExpr(VarExpr(pathIdx), SubExpr(VarExpr(sibling), VarExpr(currentHash))),
					VarExpr(currentHash),
				),
				fmt.Sprintf("left[%d] = select(pathIdx, sibling, current)", i),
			),
		)

		// right = pathIndex * current + (1 - pathIndex) * sibling
		// right = pathIndex * (current - sibling) + sibling
		constraints = append(constraints,
			EqualConstraint(
				VarExpr(rightWitness.Name),
				AddExpr(
					MulExpr(VarExpr(pathIdx), SubExpr(VarExpr(currentHash), VarExpr(sibling))),
					VarExpr(sibling),
				),
				fmt.Sprintf("right[%d] = select(pathIdx, current, sibling)", i),
			),
		)

		// Hash the pair
		constraints = append(constraints,
			PoseidonConstraint(
				VarExpr(nextHash.Name),
				VarExpr(leftWitness.Name),
				VarExpr(rightWitness.Name),
				fmt.Sprintf("h[%d] = Poseidon(left, right)", i),
			),
		)

		currentHash = nextHash.Name
	}

	// Step 3: Computed root must equal committed root
	constraints = append(constraints,
		EqualConstraint(
			VarExpr(currentHash),
			VarExpr(proof.Root),
			"computed root == committed root",
		),
	)

	c.constraints = append(c.constraints, constraints...)
	return constraints
}

// CompileStateAccess generates Merkle proof constraints for a state access.
// This is the main entry point - given a StateAccess from the guard compiler,
// generate all the constraints needed to verify it.
func (c *MerkleProofCompiler) CompileStateAccess(access *StateAccess, stateRoot string) (*MerkleProof, []*Constraint) {
	var constraints []*Constraint

	// Create witnesses for the proof path
	pathElements := make([]string, MerkleProofDepth)
	pathIndices := make([]string, MerkleProofDepth)

	for i := 0; i < MerkleProofDepth; i++ {
		pathElements[i] = c.witnesses.AddComputed(
			fmt.Sprintf("%s_path_%d", access.WitnessName, i)).Name
		pathIndices[i] = c.witnesses.AddComputed(
			fmt.Sprintf("%s_idx_%d", access.WitnessName, i)).Name
	}

	// For nested maps (e.g., allowances[owner][spender]), we need to hash keys
	var keyWitness string
	if access.IsNested && len(access.KeyBindings) >= 2 {
		// key = Poseidon(key1, key2)
		compositeKey := c.witnesses.AddComputed(fmt.Sprintf("%s_composite_key", access.WitnessName))
		key1 := c.witnesses.AddBinding(access.KeyBindings[0])
		key2 := c.witnesses.AddBinding(access.KeyBindings[1])

		compositeConstraint := PoseidonConstraint(
			VarExpr(compositeKey.Name),
			VarExpr(key1.Name),
			VarExpr(key2.Name),
			fmt.Sprintf("compositeKey = Poseidon(%s, %s)", access.KeyBindings[0], access.KeyBindings[1]),
		)
		constraints = append(constraints, compositeConstraint)
		c.constraints = append(c.constraints, compositeConstraint)
		keyWitness = compositeKey.Name
	} else if len(access.KeyBindings) > 0 {
		keyWitness = c.witnesses.AddBinding(access.KeyBindings[0]).Name
	} else {
		// Scalar state (e.g., totalSupply) - use place ID as key
		keyWitness = c.witnesses.AddConstant(access.PlaceID).Name
	}

	proof := &MerkleProof{
		Key:          keyWitness,
		Value:        access.WitnessName,
		PathElements: pathElements,
		PathIndices:  pathIndices,
		Root:         stateRoot,
	}

	proofConstraints := c.CompileProof(proof)
	constraints = append(constraints, proofConstraints...)
	return proof, constraints
}

// CompileAllStateAccesses generates Merkle proof constraints for all state accesses
// from a guard compilation result.
func (c *MerkleProofCompiler) CompileAllStateAccesses(
	stateReads []*StateAccess,
	preStateRoot string,
) ([]*MerkleProof, []*Constraint) {
	var allProofs []*MerkleProof
	var allConstraints []*Constraint

	for _, access := range stateReads {
		proof, constraints := c.CompileStateAccess(access, preStateRoot)
		allProofs = append(allProofs, proof)
		allConstraints = append(allConstraints, constraints...)
	}

	return allProofs, allConstraints
}

// Constraints returns all generated constraints.
func (c *MerkleProofCompiler) Constraints() []*Constraint {
	return c.constraints
}

// PoseidonConstraint creates a constraint for Poseidon hash.
// output = Poseidon(left, right)
//
// Note: This is a placeholder - actual Poseidon implementation requires
// the full permutation constraints. In gnark, this maps to:
//   hash.Poseidon(api, left, right)
func PoseidonConstraint(output, left, right *Expr, tag string) *Constraint {
	return &Constraint{
		Type:  Poseidon,
		Left:  left,
		Right: right,
		Out:   output,
		Tag:   tag,
	}
}
