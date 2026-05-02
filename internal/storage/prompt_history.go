package storage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

const (
	defaultPromptHistoryLimit = 5000
	promptHistoryFileName     = "prompt_history.jsonl"
	promptHistoryMaxLineBytes = 1024 * 1024
)

type PromptEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Workspace string            `json:"workspace"`
	SessionID corepkg.SessionID `json:"session_id"`
	Prompt    string            `json:"prompt"`
}

type PromptHistoryStore struct {
	path string
}

var promptAppendMu sync.Mutex

type PromptHistoryWriter interface {
	Append(workspace string, sessionID corepkg.SessionID, prompt string, at time.Time) error
}

type NopPromptHistoryStore struct{}

func (NopPromptHistoryStore) Append(string, corepkg.SessionID, string, time.Time) error {
	return nil
}

func AppendPrompt(workspace string, sessionID corepkg.SessionID, prompt string, at time.Time) error {
	store, err := NewDefaultPromptHistoryStore()
	if err != nil {
		return err
	}
	return store.Append(workspace, sessionID, prompt, at)
}

func LoadRecentPrompts(limit int) ([]PromptEntry, error) {
	store, err := NewDefaultPromptHistoryStore()
	if err != nil {
		return nil, err
	}
	return store.LoadRecent(limit)
}

func NewDefaultPromptHistoryStore() (*PromptHistoryStore, error) {
	path, err := DefaultPromptHistoryPath()
	if err != nil {
		return nil, err
	}
	return &PromptHistoryStore{path: path}, nil
}

func DefaultPromptHistoryPath() (string, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(home, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, promptHistoryFileName), nil
}

func (s *PromptHistoryStore) Append(workspace string, sessionID corepkg.SessionID, prompt string, at time.Time) error {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}

	entry := PromptEntry{
		Timestamp: at,
		Workspace: strings.TrimSpace(workspace),
		SessionID: corepkg.SessionID(strings.TrimSpace(string(sessionID))),
		Prompt:    prompt,
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	promptAppendMu.Lock()
	defer promptAppendMu.Unlock()

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(payload, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *PromptHistoryStore) LoadRecent(limit int) ([]PromptEntry, error) {
	if limit <= 0 {
		limit = defaultPromptHistoryLimit
	}

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	entries := make([]PromptEntry, 0, minInt(limit, 256))
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), promptHistoryMaxLineBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry PromptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entry.Prompt = strings.TrimSpace(entry.Prompt)
		if entry.Prompt == "" {
			continue
		}
		if entry.Timestamp.IsZero() {
			entry.Timestamp = time.Now().UTC()
		} else {
			entry.Timestamp = entry.Timestamp.UTC()
		}

		entries = append(entries, entry)
		if len(entries) > limit {
			copy(entries, entries[len(entries)-limit:])
			entries = entries[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
