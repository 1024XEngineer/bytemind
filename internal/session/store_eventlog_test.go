package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
)

func TestStoreSaveWritesNewEventLayoutOnly(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(t.TempDir())
	sess.ID = "layout-only"
	sess.Messages = []llm.Message{llm.NewUserTextMessage("hello")}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	paths, err := store.pathForSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Events); err != nil {
		t.Fatalf("expected events file to exist, got %v", err)
	}
	if _, err := os.Stat(paths.Snapshot); err != nil {
		t.Fatalf("expected snapshot file to exist, got %v", err)
	}
	if _, err := os.Stat(paths.Legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected legacy file to be absent, got %v", err)
	}
}

func TestStoreLoadReplaysSnapshotPlusIncrementalEvents(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store.snapshotEveryN = 1000
	store.snapshotEveryT = 24 * time.Hour

	sess := New(t.TempDir())
	sess.ID = "resume-replay"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	sess.Messages = append(sess.Messages, llm.NewUserTextMessage("incremental"))
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	paths, err := store.pathForSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := readSnapshotFile(paths.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	events, err := readEventsFromFile(paths.Events, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 3 {
		t.Fatalf("expected incremental events after snapshot, got %d", len(events))
	}
	if snap.LastSeq >= events[len(events)-1].Seq {
		t.Fatalf("expected snapshot to be older than latest event, snap=%d latest=%d", snap.LastSeq, events[len(events)-1].Seq)
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "incremental" {
		t.Fatalf("expected replayed incremental message, got %#v", loaded.Messages)
	}
}

func TestStoreReadFromOffsetBoundariesAndLimit(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(t.TempDir())
	sess.ID = "offset-boundary"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	sess.Messages = append(sess.Messages, llm.NewUserTextMessage("offset event"))
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	paths, err := store.pathForSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(paths.Events)
	if err != nil {
		t.Fatal(err)
	}
	fileSize := info.Size()

	if _, _, err := store.ReadFrom(sess.ID, -1, 1); err == nil {
		t.Fatal("expected negative offset to return error")
	}

	events, next, err := store.ReadFrom(sess.ID, fileSize, 1)
	if err != nil {
		t.Fatalf("ReadFrom at file size failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty result at file size, got %d", len(events))
	}
	if next != fileSize {
		t.Fatalf("expected next offset %d, got %d", fileSize, next)
	}

	events, next, err = store.ReadFrom(sess.ID, fileSize+128, 1)
	if err != nil {
		t.Fatalf("ReadFrom past file size failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty result past file size, got %d", len(events))
	}
	if next != fileSize {
		t.Fatalf("expected next offset clamped to %d, got %d", fileSize, next)
	}

	firstBatch, firstNext, err := store.ReadFrom(sess.ID, 0, 1)
	if err != nil {
		t.Fatalf("ReadFrom first batch failed: %v", err)
	}
	if len(firstBatch) != 1 {
		t.Fatalf("expected limit=1 to return one event, got %d", len(firstBatch))
	}
	if firstNext <= 0 {
		t.Fatalf("expected next offset to advance, got %d", firstNext)
	}

	secondBatch, secondNext, err := store.ReadFrom(sess.ID, firstNext, 1)
	if err != nil {
		t.Fatalf("ReadFrom second batch failed: %v", err)
	}
	if len(secondBatch) > 1 {
		t.Fatalf("expected second batch size <= 1, got %d", len(secondBatch))
	}
	if secondNext < firstNext {
		t.Fatalf("expected next offset to be monotonic, first=%d second=%d", firstNext, secondNext)
	}

	defaultLimitBatch, _, err := store.ReadFrom(sess.ID, 0, 0)
	if err != nil {
		t.Fatalf("ReadFrom with default limit failed: %v", err)
	}
	if len(defaultLimitBatch) == 0 {
		t.Fatal("expected default limit path to return events")
	}
}

func TestStoreReadFromSkipsCorruptedLines(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	paths, err := store.pathForWorkspaceSession(workspace, "offset-corrupted")
	if err != nil {
		t.Fatal(err)
	}

	base := New(workspace)
	base.ID = "offset-corrupted"
	basePayload, _ := json.Marshal(fullSessionPayload{Session: *base})
	appendPayload, _ := json.Marshal(turnAppendedPayload{
		Messages: []llm.Message{llm.NewUserTextMessage("ok")},
	})

	events := []SessionEvent{
		{
			EventID:       "r-1",
			SessionID:     "offset-corrupted",
			Seq:           1,
			Type:          eventTypeSessionCreated,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       basePayload,
		},
		{
			EventID:       "r-2",
			SessionID:     "offset-corrupted",
			Seq:           2,
			Type:          eventTypeTurnAppended,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       appendPayload,
		},
	}
	if err := appendEventLine(store.files, paths.Events, events[0]); err != nil {
		t.Fatal(err)
	}
	if err := store.files.AppendLine(paths.Events, []byte(`{bad-json`)); err != nil {
		t.Fatal(err)
	}
	if err := appendEventLine(store.files, paths.Events, events[1]); err != nil {
		t.Fatal(err)
	}

	read, next, err := store.ReadFrom("offset-corrupted", 0, 10)
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if len(read) != 2 {
		t.Fatalf("expected corrupted line to be skipped, got %d events", len(read))
	}
	if read[0].EventID != "r-1" || read[1].EventID != "r-2" {
		t.Fatalf("unexpected event order: %#v", read)
	}
	info, err := os.Stat(paths.Events)
	if err != nil {
		t.Fatal(err)
	}
	if next != info.Size() {
		t.Fatalf("expected next offset %d, got %d", info.Size(), next)
	}
}

func TestStoreReadFromKeepsOffsetForTrailingPartialLine(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	paths, err := store.pathForWorkspaceSession(workspace, "offset-partial")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Events), 0o755); err != nil {
		t.Fatal(err)
	}

	event := SessionEvent{
		EventID:       "p-1",
		SessionID:     "offset-partial",
		Seq:           1,
		Type:          eventTypeSessionCreated,
		TS:            time.Now().UTC(),
		SchemaVersion: eventSchemaVersion,
		Payload:       json.RawMessage(`{"session":{"id":"offset-partial"}}`),
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	cut := len(encoded) / 2
	if cut <= 0 || cut >= len(encoded) {
		t.Fatalf("unexpected cut index %d for payload size %d", cut, len(encoded))
	}

	if err := os.WriteFile(paths.Events, encoded[:cut], 0o644); err != nil {
		t.Fatal(err)
	}
	items, next, err := store.ReadFrom("offset-partial", 0, 10)
	if err != nil {
		t.Fatalf("ReadFrom on trailing partial line failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no events for trailing partial line, got %d", len(items))
	}
	if next != 0 {
		t.Fatalf("expected next offset to stay at line start 0, got %d", next)
	}

	file, err := os.OpenFile(paths.Events, os.O_WRONLY|os.O_APPEND, 0o644)
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

	items, next, err = store.ReadFrom("offset-partial", 0, 10)
	if err != nil {
		t.Fatalf("ReadFrom after partial completion failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one event after completion, got %d", len(items))
	}
	if items[0].EventID != "p-1" {
		t.Fatalf("expected event id %q, got %q", "p-1", items[0].EventID)
	}
	info, err := os.Stat(paths.Events)
	if err != nil {
		t.Fatal(err)
	}
	if next != info.Size() {
		t.Fatalf("expected next offset %d, got %d", info.Size(), next)
	}
}

func TestReplaySkipsDuplicateEventID(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	paths, err := store.pathForWorkspaceSession(workspace, "dedupe")
	if err != nil {
		t.Fatal(err)
	}

	base := New(workspace)
	base.ID = "dedupe"
	base.Messages = nil
	base.Conversation.Timeline = nil
	basePayload, _ := json.Marshal(fullSessionPayload{Session: *base})
	appendPayload, _ := json.Marshal(turnAppendedPayload{
		Messages: []llm.Message{llm.NewUserTextMessage("once")},
	})

	events := []SessionEvent{
		{
			EventID:       "e-1",
			SessionID:     "dedupe",
			Seq:           1,
			Type:          eventTypeSessionCreated,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       basePayload,
		},
		{
			EventID:       "e-2",
			SessionID:     "dedupe",
			Seq:           2,
			Type:          eventTypeTurnAppended,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       appendPayload,
		},
		{
			EventID:       "e-2",
			SessionID:     "dedupe",
			Seq:           3,
			Type:          eventTypeTurnAppended,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       appendPayload,
		},
	}
	for _, event := range events {
		if err := appendEventLine(store.files, paths.Events, event); err != nil {
			t.Fatal(err)
		}
	}

	loaded, err := store.Replay("dedupe")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("expected duplicate event_id to be ignored, got %#v", loaded.Messages)
	}
}

func TestReplayRejectsNonMonotonicSeq(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	paths, err := store.pathForWorkspaceSession(workspace, "non-monotonic")
	if err != nil {
		t.Fatal(err)
	}

	base := New(workspace)
	base.ID = "non-monotonic"
	basePayload, _ := json.Marshal(fullSessionPayload{Session: *base})
	appendPayload, _ := json.Marshal(turnAppendedPayload{
		Messages: []llm.Message{llm.NewUserTextMessage("bad-order")},
	})
	events := []SessionEvent{
		{
			EventID:       "n-1",
			SessionID:     "non-monotonic",
			Seq:           1,
			Type:          eventTypeSessionCreated,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       basePayload,
		},
		{
			EventID:       "n-2",
			SessionID:     "non-monotonic",
			Seq:           3,
			Type:          eventTypeTurnAppended,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       appendPayload,
		},
		{
			EventID:       "n-3",
			SessionID:     "non-monotonic",
			Seq:           2,
			Type:          eventTypeSnapshotCompacted,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       basePayload,
		},
	}
	for _, event := range events {
		if err := appendEventLine(store.files, paths.Events, event); err != nil {
			t.Fatal(err)
		}
	}

	_, err = store.Replay("non-monotonic")
	if err == nil || !strings.Contains(err.Error(), "non-monotonic event seq") {
		t.Fatalf("expected non-monotonic seq error, got %v", err)
	}
}

func TestSaveAppendsMonotonicSeq(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(t.TempDir())
	sess.ID = "seq-check"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	sess.Messages = append(sess.Messages, llm.NewUserTextMessage("a"))
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	sess.Messages = append(sess.Messages, llm.NewAssistantTextMessage("b"))
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	paths, err := store.pathForSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	events, err := readEventsFromFile(paths.Events, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 3 {
		t.Fatalf("expected events to be appended, got %d", len(events))
	}
	last := int64(0)
	for i, event := range events {
		if event.Seq <= last {
			t.Fatalf("event[%d] has non-monotonic seq %d <= %d", i, event.Seq, last)
		}
		last = event.Seq
	}
}

func TestStoreLoadFallsBackToLegacySnapshot(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	projectID := storagepkg.WorkspaceProjectID(workspace)
	legacyPath := filepath.Join(dir, projectID, "legacy-only.jsonl")
	legacy := New(workspace)
	legacy.ID = "legacy-only"
	legacy.Messages = []llm.Message{llm.NewUserTextMessage("legacy")}
	if err := writeSessionSnapshot(store.files, legacyPath, legacy); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load("legacy-only")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "legacy" {
		t.Fatalf("expected legacy load fallback, got %#v", got.Messages)
	}
}

func TestStoreReadEventsAndSnapshot(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(t.TempDir())
	sess.ID = "read-events-snapshot"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	sess.Messages = append(sess.Messages, llm.NewUserTextMessage("hello"))
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	events, err := store.ReadEvents(sess.ID, 0)
	if err != nil {
		t.Fatalf("ReadEvents failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected ReadEvents to return at least one event")
	}

	if err := store.Snapshot(sess.ID); err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	paths, err := store.pathForSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Snapshot); err != nil {
		t.Fatalf("expected snapshot to exist after Snapshot(), got %v", err)
	}
}

func TestStoreReadEventsAndSnapshotOnLegacySource(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	projectID := storagepkg.WorkspaceProjectID(workspace)
	legacyPath := filepath.Join(dir, projectID, "legacy-read-events.jsonl")
	legacy := New(workspace)
	legacy.ID = "legacy-read-events"
	if err := writeSessionSnapshot(store.files, legacyPath, legacy); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ReadEvents("legacy-read-events", 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ReadEvents on legacy source to return os.ErrNotExist, got %v", err)
	}
	if err := store.Snapshot("legacy-read-events"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected Snapshot on legacy source to return os.ErrNotExist, got %v", err)
	}
}

func TestReadEventsFromFileHandlesLargeLines(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	paths, err := store.pathForWorkspaceSession(workspace, "large-line")
	if err != nil {
		t.Fatal(err)
	}

	largePayload := map[string]any{
		"blob": strings.Repeat("x", 200*1024),
	}
	rawPayload, err := json.Marshal(largePayload)
	if err != nil {
		t.Fatal(err)
	}
	event := SessionEvent{
		EventID:       "large-1",
		SessionID:     "large-line",
		Seq:           1,
		Type:          eventTypeSnapshotCompacted,
		TS:            time.Now().UTC(),
		SchemaVersion: eventSchemaVersion,
		Payload:       rawPayload,
	}
	if err := appendEventLine(store.files, paths.Events, event); err != nil {
		t.Fatal(err)
	}

	events, err := readEventsFromFile(paths.Events, 0)
	if err != nil {
		t.Fatalf("readEventsFromFile should parse large line, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].EventID != "large-1" {
		t.Fatalf("expected event_id large-1, got %q", events[0].EventID)
	}
}

func TestReadEventsFromFileSkipsCorruptedLines(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	paths, err := store.pathForWorkspaceSession(workspace, "read-events-corrupted")
	if err != nil {
		t.Fatal(err)
	}

	makeEvent := func(id string, seq int64) SessionEvent {
		return SessionEvent{
			EventID:       id,
			SessionID:     "read-events-corrupted",
			Seq:           seq,
			Type:          eventTypeSessionCreated,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       json.RawMessage(`{"session":{"id":"read-events-corrupted"}}`),
		}
	}
	if err := appendEventLine(store.files, paths.Events, makeEvent("ok-1", 1)); err != nil {
		t.Fatal(err)
	}
	if err := store.files.AppendLine(paths.Events, []byte(`{this-is-bad-json`)); err != nil {
		t.Fatal(err)
	}
	if err := appendEventLine(store.files, paths.Events, makeEvent("ok-2", 2)); err != nil {
		t.Fatal(err)
	}

	events, err := readEventsFromFile(paths.Events, 0)
	if err != nil {
		t.Fatalf("readEventsFromFile should skip corrupted lines, got %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 valid events, got %d", len(events))
	}
	if events[0].EventID != "ok-1" || events[1].EventID != "ok-2" {
		t.Fatalf("unexpected events order: %#v", events)
	}
}

func TestReplaySkipsDuplicateEventIDAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	storeA, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	paths, err := storeA.pathForWorkspaceSession(workspace, "dedupe-restart")
	if err != nil {
		t.Fatal(err)
	}

	base := New(workspace)
	base.ID = "dedupe-restart"
	base.Messages = nil
	base.Conversation.Timeline = nil
	basePayload, _ := json.Marshal(fullSessionPayload{Session: *base})
	appendPayload, _ := json.Marshal(turnAppendedPayload{
		Messages: []llm.Message{llm.NewUserTextMessage("once")},
	})

	events := []SessionEvent{
		{
			EventID:       "re-1",
			SessionID:     "dedupe-restart",
			Seq:           1,
			Type:          eventTypeSessionCreated,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       basePayload,
		},
		{
			EventID:       "re-2",
			SessionID:     "dedupe-restart",
			Seq:           2,
			Type:          eventTypeTurnAppended,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       appendPayload,
		},
		{
			EventID:       "re-2",
			SessionID:     "dedupe-restart",
			Seq:           3,
			Type:          eventTypeTurnAppended,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       appendPayload,
		},
	}
	for _, event := range events {
		if err := appendEventLine(storeA.files, paths.Events, event); err != nil {
			t.Fatal(err)
		}
	}

	replayedA, err := storeA.Replay("dedupe-restart")
	if err != nil {
		t.Fatal(err)
	}
	if len(replayedA.Messages) != 1 {
		t.Fatalf("expected duplicate event_id to be ignored before restart, got %#v", replayedA.Messages)
	}

	storeB, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	replayedB, err := storeB.Replay("dedupe-restart")
	if err != nil {
		t.Fatal(err)
	}
	if len(replayedB.Messages) != 1 || replayedB.Messages[0].Content != "once" {
		t.Fatalf("expected duplicate event_id to stay deduped after restart, got %#v", replayedB.Messages)
	}
}

func TestStoreReadFromOffsetRemainsMonotonicAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	storeA, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	sess := New(t.TempDir())
	sess.ID = "offset-restart-monotonic"
	if err := storeA.Save(sess); err != nil {
		t.Fatal(err)
	}
	sess.Messages = append(sess.Messages, llm.NewUserTextMessage("first"))
	if err := storeA.Save(sess); err != nil {
		t.Fatal(err)
	}
	sess.Messages = append(sess.Messages, llm.NewAssistantTextMessage("second"))
	if err := storeA.Save(sess); err != nil {
		t.Fatal(err)
	}

	first, next1, err := storeA.ReadFrom(sess.ID, 0, 1)
	if err != nil {
		t.Fatalf("ReadFrom first page failed: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected first page size 1, got %d", len(first))
	}

	storeB, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, next2, err := storeB.ReadFrom(sess.ID, next1, 2)
	if err != nil {
		t.Fatalf("ReadFrom second page after restart failed: %v", err)
	}
	if len(second) == 0 {
		t.Fatal("expected more events after restart")
	}
	if next2 < next1 {
		t.Fatalf("expected next offset monotonic across restart, first=%d second=%d", next1, next2)
	}

	tail, next3, err := storeB.ReadFrom(sess.ID, next2, 1024)
	if err != nil {
		t.Fatalf("ReadFrom tail after restart failed: %v", err)
	}
	if next3 < next2 {
		t.Fatalf("expected tail next offset monotonic across restart, second=%d tail=%d", next2, next3)
	}

	empty, next4, err := storeB.ReadFrom(sess.ID, next3, 1024)
	if err != nil {
		t.Fatalf("ReadFrom final tail after restart failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected final tail to be empty, got %d events", len(empty))
	}
	if next4 != next3 {
		t.Fatalf("expected stable final tail next offset %d, got %d", next3, next4)
	}
	if len(tail) == 0 && next3 == next2 {
		t.Fatal("expected progress while consuming tail events after restart")
	}
}

