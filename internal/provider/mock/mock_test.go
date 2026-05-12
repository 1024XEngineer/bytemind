package mock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestMockProviderDefaultReturnsText(t *testing.T) {
	p := New("")
	msg, err := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content == "" {
		t.Fatal("expected non-empty content")
	}
	if len(msg.ToolCalls) > 0 {
		t.Fatal("expected no tool calls in default preset")
	}
}

func TestMockProviderBugfixDemoPreset(t *testing.T) {
	p := New("bugfix-demo")
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		msg, err := p.CreateMessage(ctx, llm.ChatRequest{})
		if err != nil {
			break
		}
		if len(msg.ToolCalls) > 0 {
			name := msg.ToolCalls[0].Function.Name
			if name == "" {
				t.Fatalf("step %d: tool call with empty name", i)
			}
			if !json.Valid([]byte(msg.ToolCalls[0].Function.Arguments)) {
				t.Fatalf("step %d: invalid JSON args: %q", i, msg.ToolCalls[0].Function.Arguments)
			}
		}
		if len(msg.ToolCalls) == 0 && msg.Content != "" {
			break
		}
	}
}

func TestMockProviderDynamicReplaceReadsFile(t *testing.T) {
	dir := t.TempDir()
	src := `package main

func CalculateAverage(nums []float64) float64 {
	total := 0.0
	for _, n := range nums {
		total += n
	}
	return total / float64(len(nums))
}`
	if err := os.WriteFile(filepath.Join(dir, "calculator.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewFromSteps([]Step{
		{
			Type: StepReadAndReplace,
			Replace: &ReadAndReplaceStep{
				Path:    "calculator.go",
				OldLine: "return total / float64(len(nums))",
				NewLines: `if len(nums) == 0 {
		return 0
	}
	return total / float64(len(nums))`,
			},
		},
	})
	p.SetWorkspace(dir)

	msg, err := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) == 0 {
		t.Fatal("expected tool call")
	}
	if msg.ToolCalls[0].Function.Name != "replace_in_file" {
		t.Fatalf("expected replace_in_file, got %s", msg.ToolCalls[0].Function.Name)
	}

	var args struct {
		Path      string `json:"path"`
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.Unmarshal([]byte(msg.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("invalid JSON args: %v", err)
	}
	if args.Old == "" {
		t.Fatal("expected non-empty old (should have been read from file)")
	}
	if !strings.Contains(args.Old, "return total / float64") {
		t.Fatalf("old should contain the target line, got: %q", args.Old)
	}
	if !strings.Contains(args.New, "len(nums) == 0") {
		t.Fatalf("new should contain the guard, got: %q", args.New)
	}
}

func TestMockProviderDynamicReplaceFileNotFound(t *testing.T) {
	p := NewFromSteps([]Step{
		{
			Type: StepReadAndReplace,
			Replace: &ReadAndReplaceStep{
				Path:    "nonexistent.go",
				OldLine: "anything",
				NewLines: "replacement",
			},
		},
	})

	msg, err := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) == 0 {
		t.Fatal("expected tool call even with missing file")
	}
	var args struct {
		Old string `json:"old"`
	}
	if err := json.Unmarshal([]byte(msg.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("invalid JSON args: %v", err)
	}
	if args.Old != "" {
		t.Fatalf("expected empty old for missing file, got: %q", args.Old)
	}
}

func TestMockProviderTextOnlyPreset(t *testing.T) {
	p := New("text-only")
	msg, err := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.Content, "mock text-only") {
		t.Fatalf("expected text-only content, got %q", msg.Content)
	}
}

