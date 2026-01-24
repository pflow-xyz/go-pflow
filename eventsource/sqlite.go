package eventsource

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	_ "modernc.org/sqlite"
)

// SQLiteStore is a SQLite-backed event store.
type SQLiteStore struct {
	db            *sql.DB
	mu            sync.RWMutex
	subscriptions map[string]*sqliteSubscription
	closed        bool
}

// NewSQLiteStore creates a new SQLite event store.
// dsn can be ":memory:" for in-memory or a file path.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}

	store := &SQLiteStore{
		db:            db,
		subscriptions: make(map[string]*sqliteSubscription),
	}

	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	return store, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			stream_id TEXT NOT NULL,
			type TEXT NOT NULL,
			version INTEGER NOT NULL,
			timestamp TEXT NOT NULL,
			data TEXT NOT NULL,
			metadata TEXT,
			UNIQUE(stream_id, version)
		);

		CREATE INDEX IF NOT EXISTS idx_events_stream ON events(stream_id, version);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

		CREATE TABLE IF NOT EXISTS snapshots (
			stream_id TEXT PRIMARY KEY,
			version INTEGER NOT NULL,
			state BLOB NOT NULL,
			created_at TEXT NOT NULL
		);

		-- Search index table for queryable fields
		CREATE TABLE IF NOT EXISTS search_index (
			stream_id TEXT NOT NULL,
			field TEXT NOT NULL,
			value TEXT NOT NULL,
			event_type TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (stream_id, field)
		);

		CREATE INDEX IF NOT EXISTS idx_search_field ON search_index(field, value);
		CREATE INDEX IF NOT EXISTS idx_search_value ON search_index(value);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Create FTS5 virtual table for full-text search (ignore error if already exists)
	fts := `CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(
		stream_id,
		title,
		content,
		tags,
		content='',
		contentless_delete=1
	)`
	s.db.Exec(fts) // Ignore error - FTS5 may not be available in all builds

	return nil
}

// Append adds events to a stream with optimistic concurrency control.
func (s *SQLiteStore) Append(ctx context.Context, streamID string, expectedVersion int, events []*Event) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current version
	var currentVersion int
	err = tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), -1) FROM events WHERE stream_id = ?",
		streamID,
	).Scan(&currentVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to get stream version: %w", err)
	}

	// Check concurrency
	if expectedVersion >= 0 && currentVersion != expectedVersion {
		return 0, ErrConcurrencyConflict
	}

	// Insert events
	stmt, err := tx.PrepareContext(ctx,
		"INSERT INTO events (id, stream_id, type, version, timestamp, data, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)",
	)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for i, event := range events {
		event.ID = uuid.New().String()
		event.StreamID = streamID
		event.Version = currentVersion + i + 1

		var metadata []byte
		if event.Metadata != nil {
			metadata, _ = json.Marshal(event.Metadata)
		}

		_, err = stmt.ExecContext(ctx,
			event.ID,
			event.StreamID,
			event.Type,
			event.Version,
			event.Timestamp.Format(time.RFC3339Nano),
			string(event.Data),
			string(metadata),
		)
		if err != nil {
			return 0, fmt.Errorf("failed to insert event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit: %w", err)
	}

	newVersion := currentVersion + len(events)

	// Notify subscribers
	go s.notifySubscribers(events)

	return newVersion, nil
}

func (s *SQLiteStore) notifySubscribers(events []*Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
}

// Read retrieves events from a stream starting at fromVersion.
func (s *SQLiteStore) Read(ctx context.Context, streamID string, fromVersion int) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT id, stream_id, type, version, timestamp, data, metadata FROM events WHERE stream_id = ? AND version >= ? ORDER BY version",
		streamID, fromVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

// ReadAll retrieves all events matching the filter.
func (s *SQLiteStore) ReadAll(ctx context.Context, filter EventFilter) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	query := "SELECT id, stream_id, type, version, timestamp, data, metadata FROM events WHERE 1=1"
	var args []any

	if filter.StreamID != "" {
		query += " AND stream_id = ?"
		args = append(args, filter.StreamID)
	}

	if len(filter.Types) > 0 {
		query += " AND type IN ("
		for i, t := range filter.Types {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, t)
		}
		query += ")"
	}

	if filter.FromVersion > 0 {
		query += " AND version >= ?"
		args = append(args, filter.FromVersion)
	}

	if filter.ToVersion > 0 {
		query += " AND version <= ?"
		args = append(args, filter.ToVersion)
	}

	if filter.FromTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, filter.FromTime.Format(time.RFC3339Nano))
	}

	if filter.ToTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, filter.ToTime.Format(time.RFC3339Nano))
	}

	query += " ORDER BY timestamp, version"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

