package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/pflow-xyz/go-pflow/eventsource"
	"github.com/pflow-xyz/go-pflow/petri"
)

// EventSourceStore adapts eventsource.Store to graphql.Store.
// It manages Petri net instances using event sourcing.
type EventSourceStore struct {
	store eventsource.Store
	// model is color-unfolded; colorMap is nil when nothing was unfolded.
	model     *petri.PetriNet
	colorMap  *petri.ColorMap
	modelName string

	// Cache of loaded state machines
	mu    sync.RWMutex
	cache map[string]*eventsource.StateMachine[map[string]any]
}

// NewEventSourceStore creates a new store backed by an eventsource.Store.
//
// Multi-color models are unfolded first (petri.ExpandColors), so firing checks
// each color against its own arc weight instead of a summed pool. Markings
// exposed through the GraphQL API are then keyed by the expanded
// "place.color" names, which are self-describing; ColorMap() recovers the
// mapping back to base places.
func NewEventSourceStore(store eventsource.Store, model *petri.PetriNet, modelName string) *EventSourceStore {
	expanded, cm := model.ExpandColors()
	return &EventSourceStore{
		store:     store,
		model:     expanded,
		colorMap:  cm,
		modelName: modelName,
		cache:     make(map[string]*eventsource.StateMachine[map[string]any]),
	}
}

// ColorMap returns the mapping used to unfold a multi-color model, or nil.
// Use it to fold an instance's marking back to per-place totals.
func (s *EventSourceStore) ColorMap() *petri.ColorMap { return s.colorMap }

// Create creates a new Petri net instance.
func (s *EventSourceStore) Create(ctx context.Context, modelName string) (string, error) {
	id := uuid.New().String()

	// Build initial marking from model
	initialPlaces := make(map[string]int)
	for label, place := range s.model.Places {
		initialPlaces[label] = int(place.GetTokenCount())
	}

	// Create state machine
	sm := eventsource.NewStateMachine[map[string]any](id, make(map[string]any), initialPlaces)

	// Register transitions from model
	for label := range s.model.Transitions {
		t := s.buildTransition(label)
		sm.AddTransition(t)

		// Register event handler for this transition
		sm.RegisterHandler(label, func(state *map[string]any, event *eventsource.Event) error {
			// Decode event data and merge into state
			var data map[string]any
			if err := event.Decode(&data); err == nil {
				for k, v := range data {
					(*state)[k] = v
				}
			}
			return nil
		})
	}

	// Create initial event
	event, err := eventsource.NewEvent(id, "created", map[string]any{
		"modelName": modelName,
	})
	if err != nil {
		return "", err
	}

	// Register handler for created event
	sm.RegisterHandler("created", func(state *map[string]any, event *eventsource.Event) error {
		return nil
	})

	// Save initial event
	if _, err := s.store.Append(ctx, id, -1, []*eventsource.Event{event}); err != nil {
		return "", err
	}

	// Apply event to state machine
	if err := sm.Apply(event); err != nil {
		return "", err
	}

	// Cache the state machine
	s.mu.Lock()
	s.cache[id] = sm
	s.mu.Unlock()

	return id, nil
}

