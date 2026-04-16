package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	corepkg "bytemind/internal/core"
)

func TestFileTaskStoreAppendAndReadLogFromBoundaries(t *testing.T) {
	locker := NewInMemoryLocker()
	store, err := NewFileTaskStore(t.TempDir(), locker)
	if err != nil {
		t.Fatal(err)
	}

	taskID := corepkg.TaskID("task-append-read")
	offset1, err := store.AppendLog(context.Background(), taskID, TaskLogRecord{
		Type:    "status",
		EventID: "evt-1",
		Payload: []byte(`{"step":1}`),
	})
	if err != nil {
		t.Fatalf("AppendLog first event failed: %v", err)
	}
	offset2, err := store.AppendLog(context.Background(), taskID, TaskLogRecord{
		Type:    "result",
		EventID: "evt-2",
		Payload: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("AppendLog second event failed: %v", err)
	}
	if offset2 <= offset1 {
		t.Fatalf("expected second offset > first offset, got first=%d second=%d", offset1, offset2)
	}

	batch1, next1, err := store.ReadLogFrom(context.Background(), taskID, 0, 1)
	if err != nil {
		t.Fatalf("ReadLogFrom first batch failed: %v", err)
	}
	if len(batch1) != 1 {
		t.Fatalf("expected one record from first batch, got %d", len(batch1))
	}
	if next1 <= 0 {
		t.Fatalf("expected next offset to advance, got %d", next1)
	}

	batch2, next2, err := store.ReadLogFrom(context.Background(), taskID, next1, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom second batch failed: %v", err)
	}
	if len(batch2) == 0 {
		t.Fatal("expected second batch to contain remaining records")
	}
	if next2 < next1 {
		t.Fatalf("expected next offset to be monotonic, first=%d second=%d", next1, next2)
	}

	path := store.taskLogPath("task-append-read")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	fileSize := info.Size()

	emptyAtSize, nextAtSize, err := store.ReadLogFrom(context.Background(), taskID, fileSize, 1)
	if err != nil {
		t.Fatalf("ReadLogFrom at size failed: %v", err)
	}
	if len(emptyAtSize) != 0 {
		t.Fatalf("expected empty result at file size, got %d", len(emptyAtSize))
	}
	if nextAtSize != fileSize {
		t.Fatalf("expected next offset %d, got %d", fileSize, nextAtSize)
	}

	emptyPastSize, nextPastSize, err := store.ReadLogFrom(context.Background(), taskID, fileSize+50, 1)
	if err != nil {
		t.Fatalf("ReadLogFrom past size failed: %v", err)
	}
	if len(emptyPastSize) != 0 {
		t.Fatalf("expected empty result past size, got %d", len(emptyPastSize))
	}
	if nextPastSize != fileSize {
		t.Fatalf("expected next offset clamped to %d, got %d", fileSize, nextPastSize)
	}

	if _, _, err := store.ReadLogFrom(context.Background(), taskID, -1, 1); err == nil {
		t.Fatal("expected negative offset to fail")
	}

	defaultLimitRecords, _, err := store.ReadLogFrom(context.Background(), taskID, 0, 0)
	if err != nil {
		t.Fatalf("ReadLogFrom default limit failed: %v", err)
	}
	if len(defaultLimitRecords) == 0 {
		t.Fatal("expected default limit path to return records")
	}
}

func TestFileTaskStoreReadLogFromSkipsCorruptedLines(t *testing.T) {
	store, err := NewFileTaskStore(t.TempDir(), NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}

	taskID := corepkg.TaskID("task-corrupted")
	if _, err := store.AppendLog(context.Background(), taskID, TaskLogRecord{
		Type:    "status",
		EventID: "evt-1",
		Payload: []byte(`{"phase":"start"}`),
	}); err != nil {
		t.Fatalf("AppendLog first event failed: %v", err)
	}

	path := store.taskLogPath("task-corrupted")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("{bad-json\n")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AppendLog(context.Background(), taskID, TaskLogRecord{
		Type:    "result",
		EventID: "evt-2",
		Payload: []byte(`{"phase":"done"}`),
	}); err != nil {
		t.Fatalf("AppendLog second event failed: %v", err)
	}

	records, next, err := store.ReadLogFrom(context.Background(), taskID, 0, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected two valid records after skipping corruption, got %d", len(records))
	}
	if records[0].EventID != "evt-1" || records[1].EventID != "evt-2" {
		t.Fatalf("unexpected record sequence: %#v", records)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if next != info.Size() {
		t.Fatalf("expected next offset %d, got %d", info.Size(), next)
	}
}

func TestFileTaskStoreReadLogFromKeepsOffsetForTrailingPartialLine(t *testing.T) {
	store, err := NewFileTaskStore(t.TempDir(), NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	taskID := corepkg.TaskID("task-partial")
	path := store.taskLogPath("task-partial")

	envelope := taskLogEnvelope{
		Version:   taskLogSchemaVersion,
		Timestamp: time.Now().UTC(),
		EventID:   "evt-partial",
		Type:      "status",
		Payload:   json.RawMessage(`{"ok":true}`),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	cut := len(encoded) / 2
	if cut <= 0 || cut >= len(encoded) {
		t.Fatalf("unexpected cut index %d for payload size %d", cut, len(encoded))
	}
	if err := os.WriteFile(path, encoded[:cut], 0o644); err != nil {
		t.Fatal(err)
	}

	records, next, err := store.ReadLogFrom(context.Background(), taskID, 0, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom on trailing partial line failed: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no records for trailing partial line, got %d", len(records))
	}
	if next != 0 {
		t.Fatalf("expected next offset to stay at line start 0, got %d", next)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(encoded[cut:], '\n')); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	records, next, err = store.ReadLogFrom(context.Background(), taskID, 0, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom after partial completion failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record after completion, got %d", len(records))
	}
	if records[0].EventID != "evt-partial" {
		t.Fatalf("expected event id %q, got %q", "evt-partial", records[0].EventID)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if next != info.Size() {
		t.Fatalf("expected next offset %d, got %d", info.Size(), next)
	}
}

func TestFileTaskStoreAppendLogHonorsLockTimeout(t *testing.T) {
	locker := NewInMemoryLocker()
	store, err := NewFileTaskStore(t.TempDir(), locker)
	if err != nil {
		t.Fatal(err)
	}

	heldUnlock, err := locker.LockTask(context.Background(), corepkg.TaskID("task-timeout"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = heldUnlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = store.AppendLog(ctx, corepkg.TaskID("task-timeout"), TaskLogRecord{
		Type:    "status",
		EventID: "evt-timeout",
	})
	if err == nil {
		t.Fatal("expected AppendLog to fail on lock timeout")
	}
	if !hasErrorCode(err, ErrCodeLockTimeout) {
		t.Fatalf("expected lock timeout error code, got %v", err)
	}
}

func TestNormalizeTaskIDRejectsInvalidForms(t *testing.T) {
	tests := []corepkg.TaskID{
		"",
		" ",
		".",
		"..",
		"../escape",
		"..\\escape",
		"a/b",
		"a\\b",
		"/abs",
		"\\abs",
	}
	for _, raw := range tests {
		if _, err := normalizeTaskID(raw); err == nil {
			t.Fatalf("expected normalizeTaskID(%q) to fail", raw)
		}
	}

	id, err := normalizeTaskID(corepkg.TaskID("task-ok"))
	if err != nil {
		t.Fatalf("expected valid task id, got %v", err)
	}
	if id != "task-ok" {
		t.Fatalf("unexpected normalized id %q", id)
	}
}

func TestFileTaskStoreReturnsEmptyForMissingTaskLog(t *testing.T) {
	store, err := NewFileTaskStore(t.TempDir(), NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	records, next, err := store.ReadLogFrom(context.Background(), corepkg.TaskID("missing"), 0, 10)
	if err != nil {
		t.Fatalf("expected missing task log to return empty, got %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no records for missing task log, got %d", len(records))
	}
	if next != 0 {
		t.Fatalf("expected next offset 0 for missing log, got %d", next)
	}
}

func TestFileTaskStoreAppendLogRejectsInvalidTaskID(t *testing.T) {
	store, err := NewFileTaskStore(t.TempDir(), NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AppendLog(context.Background(), corepkg.TaskID("../bad"), TaskLogRecord{Type: "status"})
	if err == nil {
		t.Fatal("expected invalid task id to fail")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
