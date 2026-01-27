package prover

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"github.com/consensys/gnark/frontend"
)

// WitnessFactory creates circuit assignments from raw witness maps.
// Each application registers its own factory for its circuit types.
type WitnessFactory interface {
	CreateAssignment(circuitName string, witness map[string]string) (frontend.Circuit, error)
}

// Service is the HTTP service for the prover.
type Service struct {
	prover  *Prover
	factory WitnessFactory
	started time.Time
}

// NewService creates a new prover service.
// The prover should already have circuits registered by the caller.
// The factory converts raw witness maps into typed circuit assignments.
func NewService(prover *Prover, factory WitnessFactory) *Service {
	return &Service{
		prover:  prover,
		factory: factory,
		started: time.Now(),
	}
}

// ListCircuits returns the list of registered circuit names.
func (s *Service) ListCircuits() []string {
	return s.prover.ListCircuits()
}

// Prover returns the underlying prover for use by other services.
func (s *Service) Prover() *Prover {
	return s.prover
}

// Handler returns the HTTP handler for the service.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /circuits", s.handleListCircuits)
	mux.HandleFunc("GET /circuits/{name}", s.handleCircuitInfo)
	mux.HandleFunc("POST /prove/{circuit}", s.handleProve)
	mux.HandleFunc("GET /verifier/{circuit}", s.handleExportVerifier)

	return mux
}

// HealthResponse is the response for the health endpoint.
type HealthResponse struct {
	Status   string   `json:"status"`
	Uptime   string   `json:"uptime"`
	Circuits []string `json:"circuits"`
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:   "ok",
		Uptime:   time.Since(s.started).String(),
		Circuits: s.prover.ListCircuits(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// CircuitInfo provides metadata about a registered circuit.
type CircuitInfo struct {
	Name        string `json:"name"`
	Constraints int    `json:"constraints"`
	PublicVars  int    `json:"public_vars"`
	PrivateVars int    `json:"private_vars"`
}

// GetCircuitInfo returns metadata for all registered circuits.
func GetCircuitInfo(p *Prover) []CircuitInfo {
	names := p.ListCircuits()
	infos := make([]CircuitInfo, 0, len(names))
	for _, name := range names {
		cc, ok := p.GetCircuit(name)
		if !ok {
			continue
		}
		infos = append(infos, CircuitInfo{
			Name:        cc.Name,
			Constraints: cc.Constraints,
			PublicVars:  cc.PublicVars,
			PrivateVars: cc.PrivateVars,
		})
	}
	return infos
}

// CircuitListResponse lists all available circuits.
type CircuitListResponse struct {
	Circuits []CircuitInfo `json:"circuits"`
}

func (s *Service) handleListCircuits(w http.ResponseWriter, r *http.Request) {
	resp := CircuitListResponse{
		Circuits: GetCircuitInfo(s.prover),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Service) handleCircuitInfo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	cc, ok := s.prover.GetCircuit(name)
	if !ok {
		http.Error(w, fmt.Sprintf("circuit %q not found", name), http.StatusNotFound)
		return
	}

	info := CircuitInfo{
		Name:        cc.Name,
		Constraints: cc.Constraints,
		PublicVars:  cc.PublicVars,
		PrivateVars: cc.PrivateVars,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// ProveRequest is the request body for proof generation.
type ProveRequest struct {
	Witness map[string]string `json:"witness"`
}

// ProveResponse is the response from proof generation.
type ProveResponse struct {
	Proof       *ProofResult `json:"proof,omitempty"`
	Error       string       `json:"error,omitempty"`
	ProofTimeMs int64        `json:"proof_time_ms"`
	CircuitName string       `json:"circuit_name"`
	Constraints int          `json:"constraints"`
}

func (s *Service) handleProve(w http.ResponseWriter, r *http.Request) {
	circuitName := r.PathValue("circuit")

	cc, ok := s.prover.GetCircuit(circuitName)
	if !ok {
		http.Error(w, fmt.Sprintf("circuit %q not found", circuitName), http.StatusNotFound)
		return
	}

	var req ProveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Create circuit assignment from witness using the factory
	assignment, err := s.factory.CreateAssignment(circuitName, req.Witness)
	if err != nil {
		resp := ProveResponse{
			Error:       err.Error(),
			CircuitName: circuitName,
			Constraints: cc.Constraints,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Generate proof
	start := time.Now()
	proof, err := s.prover.Prove(circuitName, assignment)
	elapsed := time.Since(start)

	resp := ProveResponse{
		CircuitName: circuitName,
		Constraints: cc.Constraints,
		ProofTimeMs: elapsed.Milliseconds(),
	}

	if err != nil {
		resp.Error = err.Error()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Proof = proof

	slog.Info("Proof generated",
		"circuit", circuitName,
		"constraints", cc.Constraints,
		"elapsed", elapsed,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Service) handleExportVerifier(w http.ResponseWriter, r *http.Request) {
	circuitName := r.PathValue("circuit")

	solidity, err := s.prover.ExportVerifier(circuitName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(solidity))
}

// ParseBigInt parses a hex or decimal string into a big.Int.
// Exported as a utility for WitnessFactory implementations.
func ParseBigInt(val string) (*big.Int, error) {
	if len(val) > 2 && val[:2] == "0x" {
		var bi big.Int
		_, ok := bi.SetString(val[2:], 16)
		if !ok {
			return nil, fmt.Errorf("invalid hex value: %s", val)
		}
		return &bi, nil
	}
	var bi big.Int
	_, ok := bi.SetString(val, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal value: %s", val)
	}
	return &bi, nil
}

// ParseWitnessField parses a witness field, returning 0 for missing values.
func ParseWitnessField(witness map[string]string, key string) (interface{}, error) {
	val, ok := witness[key]
	if !ok {
		return 0, nil
	}
	return ParseBigInt(val)
}
