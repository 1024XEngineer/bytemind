package tools

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func buildDarwinSandboxProfile(execCtx *ExecutionContext, allowNetwork bool) (string, error) {
	roots, err := darwinSandboxWritableRoots(execCtx)
	if err != nil {
		return "", err
	}

	lines := []string{
		"(version 1)",
		"(deny default)",
		"(import \"system.sb\")",
		"(allow process*)",
		"(allow file-read*)",
	}
	if allowNetwork {
		lines = append(lines, "(allow network*)")
	}
	for _, root := range roots {
		lines = append(lines, fmt.Sprintf("(allow file-write* (subpath \"%s\"))", escapeDarwinSandboxLiteral(root)))
	}
	return strings.Join(lines, "\n"), nil
}

func darwinSandboxWritableRoots(execCtx *ExecutionContext) ([]string, error) {
	if execCtx == nil {
		return nil, errors.New("workspace is required for darwin system sandbox mode")
	}
	workspace := strings.TrimSpace(execCtx.Workspace)
	if workspace == "" {
		return nil, errors.New("workspace is required for darwin system sandbox mode")
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	allowedRoots, err := resolveAllowedRoots(filepath.Clean(absWorkspace), writableRootsFromExecContext(execCtx))
	if err != nil {
		return nil, err
	}
	allowedRoots = append(allowedRoots, "/tmp", "/private/tmp")

	seen := map[string]struct{}{}
	out := make([]string, 0, len(allowedRoots))
	for _, root := range allowedRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		canonical, err := canonicalPathForAccess(root)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(canonical)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, canonical)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	if len(out) == 0 {
		return nil, errors.New("no writable roots available for darwin system sandbox mode")
	}
	return out, nil
}

func escapeDarwinSandboxLiteral(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func buildDarwinSandboxShellArgs(profile, command string) []string {
	return []string{
		"-p",
		profile,
		"sh",
		"-lc",
		command,
	}
}

func buildDarwinSandboxWorkerArgs(profile, executablePath string) []string {
	return []string{
		"-p",
		profile,
		executablePath,
		sandboxWorkerSubcommand,
		sandboxWorkerStdioFlag,
	}
}
