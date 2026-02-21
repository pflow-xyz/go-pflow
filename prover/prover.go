package prover

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// Prover manages circuit compilation, setup, and proof generation.
type Prover struct {
	mu       sync.RWMutex
	circuits map[string]*CompiledCircuit
	curve    ecc.ID
	keyDir   string // optional directory for persisting keys
}

// CompiledCircuit holds the compiled circuit and keys.
type CompiledCircuit struct {
	Name        string
	CS          constraint.ConstraintSystem
	ProvingKey  groth16.ProvingKey
	VerifyingKey groth16.VerifyingKey
	Constraints int
	PublicVars  int
	PrivateVars int
}

// ProofResult contains the generated proof and public inputs.
type ProofResult struct {
	// Proof points for Solidity verification
	A            [2]*big.Int   `json:"a"`
	B            [2][2]*big.Int `json:"b"`
	C            [2]*big.Int   `json:"c"`

	// Raw proof as flat array for L1 submission: [A.X, A.Y, B.X[0], B.X[1], B.Y[0], B.Y[1], C.X, C.Y]
	RawProof []*big.Int `json:"raw_proof"`

	// Public inputs (as hex strings for Solidity)
	PublicInputs []string `json:"public_inputs"`

	// Metadata
	CircuitName string `json:"circuit_name"`
	Constraints int    `json:"constraints"`
}

// NewProver creates a new prover instance.
func NewProver() *Prover {
	return &Prover{
		circuits: make(map[string]*CompiledCircuit),
		curve:    ecc.BN254, // Ethereum's alt_bn128
	}
}

// NewProverWithKeyDir creates a prover that persists keys to keyDir.
// Keys are saved after setup and loaded on subsequent runs, ensuring the
// same proving/verifying key pair across restarts.
func NewProverWithKeyDir(keyDir string) *Prover {
	return &Prover{
		circuits: make(map[string]*CompiledCircuit),
		curve:    ecc.BN254,
		keyDir:   keyDir,
	}
}

// LoadOrCompile compiles the circuit, then either loads cached keys from disk
// (if the constraint system hash matches) or runs groth16.Setup and saves the
// new keys. The compiled circuit is stored in the prover's registry.
func (p *Prover) LoadOrCompile(name string, circuit frontend.Circuit) (*CompiledCircuit, error) {
	if p.keyDir == "" {
		cc, err := p.CompileCircuit(name, circuit)
		if err != nil {
			return nil, err
		}
		p.StoreCircuit(name, cc)
		return cc, nil
	}

	// Compile to get the current constraint system hash.
	cs, err := frontend.Compile(p.curve.ScalarField(), r1cs.NewBuilder, circuit)
	if err != nil {
		return nil, fmt.Errorf("circuit compilation failed: %w", err)
	}

	currentHash, err := hashConstraintSystem(cs)
	if err != nil {
		return nil, fmt.Errorf("hash constraint system: %w", err)
	}

	dir := filepath.Join(p.keyDir, name)

	// Try loading cached keys.
	if savedHash, err := os.ReadFile(filepath.Join(dir, "circuit.hash")); err == nil {
		if string(savedHash) == currentHash {
			cc, err := LoadFrom(dir, p.curve)
			if err == nil {
				cc.Name = name
				p.StoreCircuit(name, cc)
				slog.Info("Loaded circuit keys from disk", "name", name, "dir", dir)
				return cc, nil
			}
			// Load failed — fall through to regenerate.
			slog.Warn("Failed to load cached keys, regenerating", "name", name, "err", err)
		} else {
			slog.Info("Circuit changed, regenerating keys", "name", name)
		}
	}

	// No cache or hash mismatch — run setup and save.
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	cc := &CompiledCircuit{
		Name:         name,
		CS:           cs,
		ProvingKey:   pk,
		VerifyingKey: vk,
		Constraints:  cs.GetNbConstraints(),
		PublicVars:   cs.GetNbPublicVariables(),
		PrivateVars:  cs.GetNbSecretVariables(),
	}

	if err := cc.SaveTo(dir); err != nil {
		slog.Warn("Failed to save keys to disk", "name", name, "err", err)
		// Non-fatal — the prover still works, just without persistence.
	} else {
		slog.Info("Saved circuit keys to disk", "name", name, "dir", dir)
	}

	p.StoreCircuit(name, cc)
	return cc, nil
}

