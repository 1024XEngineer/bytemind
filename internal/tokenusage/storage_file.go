package tokenusage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type filePayload struct {
	Version    int                      `json:"version"`
	UpdatedAt  time.Time                `json:"updated_at"`
	Sessions   map[string]*SessionStats `json:"sessions"`
	Historical *HistoricalData          `json:"historical"`
}

// FileStorage JSON文件存储实现。
type FileStorage struct {
	mu      sync.RWMutex
	path    string
	payload filePayload
}

func NewFileStorage(path string) (*FileStorage, error) {
	if path == "" {
		return nil, wrapError(ErrCodeInvalidConfig, "storage path is required for file storage", nil)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, wrapError(ErrCodeInvalidConfig, "resolve file storage path failed", err)
	}
	st := &FileStorage{
		path: absPath,
		payload: filePayload{
			Version:    1,
			UpdatedAt:  time.Now().UTC(),
			Sessions:   map[string]*SessionStats{},
			Historical: newHistoricalData(),
		},
	}
	if err := st.load(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *FileStorage) SaveSession(sessionID string, stats *SessionStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.Sessions[sessionID] = cloneSessionStats(stats)
	s.payload.UpdatedAt = time.Now().UTC()
	return s.saveLocked()
}

func (s *FileStorage) LoadSession(sessionID string) (*SessionStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats, ok := s.payload.Sessions[sessionID]
	if !ok {
		return nil, wrapError(ErrCodeNotFound, "session not found", nil)
	}
	return cloneSessionStats(stats), nil
}

func (s *FileStorage) SaveHistorical(data *HistoricalData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.Historical = cloneHistoricalData(data)
	s.payload.UpdatedAt = time.Now().UTC()
	return s.saveLocked()
}

func (s *FileStorage) LoadHistorical() (*HistoricalData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneHistoricalData(s.payload.Historical), nil
}

func (s *FileStorage) ListSessions(start, end time.Time) ([]*SessionStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*SessionStats, 0, len(s.payload.Sessions))
	for _, stats := range s.payload.Sessions {
		if !start.IsZero() && stats.LastUpdate.Before(start) {
			continue
		}
		if !end.IsZero() && stats.LastUpdate.After(end) {
			continue
		}
		out = append(out, cloneSessionStats(stats))
	}
	return out, nil
}

func (s *FileStorage) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.payload.Sessions, sessionID)
	s.payload.UpdatedAt = time.Now().UTC()
	return s.saveLocked()
}

func (s *FileStorage) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *FileStorage) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.saveLocked()
		}
		return wrapError(ErrCodeStorage, "read file storage failed", err)
	}
	if err := json.Unmarshal(data, &s.payload); err != nil {
		return wrapError(ErrCodeStorage, "decode file storage failed", err)
	}
	if s.payload.Sessions == nil {
		s.payload.Sessions = map[string]*SessionStats{}
	}
	s.payload.Historical = cloneHistoricalData(s.payload.Historical)
	return nil
}

func (s *FileStorage) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return wrapError(ErrCodeStorage, "create file storage dir failed", err)
	}
	data, err := json.MarshalIndent(s.payload, "", "  ")
	if err != nil {
		return wrapError(ErrCodeStorage, "encode file storage failed", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return wrapError(ErrCodeStorage, "write file storage failed", err)
	}
	return nil
}
