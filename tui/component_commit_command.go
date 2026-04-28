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

const commitUsage = "usage: /commit <message>"

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

func parseCommitMessage(input string) (string, error) {
	input = strings.TrimSpace(input)
	fields := strings.Fields(input)
	if len(fields) == 0 || fields[0] != "/commit" {
		return "", errors.New(commitUsage)
	}
	message := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	if message == "" {
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

	return fmt.Sprintf("Committed %s: %s", hash, message), fmt.Sprintf("Committed %s.", hash), nil
}

func gitStatusHasChanges(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "##") {
			continue
		}
		return true
	}
	return false
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
