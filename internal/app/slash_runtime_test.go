package app

import (
	"os"
	"testing"

	"bytemind/internal/llm"
	"bytemind/internal/session"
)

func TestExecuteSlashCommandHandlesResumeAndNew(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	target := session.New(workspace)
	target.ID = "resume-me"
	target.Messages = []llm.Message{
		llm.NewUserTextMessage("restore this session"),
	}
	if err := store.Save(target); err != nil {
		t.Fatal(err)
	}

	resumeOut, err := ExecuteSlashCommand(store, current, "/resume resume", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if !resumeOut.Handled || resumeOut.NextSession == nil || resumeOut.NextSession.ID != target.ID {
		t.Fatalf("unexpected resume result: %#v", resumeOut)
	}

	newOut, err := ExecuteSlashCommand(store, current, "/new", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if !newOut.Handled || newOut.NextSession == nil || newOut.NextSession.ID == current.ID {
		t.Fatalf("unexpected new result: %#v", newOut)
	}
}

func TestExecuteSlashCommandSessionsAndUnknown(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	current := session.New(t.TempDir())
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}

	sessionsOut, err := ExecuteSlashCommand(store, current, "/sessions", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if sessionsOut.Command != "sessions" || !sessionsOut.Handled {
		t.Fatalf("unexpected sessions result: %#v", sessionsOut)
	}
	if len(sessionsOut.Summaries) == 0 {
		t.Fatalf("expected session summaries, got %#v", sessionsOut)
	}

	unknownOut, err := ExecuteSlashCommand(store, current, "/wat", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if unknownOut.Command != "unknown" || !unknownOut.Handled {
		t.Fatalf("unexpected unknown result: %#v", unknownOut)
	}
	if len(unknownOut.Suggestions) == 0 {
		t.Fatalf("expected suggestions for unknown command, got %#v", unknownOut)
	}
}

func TestExecuteSlashCommandCleansZeroSessionsBeforeNew(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	zero := session.New(workspace)
	zero.ID = "zero-cleanup"
	if err := store.Save(zero); err != nil {
		t.Fatal(err)
	}

	out, err := ExecuteSlashCommand(store, current, "/new", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if out.NextSession == nil || out.NextSession.ID == current.ID {
		t.Fatalf("expected /new to create a replacement session, got %#v", out)
	}
	if _, err := store.Load(zero.ID); !os.IsNotExist(err) {
		t.Fatalf("expected zero-message session to be cleaned before /new, got %v", err)
	}
	if _, err := store.Load(current.ID); err != nil {
		t.Fatalf("expected active session to be preserved during cleanup, got %v", err)
	}
}

func TestExecuteSlashCommandCleansZeroSessionsBeforeResume(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	zero := session.New(workspace)
	zero.ID = "zero-cleanup"
	if err := store.Save(zero); err != nil {
		t.Fatal(err)
	}
	target := session.New(workspace)
	target.ID = "resume-target"
	target.Messages = []llm.Message{
		llm.NewUserTextMessage("resume keeps user input"),
	}
	if err := store.Save(target); err != nil {
		t.Fatal(err)
	}

	out, err := ExecuteSlashCommand(store, current, "/resume resume-target", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if out.NextSession == nil || out.NextSession.ID != target.ID {
		t.Fatalf("expected /resume to restore the target session, got %#v", out)
	}
	if _, err := store.Load(zero.ID); !os.IsNotExist(err) {
		t.Fatalf("expected zero-message session to be cleaned before /resume, got %v", err)
	}
	if _, err := store.Load(current.ID); err != nil {
		t.Fatalf("expected active session to be preserved during cleanup, got %v", err)
	}
}
