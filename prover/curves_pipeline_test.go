package prover

import (
	"context"
	"os"
	"testing"
)

// requireSlowProver skips unless PROVER_SLOW_TESTS=1. NewAggregationPipeline
// runs a Groth16 trusted setup for the recursion aggregator on BW6-761 —
// minutes of work — which is far too heavy for a default test run. The fast
// per-curve Prove/Verify contract is covered unconditionally above; these
// exercise the pipeline plumbing when explicitly requested:
//
//	PROVER_SLOW_TESTS=1 go test ./prover/ -run TestPipeline -timeout 30m
func requireSlowProver(t *testing.T) {
	t.Helper()
	if os.Getenv("PROVER_SLOW_TESTS") != "1" {
		t.Skip("set PROVER_SLOW_TESTS=1 to run pipeline tests (BW6-761 trusted setup takes minutes)")
	}
}

// TestCurveProverRoundTrip proves and verifies a minimal circuit on each of
// the three recursion-stack curves. This is the core Prove/Verify contract
// that the aggregation pipeline builds on, previously untested per curve.
func TestCurveProverRoundTrip(t *testing.T) {
	for _, cfg := range []CurveConfig{BLS12_377Config, BW6_761Config, BN254Config} {
		t.Run(cfg.Name, func(t *testing.T) {
			cp := NewCurveProver(cfg)
			if cp.CurveID() != cfg.ID || cp.Config().Name != cfg.Name {
				t.Fatalf("prover config mismatch: %v", cp.Config())
			}

			if err := cp.RegisterCircuit("square", &SimpleTestCircuit{}); err != nil {
				t.Fatalf("RegisterCircuit: %v", err)
			}

			// 25 == 5*5: valid witness proves and verifies.
			proof, err := cp.Prove("square", &SimpleTestCircuit{X: 25, Y: 5})
			if err != nil {
				t.Fatalf("Prove: %v", err)
			}
			if proof.Curve != cfg.ID || proof.Constraints == 0 {
				t.Errorf("proof metadata: curve=%v constraints=%d", proof.Curve, proof.Constraints)
			}
			if err := cp.Verify("square", proof); err != nil {
				t.Errorf("Verify rejected a valid proof: %v", err)
			}

			// An invalid witness must fail at proving time.
			if _, err := cp.Prove("square", &SimpleTestCircuit{X: 26, Y: 5}); err == nil {
				t.Error("Prove accepted a witness that does not satisfy the circuit")
			}

			// Unknown circuit names are errors, not panics.
			if _, err := cp.Prove("ghost", &SimpleTestCircuit{X: 25, Y: 5}); err == nil {
				t.Error("Prove on an unregistered circuit must fail")
			}
			if err := cp.Verify("ghost", proof); err == nil {
				t.Error("Verify on an unregistered circuit must fail")
			}
		})
	}
}

// TestCurveProverCircuitRegistry covers Store/Get/List without proving.
func TestCurveProverCircuitRegistry(t *testing.T) {
	cp := NewCurveProver(BN254Config)

	cc, err := cp.CompileCircuit("square", &SimpleTestCircuit{})
	if err != nil {
		t.Fatalf("CompileCircuit: %v", err)
	}
	if cc.Constraints == 0 || cc.PublicVars == 0 {
		t.Errorf("compiled circuit metadata: %+v", cc)
	}

	// Not stored yet.
	if _, ok := cp.GetCircuit("square"); ok {
		t.Error("CompileCircuit should not store")
	}
	cp.StoreCircuit("square", cc)
	if got, ok := cp.GetCircuit("square"); !ok || got != cc {
		t.Error("StoreCircuit/GetCircuit round trip failed")
	}
	if _, ok := cp.GetCircuit("missing"); ok {
		t.Error("GetCircuit invented a circuit")
	}
}

// TestPipelinePendingManagement covers the batch bookkeeping, which is pure
// state and needs no proving at all — the pipeline constructor is what costs
// time (it compiles the aggregator and wrapper circuits), so this test builds
// the pending logic through a pipeline once and reuses it.
func TestPipelinePendingManagement(t *testing.T) {
	requireSlowProver(t)

	pipeline, err := NewAggregationPipeline(PipelineConfig{BatchSize: 2})
	if err != nil {
		t.Fatalf("NewAggregationPipeline: %v", err)
	}

	if got := pipeline.GetPendingCount(); got != 0 {
		t.Errorf("initial pending = %d", got)
	}

	// First proof: below batch size.
	if full := pipeline.AddPendingInner(&InnerProofResult{BatchNumber: 1}); full {
		t.Error("AddPendingInner reported full at 1/2")
	}
	// Second: batch is full.
	if full := pipeline.AddPendingInner(&InnerProofResult{BatchNumber: 2}); !full {
		t.Error("AddPendingInner should report full at 2/2")
	}
	if got := pipeline.GetPendingCount(); got != 2 {
		t.Errorf("pending = %d, want 2", got)
	}

	drained := pipeline.DrainPending()
	if len(drained) != 2 || drained[0].BatchNumber != 1 || drained[1].BatchNumber != 2 {
		t.Errorf("drained = %+v", drained)
	}
	if got := pipeline.GetPendingCount(); got != 0 {
		t.Errorf("pending after drain = %d, want 0", got)
	}

	// Aggregate with the wrong count fails fast, before any proving.
	if _, err := pipeline.Aggregate(context.Background(), drained[:1]); err == nil {
		t.Error("Aggregate must reject a short batch")
	}
}

// TestPipelineProveInner runs one real inner proof through the pipeline and
// checks the metadata is threaded onto the result.
func TestPipelineProveInner(t *testing.T) {
	requireSlowProver(t)

	pipeline, err := NewAggregationPipeline(PipelineConfig{BatchSize: 2})
	if err != nil {
		t.Fatalf("NewAggregationPipeline: %v", err)
	}

	if err := pipeline.RegisterInnerCircuit("square", &SimpleTestCircuit{}); err != nil {
		t.Fatalf("RegisterInnerCircuit: %v", err)
	}

	meta := &BatchMetadata{BatchNumber: 42, PrevStateRoot: [32]byte{1}, NewStateRoot: [32]byte{2}}
	res, err := pipeline.ProveInner(context.Background(), "square", &SimpleTestCircuit{X: 9, Y: 3}, meta)
	if err != nil {
		t.Fatalf("ProveInner: %v", err)
	}
	if res.BatchNumber != 42 || res.PrevStateRoot != meta.PrevStateRoot || res.NewStateRoot != meta.NewStateRoot {
		t.Errorf("metadata not threaded: %+v", res)
	}
	if res.Proof == nil || res.PublicWitness == nil {
		t.Error("proof or public witness missing")
	}
}
