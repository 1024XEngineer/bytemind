package tools

import (
	"strings"
	"testing"
)

func TestBuildSandboxEnvRequiredModeUsesAllowlistAndDropsSensitive(t *testing.T) {
	env := buildSandboxEnv([]string{
		"PATH=/usr/bin:/bin",
		"HOME=/home/user",
		"LANG=en_US.UTF-8",
		"OPENAI_API_KEY=secret",
		"RANDOM_FLAG=1",
	}, sandboxEnvOptions{
		GOOS:          "linux",
		RequiredMode:  true,
		DropSensitive: true,
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "OPENAI_API_KEY=") {
		t.Fatalf("expected sensitive key to be removed, got %q", joined)
	}
	if strings.Contains(joined, "RANDOM_FLAG=") {
		t.Fatalf("expected non-allowlisted key to be removed in required mode, got %q", joined)
	}
	if !strings.Contains(joined, "PATH=/usr/bin:/bin") {
		t.Fatalf("expected PATH to be preserved, got %q", joined)
	}
	if !strings.Contains(joined, "HOME=/home/user") {
		t.Fatalf("expected HOME to be preserved, got %q", joined)
	}
}

func TestBuildSandboxEnvAlwaysDropAndForceSet(t *testing.T) {
	env := buildSandboxEnv([]string{
		"PATH=/bin",
		"BYTEMIND_SANDBOX_WORKER=0",
		"BYTEMIND_API_KEY=secret",
	}, sandboxEnvOptions{
		GOOS: "linux",
		AlwaysDrop: map[string]struct{}{
			"BYTEMIND_API_KEY": {},
		},
		ForceSet: map[string]string{
			"BYTEMIND_SANDBOX_WORKER": "1",
		},
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "BYTEMIND_API_KEY=") {
		t.Fatalf("expected BYTEMIND_API_KEY to be removed, got %q", joined)
	}
	if !strings.Contains(joined, "BYTEMIND_SANDBOX_WORKER=1") {
		t.Fatalf("expected forced sandbox worker marker, got %q", joined)
	}
}