// RegisterCircuit compiles a circuit and runs trusted setup.
func (p *Prover) RegisterCircuit(name string, circuit frontend.Circuit) error {
	cc, err := p.CompileCircuit(name, circuit)
	if err != nil {
		return err
	}
	p.StoreCircuit(name, cc)
	return nil
}

// CompileCircuit compiles a circuit and runs trusted setup without storing it.
// This is useful for parallel compilation where storage happens later.
func (p *Prover) CompileCircuit(name string, circuit frontend.Circuit) (*CompiledCircuit, error) {
	// Compile to R1CS
	cs, err := frontend.Compile(p.curve.ScalarField(), r1cs.NewBuilder, circuit)
	if err != nil {
		return nil, fmt.Errorf("circuit compilation failed: %w", err)
	}

	// Trusted setup (in production, use ceremony or universal setup)
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	return &CompiledCircuit{
		Name:         name,
		CS:           cs,
		ProvingKey:   pk,
		VerifyingKey: vk,
		Constraints:  cs.GetNbConstraints(),
		PublicVars:   cs.GetNbPublicVariables(),
		PrivateVars:  cs.GetNbSecretVariables(),
	}, nil
}

// StoreCircuit stores a pre-compiled circuit in the prover's registry.
func (p *Prover) StoreCircuit(name string, cc *CompiledCircuit) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.circuits[name] = cc
}

// GetCircuit returns a compiled circuit by name.
func (p *Prover) GetCircuit(name string) (*CompiledCircuit, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	cc, ok := p.circuits[name]
	return cc, ok
}

// ListCircuits returns all registered circuit names.
func (p *Prover) ListCircuits() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	names := make([]string, 0, len(p.circuits))
	for name := range p.circuits {
		names = append(names, name)
	}
	return names
}

// Prove generates a Groth16 proof for the given circuit and witness.
func (p *Prover) Prove(circuitName string, assignment frontend.Circuit) (*ProofResult, error) {
	p.mu.RLock()
	cc, ok := p.circuits[circuitName]
	p.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("circuit %q not registered", circuitName)
	}

	// Create witness from assignment
	witness, err := frontend.NewWitness(assignment, p.curve.ScalarField())
	if err != nil {
		return nil, fmt.Errorf("witness creation failed: %w", err)
	}

	// Generate proof
	proof, err := groth16.Prove(cc.CS, cc.ProvingKey, witness)
	if err != nil {
		return nil, fmt.Errorf("proof generation failed: %w", err)
	}

	// Extract public witness
	publicWitness, err := witness.Public()
	if err != nil {
		return nil, fmt.Errorf("public witness extraction failed: %w", err)
	}

	// Convert proof to Solidity-compatible format
	result, err := proofToSolidity(proof, publicWitness, cc)
	if err != nil {
		return nil, fmt.Errorf("proof conversion failed: %w", err)
	}

	return result, nil
}

// Verify verifies a proof locally (before on-chain submission).
func (p *Prover) Verify(circuitName string, assignment frontend.Circuit) error {
	p.mu.RLock()
	cc, ok := p.circuits[circuitName]
	p.mu.RUnlock()

	if !ok {
		return fmt.Errorf("circuit %q not registered", circuitName)
	}

	// Create witness
	witness, err := frontend.NewWitness(assignment, p.curve.ScalarField())
	if err != nil {
		return fmt.Errorf("witness creation failed: %w", err)
	}

	// Generate proof
	proof, err := groth16.Prove(cc.CS, cc.ProvingKey, witness)
	if err != nil {
		return fmt.Errorf("proof generation failed: %w", err)
	}

	// Extract public witness for verification
	publicWitness, err := witness.Public()
	if err != nil {
		return fmt.Errorf("public witness extraction failed: %w", err)
	}

	// Verify
	return groth16.Verify(proof, cc.VerifyingKey, publicWitness)
}

