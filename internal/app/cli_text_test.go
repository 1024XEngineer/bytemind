package app

import (
	"strings"
	"testing"
)

func TestDefaultUsageLinesIncludeInstall(t *testing.T) {
	lines := DefaultUsageLines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "bytemind install") {
		t.Fatalf("expected usage to include install command, got %q", joined)
	}
	if !strings.Contains(joined, "install without Go (macOS/Linux)") {
		t.Fatalf("expected usage to include no-go install tip, got %q", joined)
	}
	if !strings.Contains(joined, "-approval-mode interactive|away") {
		t.Fatalf("expected usage to include approval-mode flag, got %q", joined)
	}
	if !strings.Contains(joined, "-away-policy auto_deny_continue|fail_fast") {
		t.Fatalf("expected usage to include away-policy flag, got %q", joined)
	}
}

func TestDefaultHelpLines(t *testing.T) {
	lines := DefaultHelpLines()
	if len(lines) == 0 {
		t.Fatal("expected help lines")
	}
	if lines[0].Usage != "/help" {
		t.Fatalf("expected first help usage /help, got %#v", lines[0])
	}
}
