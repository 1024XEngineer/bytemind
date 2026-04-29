package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
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

func TestNewDefaultTaskStoreUsesBytemindHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)

	store, err := NewDefaultTaskStore(NewInMemoryLocker())
	if err != nil {
		t.Fatalf("expected NewDefaultTaskStore to succeed, got %v", err)
	}
	expectedRoot := filepath.Join(home, "tasks")
	if store.root != expectedRoot {
		t.Fatalf("expected root %q, got %q", expectedRoot, store.root)
	}
}

func TestNewFileTaskStoreCreatesDefaultLockerWhenNil(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks")
	store, err := NewFileTaskStore(root, nil)
	if err != nil {
		t.Fatalf("expected NewFileTaskStore to succeed, got %v", err)
	}
	if store.locker == nil {
		t.Fatal("expected default locker when locker is nil")
	}
}

func TestNewFileTaskStoreWithOptionsAllowsDisablingSync(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks")
	store, err := NewFileTaskStoreWithOptions(root, NewInMemoryLocker(), TaskStoreOptions{
		SyncOnAppend: false,
	})
	if err != nil {
		t.Fatalf("expected NewFileTaskStoreWithOptions to succeed, got %v", err)
	}
	if store.syncOnAppend {
		t.Fatal("expected syncOnAppend=false when options disable fsync")
	}
	taskID := corepkg.TaskID("task-no-fsync")
	if _, err := store.AppendLog(context.Background(), taskID, TaskLogRecord{
		Type:    "status",
		EventID: "evt-no-fsync",
		Payload: []byte(`{"ok":true}`),
	}); err != nil {
		t.Fatalf("AppendLog failed with sync disabled: %v", err)
	}
	records, _, err := store.ReadLogFrom(context.Background(), taskID, 0, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom failed with sync disabled: %v", err)
	}
	if len(records) != 1 || records[0].EventID != "evt-no-fsync" {
		t.Fatalf("unexpected records with sync disabled: %#v", records)
	}
}

