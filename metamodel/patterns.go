// Package metamodel provides composable higher-level patterns built on Petri nets.
// These patterns provide reusable abstractions for common use cases.
package metamodel

import (
	"fmt"
)

// ============================================================================
// StateMachine Pattern
// ============================================================================

// StateMachine provides a finite state machine built on a Petri net.
// Each state is a place with a token, and transitions move the token
// between states.
type StateMachine[S comparable] struct {
	net     *PetriNet[TokenState[S]]
	current S
}

// NewStateMachine creates a new state machine with the given initial state.
func NewStateMachine[S comparable](name string, initial S) *StateMachine[S] {
	net := NewPetriNet[TokenState[S]](name)
	// Use a simple state ID format and store the metadata
	initialID := stateID(initial)
	net.AddPlace(NewGenericPlace(initialID, NewTokenState(1, initial)))
	return &StateMachine[S]{
		net:     net,
		current: initial,
	}
}

// stateID generates a consistent ID for a state value.
func stateID[S any](s S) string {
	return fmt.Sprintf("%v", s)
}

// AddState adds a state to the state machine.
func (sm *StateMachine[S]) AddState(state S) *StateMachine[S] {
	id := stateID(state)
	if sm.net.PlaceByID(id) == nil {
		// Store the state value in the metadata so we can retrieve it later
		sm.net.AddPlace(NewGenericPlace(id, NewTokenState(0, state)))
	}
	return sm
}

// AddTransition adds a transition between states.
func (sm *StateMachine[S]) AddTransition(id string, from, to S) *StateMachine[S] {
	fromID := stateID(from)
	toID := stateID(to)

	// Ensure states exist
	sm.AddState(from)
	sm.AddState(to)

	// Add transition
	trans := NewGenericTransition[TokenState[S], TokenState[S]](id)
	sm.net.AddTransition(trans)

	// Add arcs
	sm.net.AddArc(NewGenericArc[TokenState[S]](fromID, id))
	sm.net.AddArc(NewGenericArc[TokenState[S]](id, toID))

	return sm
}

// AddGuardedTransition adds a transition with a guard condition.
func (sm *StateMachine[S]) AddGuardedTransition(id string, from, to S, guard func(S) bool, guardExpr string) *StateMachine[S] {
	fromID := stateID(from)
	toID := stateID(to)

	// Ensure states exist
	sm.AddState(from)
	sm.AddState(to)

	// Add transition with guard
	trans := NewGenericTransition[TokenState[S], TokenState[S]](id).
		WithGuard(func(ts TokenState[S]) bool {
			return guard(ts.Metadata)
		}, guardExpr)
	sm.net.AddTransition(trans)

	// Add arcs
	sm.net.AddArc(NewGenericArc[TokenState[S]](fromID, id))
	sm.net.AddArc(NewGenericArc[TokenState[S]](id, toID))

	return sm
}

// Current returns the current state.
func (sm *StateMachine[S]) Current() S {
	return sm.current
}

// CanTransition returns true if the state machine can transition to the given state.
func (sm *StateMachine[S]) CanTransition(transitionID string) bool {
	// Find the transition
	trans := sm.net.TransitionByID(transitionID)
	if trans == nil {
		return false
	}

	// Check if current state has outgoing arc to this transition
	currentID := stateID(sm.current)
	for _, arc := range sm.net.InputArcs(transitionID) {
		if arc.From == currentID {
			// Check guard
			currentPlace := sm.net.PlaceByID(currentID)
			if currentPlace != nil {
				return trans.CanFire(currentPlace.Initial)
			}
		}
	}
	return false
}

// Transition attempts to fire a transition and move to a new state.
func (sm *StateMachine[S]) Transition(transitionID string) error {
	if !sm.CanTransition(transitionID) {
		return fmt.Errorf("cannot transition via %s from state %v", transitionID, sm.current)
	}

	// Find the target state by looking at output arcs
	for _, arc := range sm.net.OutputArcs(transitionID) {
		place := sm.net.PlaceByID(arc.To)
		if place != nil {
			// The metadata of the place contains the state value
			sm.current = place.Initial.Metadata
			return nil
		}
	}

	return fmt.Errorf("transition %s has no output state", transitionID)
}

// AvailableTransitions returns all transitions that can be fired from current state.
func (sm *StateMachine[S]) AvailableTransitions() []string {
	var available []string
	for _, trans := range sm.net.Transitions {
		if sm.CanTransition(trans.ID) {
			available = append(available, trans.ID)
		}
	}
	return available
}

