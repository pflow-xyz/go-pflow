package compat

import (
	"strings"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/statemachine"
	"github.com/pflow-xyz/go-pflow/workflow"
)

// TestMeasureStatemachineLoss runs a representative chart through the
// bridge and reports the diagnostics. Skips on failure so the test
// captures the loss profile without breaking the suite — this is a
// measurement instrument, not a regression test.
func TestMeasureStatemachineLoss(t *testing.T) {
	chart := statemachine.NewChart("traffic").
		Region("light").
		State("red").Initial().
		State("green").
		State("yellow").
		EndRegion().
		When("timer").In("light:red").GoTo("light:green").
		When("timer").In("light:green").GoTo("light:yellow").
		When("timer").In("light:yellow").GoTo("light:red").
		Build()

	net := chart.ToPetriNet()
	m, diag, err := ToModel(net)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("statemachine → tokenmodel/petri:")
	t.Logf("  places:      %d", len(m.Places))
	t.Logf("  transitions: %d", len(m.Transitions))
	t.Logf("  arcs:        %d", len(m.Arcs))
	t.Logf("  invariants:  %d", len(m.Invariants))
	t.Logf("  diagnostics: %d", len(diag.Notes))
	if diag.HasLoss() {
		t.Logf("  loss profile:")
		for _, n := range diag.Notes[:min(5, len(diag.Notes))] {
			t.Logf("    - %s", n)
		}
		if len(diag.Notes) > 5 {
			t.Logf("    (+ %d more)", len(diag.Notes)-5)
		}
	}

	// Verdict: a statemachine that crosses with zero or only minor
	// diagnostics (e.g. floor of integer-valued floats — not a real loss)
	// is a good migration candidate.
	realLoss := 0
	for _, n := range diag.Notes {
		// "floored to" on an integer value (e.g. "1 floored to 1") is
		// reported because the source was a float; it's not actual data
		// loss. Same for capacity=0.
		if !strings.Contains(n, "weight ") || strings.Contains(n, "schema") || strings.Contains(n, "guard") {
			realLoss++
		}
	}
	t.Logf("  real-loss diagnostics (non-trivial): %d", realLoss)
}

func TestMeasureWorkflowLoss(t *testing.T) {
	wf := workflow.New("order").
		ManualTask("receive", "Receive", 2*time.Minute).
		AutoTask("validate", "Validate", 30*time.Second).
		ManualTask("ship", "Ship", 5*time.Minute).
		From("receive").Then("validate").To("ship").
		Start("receive").End("ship").
		WithSLA(4 * time.Hour).
		Build()
	net := wf.ToPetriNet()

	m, diag, err := ToModel(net)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("workflow → tokenmodel/petri:")
	t.Logf("  places:      %d", len(m.Places))
	t.Logf("  transitions: %d", len(m.Transitions))
	t.Logf("  arcs:        %d", len(m.Arcs))
	t.Logf("  invariants:  %d", len(m.Invariants))
	t.Logf("  diagnostics: %d", len(diag.Notes))
	for _, n := range diag.Notes[:min(5, len(diag.Notes))] {
		t.Logf("    - %s", n)
	}
	if len(diag.Notes) > 5 {
		t.Logf("    (+ %d more)", len(diag.Notes)-5)
	}
}

func TestMeasureSummary(t *testing.T) {
	// Run the two measurements above silently and print a side-by-side
	// summary so you can see the facade migration cost at a glance.
	smChart := statemachine.NewChart("c").
		Region("r").State("a").Initial().State("b").EndRegion().
		When("e").In("r:a").GoTo("r:b").
		Build()
	smNet := smChart.ToPetriNet()
	_, smDiag, _ := ToModel(smNet)

	wf := workflow.New("w").
		ManualTask("a", "A", time.Minute).
		ManualTask("b", "B", time.Minute).
		From("a").To("b").
		Start("a").End("b").
		Build()
	wfNet := wf.ToPetriNet()
	_, wfDiag, _ := ToModel(wfNet)

	t.Logf("")
	t.Logf("FACADE MIGRATION READINESS")
	t.Logf("  facade        diagnostics")
	t.Logf("  -----------   -----------")
	t.Logf("  statemachine  %d", len(smDiag.Notes))
	t.Logf("  workflow      %d", len(wfDiag.Notes))
	if strings.Join(smDiag.Notes, "") == "" {
		t.Logf("")
		t.Logf("  statemachine crosses cleanly — Phase 2 target confirmed.")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