func (s *SQLiteStore) scanEvents(rows *sql.Rows) ([]*Event, error) {
	var events []*Event

	for rows.Next() {
		var (
			event        Event
			timestampStr string
			dataStr      string
			metadataStr  sql.NullString
		)

		err := rows.Scan(
			&event.ID,
			&event.StreamID,
			&event.Type,
			&event.Version,
			&timestampStr,
			&dataStr,
			&metadataStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		event.Timestamp, _ = time.Parse(time.RFC3339Nano, timestampStr)
		event.Data = json.RawMessage(dataStr)

		if metadataStr.Valid && metadataStr.String != "" {
			json.Unmarshal([]byte(metadataStr.String), &event.Metadata)
		}

		events = append(events, &event)
	}

	return events, rows.Err()
}

// StreamVersion returns the current version of a stream.
func (s *SQLiteStore) StreamVersion(ctx context.Context, streamID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	var version int
	err := s.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), -1) FROM events WHERE stream_id = ?",
		streamID,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to get stream version: %w", err)
	}

	return version, nil
}

// Subscribe creates a subscription for new events.
func (s *SQLiteStore) Subscribe(ctx context.Context, filter EventFilter) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	sub := &sqliteSubscription{
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

// Close releases resources.
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true

	for _, sub := range s.subscriptions {
		close(sub.events)
		close(sub.errors)
	}
	s.subscriptions = nil

	return s.db.Close()
}