func TestStoreReadFromCorruptedAndPartialLineAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	storeA, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	paths, err := storeA.pathForWorkspaceSession(workspace, "offset-restart-corrupted-partial")
	if err != nil {
		t.Fatal(err)
	}

	base := New(workspace)
	base.ID = "offset-restart-corrupted-partial"
	basePayload, _ := json.Marshal(fullSessionPayload{Session: *base})
	appendPayload, _ := json.Marshal(turnAppendedPayload{
		Messages: []llm.Message{llm.NewUserTextMessage("ok")},
	})
	events := []SessionEvent{
		{
			EventID:       "rs-1",
			SessionID:     "offset-restart-corrupted-partial",
			Seq:           1,
			Type:          eventTypeSessionCreated,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       basePayload,
		},
		{
			EventID:       "rs-2",
			SessionID:     "offset-restart-corrupted-partial",
			Seq:           2,
			Type:          eventTypeTurnAppended,
			TS:            time.Now().UTC(),
			SchemaVersion: eventSchemaVersion,
			Payload:       appendPayload,
		},
	}
	if err := appendEventLine(storeA.files, paths.Events, events[0]); err != nil {
		t.Fatal(err)
	}
	if err := storeA.files.AppendLine(paths.Events, []byte(`{bad-json`)); err != nil {
		t.Fatal(err)
	}
	if err := appendEventLine(storeA.files, paths.Events, events[1]); err != nil {
		t.Fatal(err)
	}

	partialEvent := SessionEvent{
		EventID:       "rs-partial",
		SessionID:     "offset-restart-corrupted-partial",
		Seq:           3,
		Type:          eventTypeTurnAppended,
		TS:            time.Now().UTC(),
		SchemaVersion: eventSchemaVersion,
		Payload:       appendPayload,
	}
	encoded, err := json.Marshal(partialEvent)
	if err != nil {
		t.Fatal(err)
	}
	cut := len(encoded) / 2
	if cut <= 0 || cut >= len(encoded) {
		t.Fatalf("unexpected cut index %d for payload size %d", cut, len(encoded))
	}

	infoBeforePartial, err := os.Stat(paths.Events)
	if err != nil {
		t.Fatal(err)
	}
	partialStart := infoBeforePartial.Size()
	file, err := os.OpenFile(paths.Events, os.O_WRONLY|os.O_APPEND, 0o644)
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

	readA, nextA, err := storeA.ReadFrom("offset-restart-corrupted-partial", 0, 10)
	if err != nil {
		t.Fatalf("ReadFrom before restart failed: %v", err)
	}
	if len(readA) != 2 {
		t.Fatalf("expected two valid events before partial tail, got %d", len(readA))
	}
	if readA[0].EventID != "rs-1" || readA[1].EventID != "rs-2" {
		t.Fatalf("unexpected events before restart: %#v", readA)
	}
	if nextA != partialStart {
		t.Fatalf("expected next offset pinned at partial start %d, got %d", partialStart, nextA)
	}

	storeB, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	readB, nextB, err := storeB.ReadFrom("offset-restart-corrupted-partial", nextA, 10)
	if err != nil {
		t.Fatalf("ReadFrom at pinned offset after restart failed: %v", err)
	}
	if len(readB) != 0 {
		t.Fatalf("expected no events at partial tail after restart, got %d", len(readB))
	}
	if nextB != nextA {
		t.Fatalf("expected next offset to stay pinned after restart at %d, got %d", nextA, nextB)
	}

	file, err = os.OpenFile(paths.Events, os.O_WRONLY|os.O_APPEND, 0o644)
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

	resumed, nextC, err := storeB.ReadFrom("offset-restart-corrupted-partial", nextA, 10)
	if err != nil {
		t.Fatalf("ReadFrom resumed tail failed: %v", err)
	}
	if len(resumed) != 1 {
		t.Fatalf("expected one resumed event, got %d", len(resumed))
	}
	if resumed[0].EventID != "rs-partial" {
		t.Fatalf("expected resumed event id %q, got %q", "rs-partial", resumed[0].EventID)
	}
	infoAfterResume, err := os.Stat(paths.Events)
	if err != nil {
		t.Fatal(err)
	}
	if nextC != infoAfterResume.Size() {
		t.Fatalf("expected next offset %d after resume, got %d", infoAfterResume.Size(), nextC)
	}
}
