package tools

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildRequiredLinuxShellCommandIncludesIsolationSteps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("linux shell isolation command assertion is not stable on windows path aliasing")
	}
	workspace := t.TempDir()
	writable := filepath.Join(workspace, "out")
	command, err := buildRequiredLinuxShellCommand("go test ./...", &ExecutionContext{
		Workspace:     workspace,
		WritableRoots: []string{writable},
	})
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if !strings.Contains(command, "mount -o remount,ro /") {
		t.Fatalf("expected read-only remount step, got %q", command)
	}
	workspaceCanonical, err := canonicalPathForAccess(filepath.Clean(workspace))
	if err != nil {
		t.Fatalf("resolve workspace canonical path: %v", err)
	}
	workspaceQuoted := shellSingleQuote(workspaceCanonical)
	if !containsCommandFragment(command, "mount --bind "+workspaceQuoted+" "+workspaceQuoted) {
		t.Fatalf("expected workspace bind step, got %q", command)
	}
	writableCanonical, err := canonicalPathForAccess(filepath.Clean(writable))
	if err != nil {
		t.Fatalf("resolve writable canonical path: %v", err)
	}
	writableQuoted := shellSingleQuote(writableCanonical)
	if !containsCommandFragment(command, "mount --bind "+writableQuoted+" "+writableQuoted) {
		t.Fatalf("expected writable root bind step, got %q", command)
	}
	if !strings.Contains(command, "go test ./...") {
		t.Fatalf("expected original command suffix, got %q", command)
	}
}

func TestBuildRequiredLinuxShellCommandRequiresWorkspace(t *testing.T) {
	_, err := buildRequiredLinuxShellCommand("git status", &ExecutionContext{})
	if err == nil {
		t.Fatal("expected missing workspace error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShellSingleQuoteEscapesApostrophe(t *testing.T) {
	got := shellSingleQuote("a'b")
	if got != `'a'"'"'b'` {
		t.Fatalf("unexpected quoting: %q", got)
	}
}

func containsCommandFragment(command, fragment string) bool {
	if strings.Contains(command, fragment) {
		return true
	}
	if runtime.GOOS == "windows" {
		norm := func(value string) string {
			value = strings.ReplaceAll(value, `\`, `/`)
			return strings.ToLower(value)
		}
		return strings.Contains(norm(command), norm(fragment))
	}
	return false
}
