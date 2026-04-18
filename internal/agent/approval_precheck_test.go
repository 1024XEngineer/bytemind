package agent

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

func TestBuildApprovalPrecheckSummaryInteractive(t *testing.T) {
	summary := buildApprovalPrecheckSummary(approvalPrecheckSummaryInput{
		ToolNames:      []string{"list_files", "run_shell", "write_file", "apply_patch"},
		ApprovalPolicy: "on-request",
		ApprovalMode:   "interactive",
	})
	for _, want := range []string{
		"approval precheck",
		"run_shell",
		"workspace-modifying tools: apply_patch, write_file",
		"interactive mode",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected summary to contain %q, got %q", want, summary)
		}
	}
}

func TestBuildApprovalPrecheckSummaryAwayFailFast(t *testing.T) {
	summary := buildApprovalPrecheckSummary(approvalPrecheckSummaryInput{
		ToolNames:      []string{"run_shell"},
		ApprovalPolicy: "always",
		ApprovalMode:   "away",
		AwayPolicy:     "fail_fast",
	})
	for _, want := range []string{
		"approval precheck",
		"approval_policy=always",
		"away_policy=fail_fast",
		"fail_fast",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected summary to contain %q, got %q", want, summary)
		}
	}
}

func TestRunPromptWritesApprovalPrecheckNotice(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{{
		Role:    llm.RoleAssistant,
		Content: "done",
	}}}

	var out bytes.Buffer
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:       config.ProviderConfig{Model: "test-model"},
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
			AwayPolicy:     "auto_deny_continue",
			MaxIterations:  2,
			Stream:         false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "hello", string(planpkg.ModeBuild), &out)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	for _, want := range []string{
		"approval precheck",
		"run_shell",
		"workspace-modifying tools",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected output to contain %q, got %q", want, out.String())
		}
	}
}

func TestRunPromptSkipsApprovalPrecheckWhenPolicyNever(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{{
		Role:    llm.RoleAssistant,
		Content: "done",
	}}}

	var out bytes.Buffer
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:       config.ProviderConfig{Model: "test-model"},
			ApprovalPolicy: "never",
			ApprovalMode:   "interactive",
			MaxIterations:  2,
			Stream:         false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	_, err = runner.RunPrompt(context.Background(), sess, "hello", string(planpkg.ModeBuild), &out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "approval precheck") {
		t.Fatalf("did not expect approval precheck under policy never, got %q", out.String())
	}
}

