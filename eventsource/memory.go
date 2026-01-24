package eventsource

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// MemoryStore is an in-memory event store for testing and development.
type MemoryStore struct {
	mu            sync.RWMutex
	streams       map[string][]*Event
	subscriptions map[string]*memorySubscription
	closed        bool
}

// NewMemoryStore creates a new in-memory event store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		streams:       make(map[string][]*Event),
		subscriptions: make(map[string]*memorySubscription),
	}
}

// Append adds events to a stream with optimistic concurrency control.
func (s *MemoryStore) Append(ctx context.Context, streamID string, expectedVersion int, events []*Event) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	stream := s.streams[streamID]
	currentVersion := len(stream) - 1

	// Check concurrency
	if expectedVersion >= 0 && currentVersion != expectedVersion {
		return 0, ErrConcurrencyConflict
	}

	// Assign versions and IDs to events
	for i, event := range events {
		event.ID = uuid.New().String()
		event.StreamID = streamID
		event.Version = currentVersion + i + 1
	}

	// Append to stream
	s.streams[streamID] = append(stream, events...)
	newVersion := len(s.streams[streamID]) - 1

	// Notify subscribers
	for _, sub := range s.subscriptions {
		for _, event := range events {
			if sub.matches(event) {
				select {
				case sub.events <- event:
				default:
					// Drop if buffer full
				}
			}
		}
	}

	return newVersion, nil
}

// Read retrieves events from a stream starting at fromVersion.
func (s *MemoryStore) Read(ctx context.Context, streamID string, fromVersion int) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	stream, exists := s.streams[streamID]
	if !exists {
		return nil, nil
	}

	if fromVersion >= len(stream) {
		return nil, nil
	}

	if fromVersion < 0 {
		fromVersion = 0
	}

	// Return a copy to prevent modification
	result := make([]*Event, len(stream)-fromVersion)
	copy(result, stream[fromVersion:])
	return result, nil
}

// ReadAll retrieves all events matching the filter.
func (s *MemoryStore) ReadAll(ctx context.Context, filter EventFilter) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var result []*Event

	for streamID, stream := range s.streams {
		if filter.StreamID != "" && filter.StreamID != streamID {
			continue
		}

		for _, event := range stream {
			if s.matchesFilter(event, filter) {
				result = append(result, event)
				if filter.Limit > 0 && len(result) >= filter.Limit {
					return result, nil
				}
			}
		}
	}

	return result, nil
}

// StreamVersion returns the current version of a stream.
func (s *MemoryStore) StreamVersion(ctx context.Context, streamID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	stream, exists := s.streams[streamID]
	if !exists {
		return -1, nil
	}

	return len(stream) - 1, nil
}

// Subscribe creates a subscription for new events.
func (s *MemoryStore) Subscribe(ctx context.Context, filter EventFilter) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	sub := &memorySubscription{
		id:     uuid.New().String(),
		filter: filter,
		events: make(chan *Event, 100),
		errors: make(chan error, 1),
		done:   make(chan struct{}),
		store:  s,
	}

	s.subscriptions[sub.id] = sub

	go func() {
		<-ctx.Done()
		sub.Close()
	}()

	return sub, nil
}

// DeleteStream removes all events for a stream.
func (s *MemoryStore) DeleteStream(ctx context.Context, streamID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	delete(s.streams, streamID)
	return nil
}

// Close releases resources.
func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true

	for _, sub := range s.subscriptions {
		close(sub.events)
		close(sub.errors)
	}
	s.subscriptions = nil

	return nil
}

func (s *MemoryStore) matchesFilter(event *Event, filter EventFilter) bool {
	if filter.FromVersion > 0 && event.Version < filter.FromVersion {
		return false
	}
	if filter.ToVersion > 0 && event.Version > filter.ToVersion {
		return false
	}
	if filter.FromTime != nil && event.Timestamp.Before(*filter.FromTime) {
		return false
	}
	if filter.ToTime != nil && event.Timestamp.After(*filter.ToTime) {
		return false
	}
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if t == event.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// memorySubscription implements Subscription.
type memorySubscription struct {
	id     string
	filter EventFilter
	events chan *Event
	errors chan error
	done   chan struct{}
	store  *MemoryStore
	closed bool
	mu     sync.Mutex
}

func (s *memorySubscription) Events() <-chan *Event {
	return s.events
}

func (s *memorySubscription) Errors() <-chan error {
	return s.errors
}

func (s *memorySubscription) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	s.store.mu.Lock()
	delete(s.store.subscriptions, s.id)
	s.store.mu.Unlock()

	close(s.done)
	return nil
}

func (s *memorySubscription) matches(event *Event) bool {
	if s.filter.StreamID != "" && s.filter.StreamID != event.StreamID {
		return false
	}
	if len(s.filter.Types) > 0 {
		found := false
		for _, t := range s.filter.Types {
			if t == event.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ListInstances returns a paginated list of aggregate instances.
// For MemoryStore, we can only list stream IDs and their event counts.
func (s *MemoryStore) ListInstances(ctx context.Context, place, from, to string, page, perPage int) ([]Instance, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, 0, ErrStoreClosed
	}

	// Collect all stream IDs
	var instances []Instance
	for streamID, events := range s.streams {
		if len(events) == 0 {
			continue
		}

		// Build state from the last event that has state info
		state := make(map[string]int)
		var updatedAt = events[len(events)-1].Timestamp
		for _, event := range events {
			// Try to extract state from event data
			var eventData map[string]interface{}
			if err := json.Unmarshal(event.Data, &eventData); err == nil {
				if eventState, ok := eventData["state"].(map[string]interface{}); ok {
					for k, v := range eventState {
						if intVal, ok := v.(float64); ok {
							state[k] = int(intVal)
						} else if intVal, ok := v.(int); ok {
							state[k] = intVal
						}
					}
				}
			}
		}

		// If we have a place filter, check if the instance matches
		if place != "" {
			tokens, ok := state[place]
			if !ok || tokens <= 0 {
				continue
			}
		}

		instances = append(instances, Instance{
			ID:        streamID,
			Version:   len(events) - 1,
			State:     state,
			UpdatedAt: updatedAt,
		})
	}

	// Sort by ID for consistent pagination
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].ID < instances[j].ID
	})

	total := len(instances)

	// Apply pagination
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	start := (page - 1) * perPage
	if start >= len(instances) {
		return []Instance{}, total, nil
	}
	end := start + perPage
	if end > len(instances) {
		end = len(instances)
	}

	return instances[start:end], total, nil
}

// GetStats returns aggregate statistics.
func (s *MemoryStore) GetStats(ctx context.Context) (*Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	return &Stats{
		TotalInstances: len(s.streams),
		ByPlace:        make(map[string]int), // Not tracked in memory store
	}, nil
}

// Ensure MemoryStore implements Store and AdminStore.
var (
	_ Store      = (*MemoryStore)(nil)
	_ AdminStore = (*MemoryStore)(nil)
)
