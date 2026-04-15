package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/config"
	"bytemind/internal/provider"
)

func TestStartupIssueHint(t *testing.T) {
	cases := []struct {
		name string
		in   provider.Availability
		want string
	}{
		{
			name: "missing key",
			in:   provider.Availability{Reason: "missing api key"},
			want: "No API key is configured yet.",
		},
		{
			name: "unauthorized",
			in:   provider.Availability{Reason: "API key unauthorized"},
			want: "The API key was rejected by the provider.",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := StartupIssueHint(tt.in); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestConfigPathHint(t *testing.T) {
	workspace := t.TempDir()
	explicit := filepath.Join(workspace, "custom.json")
	got := ConfigPathHint(workspace, explicit)
	if !strings.HasSuffix(got, "custom.json") {
		t.Fatalf("expected explicit config path, got %q", got)
	}

	defaultPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(defaultPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = ConfigPathHint(workspace, "")
	if filepath.Clean(got) != filepath.Clean(defaultPath) {
		t.Fatalf("expected workspace config path %q, got %q", defaultPath, got)
	}
}

func TestCompactLine(t *testing.T) {
	got := CompactLine("hello world", 8)
	if got != "hello..." {
		t.Fatalf("unexpected compact line: %q", got)
	}
}

func TestBuildStartupGuideIncludesReasonAndPath(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Config{
		Provider: config.ProviderConfig{
			APIKeyEnv: "BYTEMIND_API_KEY",
		},
	}
	guide := BuildStartupGuide(cfg, provider.Availability{
		Ready:  false,
		Reason: "API key unauthorized",
		Detail: "provider error 401",
	}, workspace, "")

	if !guide.Active {
		t.Fatal("expected active startup guide")
	}
	if !strings.Contains(guide.Status, "guide you through provider") {
		t.Fatalf("unexpected guide status: %q", guide.Status)
	}
	if guide.CurrentField != "type" {
		t.Fatalf("expected startup guide to begin at provider step, got %q", guide.CurrentField)
	}
	if len(guide.Lines) == 0 {
		t.Fatal("expected guide lines")
	}
	joined := strings.Join(guide.Lines, "\n")
	if !strings.Contains(joined, "Paste your API key") {
		t.Fatalf("expected key input hint, got %q", joined)
	}
}
