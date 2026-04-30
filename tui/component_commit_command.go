package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

const commitUsage = "Usage: /commit <message>\nExample: /commit fix(/commit): improve commit feedback"
const undoCommitUsage = "Usage: /undo-commit\nUndo the last commit created by /commit in this session."

func (m *model) runCommitCommand(input string) error {
	message, err := parseCommitMessage(input)
	if err != nil {
		return m.finishCommitCommand(input, commitUsage, "Commit message required.")
	}

	response, status, err := executeGitCommit(context.Background(), m.workspace, message)
	if err != nil {
		return m.finishCommitCommand(input, err.Error(), "Commit failed.")
	}
	return m.finishCommitCommand(input, response, status)
}

func (m *model) runUndoCommitCommand(input string) error {
	hash, ok := m.latestSessionCommitHash()
	if !ok {
		return m.finishCommitCommand(input, undoCommitUsage, "No session commit to undo.")
	}

	response, status, err := executeGitUndoCommit(context.Background(), m.workspace, hash)
	if err != nil {
		return m.finishCommitCommand(input, err.Error(), "Undo commit failed.")
	}
	return m.finishCommitCommand(input, response, status)
}

func (m *model) finishCommitCommand(input, response, status string) error {
	m.appendCommandExchange(input, response)
	m.statusNote = status
	if err := m.recordCommandExchange(input, response); err != nil {
		m.statusNote = "Command shown, but session save failed: " + err.Error()
		return nil
	}
	return nil
}

func (m *model) recordCommandExchange(command, response string) error {
	if m == nil || m.sess == nil || m.store == nil {
		return nil
	}
	command = strings.TrimSpace(command)
	response = strings.TrimSpace(response)
	if command == "" || response == "" {
		return nil
	}
	m.sess.Messages = append(m.sess.Messages,
		llm.NewUserTextMessage(command),
		llm.NewAssistantTextMessage(response),
	)
	return m.store.Save(m.sess)
}

func (m *model) latestSessionCommitHash() (string, bool) {
	if m == nil || m.sess == nil {
		return "", false
	}
	for i := len(m.sess.Messages) - 1; i >= 0; i-- {
		msg := m.sess.Messages[i]
		text := strings.TrimSpace(msg.Text())
		switch {
		case msg.Role == llm.RoleUser && strings.HasPrefix(text, "/undo-commit"):
			return "", false
		case msg.Role == llm.RoleAssistant && strings.HasPrefix(text, "Commit undone."):
			return "", false
		case msg.Role == llm.RoleAssistant && strings.HasPrefix(text, "Commit created."):
			hash, ok := parseCommitHashFromResponse(text)
			if !ok {
				return "", false
			}
			if i > 0 && m.sess.Messages[i-1].Role == llm.RoleUser && strings.HasPrefix(strings.TrimSpace(m.sess.Messages[i-1].Text()), "/commit") {
				return hash, true
			}
			return "", false
		}
	}
	return "", false
}

func parseCommitHashFromResponse(response string) (string, bool) {
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Hash:") {
			continue
		}
		hash := strings.TrimSpace(strings.TrimPrefix(line, "Hash:"))
		hash = strings.Trim(hash, "`")
		return hash, hash != ""
	}
	return "", false
}

func parseCommitMessage(input string) (string, error) {
	input = strings.TrimSpace(input)
	fields := strings.Fields(input)
	if len(fields) == 0 || fields[0] != "/commit" {
		return "", errors.New(commitUsage)
	}
	message := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	if message == "" || message == "<message>" {
		return "", errors.New(commitUsage)
	}
	return message, nil
}

func executeGitCommit(ctx context.Context, workspace, message string) (response string, status string, err error) {
	statusOutput, err := runGit(ctx, workspace, "status", "--short", "--branch")
	if err != nil {
		return "", "", formatGitCommandError("Git status", statusOutput, err)
	}
	if !gitStatusHasChanges(statusOutput) {
		return "No changes to commit.", "No changes to commit.", nil
	}

	addOutput, err := runGit(ctx, workspace, "add", "-A")
	if err != nil {
		return "", "", formatGitCommandError("git add -A", addOutput, err)
	}

	commitOutput, err := runGit(ctx, workspace, "commit", "-m", message)
	if err != nil {
		return "", "", formatCommitError(commitOutput, err)
	}

	hash, err := runGit(ctx, workspace, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", "", formatGitCommandError("git rev-parse --short HEAD", hash, err)
	}
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", "", fmt.Errorf("Commit created, but git did not return a commit hash.")
	}

	changedFiles := countGitStatusChanges(statusOutput)
	response = fmt.Sprintf("Commit created.\n\nHash: `%s`\nMessage: %s\nFiles included: %d\n\nByteMind staged all current changes with `git add -A` before committing.", hash, message, changedFiles)
	return response, fmt.Sprintf("Commit created: %s", hash), nil
}

