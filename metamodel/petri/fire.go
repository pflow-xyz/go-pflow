package petri

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/metamodel/guard"
)

// Marking represents the current token state of all places.
type Marking map[string]int

// Clone creates a deep copy of the marking.
func (m Marking) Clone() Marking {
	clone := make(Marking)
	for k, v := range m {
		clone[k] = v
	}
	return clone
}

// Bindings represent variable bindings for colored/parameterized transitions.
type Bindings map[string]interface{}

// State holds the runtime state of a Petri net execution.
type State struct {
	Model           *Model
	Marking         Marking
	Sequence        uint64
	CheckInvariants bool // If true, check invariants after each Fire (default: true)
}

// NewState creates a new execution state from a model.
func NewState(m *Model) *State {
	marking := make(Marking)
	for _, p := range m.Places {
		marking[p.ID] = p.Initial
	}
	return &State{
		Model:           m,
		Marking:         marking,
		Sequence:        0,
		CheckInvariants: true, // Auto-check by default
	}
}

// Clone creates a deep copy of the state.
func (s *State) Clone() *State {
	marking := make(Marking)
	for k, v := range s.Marking {
		marking[k] = v
	}
	return &State{
		Model:           s.Model,
		Marking:         marking,
		Sequence:        s.Sequence,
		CheckInvariants: s.CheckInvariants,
	}
}

// Tokens returns the token count at a place.
func (s *State) Tokens(placeID string) int {
	return s.Marking[placeID]
}

// SetTokens sets the token count at a place.
func (s *State) SetTokens(placeID string, count int) {
	s.Marking[placeID] = count
}

// Enabled returns true if a transition can fire.
func (s *State) Enabled(transitionID string) bool {
	t := s.Model.TransitionByID(transitionID)
	if t == nil {
		return false
	}

	// Check all input arcs have sufficient tokens (all arcs have weight 1)
	// Skip keyed arcs - their validation is handled by guard expressions
	for _, arc := range s.Model.InputArcs(transitionID) {
		if len(arc.Keys) > 0 {
			// Keyed arcs use map-based access; guard expression validates
			continue
		}
		if s.Marking[arc.Source] < 1 {
			return false
		}
	}

	return true
}

// EnabledTransitions returns all transitions that can fire.
func (s *State) EnabledTransitions() []string {
	var enabled []string
	for _, t := range s.Model.Transitions {
		if s.Enabled(t.ID) {
			enabled = append(enabled, t.ID)
		}
	}
	return enabled
}

// Fire executes a transition, consuming and producing tokens.
// If CheckInvariants is true, invariants are verified after firing.
func (s *State) Fire(transitionID string) error {
	if !s.Enabled(transitionID) {
		return ErrTransitionNotEnabled
	}

	// Consume tokens from input places (all arcs have weight 1)
	// Skip keyed arcs - their state is managed by the engine layer
	for _, arc := range s.Model.InputArcs(transitionID) {
		if len(arc.Keys) > 0 {
			continue
		}
		s.Marking[arc.Source]--
	}

	// Produce tokens at output places
	// Skip keyed arcs - their state is managed by the engine layer
	for _, arc := range s.Model.OutputArcs(transitionID) {
		if len(arc.Keys) > 0 {
			continue
		}
		s.Marking[arc.Target]++
	}

	s.Sequence++

	// Check invariants if enabled
	if s.CheckInvariants {
		if violations := s.Invariants(); len(violations) > 0 {
			v := violations[0]
			if v.Err != nil {
				return fmt.Errorf("%w: %s: %v", ErrInvariantEvaluation, v.Invariant.ID, v.Err)
			}
			return fmt.Errorf("%w: %s", ErrInvariantViolated, v.Invariant.ID)
		}
	}

	return nil
}

// Invariants checks all model invariants against the current marking.
// Returns a slice of violations (empty if all invariants hold).
func (s *State) Invariants() []InvariantViolation {
	var violations []InvariantViolation

	for _, inv := range s.Model.Invariants {
		// Convert petri.Marking to guard.Marking
		guardMarking := make(guard.Marking)
		for k, v := range s.Marking {
			guardMarking[k] = v
		}

		ok, err := guard.EvaluateInvariant(inv.Expr, guardMarking)
		if err != nil {
			violations = append(violations, InvariantViolation{
				Invariant: inv,
				Marking:   s.Marking.Clone(),
				Err:       err,
			})
		} else if !ok {
			violations = append(violations, InvariantViolation{
				Invariant: inv,
				Marking:   s.Marking.Clone(),
				Err:       nil,
			})
		}
	}

	return violations
}

// FireWithBindings executes a transition with variable bindings.
// This is used for colored/parameterized Petri nets.
func (s *State) FireWithBindings(transitionID string, bindings Bindings) error {
	t := s.Model.TransitionByID(transitionID)
	if t == nil {
		return ErrTransitionNotFound
	}

	// Evaluate guard if present
	if t.Guard != "" {
		ok, err := guard.Evaluate(t.Guard, bindings, nil)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrGuardEvaluation, err)
		}
		if !ok {
			return ErrGuardNotSatisfied
		}
	}

	return s.Fire(transitionID)
}

// FireWithGuardFuncs executes a transition with bindings and custom guard functions.
func (s *State) FireWithGuardFuncs(transitionID string, bindings Bindings, funcs map[string]guard.GuardFunc) error {
	t := s.Model.TransitionByID(transitionID)
	if t == nil {
		return ErrTransitionNotFound
	}

	// Evaluate guard if present
	if t.Guard != "" {
		ok, err := guard.Evaluate(t.Guard, bindings, funcs)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrGuardEvaluation, err)
		}
		if !ok {
			return ErrGuardNotSatisfied
		}
	}

	return s.Fire(transitionID)
}

// CanReach returns true if the target marking is reachable from current state.
// This is a simple BFS; complex reachability requires more sophisticated analysis.
func (s *State) CanReach(target Marking, maxSteps int) bool {
	visited := make(map[string]bool)
	queue := []*State{s.Clone()}

	for len(queue) > 0 && maxSteps > 0 {
		current := queue[0]
		queue = queue[1:]
		maxSteps--

		key := current.markingKey()
		if visited[key] {
			continue
		}
		visited[key] = true

		if current.matchesMarking(target) {
			return true
		}

		for _, tid := range current.EnabledTransitions() {
			next := current.Clone()
			next.Fire(tid)
			queue = append(queue, next)
		}
	}

	return false
}

func (s *State) markingKey() string {
	// Simple serialization for visited set
	result := ""
	for _, p := range s.Model.Places {
		result += p.ID + ":" + string(rune(s.Marking[p.ID])) + ";"
	}
	return result
}

func (s *State) matchesMarking(target Marking) bool {
	for k, v := range target {
		if s.Marking[k] != v {
			return false
		}
	}
	return true
}
