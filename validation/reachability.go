package validation

import (
	"fmt"
	"sort"
	"strings"
)

// Marking represents a state of the Petri net (token distribution)
type Marking map[string]float64

// String returns a canonical string representation
func (m Marking) String() string {
	// Sort keys for consistent representation
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%.2f", k, m[k]))
	}
	return strings.Join(parts, ",")
}

// Copy creates a copy of the marking
func (m Marking) Copy() Marking {
	result := make(Marking, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// Equals checks if two markings are equal
func (m Marking) Equals(other Marking) bool {
	if len(m) != len(other) {
		return false
	}
	for k, v := range m {
		if other[k] != v {
			return false
		}
	}
	return true
}

// ReachabilityGraph represents the state space
type ReachabilityGraph struct {
	InitialMarking Marking
	States         map[string]*State
	StateCount     int
	MaxDepth       int
	Bounded        bool
	BoundLimit     int // For terminating unbounded exploration
}

// State represents a node in the reachability graph
type State struct {
	Marking      Marking
	ID           int
	EnabledTrans []string
	IsTerminal   bool
	IsDeadlock   bool
}

// ReachabilityResult contains analysis results
type ReachabilityResult struct {
	Reachable       int            `json:"reachable"`
	Bounded         bool           `json:"bounded"`
	MaxTokens       map[string]int `json:"maxTokens"`
	TerminalStates  []string       `json:"terminalStates"`
	DeadlockStates  []string       `json:"deadlockStates"`
	HasCycles       bool           `json:"hasCycles"`
	MaxDepth        int            `json:"maxDepth"`
	Truncated       bool           `json:"truncated"`
	TruncatedReason string         `json:"truncatedReason,omitempty"`
}

// AnalyzeReachability performs reachability analysis
func (v *Validator) AnalyzeReachability(maxStates int) *ReachabilityResult {
	// Get initial marking
	initial := make(Marking)
	for name, place := range v.net.Places {
		initial[name] = place.GetTokenCount()
	}

	// Build reachability graph
	graph := &ReachabilityGraph{
		InitialMarking: initial,
		States:         make(map[string]*State),
		Bounded:        true,
		BoundLimit:     1000, // Safety limit for unbounded nets
	}

	// Explore state space
	truncated := false
	truncatedReason := ""

	queue := []Marking{initial}
	visited := make(map[string]bool)
	depth := 0

	for len(queue) > 0 && len(graph.States) < maxStates {
		// Process next marking
		current := queue[0]
		queue = queue[1:]

		key := current.String()
		if visited[key] {
			continue
		}
		visited[key] = true

		// Create state
		state := &State{
			Marking: current.Copy(),
			ID:      len(graph.States),
		}

		// Find enabled transitions
		enabled := v.findEnabledTransitions(current)
		state.EnabledTrans = enabled
		state.IsTerminal = len(enabled) == 0
		state.IsDeadlock = state.IsTerminal && !v.isGoalState(current)

		graph.States[key] = state
		graph.StateCount++

		// Fire each enabled transition
		for _, trans := range enabled {
			newMarking := v.fireTransition(current, trans)
			if newMarking != nil {
				// Check for unboundedness
				if v.exceedsLimit(newMarking, graph.BoundLimit) {
					graph.Bounded = false
					truncated = true
					truncatedReason = "unbounded net detected"
					break
				}

				queue = append(queue, newMarking)
			}
		}

		if truncated {
			break
		}

		depth++
		if depth > graph.MaxDepth {
			graph.MaxDepth = depth
		}
	}

	// Check if we hit the state limit
	if len(graph.States) >= maxStates {
		truncated = true
		truncatedReason = fmt.Sprintf("state limit reached (%d states)", maxStates)
	}

	// Analyze results
	result := &ReachabilityResult{
		Reachable:       len(graph.States),
		Bounded:         graph.Bounded,
		MaxTokens:       v.computeMaxTokens(graph),
		TerminalStates:  v.findTerminalStates(graph),
		DeadlockStates:  v.findDeadlockStates(graph),
		HasCycles:       v.detectCycles(graph),
		MaxDepth:        graph.MaxDepth,
		Truncated:       truncated,
		TruncatedReason: truncatedReason,
	}

	return result
}

// findEnabledTransitions returns transitions that can fire in the given marking
func (v *Validator) findEnabledTransitions(marking Marking) []string {
	var enabled []string

	for transName := range v.net.Transitions {
		if v.isEnabled(marking, transName) {
			enabled = append(enabled, transName)
		}
	}

	return enabled
}

// isEnabled checks if a transition can fire
func (v *Validator) isEnabled(marking Marking, transName string) bool {
	// Check all input arcs
	for _, arc := range v.net.Arcs {
		if arc.Target == transName {
			// Arc from place to transition (input)
			tokens := marking[arc.Source]
			required := arc.GetWeightSum()

			if tokens < required {
				return false
			}

			// Check inhibitor arcs
			if arc.InhibitTransition && tokens > 0 {
				return false
			}
		}
	}

	// Check output place capacities
	for _, arc := range v.net.Arcs {
		if arc.Source == transName {
			// Arc from transition to place (output)
			place := v.net.Places[arc.Target]
			if len(place.Capacity) > 0 {
				currentTokens := marking[arc.Target]
				capacity := 0.0
				for _, c := range place.Capacity {
					capacity += c
				}
				addedTokens := arc.GetWeightSum()

				if currentTokens+addedTokens > capacity {
					return false
				}
			}
		}
	}

	return true
}

// fireTransition returns new marking after firing transition
func (v *Validator) fireTransition(marking Marking, transName string) Marking {
	if !v.isEnabled(marking, transName) {
		return nil
	}

	newMarking := marking.Copy()

	// Remove tokens from input places
	for _, arc := range v.net.Arcs {
		if arc.Target == transName {
			newMarking[arc.Source] -= arc.GetWeightSum()
		}
	}

	// Add tokens to output places
	for _, arc := range v.net.Arcs {
		if arc.Source == transName {
			newMarking[arc.Target] += arc.GetWeightSum()
		}
	}

	return newMarking
}

// exceedsLimit checks if any place has too many tokens
func (v *Validator) exceedsLimit(marking Marking, limit int) bool {
	for _, tokens := range marking {
		if int(tokens) > limit {
			return true
		}
	}
	return false
}

// isGoalState checks if marking represents a valid final state
func (v *Validator) isGoalState(marking Marking) bool {
	// A goal state typically has all tokens in designated output/sink places
	// For now, just check if it's the zero marking (all tokens consumed)
	for _, tokens := range marking {
		if tokens > 0 {
			return false
		}
	}
	return true
}

// computeMaxTokens finds maximum tokens in each place across all states
func (v *Validator) computeMaxTokens(graph *ReachabilityGraph) map[string]int {
	maxTokens := make(map[string]int)

	for _, state := range graph.States {
		for place, tokens := range state.Marking {
			t := int(tokens)
			if t > maxTokens[place] {
				maxTokens[place] = t
			}
		}
	}

	return maxTokens
}

// findTerminalStates returns states with no enabled transitions
func (v *Validator) findTerminalStates(graph *ReachabilityGraph) []string {
	var terminal []string

	for key, state := range graph.States {
		if state.IsTerminal {
			terminal = append(terminal, key)
		}
	}

	return terminal
}

// findDeadlockStates returns terminal states that are not goal states
func (v *Validator) findDeadlockStates(graph *ReachabilityGraph) []string {
	var deadlocks []string

	for key, state := range graph.States {
		if state.IsDeadlock {
			deadlocks = append(deadlocks, key)
		}
	}

	return deadlocks
}

// detectCycles checks if there are cycles in the reachability graph
func (v *Validator) detectCycles(graph *ReachabilityGraph) bool {
	// Simple heuristic: if we have multiple states with the same set of enabled transitions,
	// there's likely a cycle. More sophisticated: check if initial marking is reachable from any state.

	// For now, just check if any non-initial state can reach another state with same marking
	// This is a simplified check - true cycle detection would require DFS

	// If we have more than one state and not all are terminal, likely has cycles
	hasNonTerminal := false
	for _, state := range graph.States {
		if !state.IsTerminal {
			hasNonTerminal = true
			break
		}
	}

	return len(graph.States) > 1 && hasNonTerminal
}