func executeGitUndoCommit(ctx context.Context, workspace, expectedHash string) (response string, status string, err error) {
	head, err := runGit(ctx, workspace, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", "", formatGitCommandError("git rev-parse --short HEAD", head, err)
	}
	head = strings.TrimSpace(head)
	if head == "" {
		return "", "", errors.New("Undo commit failed: current HEAD could not be resolved.")
	}
	if head != expectedHash {
		return "", "", fmt.Errorf("Undo commit blocked: current HEAD is %s, but the last /commit in this session created %s.", head, expectedHash)
	}

	statusOutput, err := runGit(ctx, workspace, "status", "--short", "--branch")
	if err != nil {
		return "", "", formatGitCommandError("Git status", statusOutput, err)
	}
	if gitStatusHasChanges(statusOutput) {
		return "", "", errors.New("Undo commit blocked: the working tree has changes after that commit. Commit, stash, or discard them before undoing.")
	}

	pushed, err := isHeadPushedToUpstream(ctx, workspace)
	if err != nil {
		return "", "", err
	}
	if pushed {
		return "", "", errors.New("Undo commit blocked: the commit is already present on the upstream branch.")
	}

	changedFilesOutput, err := runGit(ctx, workspace, "diff-tree", "--no-commit-id", "--name-only", "-r", "--root", "HEAD")
	if err != nil {
		return "", "", formatGitCommandError("git diff-tree", changedFilesOutput, err)
	}
	changedFiles := countNonEmptyLines(changedFilesOutput)

	resetOutput, err := runGit(ctx, workspace, "reset", "--soft", "HEAD~1")
	if err != nil {
		return "", "", formatGitCommandError("git reset --soft HEAD~1", resetOutput, err)
	}

	response = fmt.Sprintf("Commit undone.\n\nHash: `%s`\nFiles restored to staging: %d\n\nByteMind used `git reset --soft HEAD~1`, so the file changes are still available locally.", head, changedFiles)
	return response, fmt.Sprintf("Commit undone: %s", head), nil
}

func isHeadPushedToUpstream(ctx context.Context, workspace string) (bool, error) {
	upstream, err := runGit(ctx, workspace, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		normalized := strings.ToLower(upstream)
		if strings.Contains(normalized, "no upstream") ||
			strings.Contains(normalized, "no such branch") ||
			strings.Contains(normalized, "upstream branch") {
			return false, nil
		}
		return false, formatGitCommandError("git rev-parse @{u}", upstream, err)
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return false, nil
	}

	output, err := runGit(ctx, workspace, "merge-base", "--is-ancestor", "HEAD", upstream)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, formatGitCommandError("git merge-base --is-ancestor HEAD "+upstream, output, err)
}

func gitStatusHasChanges(output string) bool {
	return countGitStatusChanges(output) > 0
}

func countGitStatusChanges(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "##") {
			continue
		}
		count++
	}
	return count
}

func countNonEmptyLines(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func runGit(ctx context.Context, workspace string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if strings.TrimSpace(workspace) != "" {
		cmd.Dir = workspace
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

func formatCommitError(output string, err error) error {
	if isGitIdentityError(output) {
		return errors.New("Commit failed: git user.name or user.email is not configured.")
	}
	return formatGitCommandError("Commit", output, err)
}

func formatGitCommandError(action, output string, err error) error {
	output = strings.TrimSpace(output)
	if output == "" && err != nil {
		output = err.Error()
	}
	if output == "" {
		output = "unknown error"
	}
	return fmt.Errorf("%s failed: %s", action, output)
}

func isGitIdentityError(output string) bool {
	normalized := strings.ToLower(output)
	return strings.Contains(normalized, "author identity unknown") ||
		strings.Contains(normalized, "unable to auto-detect email address") ||
		(strings.Contains(normalized, "user.name") && strings.Contains(normalized, "user.email"))
}
