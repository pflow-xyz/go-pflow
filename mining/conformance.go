// Package mining provides process mining algorithms including discovery and conformance checking.
package mining

import (
	"fmt"
	"sort"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/petri"
)

// =============================================================================
// Conformance Checking - Token-Based Replay
// =============================================================================

// ConformanceResult contains the results of conformance checking.
type ConformanceResult struct {
	// Overall fitness score (0.0 to 1.0)
	Fitness float64

	// Detailed metrics
	ProducedTokens  int // Tokens produced during replay
	ConsumedTokens  int // Tokens consumed during replay
	MissingTokens   int // Tokens needed but not available
	RemainingTokens int // Tokens left after replay

	// Per-trace results
	TraceResults []TraceReplayResult

	// Summary statistics
	FittingTraces    int     // Number of traces that fit perfectly
	TotalTraces      int     // Total number of traces
	FittingPercent   float64 // Percentage of fitting traces
	AvgTraceFitness  float64 // Average fitness across all traces
}

// TraceReplayResult contains the result of replaying a single trace.
type TraceReplayResult struct {
	CaseID          string
	Fitness         float64
	Fitting         bool     // True if trace fits perfectly
	MissingTokens   int      // Tokens needed but not available
	RemainingTokens int      // Tokens left after replay
	ProducedTokens  int      // Total tokens produced
	ConsumedTokens  int      // Total tokens consumed
	FiredTransitions []string // Transitions successfully fired
	MissingActivities []string // Activities that couldn't fire
	Activities      []string // Original activity sequence
}

// TokenState tracks token counts during replay.
type TokenState map[string]int

// copyState creates a deep copy of token state.
func copyState(state TokenState) TokenState {
	result := make(TokenState)
	for k, v := range state {
		result[k] = v
	}
	return result
}

// CheckConformance performs token-based replay conformance checking.
// It replays each trace from the event log against the Petri net model
// and computes fitness metrics.
func CheckConformance(log *eventlog.EventLog, net *petri.PetriNet) *ConformanceResult {
	result := &ConformanceResult{
		TraceResults: make([]TraceReplayResult, 0, log.NumCases()),
		TotalTraces:  log.NumCases(),
	}

	// Build transition lookup by label (activity name -> transition ID)
	activityToTransition := buildActivityMapping(net)

	// Find initial marking
	initialMarking := getInitialMarking(net)

	// Replay each trace
	for _, trace := range log.GetTraces() {
		traceResult := replayTrace(trace, net, activityToTransition, initialMarking)
		result.TraceResults = append(result.TraceResults, traceResult)

		result.ProducedTokens += traceResult.ProducedTokens
		result.ConsumedTokens += traceResult.ConsumedTokens
		result.MissingTokens += traceResult.MissingTokens
		result.RemainingTokens += traceResult.RemainingTokens

		if traceResult.Fitting {
			result.FittingTraces++
		}
	}

	// Calculate overall fitness using standard formula
	// fitness = 0.5 * (1 - missing/consumed) + 0.5 * (1 - remaining/produced)
	if result.ConsumedTokens > 0 && result.ProducedTokens > 0 {
		missingRatio := float64(result.MissingTokens) / float64(result.ConsumedTokens)
		remainingRatio := float64(result.RemainingTokens) / float64(result.ProducedTokens)
		result.Fitness = 0.5*(1-missingRatio) + 0.5*(1-remainingRatio)
	} else if result.TotalTraces == 0 {
		result.Fitness = 1.0 // Empty log is trivially conformant
	}

	// Calculate summary statistics
	if result.TotalTraces > 0 {
		result.FittingPercent = float64(result.FittingTraces) / float64(result.TotalTraces) * 100

		totalFitness := 0.0
		for _, tr := range result.TraceResults {
			totalFitness += tr.Fitness
		}
		result.AvgTraceFitness = totalFitness / float64(result.TotalTraces)
	}

	return result
}

