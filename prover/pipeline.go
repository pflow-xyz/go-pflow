package prover

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/frontend"
)

// AggregationPipeline manages the multi-stage recursive proof generation.
// It orchestrates three provers:
//   - Inner (BLS12-377): Generates individual batch proofs
//   - Aggregation (BW6-761): Aggregates N inner proofs into one
//   - Wrapper (BN254): Wraps aggregation proof for Ethereum
type AggregationPipeline struct {
	innerProver *CurveProver // BLS12-377
	aggProver   *CurveProver // BW6-761
	wrapProver  *CurveProver // BN254

	batchSize int // Proofs per aggregation (e.g., 8)

	mu           sync.Mutex
	pendingInner []*InnerProofResult
}

// InnerProofResult contains the result of generating an inner (batch) proof.
type InnerProofResult struct {
	BatchNumber   uint64
	PrevStateRoot [32]byte
	NewStateRoot  [32]byte
	TxRoot        [32]byte
	Proof         groth16.Proof
	PublicWitness witness.Witness
}

// PipelineConfig configures the aggregation pipeline.
type PipelineConfig struct {
	// BatchSize is the number of inner proofs to aggregate (default: 8)
	BatchSize int
	// InnerCircuitName is the name of the batch circuit on BLS12-377
	InnerCircuitName string
}

// DefaultPipelineConfig returns the default pipeline configuration.
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		BatchSize:        DefaultAggregationSize,
		InnerCircuitName: "batch8",
	}
}

// NewAggregationPipeline creates a new aggregation pipeline.
func NewAggregationPipeline(config PipelineConfig) (*AggregationPipeline, error) {
	if config.BatchSize < 1 {
		config.BatchSize = DefaultAggregationSize
	}

	// Create provers for each curve
	innerProver := NewCurveProver(BLS12_377Config)
	aggProver := NewCurveProver(BW6_761Config)
	wrapProver := NewCurveProver(BN254Config)

	// Register the aggregator circuit
	if err := RegisterAggregatorCircuit(aggProver, config.BatchSize); err != nil {
		return nil, fmt.Errorf("failed to register aggregator circuit: %w", err)
	}

	// Register the wrapper circuit
	if err := RegisterWrapperCircuit(wrapProver); err != nil {
		return nil, fmt.Errorf("failed to register wrapper circuit: %w", err)
	}

	return &AggregationPipeline{
		innerProver:  innerProver,
		aggProver:    aggProver,
		wrapProver:   wrapProver,
		batchSize:    config.BatchSize,
		pendingInner: make([]*InnerProofResult, 0, config.BatchSize),
	}, nil
}

// RegisterInnerCircuit registers the batch circuit for inner proof generation.
// This must be called before ProveInner can be used.
func (p *AggregationPipeline) RegisterInnerCircuit(name string, circuit frontend.Circuit) error {
	return p.innerProver.RegisterCircuit(name, circuit)
}

// ProveInner generates a BLS12-377 proof for a single batch.
func (p *AggregationPipeline) ProveInner(ctx context.Context, circuitName string, assignment frontend.Circuit, meta *BatchMetadata) (*InnerProofResult, error) {
	// Generate proof using the inner prover with native options
	result, err := p.innerProver.Prove(circuitName, assignment)
	if err != nil {
		return nil, fmt.Errorf("inner proof generation failed: %w", err)
	}

	return &InnerProofResult{
		BatchNumber:   meta.BatchNumber,
		PrevStateRoot: meta.PrevStateRoot,
		NewStateRoot:  meta.NewStateRoot,
		TxRoot:        meta.TxRoot,
		Proof:         result.Proof,
		PublicWitness: result.PublicWitness,
	}, nil
}

// BatchMetadata contains metadata for a batch proof.
type BatchMetadata struct {
	BatchNumber   uint64
	PrevStateRoot [32]byte
	NewStateRoot  [32]byte
	TxRoot        [32]byte
}

// AddPendingInner adds an inner proof to the pending buffer.
// Returns true if the buffer is full and ready for aggregation.
func (p *AggregationPipeline) AddPendingInner(proof *InnerProofResult) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pendingInner = append(p.pendingInner, proof)
	return len(p.pendingInner) >= p.batchSize
}

// GetPendingCount returns the number of pending inner proofs.
func (p *AggregationPipeline) GetPendingCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pendingInner)
}

// DrainPending returns and clears all pending inner proofs.
func (p *AggregationPipeline) DrainPending() []*InnerProofResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	proofs := p.pendingInner
	p.pendingInner = make([]*InnerProofResult, 0, p.batchSize)
	return proofs
}

