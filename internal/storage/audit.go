package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bytemind/internal/config"
	corepkg "bytemind/internal/core"
)

type AuditEvent struct {
	EventID    corepkg.EventID   `json:"event_id"`
	SessionID  corepkg.SessionID `json:"session_id,omitempty"`
	TaskID     corepkg.TaskID    `json:"task_id,omitempty"`
	TraceID    corepkg.TraceID   `json:"trace_id,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Actor      string            `json:"actor,omitempty"`
	Action     string            `json:"action"`
	Decision   corepkg.Decision  `json:"decision,omitempty"`
	ReasonCode string            `json:"reason_code,omitempty"`
	RiskLevel  corepkg.RiskLevel `json:"risk_level,omitempty"`
	Result     string            `json:"result,omitempty"`
	LatencyMS  int64             `json:"latency_ms,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type AuditStore interface {
	Append(ctx context.Context, event AuditEvent) error
}

type NopAuditStore struct{}

func (NopAuditStore) Append(context.Context, AuditEvent) error {
	return nil
}

type FileAuditStore struct {
	dir string
	mu  sync.Mutex
}

func NewDefaultAuditStore() (AuditStore, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return nil, err
	}
	return NewFileAuditStore(filepath.Join(home, "audit"))
}

func NewFileAuditStore(dir string) (*FileAuditStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileAuditStore{dir: dir}, nil
}

func (s *FileAuditStore) Append(_ context.Context, event AuditEvent) error {
	if s == nil {
		return nil
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	if event.EventID == "" {
		event.EventID = newAuditEventID()
	}

	dayFile := filepath.Join(s.dir, event.Timestamp.Format("2006-01-02")+".jsonl")
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(dayFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(append(payload, '\n')); err != nil {
		return err
	}
	return nil
}

func newAuditEventID() corepkg.EventID {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return corepkg.EventID(time.Now().UTC().Format("20060102150405.000000000"))
	}
	return corepkg.EventID(hex.EncodeToString(buf))
}