// proofToSolidity converts a gnark proof to Solidity-compatible format.
func proofToSolidity(proof groth16.Proof, publicWitness witness.Witness, cc *CompiledCircuit) (*ProofResult, error) {
	result := &ProofResult{
		CircuitName: cc.Name,
		Constraints: cc.Constraints,
	}

	// Extract public inputs from witness
	pubBytes, err := publicWitness.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal public witness: %w", err)
	}

	// Parse public inputs (each is 32 bytes for BN254)
	// Skip the first 12 bytes (header: 4 bytes curve ID + 4 bytes nb public + 4 bytes nb secret)
	const headerSize = 12
	const elementSize = 32

	if len(pubBytes) >= headerSize {
		data := pubBytes[headerSize:]
		numElements := len(data) / elementSize
		result.PublicInputs = make([]string, numElements)

		for i := 0; i < numElements; i++ {
			start := i * elementSize
			end := start + elementSize
			if end <= len(data) {
				val := new(big.Int).SetBytes(data[start:end])
				result.PublicInputs[i] = fmt.Sprintf("0x%064x", val)
			}
		}
	}

	// Extract proof points using WriteRawTo for uncompressed format.
	// WriteTo produces compressed points (128 bytes) which lose Y-coordinates.
	// WriteRawTo produces uncompressed points (256 bytes): A(64) + B(128) + C(64).
	// The concrete BN254 proof type implements WriteRawTo; fall back to WriteTo if not available.
	type rawWriter interface {
		WriteRawTo(w io.Writer) (int64, error)
	}

	var proofBuf bytes.Buffer
	if rw, ok := proof.(rawWriter); ok {
		if _, err := rw.WriteRawTo(&proofBuf); err != nil {
			return nil, fmt.Errorf("marshal proof (raw): %w", err)
		}
	} else {
		if _, err := proof.WriteTo(&proofBuf); err != nil {
			return nil, fmt.Errorf("marshal proof: %w", err)
		}
	}
	proofBytes := proofBuf.Bytes()

	// Uncompressed layout (256 bytes):
	//   A (G1): [X 32B][Y 32B]           = 64 bytes
	//   B (G2): [X0 32B][X1 32B][Y0 32B][Y1 32B] = 128 bytes
	//   C (G1): [X 32B][Y 32B]           = 64 bytes
	if len(proofBytes) < 256 {
		return nil, fmt.Errorf("unexpected proof size %d (expected 256 for uncompressed BN254)", len(proofBytes))
	}

	// A point (G1): bytes 0-63
	result.A[0] = new(big.Int).SetBytes(proofBytes[0:32])
	result.A[1] = new(big.Int).SetBytes(proofBytes[32:64])

	// B point (G2): bytes 64-191
	result.B[0][0] = new(big.Int).SetBytes(proofBytes[64:96])
	result.B[0][1] = new(big.Int).SetBytes(proofBytes[96:128])
	result.B[1][0] = new(big.Int).SetBytes(proofBytes[128:160])
	result.B[1][1] = new(big.Int).SetBytes(proofBytes[160:192])

	// C point (G1): bytes 192-255
	result.C[0] = new(big.Int).SetBytes(proofBytes[192:224])
	result.C[1] = new(big.Int).SetBytes(proofBytes[224:256])

	// Build RawProof array: [A.X, A.Y, B.X[0], B.X[1], B.Y[0], B.Y[1], C.X, C.Y]
	result.RawProof = []*big.Int{
		result.A[0], result.A[1],
		result.B[0][0], result.B[0][1],
		result.B[1][0], result.B[1][1],
		result.C[0], result.C[1],
	}

	return result, nil
}