// SaveSnapshot stores a snapshot.
func (s *SQLiteStore) SaveSnapshot(ctx context.Context, snapshot *Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO snapshots (stream_id, version, state, created_at) VALUES (?, ?, ?, ?)`,
		snapshot.StreamID,
		snapshot.Version,
		snapshot.State,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

// LoadSnapshot retrieves the latest snapshot for a stream.
func (s *SQLiteStore) LoadSnapshot(ctx context.Context, streamID string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	var snapshot Snapshot
	err := s.db.QueryRowContext(ctx,
		"SELECT stream_id, version, state FROM snapshots WHERE stream_id = ?",
		streamID,
	).Scan(&snapshot.StreamID, &snapshot.Version, &snapshot.State)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// DeleteSnapshot removes snapshots for a stream.
func (s *SQLiteStore) DeleteSnapshot(ctx context.Context, streamID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	_, err := s.db.ExecContext(ctx, "DELETE FROM snapshots WHERE stream_id = ?", streamID)
	return err
}

// DeleteStream removes all events for a stream.
// This is used for "reset" operations that need to clear event history.
func (s *SQLiteStore) DeleteStream(ctx context.Context, streamID string) error {
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

	// Delete all events for the stream
	_, err = tx.ExecContext(ctx, "DELETE FROM events WHERE stream_id = ?", streamID)
	if err != nil {
		return fmt.Errorf("failed to delete events: %w", err)
	}

	// Also delete from search index
	_, err = tx.ExecContext(ctx, "DELETE FROM search_index WHERE stream_id = ?", streamID)
	if err != nil {
		return fmt.Errorf("failed to delete search index: %w", err)
	}

	// Delete any snapshots
	_, err = tx.ExecContext(ctx, "DELETE FROM snapshots WHERE stream_id = ?", streamID)
	if err != nil {
		return fmt.Errorf("failed to delete snapshots: %w", err)
	}

	return tx.Commit()
}

// sqliteSubscription implements Subscription.
type sqliteSubscription struct {
	id     string
	filter EventFilter
	events chan *Event
	errors chan error
	done   chan struct{}
	store  *SQLiteStore
	closed bool
	mu     sync.Mutex
}

func (s *sqliteSubscription) Events() <-chan *Event {
	return s.events
}

func (s *sqliteSubscription) Errors() <-chan error {
	return s.errors
}

func (s *sqliteSubscription) Close() error {
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

func (s *sqliteSubscription) matches(event *Event) bool {
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

// ListInstances returns a paginated list of aggregate instances with optional filters.
func (s *SQLiteStore) ListInstances(ctx context.Context, place, from, to string, page, perPage int) ([]Instance, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, 0, ErrStoreClosed
	}

	// Count total matching instances
	countQuery := `
		SELECT COUNT(DISTINCT stream_id)
		FROM events
		WHERE 1=1
	`
	var countArgs []interface{}

	if from != "" {
		countQuery += " AND timestamp >= ?"
		countArgs = append(countArgs, from)
	}
	if to != "" {
		countQuery += " AND timestamp <= ?"
		countArgs = append(countArgs, to)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count instances: %w", err)
	}

	// Build query for instances with filters
	query := `
		SELECT
			stream_id,
			MAX(version) as version,
			MAX(timestamp) as updated_at
		FROM events
		WHERE 1=1
	`
	var args []interface{}

	if from != "" {
		query += " AND timestamp >= ?"
		args = append(args, from)
	}
	if to != "" {
		query += " AND timestamp <= ?"
		args = append(args, to)
	}

	query += " GROUP BY stream_id ORDER BY updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, perPage, (page-1)*perPage)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query instances: %w", err)
	}
	defer rows.Close()

	var instances []Instance
	for rows.Next() {
		var inst Instance
		var updatedAtStr string

		if err := rows.Scan(&inst.ID, &inst.Version, &updatedAtStr); err != nil {
			return nil, 0, fmt.Errorf("failed to scan instance: %w", err)
		}

		inst.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAtStr)

		// Load current state by replaying events
		events, err := s.Read(ctx, inst.ID, 0)
		if err != nil {
			continue // Skip instances we can't read
		}

		// Build state from events (simplified - assumes token places)
		inst.State = make(map[string]int)
		for _, evt := range events {
			// Extract state from event data if available
			var eventData map[string]interface{}
			if err := json.Unmarshal(evt.Data, &eventData); err == nil {
				if state, ok := eventData["state"].(map[string]interface{}); ok {
					for k, v := range state {
						if val, ok := v.(float64); ok {
							inst.State[k] = int(val)
						}
					}
				}
			}
		}

		instances = append(instances, inst)
	}

	return instances, total, rows.Err()
}

// GetStats returns statistics about stored aggregates.
func (s *SQLiteStore) GetStats(ctx context.Context) (*Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	// Count total unique streams
	var totalInstances int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT stream_id) FROM events",
	).Scan(&totalInstances)
	if err != nil {
		return nil, fmt.Errorf("failed to count instances: %w", err)
	}

	stats := &Stats{
		TotalInstances: totalInstances,
		ByPlace:        make(map[string]int),
	}

	// Get distinct stream IDs and compute their states
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT stream_id FROM events")
	if err != nil {
		return nil, fmt.Errorf("failed to query streams: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var streamID string
		if err := rows.Scan(&streamID); err != nil {
			continue
		}

		// Load events for this stream
		events, err := s.Read(ctx, streamID, 0)
		if err != nil {
			continue
		}

		// Find places with tokens in current state
		for _, evt := range events {
			var eventData map[string]interface{}
			if err := json.Unmarshal(evt.Data, &eventData); err == nil {
				if state, ok := eventData["state"].(map[string]interface{}); ok {
					for place, val := range state {
						if v, ok := val.(float64); ok && v > 0 {
							stats.ByPlace[place]++
						}
					}
				}
			}
		}
	}

	return stats, rows.Err()
}

// SearchResult represents a search hit.
type SearchResult struct {
	StreamID  string                 `json:"stream_id"`
	EventType string                 `json:"event_type"`
	Fields    map[string]interface{} `json:"fields"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// IndexEvent indexes searchable fields from an event.
// fields is a map of field names to extract from the event data.
func (s *SQLiteStore) IndexEvent(ctx context.Context, event *Event, fields []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	var eventData map[string]interface{}
	if err := json.Unmarshal(event.Data, &eventData); err != nil {
		return nil // Skip events with non-JSON data
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Index specified fields
	for _, field := range fields {
		if val, ok := eventData[field]; ok {
			var strVal string
			switch v := val.(type) {
			case string:
				strVal = v
			case []interface{}:
				// Join array values
				parts := make([]string, 0, len(v))
				for _, item := range v {
					if s, ok := item.(string); ok {
						parts = append(parts, s)
					}
				}
				strVal = strings.Join(parts, " ")
			default:
				data, _ := json.Marshal(v)
				strVal = string(data)
			}

			_, err := tx.ExecContext(ctx,
				`INSERT OR REPLACE INTO search_index (stream_id, field, value, event_type, updated_at)
				 VALUES (?, ?, ?, ?, ?)`,
				event.StreamID, field, strVal, event.Type, now,
			)
			if err != nil {
				return fmt.Errorf("failed to index field %s: %w", field, err)
			}
		}
	}

	// Try to update FTS index (ignore errors if FTS5 not available)
	title, _ := eventData["title"].(string)
	content, _ := eventData["content"].(string)
	var tags string
	if t, ok := eventData["tags"].([]interface{}); ok {
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		tags = strings.Join(parts, " ")
	}

	if title != "" || content != "" || tags != "" {
		// Delete existing entry first
		tx.ExecContext(ctx, "DELETE FROM search_fts WHERE stream_id = ?", event.StreamID)
		// Insert new entry
		tx.ExecContext(ctx,
			"INSERT INTO search_fts (stream_id, title, content, tags) VALUES (?, ?, ?, ?)",
			event.StreamID, title, content, tags,
		)
	}

	return tx.Commit()
}

// Search performs a search across indexed fields.
// query is the search term, field optionally limits to a specific field.
func (s *SQLiteStore) Search(ctx context.Context, query string, field string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	if limit <= 0 {
		limit = 50
	}

	var results []SearchResult

	// Try FTS5 search first for better relevance
	if field == "" || field == "title" || field == "content" || field == "tags" {
		ftsQuery := `
			SELECT stream_id FROM search_fts
			WHERE search_fts MATCH ?
			ORDER BY rank
			LIMIT ?
		`
		rows, err := s.db.QueryContext(ctx, ftsQuery, query, limit)
		if err == nil {
			defer rows.Close()
			streamIDs := make(map[string]bool)
			for rows.Next() {
				var streamID string
				if err := rows.Scan(&streamID); err == nil {
					streamIDs[streamID] = true
				}
			}

			// Get full results for matched streams
			for streamID := range streamIDs {
				result := SearchResult{
					StreamID: streamID,
					Fields:   make(map[string]interface{}),
				}

				// Load indexed fields
				fieldRows, err := s.db.QueryContext(ctx,
					"SELECT field, value, event_type, updated_at FROM search_index WHERE stream_id = ?",
					streamID,
				)
				if err == nil {
					for fieldRows.Next() {
						var f, v, et, ua string
						if err := fieldRows.Scan(&f, &v, &et, &ua); err == nil {
							result.Fields[f] = v
							result.EventType = et
							result.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
						}
					}
					fieldRows.Close()
				}

				results = append(results, result)
			}

			if len(results) > 0 {
				return results, nil
			}
		}
	}

	// Fallback to LIKE search on search_index
	var sqlQuery string
	var args []interface{}

	if field != "" {
		sqlQuery = `
			SELECT DISTINCT stream_id, field, value, event_type, updated_at
			FROM search_index
			WHERE field = ? AND value LIKE ?
			ORDER BY updated_at DESC
			LIMIT ?
		`
		args = []interface{}{field, "%" + query + "%", limit}
	} else {
		sqlQuery = `
			SELECT DISTINCT stream_id, field, value, event_type, updated_at
			FROM search_index
			WHERE value LIKE ?
			ORDER BY updated_at DESC
			LIMIT ?
		`
		args = []interface{}{"%" + query + "%", limit}
	}

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	// Group by stream_id
	resultMap := make(map[string]*SearchResult)
	for rows.Next() {
		var streamID, f, v, et, ua string
		if err := rows.Scan(&streamID, &f, &v, &et, &ua); err != nil {
			continue
		}

		if _, exists := resultMap[streamID]; !exists {
			resultMap[streamID] = &SearchResult{
				StreamID:  streamID,
				EventType: et,
				Fields:    make(map[string]interface{}),
			}
			resultMap[streamID].UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
		}
		resultMap[streamID].Fields[f] = v
	}

	for _, r := range resultMap {
		results = append(results, *r)
	}

	return results, nil
}

// Ensure SQLiteStore implements Store and AdminStore.
var (
	_ Store      = (*SQLiteStore)(nil)
	_ AdminStore = (*SQLiteStore)(nil)
)
