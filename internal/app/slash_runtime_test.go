package app

import (
	"testing"

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
