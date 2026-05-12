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
		OldString string `json:"oldString"`
		NewString string `json:"newString"`
	}
	if err := json.Unmarshal([]byte(msg.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("invalid JSON args: %v", err)
	}
	if args.OldString == "" {
		t.Fatal("expected non-empty oldString (should have been read from file)")
	}
	if !strings.Contains(args.OldString, "return total / float64") {
		t.Fatalf("oldString should contain the target line, got: %q", args.OldString)
	}
	if !strings.Contains(args.NewString, "len(nums) == 0") {
		t.Fatalf("newString should contain the guard, got: %q", args.NewString)
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
		OldString string `json:"oldString"`
	}
	if err := json.Unmarshal([]byte(msg.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatalf("invalid JSON args: %v", err)
	}
	// When file is not found, oldString should be empty (graceful fallback)
	if args.OldString != "" {
		t.Fatalf("expected empty oldString for missing file, got: %q", args.OldString)
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
