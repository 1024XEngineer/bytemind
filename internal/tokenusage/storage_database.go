package tokenusage

import (
	"database/sql"
	"encoding/json"
	"time"
)

// DatabaseStorage 数据库存储实现。
// 该实现依赖外部注册的 database/sql driver。
type DatabaseStorage struct {
	db *sql.DB
}

func NewDatabaseStorage(driverName, dsn string) (*DatabaseStorage, error) {
	if driverName == "" {
		return nil, wrapError(ErrCodeInvalidConfig, "database driver is required", nil)
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, wrapError(ErrCodeStorage, "open database failed", err)
	}
	st := &DatabaseStorage{db: db}
	if err := st.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

func (s *DatabaseStorage) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS token_usage_sessions (
			session_id TEXT PRIMARY KEY,
			start_time INTEGER NOT NULL,
			last_update INTEGER NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS token_usage_historical (
			id INTEGER PRIMARY KEY,
			payload TEXT NOT NULL
		)`,
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return wrapError(ErrCodeStorage, "init token usage schema failed", err)
		}
	}
	return nil
}

func (s *DatabaseStorage) SaveSession(sessionID string, stats *SessionStats) error {
	payload, err := json.Marshal(stats)
	if err != nil {
		return wrapError(ErrCodeStorage, "encode session payload failed", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO token_usage_sessions (session_id, start_time, last_update, payload)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		   start_time=excluded.start_time,
		   last_update=excluded.last_update,
		   payload=excluded.payload`,
		sessionID,
		stats.StartTime.Unix(),
		stats.LastUpdate.Unix(),
		string(payload),
	)
	if err != nil {
		return wrapError(ErrCodeStorage, "save session failed", err)
	}
	return nil
}

func (s *DatabaseStorage) LoadSession(sessionID string) (*SessionStats, error) {
	row := s.db.QueryRow(`SELECT payload FROM token_usage_sessions WHERE session_id = ?`, sessionID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, wrapError(ErrCodeNotFound, "session not found", nil)
		}
		return nil, wrapError(ErrCodeStorage, "load session failed", err)
	}
	var stats SessionStats
	if err := json.Unmarshal([]byte(raw), &stats); err != nil {
		return nil, wrapError(ErrCodeStorage, "decode session payload failed", err)
	}
	return &stats, nil
}

func (s *DatabaseStorage) SaveHistorical(data *HistoricalData) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return wrapError(ErrCodeStorage, "encode historical payload failed", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO token_usage_historical (id, payload)
		 VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`,
		string(raw),
	)
	if err != nil {
		return wrapError(ErrCodeStorage, "save historical failed", err)
	}
	return nil
}

func (s *DatabaseStorage) LoadHistorical() (*HistoricalData, error) {
	row := s.db.QueryRow(`SELECT payload FROM token_usage_historical WHERE id = 1`)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return newHistoricalData(), nil
		}
		return nil, wrapError(ErrCodeStorage, "load historical failed", err)
	}
	var data HistoricalData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, wrapError(ErrCodeStorage, "decode historical payload failed", err)
	}
	return cloneHistoricalData(&data), nil
}

func (s *DatabaseStorage) ListSessions(start, end time.Time) ([]*SessionStats, error) {
	query := `SELECT payload FROM token_usage_sessions WHERE 1=1`
	args := make([]any, 0, 2)
	if !start.IsZero() {
		query += ` AND last_update >= ?`
		args = append(args, start.Unix())
	}
	if !end.IsZero() {
		query += ` AND last_update <= ?`
		args = append(args, end.Unix())
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, wrapError(ErrCodeStorage, "list sessions failed", err)
	}
	defer rows.Close()

	out := make([]*SessionStats, 0, 64)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, wrapError(ErrCodeStorage, "scan session payload failed", err)
		}
		var stats SessionStats
		if err := json.Unmarshal([]byte(raw), &stats); err != nil {
			return nil, wrapError(ErrCodeStorage, "decode session payload failed", err)
		}
		out = append(out, &stats)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError(ErrCodeStorage, "iterate session rows failed", err)
	}
	return out, nil
}

func (s *DatabaseStorage) DeleteSession(sessionID string) error {
	if _, err := s.db.Exec(`DELETE FROM token_usage_sessions WHERE session_id = ?`, sessionID); err != nil {
		return wrapError(ErrCodeStorage, "delete session failed", err)
	}
	return nil
}

func (s *DatabaseStorage) Cleanup() error {
	return nil
}

func (s *DatabaseStorage) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