// buildActivityMapping creates a mapping from activity names to transition IDs.
func buildActivityMapping(net *petri.PetriNet) map[string]string {
	mapping := make(map[string]string)

	for transID, trans := range net.Transitions {
		// Use LabelText if available, otherwise use Label (the transition ID)
		if trans.LabelText != nil && *trans.LabelText != "" {
			mapping[*trans.LabelText] = transID
		} else if trans.Label != "" {
			mapping[trans.Label] = transID
		} else {
			mapping[transID] = transID
		}
	}

	return mapping
}

// getInitialMarking extracts the initial marking from the Petri net.
func getInitialMarking(net *petri.PetriNet) TokenState {
	marking := make(TokenState)
	for placeID, place := range net.Places {
		if place.Initial != nil && len(place.Initial) > 0 {
			// Sum all token types
			total := 0
			for _, count := range place.Initial {
				total += int(count)
			}
			if total > 0 {
				marking[placeID] = total
			}
		}
	}
	return marking
}

// replayTrace replays a single trace against the model.
func replayTrace(trace *eventlog.Trace, net *petri.PetriNet, activityToTransition map[string]string, initialMarking TokenState) TraceReplayResult {
	result := TraceReplayResult{
		CaseID:            trace.CaseID,
		Activities:        trace.GetActivityVariant(),
		FiredTransitions:  make([]string, 0),
		MissingActivities: make([]string, 0),
	}

	// Start with initial marking
	marking := copyState(initialMarking)

	// Count initial tokens as produced
	for _, count := range marking {
		result.ProducedTokens += count
	}

	// Replay each activity
	for _, activity := range result.Activities {
		transID, ok := activityToTransition[activity]
		if !ok {
			// Activity not in model - missing
			result.MissingActivities = append(result.MissingActivities, activity)
			result.MissingTokens++ // Count as 1 missing token
			continue
		}

		// Check if transition is enabled and fire it
		missing, consumed, produced := fireTransition(net, transID, marking)
		result.MissingTokens += missing
		result.ConsumedTokens += consumed
		result.ProducedTokens += produced

		if missing == 0 {
			result.FiredTransitions = append(result.FiredTransitions, transID)
		} else {
			result.MissingActivities = append(result.MissingActivities, activity)
		}
	}

	// Count remaining tokens (excluding end place)
	for placeID, count := range marking {
		// Don't penalize tokens in end place
		if placeID != "end" && count > 0 {
			result.RemainingTokens += count
		}
	}

	// Check if there should be a token in end place
	if endCount, hasEnd := marking["end"]; hasEnd && endCount > 0 {
		// Good - process completed
	} else if hasEndPlace(net) {
		// Process didn't complete - penalize
		result.RemainingTokens++
	}

	// Calculate trace fitness
	if result.ConsumedTokens > 0 && result.ProducedTokens > 0 {
		missingRatio := float64(result.MissingTokens) / float64(result.ConsumedTokens)
		remainingRatio := float64(result.RemainingTokens) / float64(result.ProducedTokens)
		result.Fitness = 0.5*(1-missingRatio) + 0.5*(1-remainingRatio)
		if result.Fitness < 0 {
			result.Fitness = 0
		}
	} else {
		result.Fitness = 1.0 // Empty trace is trivially conformant
	}

	// Trace fits perfectly if no missing tokens and no remaining tokens
	result.Fitting = result.MissingTokens == 0 && result.RemainingTokens == 0

	return result
}

