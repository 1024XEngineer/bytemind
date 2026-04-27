package agent

import (
	"strings"
	"testing"

	"bytemind/internal/llm"
	"bytemind/internal/session"
)

func TestLatestToolResultEnvelopeParsesSystemSandboxFallback(t *testing.T) {
	sess := &session.Session{
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: `{"ok":true,"status":"error","reason_code":"tool_failed","system_sandbox":{"mode":"best_effort","backend":"none","required_capable":false,"fallback":true,"fallback_reason":"linux backend unavailable"}}`,
			},
		},
	}

	envelope, ok := latestToolResultEnvelope(sess)
	if !ok {
		t.Fatal("expected envelope to parse")
	}
	if !envelope.SystemSandbox.Fallback {
		t.Fatalf("expected fallback=true, got %#v", envelope.SystemSandbox)
	}
	if envelope.SystemSandbox.Mode != "best_effort" {
		t.Fatalf("expected mode best_effort, got %#v", envelope.SystemSandbox)
	}
	if envelope.SystemSandbox.Backend != "none" {
		t.Fatalf("expected backend none, got %#v", envelope.SystemSandbox)
	}
	if envelope.SystemSandbox.RequiredCapable {
		t.Fatalf("expected required_capable=false, got %#v", envelope.SystemSandbox)
	}
	if envelope.SystemSandbox.FallbackReason != "linux backend unavailable" {
		t.Fatalf("expected fallback_reason, got %#v", envelope.SystemSandbox)
	}
}

func TestSystemSandboxFallbackReportEntry(t *testing.T) {
	note := systemSandboxFallbackReportEntry("run_shell", toolResultEnvelope{
		SystemSandbox: struct {
			Mode            string `json:"mode"`
			Backend         string `json:"backend"`
			RequiredCapable bool   `json:"required_capable"`
			CapabilityLevel string `json:"capability_level"`
			ShellNetwork    bool   `json:"shell_network_isolation"`
			WorkerNetwork   bool   `json:"worker_network_isolation"`
			Fallback        bool   `json:"fallback"`
			FallbackReason  string `json:"fallback_reason"`
		}{
			Mode:            "best_effort",
			Backend:         "none",
			RequiredCapable: false,
			CapabilityLevel: "none",
			ShellNetwork:    false,
			WorkerNetwork:   false,
			Fallback:        true,
			FallbackReason:  "darwin backend unavailable",
		},
	})

	for _, want := range []string{
		"run_shell",
		"mode=best_effort",
		"backend=none",
		"required_capable=false",
		"capability_level=none",
		"shell_network_isolation=false",
		"worker_network_isolation=false",
		"reason=darwin backend unavailable",
	} {
		if !strings.Contains(note, want) {
			t.Fatalf("expected note to contain %q, got %q", want, note)
		}
	}
}

func TestSystemSandboxFallbackReportEntryReturnsEmptyWhenNotFallback(t *testing.T) {
	note := systemSandboxFallbackReportEntry("run_shell", toolResultEnvelope{})
	if note != "" {
		t.Fatalf("expected empty note when fallback is false, got %q", note)
	}
}