// Get retrieves an instance by ID.
func (s *EventSourceStore) Get(ctx context.Context, id string) (*Instance, error) {
	sm, err := s.loadStateMachine(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toInstance(sm), nil
}

// Fire attempts to fire a transition on an instance.
func (s *EventSourceStore) Fire(ctx context.Context, id string, transition string, bindings map[string]any) (*Instance, error) {
	sm, err := s.loadStateMachine(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check if transition can fire
	if !sm.CanFire(transition) {
		return nil, fmt.Errorf("transition %s is not enabled", transition)
	}

	// Create event with bindings as data
	event, err := sm.Fire(transition, bindings)
	if err != nil {
		return nil, err
	}

	// Save event
	if _, err := s.store.Append(ctx, id, sm.Version(), []*eventsource.Event{event}); err != nil {
		return nil, err
	}

	// Apply event to state machine
	if err := sm.Apply(event); err != nil {
		return nil, err
	}

	return s.toInstance(sm), nil
}

// List returns instances with optional filtering.
func (s *EventSourceStore) List(ctx context.Context, filter InstanceFilter) ([]*Instance, int, error) {
	// Use the cache since it has the correct Petri net state
	// The eventsource.AdminStore interface doesn't track Petri net markings
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Instance
	for _, sm := range s.cache {
		inst := s.toInstance(sm)

		// Apply place filter
		if filter.Place != "" {
			if inst.Marking[filter.Place] <= 0 {
				continue
			}
		}

		// Only include instances for this model
		if filter.ModelName != "" && inst.ModelName != filter.ModelName {
			continue
		}

		result = append(result, inst)
	}

	total := len(result)

	// Apply pagination
	page := filter.Page
	if page < 1 {
		page = 1
	}
	perPage := filter.PerPage
	if perPage < 1 {
		perPage = 20
	}

	start := (page - 1) * perPage
	if start >= len(result) {
		return []*Instance{}, total, nil
	}

	end := start + perPage
	if end > len(result) {
		end = len(result)
	}

	return result[start:end], total, nil
}

// Delete removes an instance.
func (s *EventSourceStore) Delete(ctx context.Context, id string) error {
	// Remove from cache
	s.mu.Lock()
	delete(s.cache, id)
	s.mu.Unlock()

	// Delete from event store
	return s.store.DeleteStream(ctx, id)
}

// loadStateMachine loads or retrieves a cached state machine.
func (s *EventSourceStore) loadStateMachine(ctx context.Context, id string) (*eventsource.StateMachine[map[string]any], error) {
	// Check cache first
	s.mu.RLock()
	if sm, ok := s.cache[id]; ok {
		s.mu.RUnlock()
		return sm, nil
	}
	s.mu.RUnlock()

	// Load events from store
	events, err := s.store.Read(ctx, id, 0)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("instance not found: %s", id)
	}

	// Build initial marking from model
	initialPlaces := make(map[string]int)
	for label, place := range s.model.Places {
		initialPlaces[label] = int(place.GetTokenCount())
	}

	// Create state machine
	sm := eventsource.NewStateMachine[map[string]any](id, make(map[string]any), initialPlaces)

	// Register transitions from model
	for label := range s.model.Transitions {
		t := s.buildTransition(label)
		sm.AddTransition(t)

		// Register event handler
		transitionLabel := label // capture for closure
		sm.RegisterHandler(transitionLabel, func(state *map[string]any, event *eventsource.Event) error {
			var data map[string]any
			if err := event.Decode(&data); err == nil {
				for k, v := range data {
					(*state)[k] = v
				}
			}
			return nil
		})
	}

	// Register created event handler
	sm.RegisterHandler("created", func(state *map[string]any, event *eventsource.Event) error {
		return nil
	})

	// Replay events
	for _, event := range events {
		if err := sm.Apply(event); err != nil {
			// Log but don't fail on unknown events
			continue
		}
	}

	// Cache the state machine
	s.mu.Lock()
	s.cache[id] = sm
	s.mu.Unlock()

	return sm, nil
}

// buildTransition creates an eventsource.Transition from a Petri net transition.
func (s *EventSourceStore) buildTransition(label string) eventsource.Transition {
	t := eventsource.Transition{
		ID:        label,
		EventType: label,
		Inputs:    make(map[string]int),
		Outputs:   make(map[string]int),
	}

	// Build inputs from arcs
	for _, arc := range s.model.GetInputArcs(label) {
		weight := int(arc.GetWeightSum())
		if weight < 1 {
			weight = 1
		}
		if arc.InhibitTransition {
			if t.Inhibitors == nil {
				t.Inhibitors = make(map[string]int)
			}
			t.Inhibitors[arc.Source] = weight
		} else {
			t.Inputs[arc.Source] = weight
		}
	}

	// Build outputs from arcs
	for _, arc := range s.model.GetOutputArcs(label) {
		weight := int(arc.GetWeightSum())
		if weight < 1 {
			weight = 1
		}
		// An inhibitor arc pointing transition -> place is petri's read-arc
		// encoding: it gates enablement and produces nothing. Counting it as an
		// output would have minted tokens on every firing.
		if arc.InhibitTransition {
			if t.Reads == nil {
				t.Reads = make(map[string]int)
			}
			t.Reads[arc.Target] = weight
			continue
		}
		t.Outputs[arc.Target] = weight
	}

	return t
}

// toInstance converts a state machine to a GraphQL Instance.
func (s *EventSourceStore) toInstance(sm *eventsource.StateMachine[map[string]any]) *Instance {
	places := sm.Places()
	marking := make(map[string]int)
	for k, v := range places {
		marking[k] = v
	}

	// Get state as map
	state := sm.TypedState()
	stateMap := make(map[string]any)
	for k, v := range state {
		stateMap[k] = v
	}

	return &Instance{
		ID:                 sm.ID(),
		ModelName:          s.modelName,
		Version:            sm.Version(),
		Marking:            marking,
		State:              stateMap,
		EnabledTransitions: sm.EnabledTransitions(),
	}
}

// MarshalMarking converts a marking map to JSON for GraphQL.
func MarshalMarking(marking map[string]int) json.RawMessage {
	data, _ := json.Marshal(marking)
	return data
}

// Ensure EventSourceStore implements Store.
var _ Store = (*EventSourceStore)(nil)
