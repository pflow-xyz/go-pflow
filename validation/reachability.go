package validation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/reachability"
)

// Marking represents a state of the Petri net (token distribution).
//
// Deprecated: this float-keyed marking exists for backward compatibility with
// the validation package's original API. New code should use
// reachability.Marking, which is integer-based and shared with the analysis
// engine. Discrete state-space exploration is inherently integral — tokens do
// not come in halves — and mixing the two representations was the source of
// rounding discrepancies between this package and reachability.
type Marking map[string]float64

// String returns a canonical string representation.
func (m Marking) String() string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%.2f", k, m[k]))
	}
	return strings.Join(parts, ",")
}

// Copy creates a copy of the marking.
func (m Marking) Copy() Marking {
	result := make(Marking, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// Equals checks if two markings are equal.
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

// ReachabilityResult contains analysis results.
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

// AnalyzeReachability performs reachability analysis on the validator's net.
//
// This delegates to the reachability package rather than exploring the state
// space itself. The package previously carried a second, float-based
// implementation of enabling, firing, and BFS exploration; the two drifted, and
// the copy here had a cycle detector that reported "has cycles" for any net
// with more than one state and at least one non-terminal state — true of every
// acyclic net with a transition. Sharing one engine removes that class of
// divergence, and this package keeps only the presentation shape.
func (v *Validator) AnalyzeReachability(maxStates int) *ReachabilityResult {
	analyzer := reachability.NewAnalyzer(v.net).WithMaxStates(maxStates)
	res := analyzer.Analyze()

	result := &ReachabilityResult{
		Reachable: res.StateCount,
		Bounded:   res.Bounded,
		MaxTokens: res.MaxTokens,
		HasCycles: res.HasCycle,
		MaxDepth:  res.MaxDepth,
		Truncated: res.Truncated,
	}
	if result.MaxTokens == nil {
		result.MaxTokens = map[string]int{}
	}

	// A covering witness is a definite proof of unboundedness, and finding one
	// does not require exhausting the state limit. Without this, an unbounded
	// net typically truncates first and is reported as merely "incomplete".
	if result.Bounded {
		if w := reachability.NewAnalyzer(v.net).
			WithMaxStates(maxStates).
			FindUnboundedWitness(); w != nil {
			result.Bounded = false
			result.TruncatedReason = fmt.Sprintf("unbounded: %s grow without bound",
				strings.Join(w.Places, ", "))
		}
	}

	if res.Truncated && result.TruncatedReason == "" {
		result.TruncatedReason = res.TruncateMsg
		if result.TruncatedReason == "" {
			result.TruncatedReason = fmt.Sprintf("state limit of %d reached", maxStates)
		}
	}

	for _, state := range res.Graph.States {
		if state.IsTerminal {
			result.TerminalStates = append(result.TerminalStates, state.Marking.String())
		}
		if state.IsDeadlock {
			result.DeadlockStates = append(result.DeadlockStates, state.Marking.String())
		}
	}

	// Map iteration order is random; sort so output is reproducible.
	sort.Strings(result.TerminalStates)
	sort.Strings(result.DeadlockStates)

	return result
}
