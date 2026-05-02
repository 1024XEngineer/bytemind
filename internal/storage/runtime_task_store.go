package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/1024XEngineer/bytemind/internal/config"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
)

const unknownTaskID = "unknown"

type RuntimeTaskEventAdapter struct {
	store TaskStore
}

func NewRuntimeTaskEventAdapter(store TaskStore) runtimepkg.TaskEventStore {
	if store == nil {
		return runtimepkg.NopTaskEventStore{}
	}
	return &RuntimeTaskEventAdapter{store: store}
}

func (a *RuntimeTaskEventAdapter) AppendTaskEvent(ctx context.Context, event runtimepkg.TaskEvent) error {
	if a == nil || a.store == nil {
		return nil
	}
	event.TaskID = corepkg.TaskID(sanitizeTaskID(event.TaskID))
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = a.store.AppendLog(ctx, event.TaskID, TaskLogRecord{
		Type:      runtimeTaskEventRecordType(event.Type),
		Payload:   payload,
		CreatedAt: event.Timestamp,
	})
	return err
}

func (a *RuntimeTaskEventAdapter) AppendTaskLog(ctx context.Context, taskID corepkg.TaskID, entry runtimepkg.TaskLogEntry) error {
	if a == nil || a.store == nil {
		return nil
	}
	if entry.TaskID == "" {
		entry.TaskID = taskID
	}
	entry.TaskID = corepkg.TaskID(sanitizeTaskID(entry.TaskID))
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = a.store.AppendLog(ctx, entry.TaskID, TaskLogRecord{
		Type:      TaskRecordTypeTaskLog,
		Payload:   payload,
		CreatedAt: entry.Timestamp,
	})
	return err
}

func runtimeTaskEventRecordType(eventType runtimepkg.TaskEventType) string {
	switch eventType {
	case runtimepkg.TaskEventStatus:
		return TaskRecordTypeTaskEventStatus
	case runtimepkg.TaskEventResult:
		return TaskRecordTypeTaskEventResult
	case runtimepkg.TaskEventError:
		return TaskRecordTypeTaskEventError
	case runtimepkg.TaskEventLog:
		return TaskRecordTypeTaskEventLog
	default:
		normalized := strings.TrimSpace(string(eventType))
		if normalized == "" {
			return TaskRecordTypeTaskEventStatus
		}
		replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
		normalized = replacer.Replace(normalized)
		return "task_event." + normalized
	}
}

// RuntimeTaskStore writes task events to ~/.bytemind/runtime/tasks/events|logs.
//
// Deprecated: use NewDefaultTaskStore + NewRuntimeTaskEventAdapter so runtime
// task events and logs are persisted into ~/.bytemind/tasks/<task-id>.log.
type RuntimeTaskStore struct {
	root string
	mu   sync.Mutex
}

// NewDefaultRuntimeTaskStore creates the legacy runtime task store.
//
// Deprecated: use NewDefaultTaskStore + NewRuntimeTaskEventAdapter instead.
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
	value := strings.TrimSpace(string(taskID))
	if value == "" {
		return unknownTaskID
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	value = strings.TrimSpace(replacer.Replace(value))
	if value == "" || value == "." || value == ".." {
		return unknownTaskID
	}
	return value
}