func TestPrepareRunApprovalHandlerPreApprovesRunShellAndDestructive(t *testing.T) {
	requests := make([]tools.ApprovalRequest, 0, 4)
	runner := &Runner{
		config: config.Config{
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
		},
		registry: tools.DefaultRegistry(),
		approval: func(req tools.ApprovalRequest) (bool, error) {
			requests = append(requests, req)
			return true, nil
		},
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{RunMode: planpkg.ModeBuild}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if len(requests) != 2 {
		t.Fatalf("expected two pre-approval requests, got %d (%+v)", len(requests), requests)
	}
	if !strings.Contains(requests[0].Command, "run_shell") || !strings.Contains(requests[0].Reason, "pre-approve") {
		t.Fatalf("unexpected run_shell pre-approval request: %+v", requests[0])
	}
	if !strings.Contains(requests[1].Command, "workspace-modifying tools") || !strings.Contains(requests[1].Reason, "pre-approve") {
		t.Fatalf("unexpected destructive pre-approval request: %+v", requests[1])
	}

	approved, err := handler(tools.ApprovalRequest{
		Command: "go test ./...",
		Reason:  "may modify files or environment: go",
	})
	if err != nil || !approved {
		t.Fatalf("expected pre-approved run_shell request, approved=%v err=%v", approved, err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected run_shell request to skip runtime approval prompt, got %d calls", len(requests))
	}

	approved, err = handler(tools.ApprovalRequest{
		Command: "write_file",
		Reason:  "destructive tool may modify workspace files: write_file",
	})
	if err != nil || !approved {
		t.Fatalf("expected pre-approved destructive request, approved=%v err=%v", approved, err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected destructive request to skip runtime approval prompt, got %d calls", len(requests))
	}
}

func TestPrepareRunApprovalHandlerFallsBackWhenPreApprovalDenied(t *testing.T) {
	requests := make([]tools.ApprovalRequest, 0, 4)
	runner := &Runner{
		config: config.Config{
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
		},
		registry: tools.DefaultRegistry(),
		approval: func(req tools.ApprovalRequest) (bool, error) {
			requests = append(requests, req)
			if strings.Contains(req.Reason, "pre-approve") {
				return false, nil
			}
			return true, nil
		},
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{RunMode: planpkg.ModeBuild}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if len(requests) != 2 {
		t.Fatalf("expected two pre-approval requests, got %d (%+v)", len(requests), requests)
	}

	approved, err := handler(tools.ApprovalRequest{
		Command: "go test ./...",
		Reason:  "may modify files or environment: go",
	})
	if err != nil || !approved {
		t.Fatalf("expected runtime run_shell approval fallback, approved=%v err=%v", approved, err)
	}
	if len(requests) != 3 {
		t.Fatalf("expected runtime run_shell request to call base handler after pre-approval denial, got %d calls", len(requests))
	}

	approved, err = handler(tools.ApprovalRequest{
		Command: "write_file",
		Reason:  "destructive tool may modify workspace files: write_file",
	})
	if err != nil || !approved {
		t.Fatalf("expected runtime destructive approval fallback, approved=%v err=%v", approved, err)
	}
	if len(requests) != 4 {
		t.Fatalf("expected runtime destructive request to call base handler after pre-approval denial, got %d calls", len(requests))
	}
}

func TestPrepareRunApprovalHandlerUsesStdinFallbackAndPreApproves(t *testing.T) {
	var approvalOut bytes.Buffer
	runner := &Runner{
		config: config.Config{
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
		},
		registry: tools.DefaultRegistry(),
		stdin:    strings.NewReader("y\ny\n"),
		stdout:   &approvalOut,
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{RunMode: planpkg.ModeBuild}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if prompts := strings.Count(approvalOut.String(), "Approve action"); prompts != 2 {
		t.Fatalf("expected two pre-approval prompts from stdin fallback, got %d (%q)", prompts, approvalOut.String())
	}

	approved, err := handler(tools.ApprovalRequest{
		Command: "go test ./...",
		Reason:  "may modify files or environment: go",
	})
	if err != nil || !approved {
		t.Fatalf("expected run_shell request to be auto-approved after pre-approval, approved=%v err=%v", approved, err)
	}

	approved, err = handler(tools.ApprovalRequest{
		Command: "write_file",
		Reason:  "destructive tool may modify workspace files: write_file",
	})
	if err != nil || !approved {
		t.Fatalf("expected destructive request to be auto-approved after pre-approval, approved=%v err=%v", approved, err)
	}

	if prompts := strings.Count(approvalOut.String(), "Approve action"); prompts != 2 {
		t.Fatalf("expected runtime approvals to be skipped after pre-approval, got %d prompts (%q)", prompts, approvalOut.String())
	}
}

func TestPrepareRunApprovalHandlerStdinFallbackPromptsAtRuntimeAfterDeniedPreApproval(t *testing.T) {
	var approvalOut bytes.Buffer
	runner := &Runner{
		config: config.Config{
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
		},
		registry: tools.DefaultRegistry(),
		stdin:    strings.NewReader("n\nn\ny\n"),
		stdout:   &approvalOut,
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{RunMode: planpkg.ModeBuild}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if prompts := strings.Count(approvalOut.String(), "Approve action"); prompts != 2 {
		t.Fatalf("expected two pre-approval prompts, got %d (%q)", prompts, approvalOut.String())
	}

	approved, err := handler(tools.ApprovalRequest{
		Command: "go test ./...",
		Reason:  "may modify files or environment: go",
	})
	if err != nil || !approved {
		t.Fatalf("expected runtime prompt fallback to approve run_shell request, approved=%v err=%v", approved, err)
	}
	if prompts := strings.Count(approvalOut.String(), "Approve action"); prompts != 3 {
		t.Fatalf("expected one extra runtime prompt after denied pre-approval, got %d (%q)", prompts, approvalOut.String())
	}
}
