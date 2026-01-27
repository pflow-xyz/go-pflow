package zkcompile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Pipeline orchestrates the full ZK compilation flow:
// Guard Expression → Constraints → gnark Circuit → Solidity Verifier
type Pipeline struct {
	outputDir   string
	packageName string
}

// NewPipeline creates a new ZK compilation pipeline.
func NewPipeline(outputDir, packageName string) *Pipeline {
	return &Pipeline{
		outputDir:   outputDir,
		packageName: packageName,
	}
}

// PipelineResult holds the full compilation output.
type PipelineResult struct {
	// Constraint compilation
	GuardConstraints     []*Constraint
	MerkleConstraints    []*Constraint
	InvariantConstraints []*Constraint
	TotalConstraints     int

	// Witnesses
	PublicInputs  int
	PrivateInputs int

	// Generated code
	GnarkCircuitCode string
	SolidityVerifier string
	ZKWrapperCode    string
	ProofHelperCode  string

	// Statistics
	Stats *CircuitStats
}

// Compile runs the full pipeline for a guard expression.
func (p *Pipeline) Compile(guardExpr string, circuitName string) (*PipelineResult, error) {
	result := &PipelineResult{}

	// Step 1: Compile guard expression
	guardCompiler := NewGuardCompiler()
	guardResult, err := guardCompiler.Compile(guardExpr)
	if err != nil {
		return nil, fmt.Errorf("guard compilation failed: %w", err)
	}
	result.GuardConstraints = guardResult.Constraints

	// Step 2: Compile Merkle proofs for state accesses
	merkleCompiler := NewMerkleProofCompiler(guardResult.Witnesses)
	stateRoot := guardResult.Witnesses.AddBinding("preStateRoot")
	guardResult.Witnesses.AddBinding("postStateRoot")
	proofs, merkleConstraints := merkleCompiler.CompileAllStateAccesses(guardResult.StateReads, stateRoot.Name)
	result.MerkleConstraints = merkleConstraints

	// Step 3: Generate gnark circuit code
	codegen := NewGnarkCodegen(p.packageName, circuitName)
	result.GnarkCircuitCode = codegen.GenerateCircuit(guardResult, proofs, merkleConstraints, nil)

	// Step 4: Compute statistics
	result.Stats = ComputeStats(guardResult, merkleConstraints, nil)
	result.TotalConstraints = result.Stats.TotalConstraints
	result.PublicInputs = result.Stats.PublicInputCount
	result.PrivateInputs = result.Stats.PrivateInputCount

	// Step 5: Generate ZK wrapper contract
	result.ZKWrapperCode = GenerateZKWrapper(circuitName)

	// Step 6: Generate proof helper
	result.ProofHelperCode = GenerateProofHelper(p.packageName, circuitName)

	return result, nil
}

// WriteFiles writes all generated code to the output directory.
func (p *Pipeline) WriteFiles(result *PipelineResult, circuitName string) error {
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write gnark circuit
	circuitPath := filepath.Join(p.outputDir, "circuit.go")
	if err := os.WriteFile(circuitPath, []byte(result.GnarkCircuitCode), 0644); err != nil {
		return fmt.Errorf("failed to write circuit: %w", err)
	}

	// Write wrapper contract
	wrapperPath := filepath.Join(p.outputDir, fmt.Sprintf("%sZK.sol", circuitName))
	if err := os.WriteFile(wrapperPath, []byte(result.ZKWrapperCode), 0644); err != nil {
		return fmt.Errorf("failed to write wrapper: %w", err)
	}

	// Write proof helper
	helperPath := filepath.Join(p.outputDir, "prover.go")
	if err := os.WriteFile(helperPath, []byte(result.ProofHelperCode), 0644); err != nil {
		return fmt.Errorf("failed to write prover: %w", err)
	}

	return nil
}

// Summary returns a human-readable summary of the compilation.
func (result *PipelineResult) Summary() string {
	return fmt.Sprintf(`ZK Compilation Summary
======================
Guard Constraints:     %d
Merkle Constraints:    %d
Invariant Constraints: %d
Total Constraints:     %d

Witnesses:
  Public Inputs:       %d
  Private Inputs:      %d

Generated Code:
  gnark Circuit:       %d bytes
  ZK Wrapper:          %d bytes
  Proof Helper:        %d bytes

Circuit Statistics:
%s`,
		len(result.GuardConstraints),
		len(result.MerkleConstraints),
		len(result.InvariantConstraints),
		result.TotalConstraints,
		result.PublicInputs,
		result.PrivateInputs,
		len(result.GnarkCircuitCode),
		len(result.ZKWrapperCode),
		len(result.ProofHelperCode),
		result.Stats,
	)
}

// QuickCompile is a convenience function for simple compilation.
func QuickCompile(guardExpr string) (*PipelineResult, error) {
	pipeline := NewPipeline("", "circuit")
	return pipeline.Compile(guardExpr, "Circuit")
}