// fireTransition attempts to fire a transition, returning missing, consumed, and produced token counts.
func fireTransition(net *petri.PetriNet, transID string, marking TokenState) (missing, consumed, produced int) {
	// Check input places and determine how many tokens are missing
	inputPlaces := make(map[string]int)
	for _, arc := range net.Arcs {
		if arc.Target == transID {
			weight := int(arc.GetWeightSum())
			if weight == 0 {
				weight = 1
			}
			inputPlaces[arc.Source] = weight
		}
	}

	// Check if enabled (all input places have enough tokens)
	enabled := true
	for placeID, required := range inputPlaces {
		available := marking[placeID]
		if available < required {
			missing += required - available
			enabled = false
		}
		consumed += required
	}

	// If not fully enabled, still consume available tokens and add artificial tokens
	if !enabled {
		// Force fire: consume what's available, count missing
		for placeID, required := range inputPlaces {
			available := marking[placeID]
			if available >= required {
				marking[placeID] -= required
			} else {
				marking[placeID] = 0
			}
		}
	} else {
		// Normal fire: consume all required tokens
		for placeID, required := range inputPlaces {
			marking[placeID] -= required
		}
	}

	// Produce output tokens
	for _, arc := range net.Arcs {
		if arc.Source == transID {
			weight := int(arc.GetWeightSum())
			if weight == 0 {
				weight = 1
			}
			marking[arc.Target] += weight
			produced += weight
		}
	}

	return missing, consumed, produced
}

// hasEndPlace checks if the net has a place named "end".
func hasEndPlace(net *petri.PetriNet) bool {
	_, exists := net.Places["end"]
	return exists
}

// =============================================================================
// Precision Metrics
// =============================================================================

// PrecisionResult contains the results of precision analysis.
type PrecisionResult struct {
	// ETC Precision (Escaping Edges / Total Enabled)
	// Higher is better - means model doesn't allow too many behaviors
	Precision float64

	// Number of escaping edges (transitions enabled but never taken)
	EscapingEdges int

	// Total enabled transitions across all states
	TotalEnabled int

	// Unique states visited during replay
	UniqueStates int
}

// CheckPrecision computes precision metrics using ETC (Escaping Edges) method.
// Precision measures how much behavior the model allows beyond what's in the log.
// A precision of 1.0 means the model only allows observed behavior.
func CheckPrecision(log *eventlog.EventLog, net *petri.PetriNet) *PrecisionResult {
	result := &PrecisionResult{}

	activityToTransition := buildActivityMapping(net)
	initialMarking := getInitialMarking(net)

	// Track enabled transitions at each state
	stateVisits := make(map[string]map[string]bool) // state -> set of taken transitions
	stateEnabled := make(map[string]map[string]bool) // state -> set of enabled transitions

	// Replay all traces and collect state/transition information
	for _, trace := range log.GetTraces() {
		marking := copyState(initialMarking)
		activities := trace.GetActivityVariant()

		for _, activity := range activities {
			// Record current state
			stateKey := markingToKey(marking)

			// Initialize state tracking if needed
			if _, exists := stateVisits[stateKey]; !exists {
				stateVisits[stateKey] = make(map[string]bool)
				stateEnabled[stateKey] = make(map[string]bool)

				// Find all enabled transitions at this state
				for transID := range net.Transitions {
					if isEnabled(net, transID, marking) {
						stateEnabled[stateKey][transID] = true
					}
				}
			}

			// Record which transition was taken
			if transID, ok := activityToTransition[activity]; ok {
				stateVisits[stateKey][transID] = true

				// Fire the transition to update marking
				fireTransitionSilent(net, transID, marking)
			}
		}
	}

	// Calculate precision
	// Escaping edges = enabled but never taken
	for stateKey, enabled := range stateEnabled {
		taken := stateVisits[stateKey]

		for transID := range enabled {
			result.TotalEnabled++
			if !taken[transID] {
				result.EscapingEdges++
			}
		}
	}

	result.UniqueStates = len(stateEnabled)

	// Precision = 1 - (escaping / total enabled)
	if result.TotalEnabled > 0 {
		result.Precision = 1.0 - float64(result.EscapingEdges)/float64(result.TotalEnabled)
	} else {
		result.Precision = 1.0
	}

	return result
}

// markingToKey creates a string key from a marking for state comparison.
func markingToKey(marking TokenState) string {
	// Sort places for consistent key generation
	places := make([]string, 0, len(marking))
	for p := range marking {
		places = append(places, p)
	}
	sort.Strings(places)

	key := ""
	for _, p := range places {
		if marking[p] > 0 {
			key += fmt.Sprintf("%s:%d,", p, marking[p])
		}
	}
	return key
}

