package eventsource

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Common aggregate errors.
var (
	ErrAggregateNotFound = errors.New("aggregate not found")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrCommandRejected   = errors.New("command rejected by guard")
)

// Aggregate is the interface for event-sourced aggregates.
type Aggregate interface {
	// ID returns the aggregate identifier.
	ID() string

	// Version returns the current event version.
	Version() int

	// Apply applies an event to update the aggregate state.
	// This should be a pure function with no side effects.
	Apply(event *Event) error

	// State returns the current aggregate state.
	State() any
}

// Command represents an intent to change aggregate state.
type Command struct {
	// Type is the command type name.
	Type string

	// AggregateID is the target aggregate.
	AggregateID string

	// Payload contains the command data.
	Payload any

	// Metadata contains optional context.
	Metadata map[string]string
}

// CommandHandler processes commands and produces events.
type CommandHandler func(ctx context.Context, agg Aggregate, cmd Command) ([]*Event, error)

// Repository provides aggregate persistence.
type Repository interface {
	// Load retrieves an aggregate by ID, replaying events to rebuild state.
	Load(ctx context.Context, id string) (Aggregate, error)

	// Save persists new events for an aggregate.
	Save(ctx context.Context, agg Aggregate, events []*Event) error

	// Execute loads an aggregate, applies a command, and saves the resulting events.
	Execute(ctx context.Context, id string, cmd Command, handler CommandHandler) error
}

// Factory creates new aggregate instances.
type Factory func(id string) Aggregate

// BaseRepository provides a standard Repository implementation.
type BaseRepository struct {
	store   Store
	factory Factory
}

// NewRepository creates a new aggregate repository.
func NewRepository(store Store, factory Factory) *BaseRepository {
	return &BaseRepository{
		store:   store,
		factory: factory,
	}
}

// Load retrieves an aggregate by ID.
func (r *BaseRepository) Load(ctx context.Context, id string) (Aggregate, error) {
	agg := r.factory(id)

	events, err := r.store.Read(ctx, id, 0)
	if err != nil {
		return nil, err
	}

	for _, event := range events {
		if err := agg.Apply(event); err != nil {
			return nil, err
		}
	}

	return agg, nil
}

// Save persists new events for an aggregate.
func (r *BaseRepository) Save(ctx context.Context, agg Aggregate, events []*Event) error {
	if len(events) == 0 {
		return nil
	}

	_, err := r.store.Append(ctx, agg.ID(), agg.Version(), events)
	return err
}

// Execute loads an aggregate, applies a command, and saves the resulting events.
func (r *BaseRepository) Execute(ctx context.Context, id string, cmd Command, handler CommandHandler) error {
	agg, err := r.Load(ctx, id)
	if err != nil {
		return err
	}

	events, err := handler(ctx, agg, cmd)
	if err != nil {
		return err
	}

	return r.Save(ctx, agg, events)
}

// Ensure BaseRepository implements Repository.
var _ Repository = (*BaseRepository)(nil)

// Base provides common aggregate functionality that can be embedded.
type Base[S any] struct {
	id       string
	version  int
	state    S
	handlers map[string]func(*S, *Event) error
	mu       sync.RWMutex
}

// NewBase creates a new base aggregate with the given ID and initial state.
func NewBase[S any](id string, initialState S) *Base[S] {
	return &Base[S]{
		id:       id,
		version:  -1,
		state:    initialState,
		handlers: make(map[string]func(*S, *Event) error),
	}
}

// ID returns the aggregate identifier.
func (b *Base[S]) ID() string {
	return b.id
}

// Version returns the current event version.
func (b *Base[S]) Version() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.version
}

