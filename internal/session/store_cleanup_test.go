package session

import (
	"os"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestCleanupZeroMessageSessionsKeepsActiveNoReplyAndOtherWorkspace(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	otherWorkspace := t.TempDir()

	active := New(workspace)
	active.ID = "active"
	if err := store.Save(active); err != nil {
		t.Fatal(err)
	}

	zero := New(workspace)
	zero.ID = "zero"
	if err := store.Save(zero); err != nil {
		t.Fatal(err)
	}

	noReply := New(workspace)
	noReply.ID = "no-reply"
	noReply.Messages = []llm.Message{
		llm.NewUserTextMessage("pending user request"),
	}
	if err := store.Save(noReply); err != nil {
		t.Fatal(err)
	}

	other := New(otherWorkspace)
	other.ID = "other-zero"
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}

	cleanup, err := store.CleanupZeroMessageSessions(workspace, active.ID)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if len(cleanup.DeletedIDs) != 1 || cleanup.DeletedIDs[0] != zero.ID {
		t.Fatalf("expected only zero session removed, got %+v", cleanup.DeletedIDs)
	}

	if _, err := store.Load(active.ID); err != nil {
		t.Fatalf("expected active session to remain, got %v", err)
	}
	if _, err := store.Load(noReply.ID); err != nil {
		t.Fatalf("expected no-reply session to remain, got %v", err)
	}
	if _, err := store.Load(other.ID); err != nil {
		t.Fatalf("expected other-workspace session to remain, got %v", err)
	}
	if _, err := store.Load(zero.ID); !os.IsNotExist(err) {
		t.Fatalf("expected zero session to be removed, got %v", err)
	}

	second, err := store.CleanupZeroMessageSessions(workspace, active.ID)
	if err != nil {
		t.Fatalf("second cleanup failed: %v", err)
	}
	if len(second.DeletedIDs) != 0 {
		t.Fatalf("expected idempotent cleanup with no extra deletions, got %+v", second.DeletedIDs)
	}
}

func TestDeleteInWorkspaceIsIdempotent(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sess := New(workspace)
	sess.ID = "to-delete"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteInWorkspace(workspace, sess.ID); err != nil {
		t.Fatalf("expected first delete to succeed, got %v", err)
	}
	if err := store.DeleteInWorkspace(workspace, sess.ID); err != nil {
		t.Fatalf("expected second delete to be idempotent, got %v", err)
	}
	if _, err := store.Load(sess.ID); !os.IsNotExist(err) {
		t.Fatalf("expected deleted session to be missing, got %v", err)
	}
}

func TestDeleteByIDRemovesExistingAndIgnoresMissing(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sess := New(workspace)
	sess.ID = "delete-by-id"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("expected delete by id to succeed, got %v", err)
	}
	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("expected repeated delete by id to be idempotent, got %v", err)
	}
	if _, err := store.Load(sess.ID); !os.IsNotExist(err) {
		t.Fatalf("expected deleted session to be missing, got %v", err)
	}
}

func TestDeleteInWorkspaceWithEmptyWorkspaceUsesFallbackDelete(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sess := New(workspace)
	sess.ID = "workspace-fallback"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteInWorkspace("", sess.ID); err != nil {
		t.Fatalf("expected empty-workspace delete to fallback to Delete, got %v", err)
	}
	if _, err := store.Load(sess.ID); !os.IsNotExist(err) {
		t.Fatalf("expected session removed via fallback delete, got %v", err)
	}
}

func TestDeleteAndDeleteInWorkspaceRejectEmptyID(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("   "); err == nil {
		t.Fatal("expected Delete to reject empty session id")
	}
	if err := store.DeleteInWorkspace(t.TempDir(), ""); err == nil {
		t.Fatal("expected DeleteInWorkspace to reject empty session id")
	}
}

func TestStoreListIncludesTitlePreviewAndCounters(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	sess := New(workspace)
	sess.ID = "with-title"
	sess.Title = "Planned refactor session"
	sess.Messages = []llm.Message{
		llm.NewUserTextMessage("design cleanup"),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: "{}",
				},
			}},
		},
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	summaries, _, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %+v", summaries)
	}
	got := summaries[0]
	if got.Title != "Planned refactor session" {
		t.Fatalf("expected title in summary, got %+v", got)
	}
	if got.Preview != "design cleanup" {
		t.Fatalf("expected preview in summary, got %+v", got)
	}
	if got.RawMessageCount != 2 || got.MessageCount != 2 {
		t.Fatalf("expected raw message count in summary, got %+v", got)
	}
	if got.UserEffectiveInputCount != 1 || got.AssistantEffectiveOutputCount != 1 {
		t.Fatalf("unexpected effective counters, got %+v", got)
	}
	if got.ZeroMsgSession || got.NoReplySession {
		t.Fatalf("expected normal session classification, got %+v", got)
	}
	if got.ID != "with-title" {
		t.Fatalf("expected deterministic summary id, got %+v", got)
	}
}
