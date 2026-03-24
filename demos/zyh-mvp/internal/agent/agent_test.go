package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgecli/internal/config"
	"forgecli/internal/model"
)

type scriptedClient struct {
	responses []model.ChatResponse
	errors    []error
	requests  []model.ChatRequest
}

func (c *scriptedClient) Complete(_ context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	c.requests = append(c.requests, cloneRequest(req))
	if len(c.errors) > 0 {
		err := c.errors[0]
		c.errors = c.errors[1:]
		if err != nil {
			return model.ChatResponse{}, err
		}
	}
	if len(c.responses) == 0 {
		return model.ChatResponse{}, errors.New("no scripted response left")
	}
	resp := c.responses[0]
	c.responses = c.responses[1:]
	return resp, nil
}

type scriptedTerminal struct {
	approvals   []bool
	output      strings.Builder
	promptCount int
}

func (t *scriptedTerminal) Printf(format string, args ...any) {
	fmt.Fprintf(&t.output, format, args...)
}

func (t *scriptedTerminal) Println(args ...any) {
	fmt.Fprintln(&t.output, args...)
}

func (t *scriptedTerminal) PromptYesNo(_ string) (bool, error) {
	t.promptCount++
	if len(t.approvals) == 0 {
		return true, nil
	}
	value := t.approvals[0]
	t.approvals = t.approvals[1:]
	return value, nil
}

func TestAgentDefaultsToAnalyzeMode(t *testing.T) {
	agent, err := New(config.Default(), &scriptedTerminal{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &scriptedClient{})
	if err != nil {
		t.Fatal(err)
	}
	if agent.ModeName() != string(ModeAnalyze) {
		t.Fatalf("expected default mode analyze, got %s", agent.ModeName())
	}
}

func TestAgentAnalyzeModeAnswersCapabilitiesLocallyWithoutModelCall(t *testing.T) {
	repo := t.TempDir()
	readme := "# Demo\n\n## What this MVP includes\n\n- interactive chat with in-memory session context\n- read-only analysis tools\n"
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &scriptedClient{}
	agent, err := New(config.Default(), &scriptedTerminal{}, slog.New(slog.NewTextHandler(io.Discard, nil)), client)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.StartSession(repo); err != nil {
		t.Fatal(err)
	}

	reply, err := agent.RunTurn(context.Background(), "有什么功能")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "interactive chat with in-memory session context") {
		t.Fatalf("unexpected reply: %s", reply)
	}
	if len(client.requests) != 0 {
		t.Fatalf("expected no model call, got %d requests", len(client.requests))
	}
}

func TestAgentInspectionShortcutsDoNotPromptInFullMode(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &scriptedClient{responses: []model.ChatResponse{
		{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Type: "function", Function: model.FunctionCallData{Name: "run_command", Arguments: `{"command":"pwd"}`}}}}},
		{Message: model.Message{Role: "assistant", Content: "Workspace inspected."}},
	}}
	terminal := &scriptedTerminal{}

	agent, err := New(config.Default(), terminal, slog.New(slog.NewTextHandler(io.Discard, nil)), client)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.SetMode("full"); err != nil {
		t.Fatal(err)
	}
	if err := agent.StartSession(repo); err != nil {
		t.Fatal(err)
	}

	if _, err := agent.RunTurn(context.Background(), "where am I?"); err != nil {
		t.Fatal(err)
	}
	if terminal.promptCount != 0 {
		t.Fatalf("expected no approval prompt, got %d", terminal.promptCount)
	}
}

func TestAgentWriteFileRequiresApprovalInFullMode(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "demo.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &scriptedClient{responses: []model.ChatResponse{
		{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Type: "function", Function: model.FunctionCallData{Name: "write_file", Arguments: `{"path":"demo.txt","content":"new\n"}`}}}}},
		{Message: model.Message{Role: "assistant", Content: "Write was skipped after denial."}},
	}}
	terminal := &scriptedTerminal{approvals: []bool{false}}

	agent, err := New(config.Default(), terminal, slog.New(slog.NewTextHandler(io.Discard, nil)), client)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.SetMode("full"); err != nil {
		t.Fatal(err)
	}
	if err := agent.StartSession(repo); err != nil {
		t.Fatal(err)
	}

	if _, err := agent.RunTurn(context.Background(), "update demo.txt"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old\n" {
		t.Fatalf("expected file to remain unchanged, got: %s", string(content))
	}
}

func TestAgentOmitsNewFileContentFromTerminalPreview(t *testing.T) {
	repo := t.TempDir()
	client := &scriptedClient{responses: []model.ChatResponse{
		{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Type: "function", Function: model.FunctionCallData{Name: "write_file", Arguments: `{"path":"snake.html","content":"<html>\n<body>snake</body>\n</html>"}`}}}}},
		{Message: model.Message{Role: "assistant", Content: "Created snake.html."}},
	}}
	terminal := &scriptedTerminal{approvals: []bool{true}}

	agent, err := New(config.Default(), terminal, slog.New(slog.NewTextHandler(io.Discard, nil)), client)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.SetMode("full"); err != nil {
		t.Fatal(err)
	}
	if err := agent.StartSession(repo); err != nil {
		t.Fatal(err)
	}

	if _, err := agent.RunTurn(context.Background(), "create snake html"); err != nil {
		t.Fatal(err)
	}
	output := terminal.output.String()
	if !strings.Contains(output, "New file preview for snake.html omitted from terminal") {
		t.Fatalf("expected compact preview, got: %s", output)
	}
	if strings.Contains(output, "<html>") {
		t.Fatalf("expected file content not to be printed, got: %s", output)
	}
}

func TestAgentFallsBackAfterToolExecutionWhenModelFails(t *testing.T) {
	repo := t.TempDir()
	client := &scriptedClient{
		responses: []model.ChatResponse{
			{Message: model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{ID: "call-1", Type: "function", Function: model.FunctionCallData{Name: "write_file", Arguments: `{"path":"snake.html","content":"<html><body>snake</body></html>"}`}}}}},
		},
		errors: []error{nil, errors.New("context deadline exceeded")},
	}
	terminal := &scriptedTerminal{approvals: []bool{true}}

	agent, err := New(config.Default(), terminal, slog.New(slog.NewTextHandler(io.Discard, nil)), client)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.SetMode("full"); err != nil {
		t.Fatal(err)
	}
	if err := agent.StartSession(repo); err != nil {
		t.Fatal(err)
	}

	reply, err := agent.RunTurn(context.Background(), "create snake html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply, "created file snake.html") {
		t.Fatalf("expected fallback to mention created file, got: %s", reply)
	}
	if _, err := os.Stat(filepath.Join(repo, "snake.html")); err != nil {
		t.Fatalf("expected generated file to exist: %v", err)
	}
}

func cloneRequest(req model.ChatRequest) model.ChatRequest {
	cloned := model.ChatRequest{Model: req.Model, Temperature: req.Temperature, Tools: append([]model.ToolDefinition(nil), req.Tools...)}
	cloned.Messages = make([]model.Message, 0, len(req.Messages))
	for _, message := range req.Messages {
		copyMessage := message
		if len(message.ToolCalls) > 0 {
			copyMessage.ToolCalls = append([]model.ToolCall(nil), message.ToolCalls...)
		}
		cloned.Messages = append(cloned.Messages, copyMessage)
	}
	return cloned
}
