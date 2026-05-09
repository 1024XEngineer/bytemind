package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rollbackpkg "github.com/1024XEngineer/bytemind/internal/rollback"
)

const rollbackUsage = "Usage: /rollback [last|<operation-id>]\nList or undo ByteMind file edits recorded by write_file, replace_in_file, or apply_patch."

func (m *model) runRollbackCommand(input string) error {
	response, status, err := executeRollbackCommand(context.Background(), m.workspace, m.cfg.WritableRoots, input)
	if err != nil {
		return m.finishRollbackCommand(input, err.Error(), "Rollback failed.")
	}
	return m.finishRollbackCommand(input, response, status)
}

func (m *model) finishRollbackCommand(input, response, status string) error {
	m.appendCommandExchange(input, response)
	m.statusNote = status
	if err := m.recordCommandExchange(input, response); err != nil {
		m.statusNote = "Command shown, but session save failed: " + err.Error()
		return nil
	}
	return nil
}

func executeRollbackCommand(ctx context.Context, workspace string, writableRoots []string, input string) (response string, status string, err error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 || fields[0] != "/rollback" {
		return "", "", errors.New(rollbackUsage)
	}
	if len(fields) > 2 {
		return "", "", errors.New(rollbackUsage)
	}

	store, err := rollbackpkg.NewDefaultStore()
	if err != nil {
		return "", "", fmt.Errorf("Rollback unavailable: %w", err)
	}

	if len(fields) == 1 {
		ops, err := store.ListRecent(ctx, workspace, 10)
		if err != nil {
			return "", "", err
		}
		return formatRollbackList(ops), "Rollback operations listed.", nil
	}

	target := strings.TrimSpace(fields[1])
	var op *rollbackpkg.Operation
	if strings.EqualFold(target, "last") {
		op, err = store.RollbackLast(ctx, workspace, writableRoots...)
	} else {
		op, err = store.Rollback(ctx, workspace, target, writableRoots...)
	}
	if err != nil {
		return "", "", err
	}
	return formatRollbackSuccess(*op), "Rollback completed.", nil
}

func formatRollbackList(ops []rollbackpkg.Operation) string {
	if len(ops) == 0 {
		return "No ByteMind rollback operations recorded for this workspace.\n\n`/rollback` is for ByteMind file edits. `/undo-commit` is only for local git commits created by `/commit`."
	}
	lines := []string{
		"Recent ByteMind rollback operations:",
		"",
	}
	for _, op := range ops {
		lines = append(lines, fmt.Sprintf(
			"- `%s`  %s  %s  %d file(s)  %s",
			shortRollbackID(op.OperationID),
			op.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			op.ToolName,
			len(op.AffectedFiles),
			rollbackPathSummary(op),
		))
	}
	lines = append(lines, "", "Use `/rollback last` or `/rollback <operation-id>` to restore one operation.")
	return strings.Join(lines, "\n")
}

func formatRollbackSuccess(op rollbackpkg.Operation) string {
	return fmt.Sprintf(
		"Rollback completed.\n\nOperation: `%s`\nTool: %s\nFiles restored: %d\n\nThis restored ByteMind file snapshots and did not modify git history.",
		op.OperationID,
		op.ToolName,
		len(op.AffectedFiles),
	)
}

func shortRollbackID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 22 {
		return id
	}
	return id[:22]
}

func rollbackPathSummary(op rollbackpkg.Operation) string {
	paths := make([]string, 0, min(len(op.AffectedFiles), 3))
	for i, file := range op.AffectedFiles {
		if i >= 3 {
			break
		}
		path := file.Path
		if file.OpType == rollbackpkg.OpTypeMove && strings.TrimSpace(file.NewPath) != "" {
			path += " -> " + file.NewPath
		}
		paths = append(paths, path)
	}
	if len(op.AffectedFiles) > 3 {
		paths = append(paths, fmt.Sprintf("+%d more", len(op.AffectedFiles)-3))
	}
	return strings.Join(paths, ", ")
}