// Net returns the underlying Petri net.
func (sm *StateMachine[S]) Net() *PetriNet[TokenState[S]] {
	return sm.net
}

// ============================================================================
// Workflow Pattern
// ============================================================================

// WorkflowStep defines a step in a workflow with data transformation.
type WorkflowStep[D any] struct {
	TransitionID string
	Transform    func(D) D
	Validate     func(D) error
}

// Workflow provides a multi-step process with data flow.
// D is the type of data that flows through the workflow.
type Workflow[D any] struct {
	net   *PetriNet[DataState[D]]
	data  D
	steps map[string]WorkflowStep[D]
}

// NewWorkflow creates a new workflow with initial data.
func NewWorkflow[D any](name string, initial D) *Workflow[D] {
	net := NewPetriNet[DataState[D]](name)
	net.AddPlace(NewGenericPlace("start", NewDataState(initial)))

	return &Workflow[D]{
		net:   net,
		data:  initial,
		steps: make(map[string]WorkflowStep[D]),
	}
}

// AddStep adds a step to the workflow.
func (w *Workflow[D]) AddStep(id, fromPlace, toPlace string, step WorkflowStep[D]) *Workflow[D] {
	step.TransitionID = id

	// Ensure places exist
	if w.net.PlaceByID(fromPlace) == nil {
		var zero D
		w.net.AddPlace(NewGenericPlace(fromPlace, NewDataState(zero)))
	}
	if w.net.PlaceByID(toPlace) == nil {
		var zero D
		w.net.AddPlace(NewGenericPlace(toPlace, NewDataState(zero)))
	}

	// Add transition
	trans := NewGenericTransition[DataState[D], DataState[D]](id)
	if step.Transform != nil {
		trans = trans.WithAction(func(ds DataState[D]) DataState[D] {
			return ds.Transform(step.Transform)
		})
	}
	w.net.AddTransition(trans)

	// Add arcs
	w.net.AddArc(NewGenericArc[DataState[D]](fromPlace, id))
	w.net.AddArc(NewGenericArc[DataState[D]](id, toPlace))

	w.steps[id] = step
	return w
}

// Execute runs a workflow step, transforming and validating data.
func (w *Workflow[D]) Execute(stepID string) error {
	step, ok := w.steps[stepID]
	if !ok {
		return fmt.Errorf("unknown workflow step: %s", stepID)
	}

	// Validate
	if step.Validate != nil {
		if err := step.Validate(w.data); err != nil {
			return fmt.Errorf("validation failed for step %s: %w", stepID, err)
		}
	}

	// Transform
	if step.Transform != nil {
		w.data = step.Transform(w.data)
	}

	return nil
}

// Data returns the current workflow data.
func (w *Workflow[D]) Data() D {
	return w.data
}

// SetData updates the workflow data.
func (w *Workflow[D]) SetData(data D) {
	w.data = data
}

// Net returns the underlying Petri net.
func (w *Workflow[D]) Net() *PetriNet[DataState[D]] {
	return w.net
}

// ============================================================================
// ResourcePool Pattern
// ============================================================================

// ResourcePool provides token-based resource management.
// R is the type of resource metadata.
type ResourcePool[R any] struct {
	net         *PetriNet[TokenState[R]]
	available   string // place ID for available resources
	inUse       string // place ID for in-use resources
	total       int    // total resources in pool
	acquireID   string // transition ID for acquire
	releaseID   string // transition ID for release
}

// NewResourcePool creates a new resource pool with the given capacity.
func NewResourcePool[R any](name string, capacity int, metadata R) *ResourcePool[R] {
	net := NewPetriNet[TokenState[R]](name)

	// Add places
	availablePlace := NewGenericPlace("available", NewTokenState(capacity, metadata)).
		WithCapacity(capacity)
	net.AddPlace(availablePlace)

	inUsePlace := NewGenericPlace("in_use", NewEmptyTokenState[R]()).
		WithCapacity(capacity)
	net.AddPlace(inUsePlace)

	// Add acquire transition
	acquire := NewGenericTransition[TokenState[R], TokenState[R]]("acquire")
	net.AddTransition(acquire)
	net.AddArc(NewGenericArc[TokenState[R]]("available", "acquire"))
	net.AddArc(NewGenericArc[TokenState[R]]("acquire", "in_use"))

	// Add release transition
	release := NewGenericTransition[TokenState[R], TokenState[R]]("release")
	net.AddTransition(release)
	net.AddArc(NewGenericArc[TokenState[R]]("in_use", "release"))
	net.AddArc(NewGenericArc[TokenState[R]]("release", "available"))

	return &ResourcePool[R]{
		net:       net,
		available: "available",
		inUse:     "in_use",
		total:     capacity,
		acquireID: "acquire",
		releaseID: "release",
	}
}

