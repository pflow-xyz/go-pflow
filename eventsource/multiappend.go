package eventsource

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// StreamAppend is one stream's contribution to an atomic multi-stream
// append.
type StreamAppend struct {
	StreamID string
	// ExpectedVersion is the optimistic-concurrency check for this stream:
	// the current stream version, or -1 for a new stream, or -2 to skip the
	// check.
	ExpectedVersion int
	Events          []*Event
}

// MultiAppender is the optional store capability behind composed
// applications: several aggregates' event logs advance in one atomic step
// or not at all. A coordinator firing a fused (cross-entity) transition
// appends one event per member entity; without atomicity, a crash between
// appends would leave one entity believing the rendezvous happened and
// another believing it did not — a divergence replay can never repair.
//
// Discover it by type assertion; stores that cannot provide atomicity
// simply do not implement it, and callers must fail rather than fall back
// to sequential appends.
type MultiAppender interface {
	// MultiAppend appends to every stream or to none. Per-stream
	// concurrency checks follow Append's rules.
	MultiAppend(ctx context.Context, appends []StreamAppend) error
}

// MultiAppend on the memory store: every check precedes every mutation,
// all under one lock.
func (s *MemoryStore) MultiAppend(ctx context.Context, appends []StreamAppend) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	seen := map[string]bool{}
	for _, a := range appends {
		if seen[a.StreamID] {
			return fmt.Errorf("multi-append: stream %q appears twice", a.StreamID)
		}
		seen[a.StreamID] = true
		// -2 skips the check; -1 requires a new stream; >=0 requires an
		// exact match. Every check runs before any mutation.
		currentVersion := len(s.streams[a.StreamID]) - 1
		if a.ExpectedVersion >= -1 && currentVersion != a.ExpectedVersion {
			return ErrConcurrencyConflict
		}
	}

	var notify []*Event
	for _, a := range appends {
		stream := s.streams[a.StreamID]
		currentVersion := len(stream) - 1
		for i, event := range a.Events {
			event.ID = uuid.New().String()
			event.StreamID = a.StreamID
			event.Version = currentVersion + i + 1
			if event.Timestamp.IsZero() {
				event.Timestamp = time.Now()
			}
		}
		s.streams[a.StreamID] = append(stream, a.Events...)
		notify = append(notify, a.Events...)
	}

	for _, sub := range s.subscriptions {
		for _, event := range notify {
			if sub.matches(event) {
				select {
				case sub.events <- event:
				default:
				}
			}
		}
	}
	return nil
}

// MultiAppend on the SQLite store: one transaction across all streams.
func (s *SQLiteStore) MultiAppend(ctx context.Context, appends []StreamAppend) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO events (id, stream_id, type, version, timestamp, data, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)",
	)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	seen := map[string]bool{}
	var notify []*Event
	for _, a := range appends {
		if seen[a.StreamID] {
			return fmt.Errorf("multi-append: stream %q appears twice", a.StreamID)
		}
		seen[a.StreamID] = true

		var currentVersion int
		if err := tx.QueryRowContext(ctx,
			"SELECT COALESCE(MAX(version), -1) FROM events WHERE stream_id = ?",
			a.StreamID,
		).Scan(&currentVersion); err != nil {
			return fmt.Errorf("failed to get stream version: %w", err)
		}
		if a.ExpectedVersion >= -1 && currentVersion != a.ExpectedVersion {
			return ErrConcurrencyConflict
		}

		for i, event := range a.Events {
			event.ID = uuid.New().String()
			event.StreamID = a.StreamID
			event.Version = currentVersion + i + 1
			if event.Timestamp.IsZero() {
				event.Timestamp = time.Now()
			}
			var metadata []byte
			if event.Metadata != nil {
				metadata, _ = json.Marshal(event.Metadata)
			}
			if _, err := stmt.ExecContext(ctx,
				event.ID, event.StreamID, event.Type, event.Version,
				event.Timestamp.Format(time.RFC3339Nano),
				string(event.Data), string(metadata),
			); err != nil {
				return fmt.Errorf("failed to insert event: %w", err)
			}
		}
		notify = append(notify, a.Events...)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	go s.notifySubscribers(notify)
	return nil
}
