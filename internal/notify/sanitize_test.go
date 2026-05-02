package notify

import (
	"strings"
	"testing"
)

func TestSanitizeNotificationTextRedactsSecrets(t *testing.T) {
	input := "Authorization: Bearer abcdefghijklmnopqrstuvwxyz TOKEN=abc123 password=hunter2 https://x.test?a=1&token=xyz --secret my-secret"
	got := sanitizeNotificationText(input)
	lower := strings.ToLower(got)
	for _, forbidden := range []string{"abcdefghijklmnopqrstuvwxyz", "abc123", "hunter2", "token=xyz", "my-secret"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("expected %q to be redacted in %q", forbidden, got)
		}
	}
	if !strings.Contains(lower, "[redacted]") {
		t.Fatalf("expected redacted marker in %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 0); got != "" {
		t.Fatalf("expected empty string for zero limit, got %q", got)
	}
	if got := truncate("abc", 3); got != "abc" {
		t.Fatalf("expected unchanged string within limit, got %q", got)
	}
	if got := truncate("abcdef", 5); got != "ab..." {
		t.Fatalf("expected truncated string, got %q", got)
	}
}