func TestCombineTaskUnlockError(t *testing.T) {
	plain := errors.New("write failed")
	if got := combineTaskUnlockError(plain, nil, "task-1"); !errors.Is(got, plain) {
		t.Fatalf("expected base error unchanged when unlock is nil, got %v", got)
	}

	unlockErr := errors.New("unlock failed")
	onlyUnlock := combineTaskUnlockError(nil, func() error { return unlockErr }, "task-1")
	if !errors.Is(onlyUnlock, unlockErr) {
		t.Fatalf("expected unlock-only error to wrap unlock failure, got %v", onlyUnlock)
	}
	if !strings.Contains(onlyUnlock.Error(), "unlock task \"task-1\" failed") {
		t.Fatalf("expected unlock context in error, got %v", onlyUnlock)
	}

	joined := combineTaskUnlockError(plain, func() error { return unlockErr }, "task-1")
	if !errors.Is(joined, plain) || !errors.Is(joined, unlockErr) {
		t.Fatalf("expected joined error to include base and unlock errors, got %v", joined)
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

func TestFileTaskStoreReadLogFromContinuesAcrossRestart(t *testing.T) {
	root := t.TempDir()
	storeA, err := NewFileTaskStore(root, NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	taskID := corepkg.TaskID("task-restart-offset")
	for _, id := range []string{"evt-r1", "evt-r2", "evt-r3"} {
		_, err := storeA.AppendLog(context.Background(), taskID, TaskLogRecord{
			Type:    "status",
			EventID: id,
			Payload: []byte(`{"step":"ok"}`),
		})
		if err != nil {
			t.Fatalf("AppendLog(%s) failed: %v", id, err)
		}
	}

	firstPage, next1, err := storeA.ReadLogFrom(context.Background(), taskID, 0, 1)
	if err != nil {
		t.Fatalf("ReadLogFrom first page failed: %v", err)
	}
	if len(firstPage) != 1 || firstPage[0].EventID != "evt-r1" {
		t.Fatalf("unexpected first page: %#v", firstPage)
	}

	storeB, err := NewFileTaskStore(root, NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	secondPage, next2, err := storeB.ReadLogFrom(context.Background(), taskID, next1, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom after restart failed: %v", err)
	}
	if len(secondPage) != 2 {
		t.Fatalf("expected two remaining records, got %d", len(secondPage))
	}
	if secondPage[0].EventID != "evt-r2" || secondPage[1].EventID != "evt-r3" {
		t.Fatalf("unexpected second page sequence: %#v", secondPage)
	}
	if next2 <= next1 {
		t.Fatalf("expected next offset to advance across restart, first=%d second=%d", next1, next2)
	}

	empty, next3, err := storeB.ReadLogFrom(context.Background(), taskID, next2, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom tail after restart failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no tail records, got %d", len(empty))
	}
	if next3 != next2 {
		t.Fatalf("expected stable tail next offset %d, got %d", next2, next3)
	}
}

func TestReplayTaskLogDeduplicatesByEventIDAcrossPagesAndRestart(t *testing.T) {
	root := t.TempDir()
	storeA, err := NewFileTaskStore(root, NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	taskID := corepkg.TaskID("task-replay-dedupe")

	records := []TaskLogRecord{
		{Type: "status", EventID: "evt-1", Payload: []byte(`{"step":1}`)},
		{Type: "status", EventID: "evt-2", Payload: []byte(`{"step":2}`)},
		{Type: "result", EventID: "evt-2", Payload: []byte(`{"step":2,"dup":true}`)},
		{Type: "result", EventID: "evt-3", Payload: []byte(`{"step":3}`)},
	}
	for i := range records {
		if _, err := storeA.AppendLog(context.Background(), taskID, records[i]); err != nil {
			t.Fatalf("AppendLog #%d failed: %v", i, err)
		}
	}

	replayA, err := ReplayTaskLog(context.Background(), storeA, taskID, 2)
	if err != nil {
		t.Fatalf("ReplayTaskLog first store failed: %v", err)
	}
	if len(replayA) != 3 {
		t.Fatalf("expected 3 deduplicated records, got %d", len(replayA))
	}
	if replayA[0].EventID != "evt-1" || replayA[1].EventID != "evt-2" || replayA[2].EventID != "evt-3" {
		t.Fatalf("unexpected replay event order: %#v", replayA)
	}
	if replayA[0].Offset >= replayA[1].Offset || replayA[1].Offset >= replayA[2].Offset {
		t.Fatalf("expected replay offsets to be strictly increasing, got [%d,%d,%d]", replayA[0].Offset, replayA[1].Offset, replayA[2].Offset)
	}

	storeB, err := NewFileTaskStore(root, NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	replayB, err := ReplayTaskLog(context.Background(), storeB, taskID, 2)
	if err != nil {
		t.Fatalf("ReplayTaskLog after restart failed: %v", err)
	}
	if len(replayB) != len(replayA) {
		t.Fatalf("expected restart replay size %d, got %d", len(replayA), len(replayB))
	}
	for i := range replayA {
		if replayB[i].EventID != replayA[i].EventID {
			t.Fatalf("event id mismatch at %d: %q != %q", i, replayB[i].EventID, replayA[i].EventID)
		}
		if replayB[i].Offset != replayA[i].Offset {
			t.Fatalf("offset mismatch at %d: %d != %d", i, replayB[i].Offset, replayA[i].Offset)
		}
	}
}

func TestReplayTaskLogRejectsNilStore(t *testing.T) {
	_, err := ReplayTaskLog(context.Background(), nil, corepkg.TaskID("task"), 10)
	if err == nil {
		t.Fatal("expected ReplayTaskLog to reject nil store")
	}
	if !strings.Contains(err.Error(), "task store is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReplayTaskLogUsesDefaultPageSizeWhenNonPositive(t *testing.T) {
	fake := &recordingTaskStore{
		batches: [][]TaskLogRecord{
			{{EventID: "evt-1", Offset: 0}},
			{},
		},
		nextOffsets: []int64{1, 1},
	}
	records, err := ReplayTaskLog(nil, fake, corepkg.TaskID("task"), 0)
	if err != nil {
		t.Fatalf("ReplayTaskLog failed: %v", err)
	}
	if fake.lastLimit != defaultTaskReadLimit {
		t.Fatalf("expected default page size %d, got %d", defaultTaskReadLimit, fake.lastLimit)
	}
	if len(records) != 1 || records[0].EventID != "evt-1" {
		t.Fatalf("unexpected replay records: %#v", records)
	}
}

func TestFileTaskStoreReadLogFromCorruptedAndPartialLineAcrossRestart(t *testing.T) {
	root := t.TempDir()
	storeA, err := NewFileTaskStore(root, NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	taskID := corepkg.TaskID("task-restart-corrupted-partial")

	if _, err := storeA.AppendLog(context.Background(), taskID, TaskLogRecord{
		Type:    "status",
		EventID: "evt-ok-1",
		Payload: []byte(`{"phase":"start"}`),
	}); err != nil {
		t.Fatalf("AppendLog first event failed: %v", err)
	}

	path := storeA.taskLogPath("task-restart-corrupted-partial")
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

	if _, err := storeA.AppendLog(context.Background(), taskID, TaskLogRecord{
		Type:    "status",
		EventID: "evt-ok-2",
		Payload: []byte(`{"phase":"done"}`),
	}); err != nil {
		t.Fatalf("AppendLog second event failed: %v", err)
	}

	infoBeforePartial, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	partialStart := infoBeforePartial.Size()
	partialEnvelope := taskLogEnvelope{
		Version:   taskLogSchemaVersion,
		Timestamp: time.Now().UTC(),
		EventID:   "evt-partial",
		Type:      "status",
		Payload:   json.RawMessage(`{"phase":"tail"}`),
	}
	encoded, err := json.Marshal(partialEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	cut := len(encoded) / 2
	if cut <= 0 || cut >= len(encoded) {
		t.Fatalf("unexpected cut index %d for payload size %d", cut, len(encoded))
	}

	file, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(encoded[:cut]); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	recordsA, nextA, err := storeA.ReadLogFrom(context.Background(), taskID, 0, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom before restart failed: %v", err)
	}
	if len(recordsA) != 2 {
		t.Fatalf("expected two valid records before partial tail, got %d", len(recordsA))
	}
	if recordsA[0].EventID != "evt-ok-1" || recordsA[1].EventID != "evt-ok-2" {
		t.Fatalf("unexpected records before restart: %#v", recordsA)
	}
	if nextA != partialStart {
		t.Fatalf("expected next offset pinned at partial start %d, got %d", partialStart, nextA)
	}

	storeB, err := NewFileTaskStore(root, NewInMemoryLocker())
	if err != nil {
		t.Fatal(err)
	}
	recordsB, nextB, err := storeB.ReadLogFrom(context.Background(), taskID, nextA, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom at pinned offset after restart failed: %v", err)
	}
	if len(recordsB) != 0 {
		t.Fatalf("expected no records at partial tail after restart, got %d", len(recordsB))
	}
	if nextB != nextA {
		t.Fatalf("expected next offset to stay pinned after restart at %d, got %d", nextA, nextB)
	}

	file, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
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

	resumed, nextC, err := storeB.ReadLogFrom(context.Background(), taskID, nextA, 10)
	if err != nil {
		t.Fatalf("ReadLogFrom resumed tail failed: %v", err)
	}
	if len(resumed) != 1 {
		t.Fatalf("expected one resumed record after completing partial line, got %d", len(resumed))
	}
	if resumed[0].EventID != "evt-partial" {
		t.Fatalf("expected resumed event id %q, got %q", "evt-partial", resumed[0].EventID)
	}
	infoAfterResume, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if nextC != infoAfterResume.Size() {
		t.Fatalf("expected next offset %d after resume, got %d", infoAfterResume.Size(), nextC)
	}
}

type recordingTaskStore struct {
	batches     [][]TaskLogRecord
	nextOffsets []int64
	index       int
	lastLimit   int
}

func (s *recordingTaskStore) AppendLog(context.Context, corepkg.TaskID, TaskLogRecord) (int64, error) {
	return 0, errors.New("not implemented")
}

func (s *recordingTaskStore) ReadLogFrom(_ context.Context, taskID corepkg.TaskID, offset int64, limit int) ([]TaskLogRecord, int64, error) {
	s.lastLimit = limit
	if s.index >= len(s.batches) {
		return []TaskLogRecord{}, offset, nil
	}
	batch := s.batches[s.index]
	next := offset
	if s.index < len(s.nextOffsets) {
		next = s.nextOffsets[s.index]
	}
	s.index++
	return batch, next, nil
}
