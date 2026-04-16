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

func TestExecuteSlashCommandReturnsCleanupErrorForNew(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := ExecuteSlashCommand(store, current, "/new", DefaultSlashCommands()); err == nil {
		t.Fatal("expected /new to fail when cleanup cannot list sessions")
	}
}

func TestExecuteSlashCommandReturnsCleanupErrorForResume(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := ExecuteSlashCommand(store, current, "/resume any", DefaultSlashCommands()); err == nil {
		t.Fatal("expected /resume to fail when cleanup cannot list sessions")
	}
}

func TestExecuteSlashCommandHandlesEmptyInputAndBasicCommands(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	current := session.New(t.TempDir())
	current.ID = "current"

	out, err := ExecuteSlashCommand(store, current, "   ", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if out.Handled {
		t.Fatalf("expected empty input to be ignored, got %#v", out)
	}
	if out.NextSession != current {
		t.Fatalf("expected current session to remain unchanged, got %#v", out.NextSession)
	}

	quitOut, err := ExecuteSlashCommand(store, current, "/quit", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if !quitOut.Handled || !quitOut.ShouldExit || quitOut.Command != "quit" {
		t.Fatalf("expected /quit to request exit, got %#v", quitOut)
	}

	helpOut, err := ExecuteSlashCommand(store, current, "/help", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if !helpOut.Handled || helpOut.Command != "help" {
		t.Fatalf("expected /help branch, got %#v", helpOut)
	}

	sessionOut, err := ExecuteSlashCommand(store, current, "/session", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if !sessionOut.Handled || sessionOut.Command != "session" || sessionOut.SessionToDisplay != current {
		t.Fatalf("expected /session to return current session display payload, got %#v", sessionOut)
	}
}

func TestExecuteSlashCommandHandlesResumeUsageAndSessionsLimit(t *testing.T) {
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
	other := session.New(workspace)
	other.ID = "other"
	other.Messages = []llm.Message{llm.NewUserTextMessage("hello")}
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}

	usageOut, err := ExecuteSlashCommand(store, current, "/resume", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if usageOut.Command != "resume" || usageOut.UsageHint == "" {
		t.Fatalf("expected /resume usage hint branch, got %#v", usageOut)
	}

	sessionsOut, err := ExecuteSlashCommand(store, current, "/sessions 1", DefaultSlashCommands())
	if err != nil {
		t.Fatal(err)
	}
	if sessionsOut.Command != "sessions" || len(sessionsOut.Summaries) != 1 {
		t.Fatalf("expected /sessions with explicit limit to return one summary, got %#v", sessionsOut)
	}

	if _, err := ExecuteSlashCommand(store, current, "/sessions not-a-number", DefaultSlashCommands()); err == nil {
		t.Fatal("expected invalid /sessions limit to return parse error")
	}
}

func TestExecuteSlashCommandReturnsListErrorForSessions(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	current := session.New(t.TempDir())
	current.ID = "current"
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := ExecuteSlashCommand(store, current, "/sessions", DefaultSlashCommands()); err == nil {
		t.Fatal("expected /sessions to return list error when store root is missing")
	}
}