// Available returns the number of available resources.
func (rp *ResourcePool[R]) Available() int {
	place := rp.net.PlaceByID(rp.available)
	if place != nil {
		return place.Initial.Count
	}
	return 0
}

// InUse returns the number of resources currently in use.
func (rp *ResourcePool[R]) InUse() int {
	place := rp.net.PlaceByID(rp.inUse)
	if place != nil {
		return place.Initial.Count
	}
	return 0
}

// Total returns the total capacity of the pool.
func (rp *ResourcePool[R]) Total() int {
	return rp.total
}

// CanAcquire returns true if a resource can be acquired.
func (rp *ResourcePool[R]) CanAcquire() bool {
	return rp.Available() > 0
}

// Acquire attempts to acquire a resource from the pool.
func (rp *ResourcePool[R]) Acquire() error {
	if !rp.CanAcquire() {
		return fmt.Errorf("no resources available")
	}

	// Update places (simulate firing)
	availablePlace := rp.net.PlaceByID(rp.available)
	inUsePlace := rp.net.PlaceByID(rp.inUse)

	if availablePlace != nil && inUsePlace != nil {
		availablePlace.Initial = availablePlace.Initial.Add(-1)
		inUsePlace.Initial = inUsePlace.Initial.Add(1)
		return nil
	}

	return fmt.Errorf("pool state corrupted")
}

// CanRelease returns true if a resource can be released.
func (rp *ResourcePool[R]) CanRelease() bool {
	return rp.InUse() > 0
}

// Release returns a resource to the pool.
func (rp *ResourcePool[R]) Release() error {
	if !rp.CanRelease() {
		return fmt.Errorf("no resources to release")
	}

	// Update places (simulate firing)
	availablePlace := rp.net.PlaceByID(rp.available)
	inUsePlace := rp.net.PlaceByID(rp.inUse)

	if availablePlace != nil && inUsePlace != nil {
		inUsePlace.Initial = inUsePlace.Initial.Add(-1)
		availablePlace.Initial = availablePlace.Initial.Add(1)
		return nil
	}

	return fmt.Errorf("pool state corrupted")
}

// Net returns the underlying Petri net.
func (rp *ResourcePool[R]) Net() *PetriNet[TokenState[R]] {
	return rp.net
}

// ============================================================================
// Event Sourcing Pattern
// ============================================================================

// EventSourced provides event-driven state with replay capability.
// S is the state type, E is the event type.
type EventSourced[S, E any] struct {
	net     *PetriNet[DataState[S]]
	events  []E
	current S
	apply   func(S, E) S
}

// NewEventSourced creates a new event-sourced aggregate.
func NewEventSourced[S, E any](name string, initial S, apply func(S, E) S) *EventSourced[S, E] {
	net := NewPetriNet[DataState[S]](name)
	net.AddPlace(NewGenericPlace("state", NewDataState(initial)))

	return &EventSourced[S, E]{
		net:     net,
		events:  make([]E, 0),
		current: initial,
		apply:   apply,
	}
}

// State returns the current state.
func (es *EventSourced[S, E]) State() S {
	return es.current
}

// Events returns all recorded events.
func (es *EventSourced[S, E]) Events() []E {
	return es.events
}

// Apply applies an event to the current state.
func (es *EventSourced[S, E]) Apply(event E) {
	es.events = append(es.events, event)
	es.current = es.apply(es.current, event)

	// Update the net state
	place := es.net.PlaceByID("state")
	if place != nil {
		place.Initial = place.Initial.Update(es.current)
	}
}

// Replay rebuilds state from events.
func (es *EventSourced[S, E]) Replay(events []E) S {
	var zero S
	state := zero
	for _, event := range events {
		state = es.apply(state, event)
	}
	return state
}

// Project projects all events onto an initial state.
func (es *EventSourced[S, E]) Project(initial S) S {
	state := initial
	for _, event := range es.events {
		state = es.apply(state, event)
	}
	return state
}

// Version returns the number of events applied.
func (es *EventSourced[S, E]) Version() int {
	return len(es.events)
}

// Net returns the underlying Petri net.
func (es *EventSourced[S, E]) Net() *PetriNet[DataState[S]] {
	return es.net
}
