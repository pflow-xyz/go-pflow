package eventsource

import (
	"context"
	"errors"
	"time"
)

// Common errors.
var (
	ErrStreamNotFound      = errors.New("stream not found")
	ErrConcurrencyConflict = errors.New("concurrency conflict: expected version mismatch")
	ErrEventNotFound       = errors.New("event not found")
	ErrStoreClosed         = errors.New("store is closed")
)

// Store is the interface for event storage backends.
type Store interface {
	// Append adds events to a stream with optimistic concurrency control.
	// expectedVersion should match the current stream version, or -1 for new streams.
	// Returns the new stream version after appending.
	Append(ctx context.Context, streamID string, expectedVersion int, events []*Event) (int, error)

	// Read retrieves events from a stream starting at fromVersion.
	// Returns events in order from fromVersion to the end of the stream.
	Read(ctx context.Context, streamID string, fromVersion int) ([]*Event, error)

	// ReadAll retrieves all events matching the filter.
	ReadAll(ctx context.Context, filter EventFilter) ([]*Event, error)

	// StreamVersion returns the current version of a stream.
	// Returns -1 if the stream doesn't exist.
	StreamVersion(ctx context.Context, streamID string) (int, error)

	// Subscribe creates a subscription for new events.
	// The subscription receives events matching the filter as they are appended.
	Subscribe(ctx context.Context, filter EventFilter) (Subscription, error)

	// DeleteStream removes all events for a stream.
	// Use this for "reset" operations that need to clear event history.
	DeleteStream(ctx context.Context, streamID string) error

	// Close releases any resources held by the store.
	Close() error
}

// AppendResult contains the result of an append operation.
type AppendResult struct {
	// StreamID is the stream that was appended to.
	StreamID string

	// FromVersion is the version before the append.
	FromVersion int

	// ToVersion is the version after the append.
	ToVersion int

	// Events contains the appended events with assigned versions.
	Events []*Event
}

// Snapshot represents a point-in-time state snapshot.
type Snapshot struct {
	// StreamID identifies the aggregate.
	StreamID string

	// Version is the event version this snapshot represents.
	Version int

	// State is the serialized aggregate state.
	State []byte
}

// SnapshotStore provides snapshot storage for aggregate state.
type SnapshotStore interface {
	// Save stores a snapshot.
	Save(ctx context.Context, snapshot *Snapshot) error

	// Load retrieves the latest snapshot for a stream.
	// Returns nil if no snapshot exists.
	Load(ctx context.Context, streamID string) (*Snapshot, error)

	// Delete removes snapshots for a stream.
	Delete(ctx context.Context, streamID string) error
}

// Instance represents an aggregate instance summary.
type Instance struct {
	ID        string                 `json:"id"`
	Version   int                    `json:"version"`
	State     map[string]int         `json:"state"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Stats represents aggregate statistics.
type Stats struct {
	TotalInstances int            `json:"total_instances"`
	ByPlace        map[string]int `json:"by_place"`
}

// AdminStore provides administrative query capabilities for the event store.
type AdminStore interface {
	// ListInstances returns a paginated list of aggregate instances.
	// place, from, and to are optional filters.
	ListInstances(ctx context.Context, place, from, to string, page, perPage int) ([]Instance, int, error)

	// GetStats returns statistics about stored aggregates.
	GetStats(ctx context.Context) (*Stats, error)
}
