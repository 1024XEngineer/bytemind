package agent

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/tools"
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
		approval: func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
			requests = append(requests, req)
			return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
		},
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{
		RunMode:   planpkg.ModeBuild,
		UserInput: "run tests and modify source files",
	}, io.Discard)
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

	decision, err := handler(tools.ApprovalRequest{
		ToolName: "run_shell",
		Command:  "go test ./...",
		Reason:   "may modify files or environment: go",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected pre-approved run_shell request, decision=%+v err=%v", decision, err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected run_shell request to skip runtime approval prompt, got %d calls", len(requests))
	}

	decision, err = handler(tools.ApprovalRequest{
		ToolName: "write_file",
		Command:  "write_file",
		Reason:   "destructive tool may modify workspace files: write_file",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected pre-approved destructive request, decision=%+v err=%v", decision, err)
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
		approval: func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
			requests = append(requests, req)
			if strings.Contains(req.Reason, "pre-approve") {
				return tools.ApprovalDecision{Disposition: tools.ApprovalDeny}, nil
			}
			return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
		},
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{
		RunMode:   planpkg.ModeBuild,
		UserInput: "run tests and modify source files",
	}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if len(requests) != 2 {
		t.Fatalf("expected two pre-approval requests, got %d (%+v)", len(requests), requests)
	}

	decision, err := handler(tools.ApprovalRequest{
		ToolName: "run_shell",
		Command:  "go test ./...",
		Reason:   "may modify files or environment: go",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected runtime run_shell approval fallback, decision=%+v err=%v", decision, err)
	}
	if len(requests) != 3 {
		t.Fatalf("expected runtime run_shell request to call base handler after pre-approval denial, got %d calls", len(requests))
	}

	decision, err = handler(tools.ApprovalRequest{
		ToolName: "write_file",
		Command:  "write_file",
		Reason:   "destructive tool may modify workspace files: write_file",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected runtime destructive approval fallback, decision=%+v err=%v", decision, err)
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

	handler := runner.prepareRunApprovalHandler(runPromptSetup{
		RunMode:   planpkg.ModeBuild,
		UserInput: "run tests and modify source files",
	}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if prompts := strings.Count(approvalOut.String(), "Approve action"); prompts != 2 {
		t.Fatalf("expected two pre-approval prompts from stdin fallback, got %d (%q)", prompts, approvalOut.String())
	}

	decision, err := handler(tools.ApprovalRequest{
		ToolName: "run_shell",
		Command:  "go test ./...",
		Reason:   "may modify files or environment: go",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected run_shell request to be auto-approved after pre-approval, decision=%+v err=%v", decision, err)
	}

	decision, err = handler(tools.ApprovalRequest{
		ToolName: "write_file",
		Command:  "write_file",
		Reason:   "destructive tool may modify workspace files: write_file",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected destructive request to be auto-approved after pre-approval, decision=%+v err=%v", decision, err)
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

	handler := runner.prepareRunApprovalHandler(runPromptSetup{
		RunMode:   planpkg.ModeBuild,
		UserInput: "run tests and modify source files",
	}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if prompts := strings.Count(approvalOut.String(), "Approve action"); prompts != 2 {
		t.Fatalf("expected two pre-approval prompts, got %d (%q)", prompts, approvalOut.String())
	}

	decision, err := handler(tools.ApprovalRequest{
		ToolName: "run_shell",
		Command:  "go test ./...",
		Reason:   "may modify files or environment: go",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected runtime prompt fallback to approve run_shell request, decision=%+v err=%v", decision, err)
	}
	if prompts := strings.Count(approvalOut.String(), "Approve action"); prompts != 3 {
		t.Fatalf("expected one extra runtime prompt after denied pre-approval, got %d (%q)", prompts, approvalOut.String())
	}
}

func TestPrepareRunApprovalHandlerSkipsPreapprovalWhenIntentUnclear(t *testing.T) {
	requests := make([]tools.ApprovalRequest, 0, 2)
	runner := &Runner{
		config: config.Config{
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
		},
		registry: tools.DefaultRegistry(),
		approval: func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
			requests = append(requests, req)
			return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
		},
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{
		RunMode:   planpkg.ModeBuild,
		UserInput: "summarize the current architecture and explain trade-offs",
	}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if len(requests) != 0 {
		t.Fatalf("expected no pre-approval requests for low-risk intent, got %+v", requests)
	}

	decision, err := handler(tools.ApprovalRequest{
		ToolName: "run_shell",
		Command:  "go test ./...",
		Reason:   "may modify files or environment: go",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected runtime approval handler to remain available, decision=%+v err=%v", decision, err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected runtime call to use base handler, got %d requests", len(requests))
	}
}

func TestPrepareRunApprovalHandlerLearnsRuntimeApprovalsWithinRun(t *testing.T) {
	requests := make([]tools.ApprovalRequest, 0, 4)
	runner := &Runner{
		config: config.Config{
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
		},
		registry: tools.DefaultRegistry(),
		approval: func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
			requests = append(requests, req)
			return tools.ApprovalDecision{Disposition: tools.ApprovalApproveSameToolSession}, nil
		},
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{
		RunMode:   planpkg.ModeBuild,
		UserInput: "explain architecture decisions only",
	}, io.Discard)
	if handler == nil {
		t.Fatal("expected approval handler")
	}
	if len(requests) != 0 {
		t.Fatalf("expected no eager pre-approval requests, got %+v", requests)
	}

	decision, err := handler(tools.ApprovalRequest{
		ToolName: "run_shell",
		Command:  "go test ./...",
		Reason:   "may modify files or environment: go",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected first shell approval to pass, decision=%+v err=%v", decision, err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one runtime shell approval request, got %d", len(requests))
	}

	decision, err = handler(tools.ApprovalRequest{
		ToolName: "run_shell",
		Command:  "go test ./...",
		Reason:   "may modify files or environment: go",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected learned shell approval to pass, decision=%+v err=%v", decision, err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected learned shell approval not to reprompt, got %d requests", len(requests))
	}

	decision, err = handler(tools.ApprovalRequest{
		ToolName: "write_file",
		Command:  "write_file",
		Reason:   "destructive tool may modify workspace files: write_file",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected first destructive approval to pass, decision=%+v err=%v", decision, err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected one runtime destructive approval request, got %d", len(requests))
	}

	decision, err = handler(tools.ApprovalRequest{
		ToolName: "write_file",
		Command:  "write_file",
		Reason:   "destructive tool may modify workspace files: write_file",
	})
	if err != nil || !decision.Approved() {
		t.Fatalf("expected learned destructive approval to pass, decision=%+v err=%v", decision, err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected learned destructive approval not to reprompt, got %d requests", len(requests))
	}
}

func TestPrepareRunApprovalHandlerReportsUnavailableApprovalChannel(t *testing.T) {
	var out bytes.Buffer
	runner := &Runner{
		config: config.Config{
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
		},
		registry: tools.DefaultRegistry(),
		stdin:    nil,
		stdout:   io.Discard,
	}

	handler := runner.prepareRunApprovalHandler(runPromptSetup{
		RunMode:   planpkg.ModeBuild,
		UserInput: "create and write files",
	}, &out)
	if handler == nil {
		t.Fatal("expected fallback approval handler when channel is unavailable")
	}

	decision, err := handler(tools.ApprovalRequest{
		ToolName: "write_file",
		Command:  "write_file",
		Reason:   "destructive tool may modify workspace files: write_file",
	})
	if err == nil {
		t.Fatal("expected unavailable approval channel error")
	}
	if decision.Approved() {
		t.Fatal("expected approval to be denied when channel is unavailable")
	}
	if !strings.Contains(err.Error(), "approval channel unavailable") {
		t.Fatalf("expected explicit channel unavailable error, got %v", err)
	}
	if !strings.Contains(out.String(), "approval channel unavailable") {
		t.Fatalf("expected precheck notice to mention unavailable channel, got %q", out.String())
	}
}
