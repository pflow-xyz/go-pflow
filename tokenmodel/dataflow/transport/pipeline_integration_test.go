package transport

import (
	"context"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/tokenmodel/dataflow"
	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// TestPipelineWindowSubnetDistributed proves the integration: a
// dataflow.Pipeline compiles to a subnet.Bundle, and the (key, window)
// subnets within that bundle run correctly on a SubnetRunner driven by
// LocalChannelTransport. Tokens injected on the `feed` in-port flow
// through `receive` to `acc`, and once the watermark crosses w.End the
// `emit` transition drains them into `out`.
//
// This is the L3.2 milestone in miniature: not a full distributed
// Pipeline.Run() (which would require gating the watermark subnet's
// inputless `advance` transition and teaching SubnetRunner about
// bindings for guarded source assigns), but a concrete demonstration
// that the pieces compose.
func TestPipelineWindowSubnetDistributed(t *testing.T) {
	// Build a pipeline and grab its compiled bundle without running it.
	p := dataflow.NewPipeline("dist").
		WithKeys("k").
		WindowInto(dataflow.NewFixedWindows(10), 20).
		CountPerKey()
	b, err := p.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Isolate just the [0,10) window subnet plus a fake watermark feeder.
	// Replace the bundle's watermark subnet with one whose `wm` place is
	// pre-loaded — the SubnetRunner's `advance` transition is inputless
	// and would otherwise fire forever (a known gap covered by the
	// distributed-Run roadmap follow-up).
	wmSubID := "watermark"
	for i := range b.Subnets {
		if b.Subnets[i].ID == wmSubID {
			b.Subnets[i].Model = staticWatermarkModel(10) // exactly 10 tokens, no advance transition
		}
	}

	d := NewDistributedBundle(b, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer d.Stop()

	// Inject 3 elements into the [0,10) window via its `feed` in-port.
	winSubID := "win:k:[0,10)"
	for i := 0; i < 3; i++ {
		if err := d.SendToInPort(winSubID, "feed", 1); err != nil {
			t.Fatalf("SendToInPort: %v", err)
		}
	}

	if !d.Quiesce(2 * time.Second) {
		t.Fatal("did not quiesce")
	}

	// The window subnet should have moved all 3 tokens acc -> out.
	m := d.Marking(winSubID)
	if got := m["out"]; got != 3 {
		t.Errorf("distributed window subnet out = %d, want 3 (marking=%v)", got, m)
	}
	if got := m["acc"]; got != 0 {
		t.Errorf("acc should be drained, got %d", got)
	}
}

// staticWatermarkModel returns a watermark stand-in: a single `wm` place
// preloaded with `tokens` and NO transition. Used by the distributed
// integration test where the orchestrator can't yet drive the real
// watermark subnet's inputless `advance` transition.
func staticWatermarkModel(tokens int) *tmpetri.Model {
	m := tmpetri.NewModel("watermark")
	m.AddPlace(tmpetri.Place{ID: "wm", Initial: tokens})
	return m
}

// TestPipelineSpecBuildToBundle is the "L4.2 ↔ L3.1 shake hands" test:
// a spec round-tripped through JSON builds a Pipeline whose Compile()
// produces a Bundle whose Subnets match the original. Catches regressions
// in either the spec or the compiler that would break distributed runs.
func TestPipelineSpecBuildToBundle(t *testing.T) {
	orig := dataflow.NewPipeline("shake").
		WithKeys("a", "b").
		WindowInto(dataflow.NewFixedWindows(5), 10).
		CountPerKey()
	origBundle, err := orig.Compile()
	if err != nil {
		t.Fatal(err)
	}

	rebuilt, err := orig.Spec().Build()
	if err != nil {
		t.Fatal(err)
	}
	rebuiltBundle, err := rebuilt.Compile()
	if err != nil {
		t.Fatal(err)
	}

	// Same subnet IDs (order may differ; compare sets).
	got := subnetIDSet(rebuiltBundle)
	want := subnetIDSet(origBundle)
	if len(got) != len(want) {
		t.Fatalf("subnet count drift: got %d, want %d", len(got), len(want))
	}
	for id := range want {
		if !got[id] {
			t.Errorf("missing subnet after rebuild: %q", id)
		}
	}
}

func subnetIDSet(b *subnet.Bundle) map[string]bool {
	out := map[string]bool{}
	for _, s := range b.Subnets {
		out[s.ID] = true
	}
	return out
}
