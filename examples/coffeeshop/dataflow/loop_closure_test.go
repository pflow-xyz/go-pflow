package dataflow

// Closing the loop with mining: dataflow is the *generator* (spec → net →
// tokens-as-events); mining is the *recognizer* (events → net / spec).
// These tests run the coffeeshop pipeline, take its emitted history, hand
// it to the discovery functions, and report what was recovered vs. the
// truth that produced it.
//
// Two angles:
//
//   - Pipeline-shape: Pipeline.ToEventLog() → mining.DiscoverPipeline.
//     The truth is the declarative PipelineSpec we built; the recovered
//     value is a PipelineSpec the heuristic synthesised from inter-arrival
//     statistics.
//   - Process-shape: synthesise one trace per order through the conceptual
//     stages (order → drink_count → expand → ingredient_count) and feed it
//     to mining.Discover("heuristic"). The truth is the staged lowering of
//     HourlyIngredientTotals; the recovered value is a sequential Petri
//     net discovered from the traces.
//
// The recovered objects are not bit-identical to the truth — the heuristic
// can only pick from what the stream's statistics suggest — but for an
// honest, even-rate input the shape lines up.

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/examples/coffeeshop"
	"github.com/pflow-xyz/go-pflow/mining"
	df "github.com/pflow-xyz/go-pflow/tokenmodel/dataflow"
)

// TestLoopClosure_DiscoverPipeline drives the streaming drink-counts
// pipeline with the sample orders, captures the input history, and asks
// mining.DiscoverPipeline to recover a PipelineSpec. It then reports the
// recovered shape against the truth.
func TestLoopClosure_DiscoverPipeline(t *testing.T) {
	orders := SampleOrderStream()
	sort.Slice(orders, func(i, j int) bool { return orders[i].Timestamp < orders[j].Timestamp })

	// Run the same pipeline shape as HourlyDrinkCounts but as a true
	// stream so the captured event history is faithful.
	p := df.NewPipeline("drink-counts").
		WithKeys(DrinkKeys()...).
		WindowInto(df.NewFixedWindows(60), 8*60).
		CountPerKey()
	for _, o := range orders {
		if err := p.Send(o.Drink, o.Timestamp); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	if err := p.AdvanceWatermark(8 * 60); err != nil {
		t.Fatalf("advance wm: %v", err)
	}

	truthSpec := p.Spec()
	log := p.ToEventLog()

	// IdealEventsPerWindow controls window-size choice: span * ideal / N.
	// The sample averages 58/8 ≈ 7 orders per hour, so seeding the
	// heuristic with that yields a ~60-minute window. With the default
	// (50) it would pick one big window.
	opts := mining.PipelineDiscoveryOptions{
		Name:                 "discovered-drinks",
		IdealEventsPerWindow: 7,
	}
	got, err := mining.DiscoverPipeline(log, opts)
	if err != nil {
		t.Fatalf("DiscoverPipeline: %v", err)
	}

	t.Logf("truth     : kind=%s size=%d keys=%d horizon=%d",
		truthSpec.Window.Kind, truthSpec.Window.Size, len(truthSpec.Keys), truthSpec.Horizon)
	t.Logf("recovered : kind=%s size=%d keys=%d horizon=%d  (score=%.2f)",
		got.Spec.Window.Kind, got.Spec.Window.Size, len(got.Spec.Keys), got.Spec.Horizon, got.Score)
	for _, line := range got.Reasoning {
		t.Logf("  · %s", line)
	}

	if got.Spec.Window.Kind != truthSpec.Window.Kind {
		t.Errorf("window kind: got %q want %q", got.Spec.Window.Kind, truthSpec.Window.Kind)
	}
	if len(got.Spec.Keys) != len(truthSpec.Keys) {
		t.Errorf("key count: got %d want %d (got=%v truth=%v)",
			len(got.Spec.Keys), len(truthSpec.Keys), got.Spec.Keys, truthSpec.Keys)
	}
	// Window size is a heuristic — accept anything within ±50% of truth.
	lo, hi := truthSpec.Window.Size/2, truthSpec.Window.Size*2
	if got.Spec.Window.Size < lo || got.Spec.Window.Size > hi {
		t.Errorf("window size %d outside [%d, %d] (truth=%d)",
			got.Spec.Window.Size, lo, hi, truthSpec.Window.Size)
	}
}

// TestLoopClosure_DiscoverProcessNet synthesises one trace per order
// through the conceptual stages of HourlyIngredientTotals and feeds those
// traces to the heuristic miner. With every order following the same
// staged path the miner should recover a sequential staged net.
func TestLoopClosure_DiscoverProcessNet(t *testing.T) {
	orders := SampleOrderStream()
	log := stageTraceLog(orders)

	res, err := mining.Discover(log, "heuristic")
	if err != nil {
		t.Fatalf("Discover heuristic: %v", err)
	}

	// The truth: four staged transitions, one shared sequential path.
	wantStages := []string{"order_received", "drink_counted", "recipe_expanded", "ingredient_counted"}

	transitions := make([]string, 0, len(res.Net.Transitions))
	for name := range res.Net.Transitions {
		transitions = append(transitions, name)
	}
	sort.Strings(transitions)
	t.Logf("recovered transitions: %v", transitions)
	t.Logf("recovered variants=%d coverage=%.1f%% places=%d arcs=%d",
		res.NumVariants, res.CoveragePercent, len(res.Net.Places), len(res.Net.Arcs))

	for _, stage := range wantStages {
		if _, ok := res.Net.Transitions[stage]; !ok {
			t.Errorf("missing recovered transition %q", stage)
		}
	}
	if res.NumVariants != 1 {
		t.Errorf("variants = %d, want 1 (every order follows the same staged path)", res.NumVariants)
	}
	if res.CoveragePercent < 99.0 {
		t.Errorf("most-common-variant coverage = %.1f%%, want ~100%%", res.CoveragePercent)
	}
}

// stageTraceLog turns each order into a four-event trace through the
// conceptual stages. This is the trace a properly instrumented pipeline
// would emit if every stage logged a "saw this order" record; we
// synthesise it here from the recipe table.
func stageTraceLog(orders []Order) *eventlog.EventLog {
	log := eventlog.NewEventLog()
	base := time.Unix(0, 0)
	for i, o := range orders {
		caseID := fmt.Sprintf("order-%04d", i)
		t0 := base.Add(time.Duration(o.Timestamp) * time.Minute)
		stages := []string{"order_received", "drink_counted", "recipe_expanded", "ingredient_counted"}
		for j, stage := range stages {
			log.AddEvent(eventlog.Event{
				CaseID:    caseID,
				Activity:  stage,
				Timestamp: t0.Add(time.Duration(j) * time.Second),
				Attributes: map[string]any{
					"drink":       o.Drink,
					"ingredients": ingredientKeyList(o.Drink),
				},
			})
		}
	}
	return log
}

func ingredientKeyList(drink string) []string {
	recipe := coffeeshop.Recipes[drink]
	keys := make([]string, 0, len(recipe))
	for k := range recipe {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