// ExportVerifier exports the Solidity verifier for a circuit.
func (p *Prover) ExportVerifier(circuitName string) (string, error) {
	p.mu.RLock()
	cc, ok := p.circuits[circuitName]
	p.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("circuit %q not registered", circuitName)
	}

	var buf []byte
	w := &byteWriter{buf: &buf}
	err := cc.VerifyingKey.ExportSolidity(w)
	if err != nil {
		return "", fmt.Errorf("export failed: %w", err)
	}

	return string(buf), nil
}

// byteWriter is a simple io.Writer that appends to a byte slice.
type byteWriter struct {
	buf *[]byte
}

func (w *byteWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// ============ Parallel Proving ============

// ProofJob represents a proof generation job.
type ProofJob struct {
	ID          int
	CircuitName string
	Assignment  frontend.Circuit
}

// ProofJobResult is the result of a proof generation job.
type ProofJobResult struct {
	ID     int
	Proof  *ProofResult
	Error  error
	TimeMs int64
}

// ProveParallel generates multiple proofs concurrently.
// The number of concurrent workers is limited by maxWorkers.
// Results are returned in the same order as the input jobs.
func (p *Prover) ProveParallel(jobs []ProofJob, maxWorkers int) []ProofJobResult {
	if maxWorkers <= 0 {
		maxWorkers = 4 // Default to 4 workers
	}

	numJobs := len(jobs)
	results := make([]ProofJobResult, numJobs)

	// Create job channel
	jobChan := make(chan ProofJob, numJobs)
	resultChan := make(chan ProofJobResult, numJobs)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				start := time.Now()
				proof, err := p.Prove(job.CircuitName, job.Assignment)
				resultChan <- ProofJobResult{
					ID:     job.ID,
					Proof:  proof,
					Error:  err,
					TimeMs: time.Since(start).Milliseconds(),
				}
			}
		}()
	}

	// Submit jobs
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for result := range resultChan {
		results[result.ID] = result
	}

	return results
}

// ProofPool manages a pool of proof workers for continuous proving.
type ProofPool struct {
	prover     *Prover
	jobs       chan ProofJob
	results    chan ProofJobResult
	numWorkers int
	wg         sync.WaitGroup
	closed     bool
	mu         sync.Mutex
}

// NewProofPool creates a new proof worker pool.
func NewProofPool(prover *Prover, numWorkers int) *ProofPool {
	if numWorkers <= 0 {
		numWorkers = 4
	}

	pool := &ProofPool{
		prover:     prover,
		jobs:       make(chan ProofJob, numWorkers*2),
		results:    make(chan ProofJobResult, numWorkers*2),
		numWorkers: numWorkers,
	}

	// Start workers
	for i := 0; i < numWorkers; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}

	return pool
}

func (pool *ProofPool) worker() {
	defer pool.wg.Done()
	for job := range pool.jobs {
		start := time.Now()
		proof, err := pool.prover.Prove(job.CircuitName, job.Assignment)
		pool.results <- ProofJobResult{
			ID:     job.ID,
			Proof:  proof,
			Error:  err,
			TimeMs: time.Since(start).Milliseconds(),
		}
	}
}

// Submit adds a proof job to the pool.
func (pool *ProofPool) Submit(job ProofJob) error {
	pool.mu.Lock()
	if pool.closed {
		pool.mu.Unlock()
		return fmt.Errorf("pool is closed")
	}
	pool.mu.Unlock()

	pool.jobs <- job
	return nil
}

// Results returns the channel for receiving proof results.
func (pool *ProofPool) Results() <-chan ProofJobResult {
	return pool.results
}

// Close shuts down the proof pool.
func (pool *ProofPool) Close() {
	pool.mu.Lock()
	pool.closed = true
	pool.mu.Unlock()

	close(pool.jobs)
	pool.wg.Wait()
	close(pool.results)
}

// NumWorkers returns the number of workers in the pool.
func (pool *ProofPool) NumWorkers() int {
	return pool.numWorkers
}
