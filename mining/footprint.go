package mining

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/eventlog"
)

// Relation represents the ordering relation between two activities.
type Relation int

const (
	// NoRelation means the activities never directly follow each other
	NoRelation Relation = iota
	// DirectlyFollows means a > b (a directly followed by b at least once)
	DirectlyFollows
	// Causality means a -> b (a causes b: a > b and not b > a)
	Causality
	// ReverseCausality means a <- b (b causes a)
	ReverseCausality
	// Parallel means a || b (both orderings exist: a > b and b > a)
	Parallel
	// Choice means a # b (neither ordering exists, exclusive choice)
	Choice
)

// String returns the symbol for the relation.
func (r Relation) String() string {
	switch r {
	case NoRelation:
		return "#"
	case DirectlyFollows:
		return ">"
	case Causality:
		return "→"
	case ReverseCausality:
		return "←"
	case Parallel:
		return "||"
	case Choice:
		return "#"
	default:
		return "?"
	}
}

// FootprintMatrix represents the log-based ordering relations between activities.
// This is the foundation for the Alpha Miner algorithm.
type FootprintMatrix struct {
	Activities  []string                  // Ordered list of activities
	activityIdx map[string]int            // Activity name to index
	follows     map[string]map[string]int // a -> b -> count (directly follows)
	StartSet    map[string]bool           // Activities that start traces
	EndSet      map[string]bool           // Activities that end traces
}

// NewFootprintMatrix creates a footprint matrix from an event log.
func NewFootprintMatrix(log *eventlog.EventLog) *FootprintMatrix {
	fp := &FootprintMatrix{
		Activities:  log.GetActivities(),
		activityIdx: make(map[string]int),
		follows:     make(map[string]map[string]int),
		StartSet:    make(map[string]bool),
		EndSet:      make(map[string]bool),
	}

	// Build activity index
	for i, act := range fp.Activities {
		fp.activityIdx[act] = i
		fp.follows[act] = make(map[string]int)
	}

	// Extract directly-follows relations and start/end activities
	for _, trace := range log.GetTraces() {
		if len(trace.Events) == 0 {
			continue
		}

		// Start activity
		fp.StartSet[trace.Events[0].Activity] = true

		// End activity
		fp.EndSet[trace.Events[len(trace.Events)-1].Activity] = true

		// Directly-follows relations
		for i := 0; i < len(trace.Events)-1; i++ {
			a := trace.Events[i].Activity
			b := trace.Events[i+1].Activity
			fp.follows[a][b]++
		}
	}

	return fp
}

// DirectlyFollows returns true if activity a is directly followed by b at least once.
func (fp *FootprintMatrix) DirectlyFollows(a, b string) bool {
	if follows, ok := fp.follows[a]; ok {
		return follows[b] > 0
	}
	return false
}

// DirectlyFollowsCount returns the number of times a is directly followed by b.
func (fp *FootprintMatrix) DirectlyFollowsCount(a, b string) int {
	if follows, ok := fp.follows[a]; ok {
		return follows[b]
	}
	return 0
}

// GetRelation returns the ordering relation between two activities.
func (fp *FootprintMatrix) GetRelation(a, b string) Relation {
	aFollowsB := fp.DirectlyFollows(a, b)
	bFollowsA := fp.DirectlyFollows(b, a)

	if aFollowsB && bFollowsA {
		return Parallel
	} else if aFollowsB {
		return Causality
	} else if bFollowsA {
		return ReverseCausality
	}
	return Choice
}

// IsCausal returns true if a -> b (a causes b).
func (fp *FootprintMatrix) IsCausal(a, b string) bool {
	return fp.DirectlyFollows(a, b) && !fp.DirectlyFollows(b, a)
}

// IsParallel returns true if a || b (activities can occur in any order).
func (fp *FootprintMatrix) IsParallel(a, b string) bool {
	return fp.DirectlyFollows(a, b) && fp.DirectlyFollows(b, a)
}

