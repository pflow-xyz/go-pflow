// Package storage provides SQLite-based session logging for catacombs games.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store handles SQLite database operations for game session logging.
type Store struct {
	db *sql.DB
}

// Session represents a game session record.
type Session struct {
	ID           string    `json:"id"`
	Seed         int64     `json:"seed"`
	Mode         string    `json:"mode"` // "normal", "demo", "infinite"
	StartedAt    time.Time `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	FinalLevel   int       `json:"final_level"`
	FinalHP      int       `json:"final_hp"`
	TotalTicks   int       `json:"total_ticks"`
	AIEnabled    bool      `json:"ai_enabled"`
	GameOver     bool      `json:"game_over"`
	Victory      bool      `json:"victory"`
}

// Action represents a single game action/tick record.
type Action struct {
	ID           int64     `json:"id"`
	SessionID    string    `json:"session_id"`
	Tick         int       `json:"tick"`
	Timestamp    time.Time `json:"timestamp"`
	Level        int       `json:"level"`
	Action       string    `json:"action"`
	PlayerX      int       `json:"player_x"`
	PlayerY      int       `json:"player_y"`
	PlayerHP     int       `json:"player_hp"`
	PlayerMaxHP  int       `json:"player_max_hp"`
	AIMode       string    `json:"ai_mode,omitempty"`
	AITarget     string    `json:"ai_target,omitempty"`
	EnemiesAlive int       `json:"enemies_alive"`
	InCombat     bool      `json:"in_combat"`
	ExtraData    string    `json:"extra_data,omitempty"` // JSON for additional context
}

// New creates a new Store with the given database path.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return store, nil
}

// migrate creates the database schema if it doesn't exist.
func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		seed INTEGER NOT NULL,
		mode TEXT NOT NULL DEFAULT 'normal',
		started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		ended_at DATETIME,
		final_level INTEGER DEFAULT 1,
		final_hp INTEGER DEFAULT 100,
		total_ticks INTEGER DEFAULT 0,
		ai_enabled INTEGER DEFAULT 0,
		game_over INTEGER DEFAULT 0,
		victory INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS actions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		tick INTEGER NOT NULL,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		level INTEGER NOT NULL,
		action TEXT NOT NULL,
		player_x INTEGER NOT NULL,
		player_y INTEGER NOT NULL,
		player_hp INTEGER NOT NULL,
		player_max_hp INTEGER NOT NULL,
		ai_mode TEXT,
		ai_target TEXT,
		enemies_alive INTEGER DEFAULT 0,
		in_combat INTEGER DEFAULT 0,
		extra_data TEXT,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE INDEX IF NOT EXISTS idx_actions_session ON actions(session_id);
	CREATE INDEX IF NOT EXISTS idx_actions_session_tick ON actions(session_id, tick);
	CREATE INDEX IF NOT EXISTS idx_actions_level ON actions(session_id, level);
	CREATE INDEX IF NOT EXISTS idx_sessions_seed ON sessions(seed);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for custom queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

// CreateSession creates a new session record.
func (s *Store) CreateSession(id string, seed int64, mode string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, seed, mode, started_at) VALUES (?, ?, ?, ?)`,
		id, seed, mode, time.Now().UTC(),
	)
	return err
}

// UpdateSession updates session metadata.
func (s *Store) UpdateSession(id string, level, hp, ticks int, aiEnabled, gameOver, victory bool) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET final_level = ?, final_hp = ?, total_ticks = ?,
		 ai_enabled = ?, game_over = ?, victory = ?, ended_at = ?
		 WHERE id = ?`,
		level, hp, ticks, aiEnabled, gameOver, victory, time.Now().UTC(), id,
	)
	return err
}

// EndSession marks a session as ended.
func (s *Store) EndSession(id string, level, hp, ticks int, gameOver, victory bool) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET ended_at = ?, final_level = ?, final_hp = ?,
		 total_ticks = ?, game_over = ?, victory = ?
		 WHERE id = ?`,
		time.Now().UTC(), level, hp, ticks, gameOver, victory, id,
	)
	return err
}

