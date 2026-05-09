package agent

import (
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
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

func makeToolCall(name string) llm.ToolCall {
	return llm.ToolCall{
		ID: "call-" + name,
		Function: llm.ToolFunctionCall{
			Name:      name,
			Arguments: "{}",
		},
	}
}

func TestPartitionForParallelExecution_AllParallelizable(t *testing.T) {
	calls := []llm.ToolCall{
		makeToolCall("read_file"),
		makeToolCall("search_text"),
		makeToolCall("list_files"),
	}
	groups := partitionForParallelExecution(calls)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 3 {
		t.Fatalf("expected 3 calls in group, got %d", len(groups[0]))
	}
}

func TestPartitionForParallelExecution_Mixed(t *testing.T) {
	calls := []llm.ToolCall{
		makeToolCall("read_file"),
		makeToolCall("write_file"),
		makeToolCall("read_file"),
	}
	groups := partitionForParallelExecution(calls)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	// Group 0: [read_file] — was in parallel group before write_file broke it
	if groups[0][0].ToolCall.Function.Name != "read_file" {
		t.Fatalf("group 0 should be read_file, got %s", groups[0][0].ToolCall.Function.Name)
	}
	// Group 1: [write_file] — alone
	if groups[1][0].ToolCall.Function.Name != "write_file" {
		t.Fatalf("group 1 should be write_file, got %s", groups[1][0].ToolCall.Function.Name)
	}
	// Group 2: [read_file] — alone after non-parallelizable write
	if groups[2][0].ToolCall.Function.Name != "read_file" {
		t.Fatalf("group 2 should be read_file, got %s", groups[2][0].ToolCall.Function.Name)
	}
}

func TestPartitionForParallelExecution_OnlySequential(t *testing.T) {
	calls := []llm.ToolCall{
		makeToolCall("write_file"),
		makeToolCall("run_shell"),
	}
	groups := partitionForParallelExecution(calls)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if len(g) != 1 {
			t.Fatalf("expected 1 call per sequential group, got %d", len(g))
		}
	}
}

func TestPartitionForParallelExecution_DelegateSubAgents(t *testing.T) {
	calls := []llm.ToolCall{
		makeToolCall("delegate_subagent"),
		makeToolCall("delegate_subagent"),
	}
	groups := partitionForParallelExecution(calls)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group for 2 delegate_subagents, got %d", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Fatalf("expected 2 in group, got %d", len(groups[0]))
	}
}

func TestPartitionForParallelExecution_UnknownTool(t *testing.T) {
	calls := []llm.ToolCall{
		makeToolCall("unknown_tool"),
	}
	groups := partitionForParallelExecution(calls)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 1 {
		t.Fatalf("expected 1 call in group, got %d", len(groups[0]))
	}
}

func TestPartitionForParallelExecution_ExceedsCap(t *testing.T) {
	calls := make([]llm.ToolCall, 6)
	for i := range calls {
		calls[i] = makeToolCall("read_file")
	}
	groups := partitionForParallelExecution(calls)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (cap=4, 6 calls), got %d", len(groups))
	}
	if len(groups[0]) != 4 {
		t.Fatalf("expected 4 in first group, got %d", len(groups[0]))
	}
	if len(groups[1]) != 2 {
		t.Fatalf("expected 2 in second group, got %d", len(groups[1]))
	}
}

func TestPartitionForParallelExecution_IndexPreservation(t *testing.T) {
	calls := []llm.ToolCall{
		makeToolCall("delegate_subagent"),
		makeToolCall("read_file"),
		makeToolCall("delegate_subagent"),
	}
	groups := partitionForParallelExecution(calls)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	for i, c := range groups[0] {
		if c.Index != i {
			t.Fatalf("index mismatch at position %d: got %d", i, c.Index)
		}
	}
}
