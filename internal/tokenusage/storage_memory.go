package tokenusage

import (
	"sync"
	"time"
)

// MemoryStorage 内存存储实现。
type MemoryStorage struct {
	mu         sync.RWMutex
	sessions   map[string]*SessionStats
	historical *HistoricalData
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		sessions:   map[string]*SessionStats{},
		historical: newHistoricalData(),
	}
}

func (s *MemoryStorage) SaveSession(sessionID string, stats *SessionStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = cloneSessionStats(stats)
	return nil
}

func (s *MemoryStorage) LoadSession(sessionID string) (*SessionStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats, ok := s.sessions[sessionID]
	if !ok {
		return nil, wrapError(ErrCodeNotFound, "session not found", nil)
	}
	return cloneSessionStats(stats), nil
}

func (s *MemoryStorage) SaveHistorical(data *HistoricalData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.historical = cloneHistoricalData(data)
	return nil
}

func (s *MemoryStorage) LoadHistorical() (*HistoricalData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneHistoricalData(s.historical), nil
}

func (s *MemoryStorage) ListSessions(start, end time.Time) ([]*SessionStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*SessionStats, 0, len(s.sessions))
	for _, stats := range s.sessions {
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

func (s *MemoryStorage) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

func (s *MemoryStorage) Cleanup() error {
	return nil
}