// isEnabled checks if a transition is enabled at the current marking.
func isEnabled(net *petri.PetriNet, transID string, marking TokenState) bool {
	for _, arc := range net.Arcs {
		if arc.Target == transID {
			weight := int(arc.GetWeightSum())
			if weight == 0 {
				weight = 1
			}
			if marking[arc.Source] < weight {
				return false
			}
		}
	}
	return true
}

// fireTransitionSilent fires a transition without tracking metrics.
func fireTransitionSilent(net *petri.PetriNet, transID string, marking TokenState) {
	// Consume from input places
	for _, arc := range net.Arcs {
		if arc.Target == transID {
			weight := int(arc.GetWeightSum())
			if weight == 0 {
				weight = 1
			}
			if marking[arc.Source] >= weight {
				marking[arc.Source] -= weight
			}
		}
	}

	// Produce to output places
	for _, arc := range net.Arcs {
		if arc.Source == transID {
			weight := int(arc.GetWeightSum())
			if weight == 0 {
				weight = 1
			}
			marking[arc.Target] += weight
		}
	}
}

// =============================================================================
// Combined Conformance Analysis
// =============================================================================

// FullConformanceResult contains all conformance metrics.
type FullConformanceResult struct {
	Fitness   *ConformanceResult
	Precision *PrecisionResult

	// F-Score (harmonic mean of fitness and precision)
	FScore float64
}

// CheckFullConformance performs both fitness and precision checking.
func CheckFullConformance(log *eventlog.EventLog, net *petri.PetriNet) *FullConformanceResult {
	fitness := CheckConformance(log, net)
	precision := CheckPrecision(log, net)

	result := &FullConformanceResult{
		Fitness:   fitness,
		Precision: precision,
	}

	// Calculate F-Score (harmonic mean)
	if fitness.Fitness+precision.Precision > 0 {
		result.FScore = 2 * fitness.Fitness * precision.Precision / (fitness.Fitness + precision.Precision)
	}

	return result
}

// =============================================================================
// Utility Functions
// =============================================================================

// String returns a human-readable summary of conformance results.
func (r *ConformanceResult) String() string {
	return fmt.Sprintf(
		"Conformance Results:\n"+
			"  Fitness: %.2f%%\n"+
			"  Fitting traces: %d/%d (%.1f%%)\n"+
			"  Avg trace fitness: %.2f%%\n"+
			"  Missing tokens: %d\n"+
			"  Remaining tokens: %d\n",
		r.Fitness*100,
		r.FittingTraces, r.TotalTraces, r.FittingPercent,
		r.AvgTraceFitness*100,
		r.MissingTokens,
		r.RemainingTokens,
	)
}

// String returns a human-readable summary of precision results.
func (r *PrecisionResult) String() string {
	return fmt.Sprintf(
		"Precision Results:\n"+
			"  Precision: %.2f%%\n"+
			"  Escaping edges: %d/%d\n"+
			"  Unique states: %d\n",
		r.Precision*100,
		r.EscapingEdges, r.TotalEnabled,
		r.UniqueStates,
	)
}

// String returns a human-readable summary of full conformance results.
func (r *FullConformanceResult) String() string {
	return fmt.Sprintf(
		"%s\n%s"+
			"F-Score: %.2f%%\n",
		r.Fitness.String(),
		r.Precision.String(),
		r.FScore*100,
	)
}

// GetNonFittingTraces returns traces that don't fit the model.
func (r *ConformanceResult) GetNonFittingTraces() []TraceReplayResult {
	result := make([]TraceReplayResult, 0)
	for _, tr := range r.TraceResults {
		if !tr.Fitting {
			result = append(result, tr)
		}
	}
	return result
}

// GetTracesByFitness returns traces sorted by fitness (lowest first).
func (r *ConformanceResult) GetTracesByFitness() []TraceReplayResult {
	result := make([]TraceReplayResult, len(r.TraceResults))
	copy(result, r.TraceResults)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Fitness < result[j].Fitness
	})
	return result
}
