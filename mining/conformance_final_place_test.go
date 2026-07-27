package mining

import (
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/petri"
)

// linearNet builds received -> validate -> validated -> ship -> <sinkName>.
func linearNet(sinkName string) *petri.PetriNet {
	return petri.Build().
		Place("received", 1).Place("validated", 0).Place(sinkName, 0).
		Transition("validate").Transition("ship").
		Arc("received", "validate", 1).Arc("validate", "validated", 1).
		Arc("validated", "ship", 1).Arc("ship", sinkName, 1).
		Done()
}

func linearLog() *eventlog.EventLog {
	log := eventlog.NewEventLog()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	log.AddEvent(eventlog.Event{CaseID: "o1", Activity: "validate", Timestamp: base})
	log.AddEvent(eventlog.Event{CaseID: "o1", Activity: "ship", Timestamp: base.Add(time.Hour)})
	return log
}

// TestFitnessIndependentOfSinkName is the regression: replay used to exempt
// only a place literally named "end" from the leftover-token penalty, so a
// perfectly replayed trace scored below 1.0 whenever the sink had any other
// name. Fitness must not depend on what the final place is called.
func TestFitnessIndependentOfSinkName(t *testing.T) {
	var baseline float64

	for i, sink := range []string{"end", "shipped", "done", "completed"} {
		t.Run(sink, func(t *testing.T) {
			result := CheckConformance(linearLog(), linearNet(sink))

			if result.Fitness < 0.999 {
				t.Errorf("fitness = %.4f with sink %q, want 1.0 for an exactly-replayed trace",
					result.Fitness, sink)
			}
			if result.FittingTraces != 1 {
				t.Errorf("fitting traces = %d with sink %q, want 1", result.FittingTraces, sink)
			}

			if i == 0 {
				baseline = result.Fitness
			} else if result.Fitness != baseline {
				t.Errorf("fitness %.4f with sink %q differs from %.4f with sink \"end\"",
					result.Fitness, sink, baseline)
			}
		})
	}
}

// TestIncompleteTraceStillPenalized checks the fix did not simply remove the
// penalty: a case that stops before reaching the sink must still score lower.
func TestIncompleteTraceStillPenalized(t *testing.T) {
	log := eventlog.NewEventLog()
	log.AddEvent(eventlog.Event{
		CaseID:    "o1",
		Activity:  "validate", // stops before "ship"
		Timestamp: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	})

	result := CheckConformance(log, linearNet("shipped"))

	if result.Fitness >= 1.0 {
		t.Errorf("fitness = %.4f for an unfinished case, want < 1.0", result.Fitness)
	}
}

// TestFinalPlacesStructural checks sink detection directly.
func TestFinalPlacesStructural(t *testing.T) {
	net := linearNet("shipped")
	finals := finalPlaces(net)

	if !finals["shipped"] {
		t.Error("shipped has no outgoing arcs and should be a final place")
	}
	for _, p := range []string{"received", "validated"} {
		if finals[p] {
			t.Errorf("%q has an outgoing arc and should not be a final place", p)
		}
	}
}

// TestFinalPlacesIgnoresInhibitorArcs: an inhibitor tests a place without
// consuming from it, so it does not stop the place being a sink.
func TestFinalPlacesIgnoresInhibitorArcs(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("done", 0).
		Transition("t").Transition("guarded").
		Arc("a", "t", 1).Arc("t", "done", 1).
		InhibitorArc("done", "guarded", 1).
		Done()

	if !finalPlaces(net)["done"] {
		t.Error("a place whose only outgoing arc is an inhibitor should still be final")
	}
}

// TestNoFinalPlaceNoCompletionPenalty: a purely cyclic net has no sink, so
// there is no completion whose absence could be penalized.
//
// Note the trace still does not reach fitness 1.0, and that is correct rather
// than a leftover bug: token-replay fitness charges for tokens remaining after
// replay, and a cyclic net by construction always holds its token somewhere.
// What the fix guarantees here is only that no *extra* completion penalty is
// added on top.
func TestNoFinalPlaceNoCompletionPenalty(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("b", 0).
		Transition("fwd").Transition("back").
		Arc("a", "fwd", 1).Arc("fwd", "b", 1).
		Arc("b", "back", 1).Arc("back", "a", 1).
		Done()

	if len(finalPlaces(net)) != 0 {
		t.Fatalf("cyclic net should have no final places, got %v", finalPlaces(net))
	}

	log := eventlog.NewEventLog()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	log.AddEvent(eventlog.Event{CaseID: "c1", Activity: "fwd", Timestamp: base})
	log.AddEvent(eventlog.Event{CaseID: "c1", Activity: "back", Timestamp: base.Add(time.Hour)})

	result := CheckConformance(log, net)

	// Every activity replayed: nothing was missing.
	if got := result.TraceResults[0].MissingTokens; got != 0 {
		t.Errorf("missing tokens = %d, want 0 — the cycle replays exactly", got)
	}
	if got := len(result.TraceResults[0].MissingActivities); got != 0 {
		t.Errorf("missing activities = %v, want none", result.TraceResults[0].MissingActivities)
	}

	// Exactly one token remains (the one circulating), and no completion
	// penalty is stacked on top of it.
	if got := result.TraceResults[0].RemainingTokens; got != 1 {
		t.Errorf("remaining tokens = %d, want 1 (the circulating token, with no completion penalty)", got)
	}
}
