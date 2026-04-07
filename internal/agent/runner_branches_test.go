package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/tools"
)

type streamFallbackClient struct {
	streamMsg   llm.Message
	createMsg   llm.Message
	streamCalls int
	createCalls int
}

func (c *streamFallbackClient) CreateMessage(_ context.Context, _ llm.ChatRequest) (llm.Message, error) {
	c.createCalls++
	return c.createMsg, nil
}

func (c *streamFallbackClient) StreamMessage(_ context.Context, _ llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	c.streamCalls++
	if onDelta != nil && c.streamMsg.Content != "" {
		onDelta(c.streamMsg.Content)
	}
	return c.streamMsg, nil
}

func TestRunnerSetters(t *testing.T) {
	runner := NewRunner(Options{})
	if runner.observer != nil {
		t.Fatal("expected nil observer by default")
	}
	runner.SetObserver(ObserverFunc(func(Event) {}))
	if runner.observer == nil {
		t.Fatal("expected observer to be set")
	}

	if runner.approval != nil {
		t.Fatal("expected nil approval handler by default")
	}
	runner.SetApprovalHandler(func(tools.ApprovalRequest) (bool, error) { return true, nil })
	if runner.approval == nil {
		t.Fatal("expected approval handler to be set")
	}
}

func TestCompleteTurnNonStreamUsesCreateMessage(t *testing.T) {
	client := &fakeClient{replies: []llm.Message{{
		Role:    "assistant",
		Content: "done",
	}}}
	runner := NewRunner(Options{
		Config: config.Config{Stream: false},
		Client: client,
	})
	streamed := false
	msg, err := runner.completeTurn(context.Background(), llm.ChatRequest{}, io.Discard, &streamed)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "done" {
		t.Fatalf("unexpected message: %#v", msg)
	}
	if streamed {
		t.Fatalf("expected streamed=false, got true")
	}
}

func TestCompleteTurnStreamWithNilOutputStillEmitsDelta(t *testing.T) {
	client := &fakeClient{replies: []llm.Message{{
		Role:    "assistant",
		Content: "delta text",
	}}}
	events := make([]Event, 0, 2)
	runner := NewRunner(Options{
		Config: config.Config{Stream: true},
		Client: client,
		Observer: ObserverFunc(func(event Event) {
			events = append(events, event)
		}),
	})
	streamed := false
	msg, err := runner.completeTurn(context.Background(), llm.ChatRequest{}, nil, &streamed)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "delta text" {
		t.Fatalf("unexpected message: %#v", msg)
	}
	if streamed {
		t.Fatalf("expected streamed=false with nil output writer")
	}
	if len(events) != 1 || events[0].Type != EventAssistantDelta || events[0].Content != "delta text" {
		t.Fatalf("expected assistant delta event, got %#v", events)
	}
}

func TestCompleteTurnFallsBackToCreateWhenStreamReplyIsEmpty(t *testing.T) {
	client := &streamFallbackClient{
		streamMsg: llm.Message{Role: "assistant"},
		createMsg: llm.Message{Role: "assistant", Content: "fallback answer"},
	}
	runner := NewRunner(Options{
		Config: config.Config{Stream: true},
		Client: client,
	})
	streamed := false
	msg, err := runner.completeTurn(context.Background(), llm.ChatRequest{}, nil, &streamed)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "fallback answer" {
		t.Fatalf("expected fallback non-stream answer, got %#v", msg)
	}
	if client.streamCalls != 1 || client.createCalls != 1 {
		t.Fatalf("expected one stream and one create call, got stream=%d create=%d", client.streamCalls, client.createCalls)
	}
	if streamed {
		t.Fatalf("expected streamed=false when no deltas emitted, got true")
	}
}

func TestRenderToolFeedbackBranches(t *testing.T) {
	runner := NewRunner(Options{})
	var out bytes.Buffer

	runner.renderToolFeedback(&out, "list_files", `{"ok":false,"error":"boom"}`)
	runner.renderToolFeedback(&out, "search_text", `{"query":"todo","matches":[{"path":"a.go","line":7,"text":"TODO: fix this"}]}`)
	runner.renderToolFeedback(&out, "run_shell", `{"ok":false,"exit_code":2,"stdout":"line1\nline2","stderr":"err1\nerr2"}`)

	got := out.String()
	for _, want := range []string{"error", "boom", "found", "todo", "exit", "code 2", "stdout:", "stderr:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestRenderToolFeedbackAdditionalBranches(t *testing.T) {
	runner := NewRunner(Options{})
	var out bytes.Buffer

	runner.renderToolFeedback(&out, "list_files", `{"root":".","items":[{"path":"dir1","type":"dir"},{"path":"f.txt","type":"file"}]}`)
	runner.renderToolFeedback(&out, "read_file", `{"path":"a.go","start_line":1,"end_line":2,"total_lines":10,"content":"line1\nline2"}`)
	runner.renderToolFeedback(&out, "write_file", `{"path":"a.go","bytes_written":42}`)
	runner.renderToolFeedback(&out, "replace_in_file", `{"path":"a.go","replaced":1,"old_count":2}`)
	runner.renderToolFeedback(&out, "web_search", `{"query":"go release","results":[{"title":"Go Release Notes","url":"https://go.dev/doc/devel/release"}]}`)
	runner.renderToolFeedback(&out, "web_fetch", `{"url":"https://go.dev/doc/devel/release","status_code":200,"title":"Release Notes","content":"Go 1.x details","truncated":false}`)
	runner.renderToolFeedback(&out, "apply_patch", `{"operations":[{"type":"update","path":"a.go"}]}`)
	runner.renderToolFeedback(&out, "unknown_tool", `{}`)

	got := out.String()
	for _, want := range []string{"listed", "dir  dir1", "read", "wrote", "updated", "searched", "fetched", "patch", "completed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestUtilityHelpersBranches(t *testing.T) {
	if got := normalizeToolArguments("{bad"); got != "{bad" {
		t.Fatalf("expected raw fallback for invalid json, got %q", got)
	}
	if got := emptyDot("   "); got != "." {
		t.Fatalf("expected dot fallback, got %q", got)
	}
	if matches := previewMatches([]struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}{
		{Path: "a.go", Line: 1, Text: "one"},
	}); len(matches) != 1 {
		t.Fatalf("expected one match preview, got %#v", matches)
	}
	if output := previewOutput("stdout", "line1\nline2"); len(output) != 2 {
		t.Fatalf("expected two output previews, got %#v", output)
	}
}

func TestToolNamesDeduplicatesAndSkipsBlank(t *testing.T) {
	definitions := []llm.ToolDefinition{
		{Type: "function", Function: llm.FunctionDefinition{Name: "read_file"}},
		{Type: "function", Function: llm.FunctionDefinition{Name: "  "}},
		{Type: "function", Function: llm.FunctionDefinition{Name: "list_files"}},
		{Type: "function", Function: llm.FunctionDefinition{Name: "read_file"}},
	}
	got := toolNames(definitions)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `["list_files","read_file"]` {
		t.Fatalf("unexpected tool names: %s", data)
	}
}

func TestExplicitWebLookupInstruction(t *testing.T) {
	got := explicitWebLookupInstruction("请去 GitHub 找这个项目的源码实现")
	if !strings.Contains(got, "web_search/web_fetch") {
		t.Fatalf("expected explicit web lookup instruction, got %q", got)
	}

	if got := explicitWebLookupInstruction("请在当前仓库用 search_text 找 TODO"); got != "" {
		t.Fatalf("expected no explicit web lookup instruction, got %q", got)
	}
}