// Aggregate combines N inner proofs into one aggregated proof.
func (p *AggregationPipeline) Aggregate(ctx context.Context, innerProofs []*InnerProofResult) (*AggregatedBatchProof, error) {
	if len(innerProofs) != p.batchSize {
		return nil, fmt.Errorf("expected %d inner proofs, got %d", p.batchSize, len(innerProofs))
	}

	// Validate state root chain
	for i := 1; i < len(innerProofs); i++ {
		if innerProofs[i].PrevStateRoot != innerProofs[i-1].NewStateRoot {
			return nil, fmt.Errorf("state root chain broken at proof %d: prev=%x, expected=%x",
				i, innerProofs[i].PrevStateRoot, innerProofs[i-1].NewStateRoot)
		}
	}

	// Get the inner circuit's verifying key
	cc, ok := p.innerProver.GetCircuit("batch8")
	if !ok {
		return nil, fmt.Errorf("inner circuit not registered")
	}

	// Build aggregator witness
	aggWitness := &AggregatorWitness{
		PrevStateRoot:  new(big.Int).SetBytes(innerProofs[0].PrevStateRoot[:]),
		FinalStateRoot: new(big.Int).SetBytes(innerProofs[len(innerProofs)-1].NewStateRoot[:]),
		BatchStart:     innerProofs[0].BatchNumber,
		BatchEnd:       innerProofs[len(innerProofs)-1].BatchNumber,
		InnerProofs:    make([]groth16.Proof, len(innerProofs)),
		InnerWitnesses: make([]witness.Witness, len(innerProofs)),
		InnerVK:        cc.VerifyingKey,
	}

	for i, ip := range innerProofs {
		aggWitness.InnerProofs[i] = ip.Proof
		aggWitness.InnerWitnesses[i] = ip.PublicWitness
	}

	// Convert to circuit assignment
	assignment, err := aggWitness.ToAssignment()
	if err != nil {
		return nil, fmt.Errorf("failed to create aggregator assignment: %w", err)
	}

	// Generate aggregation proof
	circuitName := fmt.Sprintf("aggregator%d", p.batchSize)
	aggResult, err := p.aggProver.Prove(circuitName, assignment)
	if err != nil {
		return nil, fmt.Errorf("aggregation proof generation failed: %w", err)
	}

	return &AggregatedBatchProof{
		Proof:         aggResult.Proof,
		VerifyingKey:  aggResult.VerifyingKey,
		PublicInputs:  extractPublicInputs(aggResult.PublicWitness),
		PrevStateRoot: innerProofs[0].PrevStateRoot,
		NewStateRoot:  innerProofs[len(innerProofs)-1].NewStateRoot,
		BatchStart:    innerProofs[0].BatchNumber,
		BatchEnd:      innerProofs[len(innerProofs)-1].BatchNumber,
		NumBatches:    len(innerProofs),
	}, nil
}

// Wrap wraps an aggregation proof for Ethereum submission.
func (p *AggregationPipeline) Wrap(ctx context.Context, aggProof *AggregatedBatchProof) (*WrappedProof, error) {
	// Get the aggregator circuit's verifying key
	circuitName := fmt.Sprintf("aggregator%d", p.batchSize)
	cc, ok := p.aggProver.GetCircuit(circuitName)
	if !ok {
		return nil, fmt.Errorf("aggregator circuit not registered")
	}

	// Build wrapper witness
	wrapWitness := &WrapperWitness{
		PrevStateRoot:      new(big.Int).SetBytes(aggProof.PrevStateRoot[:]),
		FinalStateRoot:     new(big.Int).SetBytes(aggProof.NewStateRoot[:]),
		BatchStart:         aggProof.BatchStart,
		BatchEnd:           aggProof.BatchEnd,
		AggregationProof:   aggProof.Proof,
		AggregationWitness: publicWitnessFromInputs(aggProof.PublicInputs, ecc.BW6_761),
		AggregationVK:      cc.VerifyingKey,
	}

	// Convert to circuit assignment
	assignment, err := wrapWitness.ToAssignment()
	if err != nil {
		return nil, fmt.Errorf("failed to create wrapper assignment: %w", err)
	}

	// Generate wrapper proof
	wrapResult, err := p.wrapProver.Prove("wrapper", assignment)
	if err != nil {
		return nil, fmt.Errorf("wrapper proof generation failed: %w", err)
	}

	// Convert to Ethereum format
	rawProof := extractRawProof(wrapResult.Proof)
	publicInputs := extractPublicInputStrings(wrapResult.PublicWitness)

	return &WrappedProof{
		A:             [2]*big.Int{rawProof[0], rawProof[1]},
		B:             [2][2]*big.Int{{rawProof[2], rawProof[3]}, {rawProof[4], rawProof[5]}},
		C:             [2]*big.Int{rawProof[6], rawProof[7]},
		RawProof:      rawProof,
		PublicInputs:  publicInputs,
		PrevStateRoot: aggProof.PrevStateRoot,
		NewStateRoot:  aggProof.NewStateRoot,
		BatchStart:    aggProof.BatchStart,
		BatchEnd:      aggProof.BatchEnd,
		NumBatches:    aggProof.NumBatches,
	}, nil
}

