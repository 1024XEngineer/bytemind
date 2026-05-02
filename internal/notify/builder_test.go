package notify

import (
	"strings"
	"testing"
)

func TestBuildApprovalRequiredMessageIncludesReasonAndCommand(t *testing.T) {
	msg := BuildApprovalRequiredMessage("go test ./...", "run focused tests")
	if msg.Event != EventApprovalRequired {
		t.Fatalf("expected approval_required event, got %q", msg.Event)
	}
	if msg.Title != titleApprovalRequired {
		t.Fatalf("unexpected title: %q", msg.Title)
	}
	if !strings.Contains(msg.Body, "原因:") || !strings.Contains(msg.Body, "命令:") {
		t.Fatalf("expected reason and command in body, got %q", msg.Body)
	}
	if !strings.Contains(msg.Key, "approval_required|") {
		t.Fatalf("expected stable dedupe key, got %q", msg.Key)
	}
}

func TestBuildApprovalRequiredMessageKeyUsesFullSanitizedText(t *testing.T) {
	longPrefix := strings.Repeat("segment-", 30)
	commandOne := "run " + longPrefix + "one"
	commandTwo := "run " + longPrefix + "two"

	msgOne := BuildApprovalRequiredMessage(commandOne, "same reason")
	msgTwo := BuildApprovalRequiredMessage(commandTwo, "same reason")

	if msgOne.Key == msgTwo.Key {
		t.Fatalf("expected different keys for different long commands, got same key %q", msgOne.Key)
	}
}

func TestBuildApprovalRequiredMessageKeyNormalizesWhitespaceAndCase(t *testing.T) {
	msgOne := BuildApprovalRequiredMessage("  GO   TEST   ./TUI  ", "  Run Focused Tests ")
	msgTwo := BuildApprovalRequiredMessage("go test ./tui", "run focused tests")

	if msgOne.Key != msgTwo.Key {
		t.Fatalf("expected normalized keys to match, got %q vs %q", msgOne.Key, msgTwo.Key)
	}
}

func TestBuildRunFailedMessageRedactsDetail(t *testing.T) {
	msg := BuildRunFailedMessage(7, "authorization: bearer secret-token")
	if msg.Event != EventRunFailed {
		t.Fatalf("expected run_failed event, got %q", msg.Event)
	}
	if strings.Contains(strings.ToLower(msg.Body), "secret-token") {
		t.Fatalf("expected body to be sanitized, got %q", msg.Body)
	}
	if msg.Key != "run_failed|id=7" {
		t.Fatalf("expected run_failed key, got %q", msg.Key)
	}
}
