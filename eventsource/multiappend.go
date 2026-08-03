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
	// MultiAppend appends to every stream or to none, failing with
	// ErrConcurrencyConflict if any stream's ExpectedVersion does not hold.
	//
	// The check is stricter than Append's, which ignores any negative
	// expectation: here -1 genuinely requires a stream with no events yet, and
	// only -2 skips the check. A caller porting a call from Append must say -2
	// where it said -1, or a create-if-absent turns into a hard conflict.
	MultiAppend(ctx context.Context, appends []StreamAppend) error
}

// mergeStreamAppends collapses repeated stream IDs into one entry per stream,
// preserving first-appearance order.
//
// A coordinator that fires a member subnet and *also* fences a read on that
// same subnet names the stream twice: once carrying the member's events, once
// carrying no events at all and only the version the read was taken at. That is
// one intent, not two, so the fence merges into the append it duplicates rather
// than being refused.
//
// Two entries that both carry events stay an error: they are two independent
// appends computed against the same history, and silently concatenating them
// would make the second one's expected version a lie. A fence whose expected
// version disagrees with the append's is the same mistake, so it errors too —
// the caller read at a version it is not appending at.
func mergeStreamAppends(appends []StreamAppend) ([]StreamAppend, error) {
	index := make(map[string]int, len(appends))
	merged := make([]StreamAppend, 0, len(appends))
	for _, a := range appends {
		i, ok := index[a.StreamID]
		if !ok {
			index[a.StreamID] = len(merged)
			merged = append(merged, a)
			continue
		}
		prev := &merged[i]
		if len(prev.Events) > 0 && len(a.Events) > 0 {
			return nil, fmt.Errorf("multi-append: stream %q appears twice", a.StreamID)
		}
		// -2 skips the check, so a real expectation on either side wins; two
		// real expectations must agree.
		switch {
		case prev.ExpectedVersion == -2:
			prev.ExpectedVersion = a.ExpectedVersion
		case a.ExpectedVersion != -2 && a.ExpectedVersion != prev.ExpectedVersion:
			return nil, fmt.Errorf("multi-append: stream %q appears twice at versions %d and %d",
				a.StreamID, prev.ExpectedVersion, a.ExpectedVersion)
		}
		if len(a.Events) > 0 {
			prev.Events = a.Events
		}
	}
	return merged, nil
}

// MultiAppend on the memory store: every check precedes every mutation,
// all under one lock.
func (s *MemoryStore) MultiAppend(ctx context.Context, appends []StreamAppend) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	appends, err := mergeStreamAppends(appends)
	if err != nil {
		return err
	}

	for _, a := range appends {
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

	appends, err := mergeStreamAppends(appends)
	if err != nil {
		return err
	}

	// The version SELECTs below and the INSERTs that follow them have to be one
	// indivisible step against every other writer of this file, in this process
	// or another; beginImmediate is what buys that.
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.close()

	stmt, err := tx.conn.PrepareContext(ctx,
		"INSERT INTO events (id, stream_id, type, version, timestamp, data, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)",
	)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	var notify []*Event
	for _, a := range appends {
		var currentVersion int
		if err := tx.conn.QueryRowContext(ctx,
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

	if err := tx.commit(ctx); err != nil {
		return err
	}

	go s.notifySubscribers(notify)
	return nil
}