// IsChoice returns true if a # b (activities are in exclusive choice).
func (fp *FootprintMatrix) IsChoice(a, b string) bool {
	return !fp.DirectlyFollows(a, b) && !fp.DirectlyFollows(b, a)
}

// GetSuccessors returns all activities that directly follow the given activity.
func (fp *FootprintMatrix) GetSuccessors(a string) []string {
	var successors []string
	if follows, ok := fp.follows[a]; ok {
		for b := range follows {
			successors = append(successors, b)
		}
	}
	sort.Strings(successors)
	return successors
}

// GetPredecessors returns all activities that directly precede the given activity.
func (fp *FootprintMatrix) GetPredecessors(b string) []string {
	var predecessors []string
	for a, follows := range fp.follows {
		if follows[b] > 0 {
			predecessors = append(predecessors, a)
		}
	}
	sort.Strings(predecessors)
	return predecessors
}

// GetCausalSuccessors returns activities b where a -> b (causal relation).
func (fp *FootprintMatrix) GetCausalSuccessors(a string) []string {
	var successors []string
	for _, b := range fp.GetSuccessors(a) {
		if fp.IsCausal(a, b) {
			successors = append(successors, b)
		}
	}
	return successors
}

// GetCausalPredecessors returns activities a where a -> b (causal relation).
func (fp *FootprintMatrix) GetCausalPredecessors(b string) []string {
	var predecessors []string
	for _, a := range fp.GetPredecessors(b) {
		if fp.IsCausal(a, b) {
			predecessors = append(predecessors, a)
		}
	}
	return predecessors
}

// GetStartActivities returns activities that start at least one trace.
func (fp *FootprintMatrix) GetStartActivities() []string {
	var starts []string
	for a := range fp.StartSet {
		starts = append(starts, a)
	}
	sort.Strings(starts)
	return starts
}

// GetEndActivities returns activities that end at least one trace.
func (fp *FootprintMatrix) GetEndActivities() []string {
	var ends []string
	for a := range fp.EndSet {
		ends = append(ends, a)
	}
	sort.Strings(ends)
	return ends
}

// String returns a formatted representation of the footprint matrix.
func (fp *FootprintMatrix) String() string {
	var sb strings.Builder

	// Header
	sb.WriteString("Footprint Matrix:\n")
	sb.WriteString("     ")
	for _, b := range fp.Activities {
		sb.WriteString(fmt.Sprintf("%4s", truncate(b, 4)))
	}
	sb.WriteString("\n")

	// Rows
	for _, a := range fp.Activities {
		sb.WriteString(fmt.Sprintf("%4s ", truncate(a, 4)))
		for _, b := range fp.Activities {
			rel := fp.GetRelation(a, b)
			sb.WriteString(fmt.Sprintf("%4s", rel.String()))
		}
		sb.WriteString("\n")
	}

	// Start/End sets
	sb.WriteString(fmt.Sprintf("\nStart activities: %v\n", fp.GetStartActivities()))
	sb.WriteString(fmt.Sprintf("End activities: %v\n", fp.GetEndActivities()))

	return sb.String()
}

// Print prints the footprint matrix to stdout.
func (fp *FootprintMatrix) Print() {
	fmt.Print(fp.String())
}

// truncate truncates a string to max length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// SetIsUnrelated checks if all pairs of activities in a set are unrelated (choice relation).
// This is used by Alpha Miner to verify candidate place sets.
func (fp *FootprintMatrix) SetIsUnrelated(activities []string) bool {
	for i := 0; i < len(activities); i++ {
		for j := i + 1; j < len(activities); j++ {
			if !fp.IsChoice(activities[i], activities[j]) {
				return false
			}
		}
	}
	return true
}

// SetsCausallyConnected checks if all activities in setA causally precede all in setB.
func (fp *FootprintMatrix) SetsCausallyConnected(setA, setB []string) bool {
	for _, a := range setA {
		for _, b := range setB {
			if !fp.IsCausal(a, b) {
				return false
			}
		}
	}
	return true
}
