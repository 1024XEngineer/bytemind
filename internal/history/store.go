package history

import (
	corepkg "bytemind/internal/core"
	"bytemind/internal/storage"
	"strings"
	"time"
)

type PromptEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Workspace string    `json:"workspace"`
	SessionID string    `json:"session_id"`
	Prompt    string    `json:"prompt"`
}

type PromptHistoryWriter interface {
	Append(workspace, sessionID, prompt string, at time.Time) error
}

type PromptHistoryStore struct {
	inner *storage.PromptHistoryStore
}

type NopPromptHistoryStore struct{}

func (NopPromptHistoryStore) Append(string, string, string, time.Time) error {
	return nil
}

type PromptSearchQuery = storage.PromptSearchQuery

func AppendPrompt(workspace, sessionID, prompt string, at time.Time) error {
	return storage.AppendPrompt(workspace, corepkg.SessionID(strings.TrimSpace(sessionID)), prompt, at)
}

func LoadRecentPrompts(limit int) ([]PromptEntry, error) {
	entries, err := storage.LoadRecentPrompts(limit)
	if err != nil {
		return nil, err
	}
	return fromStorageEntries(entries), nil
}

func NewDefaultPromptHistoryStore() (*PromptHistoryStore, error) {
	inner, err := storage.NewDefaultPromptHistoryStore()
	if err != nil {
		return nil, err
	}
	return &PromptHistoryStore{inner: inner}, nil
}

func DefaultPromptHistoryPath() (string, error) {
	return storage.DefaultPromptHistoryPath()
}

func (s *PromptHistoryStore) Append(workspace, sessionID, prompt string, at time.Time) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Append(workspace, corepkg.SessionID(strings.TrimSpace(sessionID)), prompt, at)
}

func (s *PromptHistoryStore) LoadRecent(limit int) ([]PromptEntry, error) {
	if s == nil || s.inner == nil {
		return nil, nil
	}
	entries, err := s.inner.LoadRecent(limit)
	if err != nil {
		return nil, err
	}
	return fromStorageEntries(entries), nil
}

func ParsePromptSearchQuery(raw string) PromptSearchQuery {
	return storage.ParsePromptSearchQuery(raw)
}

func FilterPromptEntries(entries []PromptEntry, rawQuery string, limit int) []PromptEntry {
	storageEntries := toStorageEntries(entries)
	filtered := storage.FilterPromptEntries(storageEntries, rawQuery, limit)
	return fromStorageEntries(filtered)
}

func toStorageEntries(entries []PromptEntry) []storage.PromptEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]storage.PromptEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, storage.PromptEntry{
			Timestamp: entry.Timestamp,
			Workspace: entry.Workspace,
			SessionID: corepkg.SessionID(strings.TrimSpace(entry.SessionID)),
			Prompt:    entry.Prompt,
		})
	}
	return out
}

func fromStorageEntries(entries []storage.PromptEntry) []PromptEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]PromptEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, PromptEntry{
			Timestamp: entry.Timestamp,
			Workspace: entry.Workspace,
			SessionID: strings.TrimSpace(string(entry.SessionID)),
			Prompt:    entry.Prompt,
		})
	}
	return out
}
