package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bytemind/internal/llm"
	storagepkg "bytemind/internal/storage"
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
