package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
)

func TestParseCommitMessageRequiresExplicitMessage(t *testing.T) {
	message, err := parseCommitMessage("/commit add commit command")
	if err != nil {
		t.Fatalf("parseCommitMessage failed: %v", err)
	}
	if message != "add commit command" {
		t.Fatalf("expected commit message to preserve words, got %q", message)
	}

	if _, err := parseCommitMessage("/commit"); err == nil || err.Error() != commitUsage {
		t.Fatalf("expected commit usage error, got %v", err)
	}
}

func TestGitStatusHasChangesIgnoresBranchLine(t *testing.T) {
	if gitStatusHasChanges("## main...origin/main [ahead 1]\n") {
		t.Fatalf("expected branch-only status to have no committable changes")
	}
	if !gitStatusHasChanges("## main\n M tui/model.go\n") {
		t.Fatalf("expected modified file status to have changes")
	}
}

func TestExecuteGitCommitCreatesCommit(t *testing.T) {
	repo := newCommitCommandTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	response, status, err := executeGitCommit(context.Background(), repo, "add commit command")
	if err != nil {
		t.Fatalf("executeGitCommit failed: %v", err)
	}
	if !strings.Contains(response, "Committed ") || !strings.Contains(response, "add commit command") {
		t.Fatalf("expected response with hash and message, got %q", response)
	}
	if !strings.HasPrefix(status, "Committed ") {
		t.Fatalf("expected committed status, got %q", status)
	}

	subject := runGitForCommitCommandTest(t, repo, "log", "-1", "--format=%s")
	if subject != "add commit command" {
		t.Fatalf("expected git log subject to match message, got %q", subject)
	}
}

func TestRunCommitCommandRecordsCommandExchange(t *testing.T) {
	repo := newCommitCommandTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(repo)
	m := model{
		workspace: repo,
		store:     store,
		sess:      sess,
	}

	if err := m.runCommitCommand("/commit save local work"); err != nil {
		t.Fatalf("runCommitCommand failed: %v", err)
	}

	if len(sess.Messages) != 2 {
		t.Fatalf("expected command exchange to be recorded, got %#v", sess.Messages)
	}
	if sess.Messages[0].Role != llm.RoleUser || sess.Messages[0].Text() != "/commit save local work" {
		t.Fatalf("expected user command message, got %#v", sess.Messages[0])
	}
	if sess.Messages[1].Role != llm.RoleAssistant || !strings.Contains(sess.Messages[1].Text(), "Committed ") {
		t.Fatalf("expected assistant commit result, got %#v", sess.Messages[1])
	}
	if len(m.chatItems) != 2 {
		t.Fatalf("expected command exchange in chat view, got %#v", m.chatItems)
	}
}

func TestHandleSlashCommitRequiresMessage(t *testing.T) {
	m := model{}

	if err := m.handleSlashCommand("/commit"); err != nil {
		t.Fatalf("handleSlashCommand failed: %v", err)
	}
	if m.statusNote != "Commit message required." {
		t.Fatalf("expected usage status note, got %q", m.statusNote)
	}
	if len(m.chatItems) != 2 || m.chatItems[1].Body != commitUsage {
		t.Fatalf("expected usage command exchange, got %#v", m.chatItems)
	}
}

func TestCommandPaletteExactCommitShowsUsage(t *testing.T) {
	input := textarea.New()
	input.SetValue("/commit")
	m := model{
		input:       input,
		commandOpen: true,
	}

	got, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if updated.statusNote != "Commit message required." {
		t.Fatalf("expected commit usage status, got %q", updated.statusNote)
	}
	if len(updated.chatItems) != 2 || updated.chatItems[1].Body != commitUsage {
		t.Fatalf("expected usage command exchange, got %#v", updated.chatItems)
	}
}

func TestExecuteGitCommitReportsNoChanges(t *testing.T) {
	repo := newCommitCommandTestRepo(t)

	response, status, err := executeGitCommit(context.Background(), repo, "no changes")
	if err != nil {
		t.Fatalf("executeGitCommit failed: %v", err)
	}
	if response != "No changes to commit." || status != "No changes to commit." {
		t.Fatalf("expected no-changes response and status, got %q / %q", response, status)
	}
}

func TestFormatCommitErrorDetectsMissingIdentity(t *testing.T) {
	err := formatCommitError("Author identity unknown\nfatal: unable to auto-detect email address", nil)
	if err == nil || err.Error() != "Commit failed: git user.name or user.email is not configured." {
		t.Fatalf("expected identity error, got %v", err)
	}
}

func newCommitCommandTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitForCommitCommandTest(t, repo, "init")
	runGitForCommitCommandTest(t, repo, "config", "user.name", "ByteMind Test")
	runGitForCommitCommandTest(t, repo, "config", "user.email", "bytemind-test@example.com")
	return repo
}

func runGitForCommitCommandTest(t *testing.T, repo string, args ...string) string {
	t.Helper()
	output, err := runGit(context.Background(), repo, args...)
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}