// FullAggregate performs the complete aggregation pipeline:
// 1. Verifies inner proofs form a valid chain
// 2. Aggregates inner proofs into one proof
// 3. Wraps for Ethereum submission
func (p *AggregationPipeline) FullAggregate(ctx context.Context, innerProofs []*InnerProofResult) (*WrappedProof, error) {
	// Step 1: Aggregate
	aggProof, err := p.Aggregate(ctx, innerProofs)
	if err != nil {
		return nil, fmt.Errorf("aggregation failed: %w", err)
	}

	// Step 2: Wrap for Ethereum
	wrapped, err := p.Wrap(ctx, aggProof)
	if err != nil {
		return nil, fmt.Errorf("wrapping failed: %w", err)
	}

	return wrapped, nil
}

// extractPublicInputs extracts public inputs from a witness.
func extractPublicInputs(w witness.Witness) []*big.Int {
	if w == nil {
		return nil
	}

	pubBytes, err := w.MarshalBinary()
	if err != nil {
		return nil
	}

	const headerSize = 12
	const elementSize = 32

	if len(pubBytes) < headerSize {
		return nil
	}

	data := pubBytes[headerSize:]
	numElements := len(data) / elementSize
	inputs := make([]*big.Int, numElements)

	for i := 0; i < numElements; i++ {
		start := i * elementSize
		end := start + elementSize
		if end <= len(data) {
			inputs[i] = new(big.Int).SetBytes(data[start:end])
		}
	}

	return inputs
}

// extractPublicInputStrings extracts public inputs as hex strings.
func extractPublicInputStrings(w witness.Witness) []string {
	inputs := extractPublicInputs(w)
	strs := make([]string, len(inputs))
	for i, input := range inputs {
		if input != nil {
			strs[i] = fmt.Sprintf("0x%064x", input)
		}
	}
	return strs
}

// extractRawProof extracts the 8 proof elements from a groth16 proof.
func extractRawProof(proof groth16.Proof) [8]*big.Int {
	result := [8]*big.Int{}
	for i := range result {
		result[i] = big.NewInt(0)
	}

	var buf bytes.Buffer
	if _, err := proof.WriteTo(&buf); err != nil {
		return result
	}
	proofBytes := buf.Bytes()

	// Uncompressed format: A (64 bytes) + B (128 bytes) + C (64 bytes) = 256 bytes
	if len(proofBytes) >= 256 {
		result[0] = new(big.Int).SetBytes(proofBytes[0:32])   // A.X
		result[1] = new(big.Int).SetBytes(proofBytes[32:64])  // A.Y
		result[2] = new(big.Int).SetBytes(proofBytes[64:96])  // B.X[0]
		result[3] = new(big.Int).SetBytes(proofBytes[96:128]) // B.X[1]
		result[4] = new(big.Int).SetBytes(proofBytes[128:160]) // B.Y[0]
		result[5] = new(big.Int).SetBytes(proofBytes[160:192]) // B.Y[1]
		result[6] = new(big.Int).SetBytes(proofBytes[192:224]) // C.X
		result[7] = new(big.Int).SetBytes(proofBytes[224:256]) // C.Y
	}

	return result
}

// publicWitnessFromInputs creates a witness from public input values.
// This is used to reconstruct a witness for verification.
func publicWitnessFromInputs(inputs []*big.Int, curve ecc.ID) witness.Witness {
	// Create a minimal witness with the public inputs
	// The witness format is: header (12 bytes) + elements (32 bytes each)
	const headerSize = 12
	const elementSize = 32

	numPublic := len(inputs)
	data := make([]byte, headerSize+numPublic*elementSize)

	// Write header
	// bytes 0-3: curve ID (little endian)
	curveID := uint32(curve)
	data[0] = byte(curveID)
	data[1] = byte(curveID >> 8)
	data[2] = byte(curveID >> 16)
	data[3] = byte(curveID >> 24)
	// bytes 4-7: number of public inputs
	data[4] = byte(numPublic)
	data[5] = byte(numPublic >> 8)
	data[6] = byte(numPublic >> 16)
	data[7] = byte(numPublic >> 24)
	// bytes 8-11: number of secret inputs (0)
	data[8] = 0
	data[9] = 0
	data[10] = 0
	data[11] = 0

	// Write public inputs
	for i, input := range inputs {
		if input != nil {
			inputBytes := input.Bytes()
			// Pad to 32 bytes
			offset := headerSize + i*elementSize + (elementSize - len(inputBytes))
			copy(data[offset:], inputBytes)
		}
	}

	// Create witness from bytes
	w, err := witness.New(curve.ScalarField())
	if err != nil {
		return nil
	}
	if err := w.UnmarshalBinary(data); err != nil {
		return nil
	}

	return w
}

// GetBatchSize returns the number of proofs per aggregation.
func (p *AggregationPipeline) BatchSize() int {
	return p.batchSize
}

// GetInnerProver returns the inner prover (BLS12-377).
func (p *AggregationPipeline) GetInnerProver() *CurveProver {
	return p.innerProver
}

// GetAggProver returns the aggregation prover (BW6-761).
func (p *AggregationPipeline) GetAggProver() *CurveProver {
	return p.aggProver
}

// GetWrapProver returns the wrapper prover (BN254).
func (p *AggregationPipeline) GetWrapProver() *CurveProver {
	return p.wrapProver
}
