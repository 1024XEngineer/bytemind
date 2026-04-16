package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"bytemind/internal/config"
	corepkg "bytemind/internal/core"
	runtimepkg "bytemind/internal/runtime"
)

type RuntimeTaskStore struct {
	root string
	mu   sync.Mutex
}

func NewDefaultRuntimeTaskStore() (*RuntimeTaskStore, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, "runtime", "tasks")
	if err := os.MkdirAll(filepath.Join(root, "events"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		return nil, err
	}
	return &RuntimeTaskStore{root: root}, nil
}

func (s *RuntimeTaskStore) AppendTaskEvent(_ context.Context, event runtimepkg.TaskEvent) error {
	if s == nil {
		return nil
	}
	taskID := sanitizeTaskID(event.TaskID)
	path := filepath.Join(s.root, "events", taskID+".jsonl")
	return s.appendJSONL(path, event)
}

func (s *RuntimeTaskStore) AppendTaskLog(_ context.Context, taskID corepkg.TaskID, entry runtimepkg.TaskLogEntry) error {
	if s == nil {
		return nil
	}
	if entry.TaskID == "" {
		entry.TaskID = taskID
	}
	path := filepath.Join(s.root, "logs", sanitizeTaskID(taskID)+".jsonl")
	return s.appendJSONL(path, entry)
}

func (s *RuntimeTaskStore) appendJSONL(path string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(payload, '\n')); err != nil {
		return err
	}
	return nil
}

func sanitizeTaskID(taskID corepkg.TaskID) string {
	value := string(taskID)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return replacer.Replace(value)
}