// State returns the current aggregate state.
func (b *Base[S]) State() any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// TypedState returns the current state with proper type.
func (b *Base[S]) TypedState() S {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// RegisterHandler registers an event handler for a specific event type.
func (b *Base[S]) RegisterHandler(eventType string, handler func(*S, *Event) error) {
	b.handlers[eventType] = handler
}

// Apply applies an event to update the aggregate state.
func (b *Base[S]) Apply(event *Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	handler, ok := b.handlers[event.Type]
	if !ok {
		return fmt.Errorf("no handler for event type: %s", event.Type)
	}

	if err := handler(&b.state, event); err != nil {
		return err
	}

	b.version = event.Version
	return nil
}

// StateMachine provides Petri net state machine semantics on top of an aggregate.
type StateMachine[S any] struct {
	*Base[S]
	places      map[string]int        // Current token counts
	transitions map[string]Transition // Transition definitions
}

// Transition defines a Petri net transition.
type Transition struct {
	// ID is the transition identifier.
	ID string

	// Inputs are the input places with required token counts.
	Inputs map[string]int

	// Outputs are the output places with produced token counts.
	Outputs map[string]int

	// Inhibitors are places that block firing if they have any tokens.
	// Unlike Inputs, inhibitor arcs don't consume tokens - they just check for absence.
	Inhibitors map[string]bool

	// Guard is an optional condition that must be true to fire.
	Guard func(state any) bool

	// EventType is the event type to emit when fired.
	EventType string
}

// NewStateMachine creates a new state machine aggregate.
func NewStateMachine[S any](id string, initialState S, initialPlaces map[string]int) *StateMachine[S] {
	places := make(map[string]int)
	for k, v := range initialPlaces {
		places[k] = v
	}

	return &StateMachine[S]{
		Base:        NewBase(id, initialState),
		places:      places,
		transitions: make(map[string]Transition),
	}
}

// AddTransition registers a transition.
func (sm *StateMachine[S]) AddTransition(t Transition) {
	sm.transitions[t.ID] = t
}

// Apply applies an event to update the state machine, including places.
// This overrides Base.Apply to also update token distribution.
func (sm *StateMachine[S]) Apply(event *Event) error {
	// First update places under our lock
	sm.mu.Lock()

	// Find the transition by event type
	var transition *Transition
	for _, t := range sm.transitions {
		if t.EventType == event.Type || t.ID == event.Type {
			transition = &t
			break
		}
	}

	// Update places if we found a matching transition
	if transition != nil {
		// Remove input tokens
		for place, count := range transition.Inputs {
			sm.places[place] -= count
		}
		// Add output tokens
		for place, count := range transition.Outputs {
			sm.places[place] += count
		}
	}

	sm.mu.Unlock()

	// Call base Apply for state handler and version update (it has its own lock)
	return sm.Base.Apply(event)
}

// CanFire checks if a transition is enabled.
func (sm *StateMachine[S]) CanFire(transitionID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	t, ok := sm.transitions[transitionID]
	if !ok {
		return false
	}

	// Check input places have enough tokens
	for place, required := range t.Inputs {
		if sm.places[place] < required {
			return false
		}
	}

	// Check inhibitor arcs - blocked if any inhibitor place has tokens
	for place := range t.Inhibitors {
		if sm.places[place] > 0 {
			return false
		}
	}

	// Check guard
	if t.Guard != nil && !t.Guard(sm.state) {
		return false
	}

	return true
}

// EnabledTransitions returns all transitions that can currently fire.
func (sm *StateMachine[S]) EnabledTransitions() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var enabled []string
	for id := range sm.transitions {
		if sm.canFireLocked(id) {
			enabled = append(enabled, id)
		}
	}
	return enabled
}

func (sm *StateMachine[S]) canFireLocked(transitionID string) bool {
	t, ok := sm.transitions[transitionID]
	if !ok {
		return false
	}

	for place, required := range t.Inputs {
		if sm.places[place] < required {
			return false
		}
	}

	// Check inhibitor arcs - blocked if any inhibitor place has tokens
	for place := range t.Inhibitors {
		if sm.places[place] > 0 {
			return false
		}
	}

	if t.Guard != nil && !t.Guard(sm.state) {
		return false
	}

	return true
}

// Fire checks if a transition can fire and creates an event.
// It does NOT update places - that's done by Apply when the event is applied.
// Returns an event representing the transition, or an error if the transition cannot fire.
func (sm *StateMachine[S]) Fire(transitionID string, data any) (*Event, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	t, ok := sm.transitions[transitionID]
	if !ok {
		return nil, fmt.Errorf("unknown transition: %s", transitionID)
	}

	// Check inputs
	for place, required := range t.Inputs {
		if sm.places[place] < required {
			return nil, fmt.Errorf("%w: insufficient tokens in %s", ErrInvalidTransition, place)
		}
	}

	// Check inhibitor arcs - blocked if any inhibitor place has tokens
	for place := range t.Inhibitors {
		if sm.places[place] > 0 {
			return nil, fmt.Errorf("%w: inhibited by tokens in %s", ErrInvalidTransition, place)
		}
	}

	// Check guard
	if t.Guard != nil && !t.Guard(sm.state) {
		return nil, ErrCommandRejected
	}

	// Create event (places are updated when Apply is called)
	eventType := t.EventType
	if eventType == "" {
		eventType = transitionID
	}

	return NewEvent(sm.id, eventType, data)
}

// Places returns a copy of the current place markings.
func (sm *StateMachine[S]) Places() map[string]int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]int)
	for k, v := range sm.places {
		result[k] = v
	}
	return result
}

// Tokens returns the token count for a specific place.
func (sm *StateMachine[S]) Tokens(place string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.places[place]
}
