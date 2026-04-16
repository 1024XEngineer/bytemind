package storage

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bytemind/internal/config"
	corepkg "bytemind/internal/core"
)

const (
	taskLogSchemaVersion = 1
	defaultTaskReadLimit = 128
	taskLogFileExt       = ".log"
)

type TaskLogRecord struct {
	TaskID    corepkg.TaskID  `json:"-"`
	Offset    int64           `json:"-"`
	EventID   string          `json:"event_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"ts"`
}

type TaskStore interface {
	AppendLog(ctx context.Context, taskID corepkg.TaskID, record TaskLogRecord) (offset int64, err error)
	ReadLogFrom(ctx context.Context, taskID corepkg.TaskID, offset int64, limit int) (records []TaskLogRecord, next int64, err error)
}

type taskLogEnvelope struct {
	Version   int             `json:"v"`
	Timestamp time.Time       `json:"ts"`
	EventID   string          `json:"event_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type FileTaskStore struct {
	root             string
	locker           Locker
	now              func() time.Time
	newEventID       func() string
	defaultReadLimit int
}

func NewDefaultTaskStore(locker Locker) (*FileTaskStore, error) {
	home, err := config.ResolveHomeDir()
	if err != nil {
		return nil, err
	}
	return NewFileTaskStore(filepath.Join(home, "tasks"), locker)
}

func NewFileTaskStore(root string, locker Locker) (*FileTaskStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("task store root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if locker == nil {
		defaultLocker, err := NewDefaultLocker(filepath.Join(root, ".locks"))
		if err != nil {
			return nil, err
		}
		locker = defaultLocker
	}
	return &FileTaskStore{
		root:   root,
		locker: locker,
		now: func() time.Time {
			return time.Now().UTC()
		},
		newEventID: func() string {
			var entropy [8]byte
			if _, err := rand.Read(entropy[:]); err != nil {
				return fmt.Sprintf("tevt-%d", time.Now().UTC().UnixNano())
			}
			return "tevt-" + hex.EncodeToString(entropy[:])
		},
		defaultReadLimit: defaultTaskReadLimit,
	}, nil
}

func (s *FileTaskStore) AppendLog(ctx context.Context, taskID corepkg.TaskID, record TaskLogRecord) (offset int64, err error) {
	if s == nil {
		return 0, errors.New("task store is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	normalizedTaskID, err := normalizeTaskID(taskID)
	if err != nil {
		return 0, err
	}

	unlock, err := s.locker.LockTask(ctx, corepkg.TaskID(normalizedTaskID))
	if err != nil {
		return 0, err
	}
	defer func() {
		err = combineTaskUnlockError(err, unlock, normalizedTaskID)
	}()

	path := s.taskLogPath(normalizedTaskID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	offset, err = file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	createdAt := record.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = s.now()
	}
	eventID := strings.TrimSpace(record.EventID)
	if eventID == "" {
		eventID = s.newEventID()
	}
	eventType := strings.TrimSpace(record.Type)
	if eventType == "" {
		eventType = "log"
	}
	envelope := taskLogEnvelope{
		Version:   taskLogSchemaVersion,
		Timestamp: createdAt,
		EventID:   eventID,
		Type:      eventType,
		Payload:   cloneRawMessage(record.Payload),
	}
	line, err := json.Marshal(envelope)
	if err != nil {
		return 0, err
	}
	line = append(line, '\n')

	if _, err := file.Write(line); err != nil {
		return 0, err
	}
	if err := file.Sync(); err != nil {
		return 0, err
	}
	return offset, nil
}

func (s *FileTaskStore) ReadLogFrom(ctx context.Context, taskID corepkg.TaskID, offset int64, limit int) ([]TaskLogRecord, int64, error) {
	if s == nil {
		return nil, 0, errors.New("task store is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if offset < 0 {
		return nil, 0, errors.New("offset must be >= 0")
	}
	if limit <= 0 {
		limit = s.defaultReadLimit
		if limit <= 0 {
			limit = defaultTaskReadLimit
		}
	}

	normalizedTaskID, err := normalizeTaskID(taskID)
	if err != nil {
		return nil, 0, err
	}
	path := s.taskLogPath(normalizedTaskID)

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []TaskLogRecord{}, 0, nil
		}
		return nil, 0, err
	}
	fileSize := info.Size()
	if offset >= fileSize {
		return []TaskLogRecord{}, fileSize, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, err
	}

	records := make([]TaskLogRecord, 0, minInt(limit, 32))
	next := offset
	reader := bufio.NewReader(file)
	for len(records) < limit {
		select {
		case <-ctx.Done():
			return records, next, ctx.Err()
		default:
		}

		lineStart := next
		line, readErr := reader.ReadBytes('\n')
		// Keep offset pinned on trailing partial line (EOF without newline)
		// so later append can complete and be re-read.
		if errors.Is(readErr, io.EOF) && len(line) > 0 && line[len(line)-1] != '\n' {
			next = lineStart
			break
		}
		if len(line) > 0 {
			next = lineStart + int64(len(line))
			payload := bytes.TrimSpace(line)
			if len(payload) > 0 {
				var envelope taskLogEnvelope
				if err := json.Unmarshal(payload, &envelope); err != nil {
					log.Printf("task store: skipped corrupted log line at offset %d in %s: %v", lineStart, path, err)
				} else {
					record := TaskLogRecord{
						TaskID:    corepkg.TaskID(normalizedTaskID),
						Offset:    lineStart,
						EventID:   strings.TrimSpace(envelope.EventID),
						Type:      strings.TrimSpace(envelope.Type),
						Payload:   cloneRawMessage(envelope.Payload),
						CreatedAt: envelope.Timestamp.UTC(),
					}
					records = append(records, record)
				}
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return records, next, readErr
		}
		if next >= fileSize {
			break
		}
	}
	if next > fileSize {
		next = fileSize
	}
	return records, next, nil
}

func (s *FileTaskStore) taskLogPath(taskID string) string {
	return filepath.Join(s.root, taskID+taskLogFileExt)
}

func normalizeTaskID(taskID corepkg.TaskID) (string, error) {
	id := strings.TrimSpace(string(taskID))
	if id == "" {
		return "", errors.New("task id is required")
	}
	if id == "." || id == ".." {
		return "", errors.New("invalid task id")
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return "", errors.New("invalid task id")
	}
	if filepath.IsAbs(id) || filepath.VolumeName(id) != "" {
		return "", errors.New("invalid task id")
	}
	if cleaned := filepath.Clean(id); cleaned != id {
		return "", errors.New("invalid task id")
	}
	return id, nil
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}

func combineTaskUnlockError(baseErr error, unlock UnlockFunc, taskID string) error {
	if unlock == nil {
		return baseErr
	}
	unlockErr := unlock()
	if unlockErr == nil {
		return baseErr
	}
	if baseErr == nil {
		return fmt.Errorf("unlock task %q failed: %w", taskID, unlockErr)
	}
	return errors.Join(baseErr, fmt.Errorf("unlock task %q failed: %w", taskID, unlockErr))
}