func TestMockProviderStreamMessage(t *testing.T) {
	p := New("")
	var deltas []string
	msg, err := p.StreamMessage(context.Background(), llm.ChatRequest{}, func(d string) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content == "" {
		t.Fatal("expected non-empty content")
	}
	if len(deltas) == 0 {
		t.Fatal("expected at least one delta")
	}
}

func TestMockProviderExhaustsSteps(t *testing.T) {
	p := New("text-only")
	ctx := context.Background()

	msg1, _ := p.CreateMessage(ctx, llm.ChatRequest{})
	if msg1.Content == "" {
		t.Fatal("expected content on first call")
	}

	msg2, err := p.CreateMessage(ctx, llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg2.Content, "Task completed") {
		t.Fatalf("expected fallback text, got %q", msg2.Content)
	}
}

func TestMockProviderNewFromSteps(t *testing.T) {
	p := NewFromSteps([]Step{
		{Type: StepToolCall, Tool: &ToolCallStep{Name: "list_files", Arguments: `{}`}},
		{Type: StepText, Text: "done"},
	})

	msg1, _ := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if len(msg1.ToolCalls) == 0 || msg1.ToolCalls[0].Function.Name != "list_files" {
		t.Fatal("expected list_files tool call")
	}

	msg2, _ := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if msg2.Content != "done" {
		t.Fatalf("expected 'done', got %q", msg2.Content)
	}
}

func TestMockProviderSetWorkspace(t *testing.T) {
	p := New("test")
	p.SetWorkspace("/tmp/test-ws")
	if p.workspace != "/tmp/test-ws" {
		t.Fatalf("expected workspace /tmp/test-ws, got %q", p.workspace)
	}
}

func TestMockProviderModel(t *testing.T) {
	p := New("test-model")
	if p.Model() != "test-model" {
		t.Fatalf("expected 'test-model', got %q", p.Model())
	}
}

func TestParseEnvStepsEmpty(t *testing.T) {
	steps := parseEnvSteps("")
	if len(steps) == 0 {
		t.Fatal("expected default preset for empty env")
	}
	if steps[0].Type != StepText {
		t.Fatal("expected text step from default preset")
	}
}

func TestParseEnvStepsToolAndText(t *testing.T) {
	env := "tool:list_files;text:done"
	steps := parseEnvSteps(env)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Type != StepToolCall || steps[0].Tool.Name != "list_files" {
		t.Fatalf("expected list_files tool step")
	}
	if steps[1].Type != StepText || steps[1].Text != "done" {
		t.Fatalf("expected 'done' text step")
	}
}

func TestParseEnvStepsReplace(t *testing.T) {
	env := "replace:test.go|old line|new line"
	steps := parseEnvSteps(env)
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Type != StepReadAndReplace {
		t.Fatal("expected ReadAndReplace step")
	}
	if steps[0].Replace.Path != "test.go" {
		t.Fatalf("expected path 'test.go', got %q", steps[0].Replace.Path)
	}
	if steps[0].Replace.OldLine != "old line" {
		t.Fatalf("expected OldLine 'old line', got %q", steps[0].Replace.OldLine)
	}
}

func TestParseEnvStepsToolWithArgs(t *testing.T) {
	env := `tool:read_file {"path":"main.go"}`
	steps := parseEnvSteps(env)
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Tool.Arguments != `{"path":"main.go"}` {
		t.Fatalf("expected custom args, got %q", steps[0].Tool.Arguments)
	}
}

func TestParseEnvStepsMixed(t *testing.T) {
	env := "tool:list_files;tool:git_diff;text:all done"
	steps := parseEnvSteps(env)
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
	if steps[2].Text != "all done" {
		t.Fatalf("expected 'all done', got %q", steps[2].Text)
	}
}

func TestDynamicReplacePartialMatch(t *testing.T) {
	dir := t.TempDir()
	src := `func CalculateAverage(nums []float64) float64 {
	total := 0.0
	for _, n := range nums {
		total += n
	}
	return total / float64(len(nums))  // inline comment
}`
	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewFromSteps([]Step{
		{
			Type: StepReadAndReplace,
			Replace: &ReadAndReplaceStep{
				Path:    "calc.go",
				OldLine: "return total / float64",
				NewLines: `if len(nums) == 0 {
		return 0
	}
	return total / float64(len(nums))`,
			},
		},
	})
	p.SetWorkspace(dir)

	msg, err := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) == 0 {
		t.Fatal("expected tool call")
	}

	var args struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.Unmarshal([]byte(msg.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("invalid JSON args: %v", err)
	}
	if args.Old == "" {
		t.Fatal("expected non-empty old via partial match")
	}
	if !strings.Contains(args.Old, "return total") {
		t.Fatalf("expected matched line content, got: %q", args.Old)
	}
	if !strings.Contains(args.New, "len(nums) == 0") {
		t.Fatalf("expected guard clause in new, got: %q", args.New)
	}
}

func TestDynamicReplaceNoMatchFallback(t *testing.T) {
	dir := t.TempDir()
	src := `package main
func main() {}`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	p := NewFromSteps([]Step{
		{
			Type: StepReadAndReplace,
			Replace: &ReadAndReplaceStep{
				Path:    "main.go",
				OldLine: "totally nonexistent line",
				NewLines: "replacement",
			},
		},
	})
	p.SetWorkspace(dir)

	msg, err := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) == 0 {
		t.Fatal("expected tool call")
	}

	var args struct {
		Old string `json:"old"`
	}
	if err := json.Unmarshal([]byte(msg.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("invalid JSON args: %v", err)
	}
	if args.Old != "" {
		t.Fatalf("expected empty old for no match, got: %q", args.Old)
	}
}

func TestSelectPresetWithEnvVar(t *testing.T) {
	os.Setenv("BYTEMIND_MOCK_STEPS", "tool:list_files;text:env_done")
	defer os.Unsetenv("BYTEMIND_MOCK_STEPS")

	p := New("unknown-preset")
	msg1, _ := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if len(msg1.ToolCalls) == 0 || msg1.ToolCalls[0].Function.Name != "list_files" {
		t.Fatal("expected list_files from env var preset")
	}

	msg2, _ := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if !strings.Contains(msg2.Content, "env_done") {
		t.Fatalf("expected 'env_done', got %q", msg2.Content)
	}
}

func TestStepToolCallWithNilTool(t *testing.T) {
	p := NewFromSteps([]Step{
		{Type: StepToolCall, Tool: nil},
	})
	msg, err := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 0 {
		t.Fatal("expected no tool calls for nil tool")
	}
}

func TestNewFromStepsOverflow(t *testing.T) {
	p := NewFromSteps([]Step{
		{Type: StepText, Text: "only one"},
	})
	msg1, _ := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if msg1.Content != "only one" {
		t.Fatalf("expected 'only one', got %q", msg1.Content)
	}
	msg2, _ := p.CreateMessage(context.Background(), llm.ChatRequest{})
	if !strings.Contains(msg2.Content, "Task completed") {
		t.Fatalf("expected fallback after exhaust, got %q", msg2.Content)
	}
}