// LogAction logs a single game action.
func (s *Store) LogAction(a *Action) error {
	_, err := s.db.Exec(
		`INSERT INTO actions (session_id, tick, timestamp, level, action,
		 player_x, player_y, player_hp, player_max_hp, ai_mode, ai_target,
		 enemies_alive, in_combat, extra_data)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.SessionID, a.Tick, time.Now().UTC(), a.Level, a.Action,
		a.PlayerX, a.PlayerY, a.PlayerHP, a.PlayerMaxHP, a.AIMode, a.AITarget,
		a.EnemiesAlive, a.InCombat, a.ExtraData,
	)
	return err
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(id string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, seed, mode, started_at, ended_at, final_level, final_hp,
		 total_ticks, ai_enabled, game_over, victory
		 FROM sessions WHERE id = ?`, id,
	)

	var sess Session
	var endedAt sql.NullTime
	err := row.Scan(&sess.ID, &sess.Seed, &sess.Mode, &sess.StartedAt, &endedAt,
		&sess.FinalLevel, &sess.FinalHP, &sess.TotalTicks, &sess.AIEnabled,
		&sess.GameOver, &sess.Victory)
	if err != nil {
		return nil, err
	}
	if endedAt.Valid {
		sess.EndedAt = &endedAt.Time
	}
	return &sess, nil
}

// GetSessionBySeed retrieves sessions by seed.
func (s *Store) GetSessionsBySeed(seed int64) ([]*Session, error) {
	rows, err := s.db.Query(
		`SELECT id, seed, mode, started_at, ended_at, final_level, final_hp,
		 total_ticks, ai_enabled, game_over, victory
		 FROM sessions WHERE seed = ? ORDER BY started_at DESC`, seed,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var sess Session
		var endedAt sql.NullTime
		err := rows.Scan(&sess.ID, &sess.Seed, &sess.Mode, &sess.StartedAt, &endedAt,
			&sess.FinalLevel, &sess.FinalHP, &sess.TotalTicks, &sess.AIEnabled,
			&sess.GameOver, &sess.Victory)
		if err != nil {
			return nil, err
		}
		if endedAt.Valid {
			sess.EndedAt = &endedAt.Time
		}
		sessions = append(sessions, &sess)
	}
	return sessions, nil
}

