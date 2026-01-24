// Package eventsource provides event sourcing infrastructure for CQRS applications.
package eventsource

import (
	"encoding/json"
	"time"
)

// Event represents a domain event in an event-sourced system.
type Event struct {
	// ID is the unique identifier for this event.
	ID string `json:"id"`

	// StreamID identifies the aggregate/entity this event belongs to.
	StreamID string `json:"stream_id"`

	// Type is the event type name (e.g., "OrderCreated", "PaymentReceived").
	Type string `json:"type"`

	// Version is the sequence number within the stream.
	Version int `json:"version"`

	// Timestamp when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Data contains the event payload as JSON.
	Data json.RawMessage `json:"data"`

	// Metadata contains optional context (correlation ID, user ID, etc.).
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewEvent creates a new event with the given type and data.
func NewEvent(streamID, eventType string, data any) (*Event, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Event{
		StreamID:  streamID,
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Data:      payload,
	}, nil
}

// Decode unmarshals the event data into the provided target.
func (e *Event) Decode(target any) error {
	return json.Unmarshal(e.Data, target)
}

// EventHandler processes events.
type EventHandler func(event *Event) error

// EventFilter defines criteria for filtering events.
type EventFilter struct {
	// StreamID filters to a specific stream.
	StreamID string

	// Types filters to specific event types.
	Types []string

	// FromVersion starts reading from this version (inclusive).
	FromVersion int

	// ToVersion stops reading at this version (inclusive, 0 = no limit).
	ToVersion int

	// FromTime filters events after this time.
	FromTime *time.Time

	// ToTime filters events before this time.
	ToTime *time.Time

	// Limit maximum number of events to return.
	Limit int
}

// Subscription represents an active event subscription.
type Subscription interface {
	// Events returns a channel that receives events.
	Events() <-chan *Event

	// Errors returns a channel that receives errors.
	Errors() <-chan error

	// Close stops the subscription.
	Close() error
}
