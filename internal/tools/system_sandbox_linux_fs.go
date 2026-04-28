package tools

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func buildRequiredLinuxShellCommand(command string, execCtx *ExecutionContext) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command cannot be empty")
	}
	roots, err := requiredLinuxSandboxRoots(execCtx)
	if err != nil {
		return "", err
	}

	steps := make([]string, 0, 4+len(roots)*3)
	steps = append(steps, "set -euo pipefail")
	for _, root := range roots {
		quoted := shellSingleQuote(root)
		steps = append(steps, fmt.Sprintf("mkdir -p %s", quoted))
	}
	steps = append(steps, "mount --make-rprivate /")
	steps = append(steps, "mount -o remount,ro /")
	for _, root := range roots {
		quoted := shellSingleQuote(root)
		steps = append(steps, fmt.Sprintf("mount --bind %s %s", quoted, quoted))
		steps = append(steps, fmt.Sprintf("mount -o remount,rw,bind %s", quoted))
	}
	steps = append(steps, withRequiredLinuxShellLimits(command))
	return strings.Join(steps, "; "), nil
}

func requiredLinuxSandboxRoots(execCtx *ExecutionContext) ([]string, error) {
	if execCtx == nil {
		return nil, errors.New("workspace is required for required system sandbox mode")
	}
	workspace := strings.TrimSpace(execCtx.Workspace)
	if workspace == "" {
		return nil, errors.New("workspace is required for required system sandbox mode")
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	roots, err := resolveAllowedRoots(filepath.Clean(absWorkspace), writableRootsFromExecContext(execCtx))
	if err != nil {
		return nil, err
	}
	roots = append(roots, "/tmp")

	seen := map[string]struct{}{}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		canonical, err := canonicalPathForAccess(root)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
	}
	if len(out) == 0 {
		return nil, errors.New("no writable roots available for required system sandbox mode")
	}
	return out, nil
}

func shellSingleQuote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