// GetActions retrieves all actions for a session.
func (s *Store) GetActions(sessionID string) ([]*Action, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, tick, timestamp, level, action, player_x, player_y,
		 player_hp, player_max_hp, ai_mode, ai_target, enemies_alive, in_combat, extra_data
		 FROM actions WHERE session_id = ? ORDER BY tick`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []*Action
	for rows.Next() {
		var a Action
		var aiMode, aiTarget, extraData sql.NullString
		err := rows.Scan(&a.ID, &a.SessionID, &a.Tick, &a.Timestamp, &a.Level,
			&a.Action, &a.PlayerX, &a.PlayerY, &a.PlayerHP, &a.PlayerMaxHP,
			&aiMode, &aiTarget, &a.EnemiesAlive, &a.InCombat, &extraData)
		if err != nil {
			return nil, err
		}
		if aiMode.Valid {
			a.AIMode = aiMode.String
		}
		if aiTarget.Valid {
			a.AITarget = aiTarget.String
		}
		if extraData.Valid {
			a.ExtraData = extraData.String
		}
		actions = append(actions, &a)
	}
	return actions, nil
}

// GetActionsForLevel retrieves actions for a specific level in a session.
func (s *Store) GetActionsForLevel(sessionID string, level int) ([]*Action, error) {
	rows, err := s.db.Query(
		`SELECT id, session_id, tick, timestamp, level, action, player_x, player_y,
		 player_hp, player_max_hp, ai_mode, ai_target, enemies_alive, in_combat, extra_data
		 FROM actions WHERE session_id = ? AND level = ? ORDER BY tick`, sessionID, level,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []*Action
	for rows.Next() {
		var a Action
		var aiMode, aiTarget, extraData sql.NullString
		err := rows.Scan(&a.ID, &a.SessionID, &a.Tick, &a.Timestamp, &a.Level,
			&a.Action, &a.PlayerX, &a.PlayerY, &a.PlayerHP, &a.PlayerMaxHP,
			&aiMode, &aiTarget, &a.EnemiesAlive, &a.InCombat, &extraData)
		if err != nil {
			return nil, err
		}
		if aiMode.Valid {
			a.AIMode = aiMode.String
		}
		if aiTarget.Valid {
			a.AITarget = aiTarget.String
		}
		if extraData.Valid {
			a.ExtraData = extraData.String
		}
		actions = append(actions, &a)
	}
	return actions, nil
}

// LevelSummary provides aggregated stats for a level.
type LevelSummary struct {
	Level      int `json:"level"`
	StartTick  int `json:"start_tick"`
	EndTick    int `json:"end_tick"`
	Ticks      int `json:"ticks"`
	StartHP    int `json:"start_hp"`
	EndHP      int `json:"end_hp"`
	MinHP      int `json:"min_hp"`
	Combats    int `json:"combats"`
	Actions    map[string]int `json:"actions"`
}

// GetLevelSummaries returns aggregated stats per level for a session.
func (s *Store) GetLevelSummaries(sessionID string) ([]*LevelSummary, error) {
	rows, err := s.db.Query(
		`SELECT level,
		 MIN(tick) as start_tick, MAX(tick) as end_tick,
		 COUNT(*) as ticks,
		 (SELECT player_hp FROM actions a2 WHERE a2.session_id = actions.session_id AND a2.level = actions.level ORDER BY tick LIMIT 1) as start_hp,
		 (SELECT player_hp FROM actions a2 WHERE a2.session_id = actions.session_id AND a2.level = actions.level ORDER BY tick DESC LIMIT 1) as end_hp,
		 MIN(player_hp) as min_hp,
		 SUM(in_combat) as combats
		 FROM actions WHERE session_id = ?
		 GROUP BY level ORDER BY level`, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []*LevelSummary
	for rows.Next() {
		var ls LevelSummary
		err := rows.Scan(&ls.Level, &ls.StartTick, &ls.EndTick, &ls.Ticks,
			&ls.StartHP, &ls.EndHP, &ls.MinHP, &ls.Combats)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, &ls)
	}

	// Get action counts per level
	for _, ls := range summaries {
		ls.Actions = make(map[string]int)
		actionRows, err := s.db.Query(
			`SELECT action, COUNT(*) FROM actions
			 WHERE session_id = ? AND level = ? GROUP BY action`,
			sessionID, ls.Level,
		)
		if err != nil {
			continue
		}
		for actionRows.Next() {
			var action string
			var count int
			actionRows.Scan(&action, &count)
			ls.Actions[action] = count
		}
		actionRows.Close()
	}

	return summaries, nil
}

// RecentSessions returns the most recent sessions.
func (s *Store) RecentSessions(limit int) ([]*Session, error) {
	rows, err := s.db.Query(
		`SELECT id, seed, mode, started_at, ended_at, final_level, final_hp,
		 total_ticks, ai_enabled, game_over, victory
		 FROM sessions ORDER BY started_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var sess Session
		var endedAt sql.NullTime
		err := rows.Scan(&sess.ID, &sess.Seed, &sess.Mode, &sess.StartedAt, &endedAt,
			&sess.FinalLevel, &sess.FinalHP, &sess.TotalTicks, &sess.AIEnabled,
			&sess.GameOver, &sess.Victory)
		if err != nil {
			return nil, err
		}
		if endedAt.Valid {
			sess.EndedAt = &endedAt.Time
		}
		sessions = append(sessions, &sess)
	}
	return sessions, nil
}

// ExportSessionJSON exports a session and its actions as JSON.
func (s *Store) ExportSessionJSON(sessionID string) ([]byte, error) {
	sess, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	actions, err := s.GetActions(sessionID)
	if err != nil {
		return nil, err
	}

	summaries, err := s.GetLevelSummaries(sessionID)
	if err != nil {
		return nil, err
	}

	export := map[string]any{
		"session":   sess,
		"actions":   actions,
		"summaries": summaries,
	}

	return json.MarshalIndent(export, "", "  ")
}
