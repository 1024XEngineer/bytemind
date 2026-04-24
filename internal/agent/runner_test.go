package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"bytemind/internal/config"
	contextpkg "bytemind/internal/context"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tokenusage"
	"bytemind/internal/tools"
)

type fakeClient struct {
	replies  []llm.Message
	requests []llm.ChatRequest
	index    int
}

type managerProbeTool struct {
	sawTaskManager bool
	sawExtensions  bool
}

func (t *managerProbeTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "manager_probe",
			Description: "Test-only tool that captures execution context manager injection.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (t *managerProbeTool) Run(_ context.Context, _ json.RawMessage, execCtx *tools.ExecutionContext) (string, error) {
	t.sawTaskManager = execCtx != nil && execCtx.TaskManager != nil
	t.sawExtensions = execCtx != nil && execCtx.Extensions != nil
	return `{"ok":true}`, nil
}

func (f *fakeClient) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	f.requests = append(f.requests, req)
	if len(f.replies) == 0 {
		return llm.Message{}, nil
	}
	if f.index >= len(f.replies) {
		return f.replies[len(f.replies)-1], nil
	}
	message := f.replies[f.index]
	f.index++
	return message, nil
}

func (f *fakeClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	message, err := f.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && message.Content != "" {
		onDelta(message.Content)
	}
	return message, nil
}

func TestRunPromptInjectsRuntimeAndExtensionsIntoExecutionContext(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	probe := &managerProbeTool{}
	registry := tools.DefaultRegistry()
	if err := registry.Register(probe, tools.RegisterOptions{Source: tools.RegistrationSourceBuiltin}); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "manager_probe",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: registry,
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "probe managers", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if !probe.sawTaskManager {
		t.Fatal("expected execution context to include task manager")
	}
	if !probe.sawExtensions {
		t.Fatal("expected execution context to include extensions manager")
	}
}

func TestRunPromptReturnsBudgetSummaryInsteadOfError(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 1,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "inspect workspace", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "Paused before a final answer.") {
		t.Fatalf("expected pause summary, got %q", answer)
	}
	if !strings.Contains(answer, "execution budget of 1 turns") {
		t.Fatalf("expected budget detail, got %q", answer)
	}
}

func TestRunPromptStopsOnRepeatedToolPlan(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	repeatedReply := llm.Message{
		Role: "assistant",
		ToolCalls: []llm.ToolCall{{
			ID:   "call-repeat",
			Type: "function",
			Function: llm.ToolFunctionCall{
				Name:      "list_files",
				Arguments: `{}`,
			},
		}},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 5,
			Stream:        false,
		},
		Client:   &fakeClient{replies: []llm.Message{repeatedReply, repeatedReply, repeatedReply, repeatedReply}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "looping task", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "repeated the") {
		t.Fatalf("expected repeat-detection summary, got %q", answer)
	}
}

func TestRunPromptCompletesMinimalToolLoop(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "Workspace inspected.",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "inspect workspace", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Workspace inspected." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) != 4 {
		t.Fatalf("expected 4 session messages, got %#v", sess.Messages)
	}
	if sess.Messages[0].Role != "user" {
		t.Fatalf("expected first message to be user, got %#v", sess.Messages[0])
	}
	if len(sess.Messages[1].ToolCalls) != 1 || sess.Messages[1].ToolCalls[0].Function.Name != "list_files" {
		t.Fatalf("expected second message to record tool call, got %#v", sess.Messages[1])
	}
	if sess.Messages[2].Role != "user" || !strings.Contains(sess.Messages[2].Content, `"items"`) {
		t.Fatalf("expected third message to be tool result, got %#v", sess.Messages[2])
	}
	if len(sess.Messages[2].Parts) != 1 || sess.Messages[2].Parts[0].ToolResult == nil {
		t.Fatalf("expected third message to carry tool_result part, got %#v", sess.Messages[2])
	}
	if sess.Messages[3].Role != "assistant" || sess.Messages[3].Content != "Workspace inspected." {
		t.Fatalf("expected final assistant message, got %#v", sess.Messages[3])
	}
}

func TestRunPromptRepairsContinueWorkWithoutToolCalls(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "<turn_intent>continue_work</turn_intent>I will inspect files first.",
		},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "<turn_intent>finalize</turn_intent>Workspace inspected.",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 6,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "inspect workspace", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Workspace inspected." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected three requests (repair + tool + finalize), got %d", len(client.requests))
	}
	secondTurnMessages := client.requests[1].Messages
	if len(secondTurnMessages) == 0 || secondTurnMessages[0].Role != llm.RoleSystem {
		t.Fatalf("expected second request to keep system prompt first, got %#v", secondTurnMessages)
	}
	lastMsg := secondTurnMessages[len(secondTurnMessages)-1]
	if lastMsg.Role != llm.RoleUser ||
		!strings.Contains(strings.ToLower(lastMsg.Text()), "ongoing work but returned no structured tool calls") {
		t.Fatalf("expected repair control note to be appended as user message, got %#v", secondTurnMessages)
	}
	if len(sess.Messages) != 4 {
		t.Fatalf("expected user + tool call + tool result + final assistant, got %#v", sess.Messages)
	}
}

func TestRunPromptRepairsPlanDecisionAcknowledgementWithoutUpdatePlan(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Mode = planpkg.ModePlan
	sess.Plan = planpkg.State{
		Goal:         "Implement the first RAG demo",
		Summary:      "Need to lock the frontend choice before convergence.",
		Phase:        planpkg.PhaseClarify,
		DecisionGaps: []string{"Choose the first frontend stack."},
		Steps: []planpkg.Step{
			{Title: "Freeze the frontend stack", Status: planpkg.StepPending},
			{Title: "Implement the minimal flow", Status: planpkg.StepPending},
		},
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "<turn_intent>finalize</turn_intent>已收到，采用 B: Streamlit + LangChain。\n你回复 start execution 我就切到 Build 模式。",
		},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name: "update_plan",
					Arguments: `{
						"summary":"Frontend choice is locked and the plan is ready for execution review.",
						"implementation_brief":"Objective: deliver the first local RAG demo in demos/ with Streamlit + LangChain.\nTechnical direction: keep the existing local Python stack, use Streamlit for UI, LangChain for orchestration, and a local vector store path.\nDeliverables: a runnable demo app, document ingestion flow, retrieval + answer generation flow, and basic local verification.\nAcceptance: another coding model should be able to implement directly from this brief.",
						"phase":"converge_ready",
						"decision_log":[{"decision":"Adopt Streamlit + LangChain for the first frontend stack","reason":"User selected option B for faster UI delivery."}],
						"decision_gaps":[],
						"scope_defined":true,
						"risk_and_rollback_defined":true,
						"verification_defined":true,
						"risks":["Frontend stack change may require adapter refactors."],
						"verification":["Run the local Streamlit app and validate one ingest + one query flow."],
						"next_action":"Present the full handoff-ready plan and ask whether to start execution.",
						"plan":[
							{"step":"Freeze the frontend stack", "status":"pending"},
							{"step":"Implement the minimal flow", "status":"pending"}
						]
					}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "<turn_intent>finalize</turn_intent>已记录，采用 B: Streamlit + LangChain。\n可选下一步：\n- Start execution\n- Adjust plan",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 6,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "b", "plan", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected three requests (repair + update_plan + finalize), got %d", len(client.requests))
	}
	secondTurnMessages := client.requests[1].Messages
	lastMsg := secondTurnMessages[len(secondTurnMessages)-1]
	if lastMsg.Role != llm.RoleUser || !strings.Contains(strings.ToLower(lastMsg.Text()), "without calling update_plan first") {
		t.Fatalf("expected plan-state repair note to be appended as user message, got %#v", secondTurnMessages)
	}
	if sess.Plan.Phase != planpkg.PhaseConvergeReady {
		t.Fatalf("expected session plan to converge after repair, got %#v", sess.Plan)
	}
	for _, want := range []string{"<proposed_plan>", "Implementation Brief", "Start execution"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("expected repaired answer to include %q, got %q", want, answer)
		}
	}
}

func TestRunPromptRepairsClarifyQuestionWithoutActiveChoice(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Mode = planpkg.ModePlan
	sess.Plan = planpkg.State{
		Goal:         "Implement the first paper RAG demo",
		Summary:      "Need to choose the target demo directory before the rest of the plan converges.",
		Phase:        planpkg.PhaseClarify,
		DecisionGaps: []string{"Choose the target demo directory."},
		Steps: []planpkg.Step{
			{Title: "Choose the target demo directory", Status: planpkg.StepPending},
			{Title: "Lock the technical path", Status: planpkg.StepPending},
			{Title: "Define the runnable acceptance path", Status: planpkg.StepPending},
		},
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "请先选目录： A 复用 demos/paper_rag_minimal（推荐） / B 复用 demos/paper_rag / C 新建 demos/paper_rag_mvp。",
		},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name: "update_plan",
					Arguments: `{
						"summary":"The first directory choice is waiting on the user before plan convergence.",
						"phase":"clarify",
						"decision_gaps":["Choose the target demo directory."],
						"active_choice":{
							"id":"target_demo_directory",
							"kind":"clarify",
							"question":"请先选目录：",
							"gap_key":"Choose the target demo directory.",
							"options":[
								{"id":"reuse_paper_rag_minimal","shortcut":"A","title":"复用 demos/paper_rag_minimal","description":"推荐，最接近最小闭环。","recommended":true},
								{"id":"reuse_paper_rag","shortcut":"B","title":"复用 demos/paper_rag","description":"复用现有目录，但改动面更大。"},
								{"id":"new_paper_rag_mvp","shortcut":"C","title":"新建 demos/paper_rag_mvp","description":"隔离更强，但会多一些样板搭建。"}
							]
						},
						"plan":[
							{"step":"Choose the target demo directory", "status":"pending"},
							{"step":"Lock the technical path", "status":"pending"},
							{"step":"Define the runnable acceptance path", "status":"pending"}
						]
					}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "<turn_intent>ask_user</turn_intent>先从下面的选项里确认目录方案。",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 6,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "继续规划", "plan", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected three requests (repair + update_plan + ask_user), got %d", len(client.requests))
	}
	secondTurnMessages := client.requests[1].Messages
	lastMsg := secondTurnMessages[len(secondTurnMessages)-1]
	if lastMsg.Role != llm.RoleUser || !strings.Contains(strings.ToLower(lastMsg.Text()), "without storing active_choice first") {
		t.Fatalf("expected clarify repair note to be appended as user message, got %#v", secondTurnMessages)
	}
	if sess.Plan.ActiveChoice == nil {
		t.Fatalf("expected session plan to store active_choice after repair, got %#v", sess.Plan)
	}
	if sess.Plan.ActiveChoice.ID != "target_demo_directory" {
		t.Fatalf("expected active_choice to preserve the directory decision key, got %#v", sess.Plan.ActiveChoice)
	}
	if !strings.Contains(answer, "确认目录方案") {
		t.Fatalf("expected repaired answer to keep a short lead sentence, got %q", answer)
	}
	if strings.Contains(answer, "demos/paper_rag_minimal") {
		t.Fatalf("expected repaired answer to avoid inlining option text once the picker can render it, got %q", answer)
	}
}

func TestRunPromptRepairsPlanRevisionAndReturnsRevisedPlanDocument(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Mode = planpkg.ModePlan
	sess.Plan = planpkg.State{
		Goal:                "Implement the first paper RAG demo",
		Summary:             "The plan is converged and ready to execute.",
		ImplementationBrief: "Objective: deliver the first local paper RAG demo.\nDeliverables: upload, retrieval, answer generation, and a minimal HTML page.",
		Phase:               planpkg.PhaseConvergeReady,
		DecisionLog: []planpkg.Decision{
			{Decision: "Use FastAPI + Jinja2 HTML for the first UI path", Reason: "Keeps the HTML GUI simple and local."},
		},
		Steps: []planpkg.Step{
			{Title: "Freeze the MVP scope", Status: planpkg.StepPending},
			{Title: "Implement the minimal UI flow", Status: planpkg.StepPending},
		},
		Verification:        []string{"Start the local app and validate one upload + one answer flow."},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
		NextAction:          "Ask whether to start execution or keep adjusting the plan.",
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "非常好，这一步很关键。建议先把 HTML GUI 收敛成页面功能、页面布局和交互细节三块，这样后面实现会更稳。",
		},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name: "update_plan",
					Arguments: `{
						"summary":"The plan stays converged and now includes a more detailed HTML GUI specification.",
						"implementation_brief":"Objective: deliver the first local paper RAG demo in demos/.\nUI Specification: refine the HTML GUI into explicit page regions and interactions.\nLayout: top bar with title and current model tag; left column for paper management and system status; right upper panel for QA; right lower panel for synthesis and retrieval results.\nInteraction Flow: upload triggers indexing with progress states; answer and synthesis are split into separate tabs; each answer lists its cited source papers.\nDeliverables: runnable FastAPI app, HTML templates, upload/index/qa/synthesis flows, and verification notes.",
						"phase":"converge_ready",
						"decision_log":[
							{"decision":"Use FastAPI + Jinja2 HTML for the first UI path","reason":"Keeps the HTML GUI simple and local."},
							{"decision":"Expand the HTML GUI spec into concrete layout and interaction sections","reason":"User asked for a more detailed UI design before execution."}
						],
						"decision_gaps":[],
						"scope_defined":true,
						"risk_and_rollback_defined":true,
						"verification_defined":true,
						"verification":[
							"Start the local app and validate one upload + one answer flow.",
							"Confirm the HTML page shows the top bar, paper panel, QA tab, and synthesis tab."
						],
						"next_action":"Ask whether to start execution or keep adjusting the plan.",
						"plan":[
							{"step":"Freeze the MVP scope", "status":"pending", "description":"Lock the minimal local paper-RAG scope and the refined HTML GUI boundaries."},
							{"step":"Implement the HTML GUI flow", "status":"pending", "description":"Build the top bar, paper management area, QA tab, and synthesis tab with clear state feedback."}
						]
					}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "<turn_intent>finalize</turn_intent>非常好，这一步很关键。现在我已经把 UI 细化进计划里了，下面是页面功能、页面布局和交互细节的整理版本。",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 6,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "我觉得 html 的界面设计还可以再细化一下，比如明确页面分区和交互。", "plan", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected three requests (repair + update_plan + finalize), got %d", len(client.requests))
	}
	secondTurnMessages := client.requests[1].Messages
	lastMsg := secondTurnMessages[len(secondTurnMessages)-1]
	if lastMsg.Role != llm.RoleUser || !strings.Contains(strings.ToLower(lastMsg.Text()), "plan-refinement feedback without updating the structured plan first") {
		t.Fatalf("expected plan-revision repair note to be appended as user message, got %#v", secondTurnMessages)
	}
	if !strings.Contains(sess.Plan.ImplementationBrief, "UI Specification") {
		t.Fatalf("expected revised plan to absorb the UI detail, got %#v", sess.Plan)
	}
	if !strings.Contains(answer, "已按你的反馈更新计划") {
		t.Fatalf("expected final answer to be condensed into a short revision acknowledgement, got %q", answer)
	}
	if strings.Contains(answer, "非常好，这一步很关键") {
		t.Fatalf("expected standalone revision prose to be removed from the final answer, got %q", answer)
	}
	for _, want := range []string{"<proposed_plan>", "UI Specification", "top bar with title", "right upper panel for QA"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("expected revised plan answer to include %q, got %q", want, answer)
		}
	}
}

func TestRunPromptRepairsBuildHandoffWithoutRestartingPlanConfirmation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Mode = planpkg.ModeBuild
	sess.Plan = planpkg.State{
		Goal:                "Implement the first RAG demo",
		Summary:             "Plan is converged and execution should begin from the repo baseline.",
		Phase:               planpkg.PhaseExecuting,
		NextAction:          "Inspect the workspace entrypoints before editing.",
		Steps:               []planpkg.Step{{Title: "Inspect the workspace entrypoints", Status: planpkg.StepInProgress}},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "<turn_intent>finalize</turn_intent>收到，准备开工。\n当前仍停在计划确认流里，请再发 continue execution 或在 UI 切到 Build。",
		},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "<turn_intent>finalize</turn_intent>我先检查了工作区入口，接下来继续实现。",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 6,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "start execution", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected three requests (repair + tool + finalize), got %d", len(client.requests))
	}
	secondTurnMessages := client.requests[1].Messages
	lastMsg := secondTurnMessages[len(secondTurnMessages)-1]
	if lastMsg.Role != llm.RoleUser || !strings.Contains(strings.ToLower(lastMsg.Text()), "already switched to build mode") {
		t.Fatalf("expected build-handoff repair note to be appended as user message, got %#v", secondTurnMessages)
	}
	if !strings.Contains(answer, "我先检查了工作区入口") {
		t.Fatalf("expected repaired build answer, got %q", answer)
	}
}

func TestRunPromptDoesNotRepairBuildHandoffWhenAssistantRequestsRealBlockerInfo(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Mode = planpkg.ModeBuild
	sess.Plan = planpkg.State{
		Goal:                "Implement the first RAG demo",
		Summary:             "Plan is converged and execution should continue from the repo baseline.",
		Phase:               planpkg.PhaseExecuting,
		NextAction:          "Run the first execution step.",
		Steps:               []planpkg.Step{{Title: "Run the first execution step", Status: planpkg.StepInProgress}},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "<turn_intent>ask_user</turn_intent>Before I proceed, I need the missing API token for the target service.",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "start execution", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected direct ask_user response without build-handoff repair retry, got %d requests", len(client.requests))
	}
	if !strings.Contains(strings.ToLower(answer), "missing api token") {
		t.Fatalf("expected answer to preserve the genuine blocker clarification, got %q", answer)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("expected user + assistant messages only, got %#v", sess.Messages)
	}
}

func TestRunPromptStopsWhenContinueWorkWithoutToolCallsKeepsRepeating(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role:    "assistant",
			Content: "<turn_intent>continue_work</turn_intent>I will continue now.",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 8,
			Stream:        false,
			ContextBudget: config.ContextBudgetConfig{
				WarningRatio:     config.DefaultContextBudgetWarningRatio,
				CriticalRatio:    config.DefaultContextBudgetCriticalRatio,
				MaxReactiveRetry: 1,
			},
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "keep going", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "ongoing work without structured tool calls") {
		t.Fatalf("expected stop summary for repeated no-tool continue turns, got %q", answer)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("expected user + summary assistant messages, got %#v", sess.Messages)
	}
}

func TestRunPromptAutoCompactsLongHistory(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	for i := 0; i < 8; i++ {
		sess.Messages = append(sess.Messages,
			llm.NewUserTextMessage(strings.Repeat("history user segment ", 30)),
			llm.NewAssistantTextMessage(strings.Repeat("history assistant segment ", 30)),
		)
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "Goal: keep working\nDecisions: use go tests\nPending: continue implementation"},
			{Role: llm.RoleAssistant, Content: "done"},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    220,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "continue implementation", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected one compaction request + one turn request, got %d", len(client.requests))
	}
	if len(client.requests[0].Tools) != 0 {
		t.Fatalf("expected compaction request to disable tools, got %#v", client.requests[0].Tools)
	}
	if len(client.requests[0].Messages) < 2 || client.requests[0].Messages[0].Role != llm.RoleSystem {
		t.Fatalf("expected compaction request with system prompt, got %#v", client.requests[0].Messages)
	}
	if !strings.Contains(strings.ToLower(client.requests[0].Messages[0].Text()), "compaction") {
		t.Fatalf("expected compaction system prompt, got %q", client.requests[0].Messages[0].Text())
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("expected compacted session to keep summary + latest user + final assistant, got %#v", sess.Messages)
	}
	if sess.Messages[0].Role != llm.RoleAssistant || !strings.Contains(sess.Messages[0].Text(), "Goal: keep working") {
		t.Fatalf("expected first message to be compaction summary, got %#v", sess.Messages[0])
	}
	if sess.Messages[1].Role != llm.RoleUser || strings.TrimSpace(sess.Messages[1].Text()) != "continue implementation" {
		t.Fatalf("expected latest user message to be preserved, got %#v", sess.Messages[1])
	}
}

func TestRunPromptAutoCompactionPreservesMostRecentCompleteToolPair(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	for i := 0; i < 6; i++ {
		sess.Messages = append(sess.Messages,
			llm.NewUserTextMessage(strings.Repeat("history user segment ", 24)),
			llm.NewAssistantTextMessage(strings.Repeat("history assistant segment ", 24)),
		)
	}
	sess.Messages = append(sess.Messages,
		llm.NewUserTextMessage("older tool task"),
		llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-old",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		llm.NewToolResultMessage("call-old", `{"ok":true,"items":["a.txt"]}`),
		llm.NewAssistantTextMessage("old tool done"),
		llm.NewUserTextMessage("recent tool task"),
		llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-recent",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"README.md"}`,
				},
			}},
		},
		llm.NewToolResultMessage("call-recent", `{"ok":true,"content":"hello"}`),
		llm.NewAssistantTextMessage("recent tool done"),
	)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "Goal: continue\nRecent: keep latest tool context"},
			{Role: llm.RoleAssistant, Content: "done"},
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    260,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "continue implementation", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected one compaction request + one turn request, got %d", len(client.requests))
	}
	if !containsToolUseID(sess.Messages, "call-recent") {
		t.Fatalf("expected compacted session to keep recent tool_use, got %#v", sess.Messages)
	}
	if !containsToolResultID(sess.Messages, "call-recent") {
		t.Fatalf("expected compacted session to keep recent tool_result, got %#v", sess.Messages)
	}
	if containsToolUseID(sess.Messages, "call-old") {
		t.Fatalf("expected older pair to be compacted into summary, got %#v", sess.Messages)
	}
}

func TestRunPromptAutoCompactionDropsIncompleteToolTail(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	for i := 0; i < 6; i++ {
		sess.Messages = append(sess.Messages,
			llm.NewUserTextMessage(strings.Repeat("history user segment ", 24)),
			llm.NewAssistantTextMessage(strings.Repeat("history assistant segment ", 24)),
		)
	}
	sess.Messages = append(sess.Messages,
		llm.NewUserTextMessage("tool task"),
		llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		llm.NewToolResultMessage("call-1", `{"ok":true}`),
		llm.NewAssistantTextMessage("done"),
		llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-open",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "read_file",
					Arguments: `{"path":"missing.txt"}`,
				},
			}},
		},
	)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "Goal: continue\nSummary: first pass"},
			{Role: llm.RoleAssistant, Content: "Goal: continue\nSummary: fallback pass"},
			{Role: llm.RoleAssistant, Content: "done"},
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    260,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "continue implementation", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected two compaction requests + one turn request after fallback, got %d", len(client.requests))
	}
	if containsToolUseID(sess.Messages, "call-open") {
		t.Fatalf("expected orphan tail tool_use to be dropped, got %#v", sess.Messages)
	}
	if err := contextpkg.ValidateToolPairInvariant(sess.Messages); err != nil {
		t.Fatalf("expected compacted session to satisfy pair invariant, got %v", err)
	}
}

func TestCompactSessionManualRewritesHistory(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Messages = append(sess.Messages,
		llm.NewUserTextMessage("first ask"),
		llm.NewAssistantTextMessage("first answer"),
		llm.NewUserTextMessage("second ask"),
		llm.NewAssistantTextMessage("second answer"),
	)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "Goal: first ask\nCompleted: first answer\nPending: second ask"},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    5000,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	summary, changed, err := runner.CompactSession(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("expected compaction to change session state")
	}
	if strings.TrimSpace(summary) == "" {
		t.Fatalf("expected non-empty compaction summary")
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("expected compacted session to keep one summary message, got %#v", sess.Messages)
	}
	if sess.Messages[0].Role != llm.RoleAssistant {
		t.Fatalf("expected compacted summary role assistant, got %#v", sess.Messages[0])
	}
	if !strings.Contains(sess.Messages[0].Text(), "Goal: first ask") {
		t.Fatalf("expected summary content to be persisted, got %#v", sess.Messages[0])
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one compaction request, got %d", len(client.requests))
	}
}

func TestRunPromptAutoCompactionFallsBackWhenSummaryIsEmpty(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	for i := 0; i < 8; i++ {
		sess.Messages = append(sess.Messages,
			llm.NewUserTextMessage(strings.Repeat("history user segment ", 30)),
			llm.NewAssistantTextMessage(strings.Repeat("history assistant segment ", 30)),
		)
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "   "},
			{Role: llm.RoleAssistant, Content: "done"},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    220,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "continue implementation", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected one compaction request + one turn request, got %d", len(client.requests))
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("expected compacted session to keep summary + latest user + final assistant, got %#v", sess.Messages)
	}
	if !strings.Contains(sess.Messages[0].Text(), "Compaction fallback summary") {
		t.Fatalf("expected fallback summary content, got %#v", sess.Messages[0])
	}
}

func TestCompactSessionManualFallsBackWhenSummaryIsEmpty(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Messages = append(sess.Messages,
		llm.NewUserTextMessage("first ask"),
		llm.NewAssistantTextMessage("first answer"),
		llm.NewUserTextMessage("second ask"),
		llm.NewAssistantTextMessage("second answer"),
	)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "   "},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    5000,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	summary, changed, err := runner.CompactSession(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("expected compaction to change session state")
	}
	if strings.TrimSpace(summary) == "" {
		t.Fatalf("expected non-empty fallback summary")
	}
	if !strings.Contains(summary, "Compaction fallback summary") {
		t.Fatalf("expected fallback marker in summary, got %q", summary)
	}
}

func TestRunPromptEncodesToolExecutionErrorsAndContinues(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "missing_tool",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "Recovered after tool failure.",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "trigger failing tool", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Recovered after tool failure." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) != 4 {
		t.Fatalf("expected 4 session messages, got %#v", sess.Messages)
	}
	if sess.Messages[2].Role != "user" {
		t.Fatalf("expected third message to be tool result, got %#v", sess.Messages[2])
	}
	if len(sess.Messages[2].Parts) != 1 || sess.Messages[2].Parts[0].ToolResult == nil {
		t.Fatalf("expected third message to carry tool_result part, got %#v", sess.Messages[2])
	}
	if !strings.Contains(sess.Messages[2].Content, `"ok":false`) || !strings.Contains(sess.Messages[2].Content, `unknown tool`) {
		t.Fatalf("expected encoded tool error payload, got %q", sess.Messages[2].Content)
	}
	if !strings.Contains(sess.Messages[2].Content, `"status":"error"`) || !strings.Contains(sess.Messages[2].Content, `"reason_code":"invalid_args"`) {
		t.Fatalf("expected tool error status and reason_code, got %q", sess.Messages[2].Content)
	}
}

func TestRunPromptAwayAutoDenyContinueKeepsRunningAfterPermissionDenied(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"x.txt","content":"x"}`,
					},
				},
				{
					ID:   "call-2",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "read_file",
						Arguments: `{"path":"x.txt"}`,
					},
				},
			},
		},
		{
			Role:    "assistant",
			Content: "continued after denied approval",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:       config.ProviderConfig{Model: "test-model"},
			MaxIterations:  4,
			Stream:         false,
			ApprovalPolicy: "on-request",
			ApprovalMode:   "away",
			AwayPolicy:     "auto_deny_continue",
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	var out bytes.Buffer
	answer, err := runner.RunPrompt(context.Background(), sess, "trigger permission path", "build", &out)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "continued after denied approval" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) < 4 {
		t.Fatalf("expected tool result message, got %#v", sess.Messages)
	}
	toolMsg := sess.Messages[2]
	if !strings.Contains(toolMsg.Content, `"ok":false`) || !strings.Contains(toolMsg.Content, "away mode") {
		t.Fatalf("expected away-mode denial payload, got %q", toolMsg.Content)
	}
	if !strings.Contains(toolMsg.Content, `"status":"denied"`) || !strings.Contains(toolMsg.Content, `"reason_code":"permission_denied"`) {
		t.Fatalf("expected denied status and permission reason_code, got %q", toolMsg.Content)
	}
	skippedMsg := sess.Messages[3]
	if !strings.Contains(skippedMsg.Content, `"status":"skipped"`) || !strings.Contains(skippedMsg.Content, `"reason_code":"denied_dependency"`) {
		t.Fatalf("expected skipped due dependency payload, got %q", skippedMsg.Content)
	}
	if !strings.Contains(skippedMsg.Content, "skipped because a prior approval-required action was denied") {
		t.Fatalf("expected skipped message to describe denied dependency, got %q", skippedMsg.Content)
	}
	for _, want := range []string{
		"Task report summary:",
		"- Skipped due to denied dependency: read_file",
		"Task report (json):",
		`"denied":["write_file"]`,
		`"skipped_due_to_denied_dependency":["read_file"]`,
		`"skipped_due_to_dependency":["read_file"]`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected successful auto_deny_continue output to contain %q, got %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), `"pending_approval"`) {
		t.Fatalf("expected away-mode task report to avoid pending_approval, got %q", out.String())
	}
}

func TestRunPromptAwayFailFastStopsAfterPermissionDenied(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"x.txt","content":"x"}`,
					},
				},
				{
					ID:   "call-2",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "read_file",
						Arguments: `{"path":"x.txt"}`,
					},
				},
			},
		},
		{
			Role:    "assistant",
			Content: "this reply should not be consumed",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:       config.ProviderConfig{Model: "test-model"},
			MaxIterations:  4,
			Stream:         false,
			ApprovalPolicy: "on-request",
			ApprovalMode:   "away",
			AwayPolicy:     "fail_fast",
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "trigger permission path", "build", io.Discard)
	if err == nil {
		t.Fatal("expected fail_fast mode to stop run after permission denial")
	}
	if strings.TrimSpace(answer) != "" {
		t.Fatalf("expected empty answer when fail_fast stops run, got %q", answer)
	}
	if !strings.Contains(err.Error(), "fail_fast stopped run") {
		t.Fatalf("expected fail_fast stop reason, got %v", err)
	}
	for _, want := range []string{
		"Task report summary:",
		"- Skipped due to denied dependency: read_file",
		"Task report (json):",
		`"denied":["write_file"]`,
		`"skipped_due_to_denied_dependency":["read_file"]`,
		`"skipped_due_to_dependency":["read_file"]`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected fail_fast error to include task report item %q, got %v", want, err)
		}
	}
	if strings.Contains(err.Error(), `"pending_approval"`) {
		t.Fatalf("expected away-mode fail_fast report to avoid pending_approval, got %v", err)
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("expected session to stop after first denied tool call, got %#v", sess.Messages)
	}
	if !strings.Contains(sess.Messages[2].Content, "away_policy=fail_fast") {
		t.Fatalf("expected denied tool payload to include fail_fast policy, got %q", sess.Messages[2].Content)
	}
	if !strings.Contains(sess.Messages[2].Content, `"status":"denied"`) || !strings.Contains(sess.Messages[2].Content, `"reason_code":"permission_denied"`) {
		t.Fatalf("expected denied status and reason code in fail_fast payload, got %q", sess.Messages[2].Content)
	}
}

func TestRunPromptRequiredSystemSandboxFailsClosedWhenBackendUnavailable(t *testing.T) {
	original := resolveAgentSystemSandboxRuntimeStatus
	resolveAgentSystemSandboxRuntimeStatus = func(enabled bool, mode string) (tools.SystemSandboxRuntimeStatus, error) {
		if !enabled {
			return tools.SystemSandboxRuntimeStatus{}, nil
		}
		return tools.SystemSandboxRuntimeStatus{}, errors.New("required backend unavailable in test")
	}
	t.Cleanup(func() {
		resolveAgentSystemSandboxRuntimeStatus = original
	})

	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{{
		Role:    "assistant",
		Content: "should not run",
	}}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     2,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "required",
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err == nil {
		t.Fatal("expected required system sandbox to fail closed when backend is unavailable")
	}
	if strings.TrimSpace(answer) != "" {
		t.Fatalf("expected empty answer on required sandbox startup failure, got %q", answer)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required") || !strings.Contains(strings.ToLower(err.Error()), "unavailable") {
		t.Fatalf("expected required unavailable error, got %v", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("expected no llm request when required sandbox fails closed at startup, got %d", len(client.requests))
	}
}

func TestRunPromptReportsSystemSandboxStartupFallbackOnSuccess(t *testing.T) {
	original := resolveAgentSystemSandboxRuntimeStatus
	resolveAgentSystemSandboxRuntimeStatus = func(enabled bool, mode string) (tools.SystemSandboxRuntimeStatus, error) {
		if !enabled {
			return tools.SystemSandboxRuntimeStatus{}, nil
		}
		return tools.SystemSandboxRuntimeStatus{
			Mode:            mode,
			BackendEnabled:  false,
			CapabilityLevel: "none",
			Fallback:        true,
			Message:         "system sandbox best_effort fallback: test backend unavailable",
			BackendName:     "",
		}, nil
	}
	t.Cleanup(func() {
		resolveAgentSystemSandboxRuntimeStatus = original
	})

	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     2,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role:    "assistant",
			Content: "done",
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	var out bytes.Buffer
	answer, err := runner.RunPrompt(context.Background(), sess, "hello", "build", &out)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	for _, want := range []string{
		"mode=best_effort backend=none state=fallback required_capable=false capability_level=none",
		"Task report summary:",
		"- System sandbox fallback: startup (mode=best_effort, backend=none, required_capable=false, capability_level=none, reason=system sandbox best_effort fallback: test backend unavailable)",
		"Task report (json):",
		`"system_sandbox_fallback":["startup (mode=best_effort, backend=none, required_capable=false, capability_level=none, reason=system sandbox best_effort fallback: test backend unavailable)"]`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected output to contain %q, got %q", want, out.String())
		}
	}
}

func TestRunPromptReportsSystemSandboxStartupActiveRequiredCapable(t *testing.T) {
	original := resolveAgentSystemSandboxRuntimeStatus
	resolveAgentSystemSandboxRuntimeStatus = func(enabled bool, mode string) (tools.SystemSandboxRuntimeStatus, error) {
		if !enabled {
			return tools.SystemSandboxRuntimeStatus{}, nil
		}
		return tools.SystemSandboxRuntimeStatus{
			Mode:            mode,
			BackendEnabled:  true,
			BackendName:     "linux_unshare",
			RequiredCapable: true,
			CapabilityLevel: "full",
			Fallback:        false,
			Message:         `system sandbox backend "linux_unshare" is active`,
		}, nil
	}
	t.Cleanup(func() {
		resolveAgentSystemSandboxRuntimeStatus = original
	})

	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     2,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "required",
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role:    "assistant",
			Content: "done",
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	var out bytes.Buffer
	answer, err := runner.RunPrompt(context.Background(), sess, "hello", "build", &out)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	for _, want := range []string{
		"mode=required backend=linux_unshare state=active required_capable=true capability_level=full",
		`(system sandbox backend "linux_unshare" is active)`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected output to contain %q, got %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Task report summary:") {
		t.Fatalf("did not expect fallback task report summary on active required-capable startup, got %q", out.String())
	}
}

func TestRunPromptFallsBackWhenAssistantReplyIsEmpty(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role: "assistant",
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "Model returned an empty response") {
		t.Fatalf("expected fallback message, got %q", answer)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("expected user and assistant messages, got %#v", sess.Messages)
	}
	if strings.TrimSpace(sess.Messages[1].Content) == "" {
		t.Fatalf("expected persisted assistant fallback message, got %#v", sess.Messages[1])
	}
}

func TestRunPromptEmitsUsageUpdatedEventWhenUsageAvailable(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	events := make([]Event, 0, 4)

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role:    llm.RoleAssistant,
			Content: "done",
			Usage: &llm.Usage{
				InputTokens:   100,
				OutputTokens:  20,
				ContextTokens: 5,
				TotalTokens:   125,
			},
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Observer: ObserverFunc(func(event Event) {
			events = append(events, event)
		}),
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
	})

	_, err = runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, event := range events {
		if event.Type == EventUsageUpdated {
			found = true
			if event.Usage.TotalTokens != 125 || event.Usage.InputTokens != 100 || event.Usage.OutputTokens != 20 || event.Usage.ContextTokens != 5 {
				t.Fatalf("unexpected usage payload: %+v", event.Usage)
			}
		}
	}
	if !found {
		t.Fatalf("expected EventUsageUpdated to be emitted, got %+v", events)
	}
}

func TestRunPromptEmitsEstimatedUsageWhenProviderUsageMissing(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	events := make([]Event, 0, 4)

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role:    llm.RoleAssistant,
			Content: "plain answer without usage payload",
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Observer: ObserverFunc(func(event Event) {
			events = append(events, event)
		}),
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
	})

	_, err = runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, event := range events {
		if event.Type == EventUsageUpdated {
			found = true
			if event.Usage.TotalTokens <= 0 {
				t.Fatalf("expected estimated usage tokens > 0, got %+v", event.Usage)
			}
		}
	}
	if !found {
		t.Fatalf("expected estimated EventUsageUpdated to be emitted, got %+v", events)
	}
}

func TestGetTokenRealtimeSnapshotReturnsSessionAndGlobalStats(t *testing.T) {
	manager, err := tokenusage.NewTokenUsageManager(&tokenusage.Config{
		StorageType:    "memory",
		EnableRealtime: true,
		BackupInterval: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	runner := NewRunner(Options{
		Workspace:    t.TempDir(),
		Config:       config.Config{},
		Client:       &fakeClient{},
		TokenManager: manager,
	})

	if err := manager.RecordTokenUsage(context.Background(), &tokenusage.TokenRecordRequest{
		SessionID:    "sess-1",
		ModelName:    "gpt-5.4",
		InputTokens:  20,
		OutputTokens: 8,
		Latency:      150 * time.Millisecond,
		Success:      true,
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := runner.GetTokenRealtimeSnapshot("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SessionTotalTokens != 28 || snapshot.SessionInputTokens != 20 || snapshot.SessionOutputTokens != 8 {
		t.Fatalf("unexpected session snapshot: %+v", snapshot)
	}
	if snapshot.GlobalTotalTokens != 28 {
		t.Fatalf("expected global total 28, got %d", snapshot.GlobalTotalTokens)
	}
	if snapshot.ActiveSessions < 1 {
		t.Fatalf("expected active sessions >= 1, got %d", snapshot.ActiveSessions)
	}
}

func TestCompactWhitespacePreservesUTF8WhenTruncating(t *testing.T) {
	text := "Continue previous context and list the key MVP test points."
	got := compactWhitespace(text, 18)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("expected valid utf-8 preview, got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated preview to end with ellipsis, got %q", got)
	}
}

func TestRunPromptAppliesActiveSkillToolAllowlist(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"Review changes",
  "tools":{"policy":"allowlist","items":["read_file","search_text","list_files"]}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "SKILL.md"), []byte("# review\nCheck correctness."), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.ActiveSkill = &session.ActiveSkill{Name: "review"}

	client := &fakeClient{replies: []llm.Message{{
		Role:    "assistant",
		Content: "reviewed",
	}}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "review this", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "reviewed" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected at least one request")
	}
	if len(client.requests[0].Messages) == 0 || client.requests[0].Messages[0].Role != "system" {
		t.Fatalf("expected first request message to be system prompt, got %#v", client.requests[0].Messages)
	}
	if !strings.Contains(client.requests[0].Messages[0].Content, "[Available Skills]") ||
		!strings.Contains(client.requests[0].Messages[0].Content, "- review: Review changes") {
		t.Fatalf("expected system prompt to include available skills list, got %q", client.requests[0].Messages[0].Content)
	}
	names := make([]string, 0, len(client.requests[0].Tools))
	for _, def := range client.requests[0].Tools {
		names = append(names, def.Function.Name)
	}
	sort.Strings(names)
	want := []string{"list_files", "read_file", "search_text"}
	if !slices.Equal(names, want) {
		t.Fatalf("unexpected tool list after allowlist filter: got=%v want=%v", names, want)
	}
}

func TestRunPromptBlocksToolCallOutsideActiveSkillPolicy(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"Review changes",
  "tools":{"policy":"allowlist","items":["read_file","search_text"]}
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.ActiveSkill = &session.ActiveSkill{Name: "review"}

	client := &fakeClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "write_file",
					Arguments: `{"path":"x.txt","content":"x"}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "recovered",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "review this", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "recovered" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) < 3 {
		t.Fatalf("expected tool result message, got %#v", sess.Messages)
	}
	toolMsg := sess.Messages[2]
	if toolMsg.Role != "user" || !strings.Contains(toolMsg.Content, "active skill policy") {
		t.Fatalf("expected policy rejection in tool message, got %#v", toolMsg)
	}
	if len(toolMsg.Parts) != 1 || toolMsg.Parts[0].ToolResult == nil {
		t.Fatalf("expected tool_result part in tool message, got %#v", toolMsg)
	}
}

func TestRunPromptDropsToolDefinitionsForNoToolModels(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	client := &fakeClient{replies: []llm.Message{{
		Role:    "assistant",
		Content: "done",
	}}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "gpt-5.4-no-tool"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected request captured")
	}
	if len(client.requests[0].Tools) != 0 {
		t.Fatalf("expected no tool definitions for no-tool model, got %#v", client.requests[0].Tools)
	}
}

func TestActivateAndClearSkillPersistsSessionState(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"Review changes",
  "tools":{"policy":"allowlist","items":["read_file","search_text"]},
  "args":[{"name":"base_ref","required":true,"type":"string"}]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   &fakeClient{},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	if _, err := runner.ActivateSkill(sess, "review", nil); err == nil {
		t.Fatal("expected missing required args to fail")
	}
	skill, err := runner.ActivateSkill(sess, "review", map[string]string{"base_ref": "main"})
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "review" {
		t.Fatalf("unexpected activated skill: %#v", skill)
	}
	if sess.ActiveSkill == nil || sess.ActiveSkill.Name != "review" {
		t.Fatalf("expected session active skill to be set, got %#v", sess.ActiveSkill)
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveSkill == nil || loaded.ActiveSkill.Args["base_ref"] != "main" {
		t.Fatalf("expected persisted active skill args, got %#v", loaded.ActiveSkill)
	}

	if err := runner.ClearActiveSkill(sess); err != nil {
		t.Fatal(err)
	}
	if sess.ActiveSkill != nil {
		t.Fatalf("expected active skill to be cleared, got %#v", sess.ActiveSkill)
	}
}

func TestAuthorSkillTranslatesChineseBriefToEnglish(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeClient{
		replies: []llm.Message{
			llm.NewAssistantTextMessage("Review backend changes and highlight regression risks."),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
			Stream:   false,
		},
		Client:   client,
		Registry: tools.DefaultRegistry(),
	})

	result, err := runner.AuthorSkill("review-plus", hanReviewBriefWithRisk())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "Review backend changes and highlight regression risks.") {
		t.Fatalf("expected translated english description in manifest, got %q", text)
	}
	if containsHanForTest(text) {
		t.Fatalf("expected no Han text in authored manifest, got %q", text)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected translation request to be sent to llm client")
	}
	if len(client.requests[0].Messages) < 2 || !strings.Contains(client.requests[0].Messages[0].Text(), "Translate the user's skill description") {
		t.Fatalf("expected translation system instruction in first request, got %#v", client.requests[0].Messages)
	}
}

func TestAuthorSkillFallsBackToEnglishWhenTranslationFails(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeClient{
		replies: []llm.Message{
			llm.NewAssistantTextMessage(hanReviewBrief()),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
			Stream:   false,
		},
		Client:   client,
		Registry: tools.DefaultRegistry(),
	})

	result, err := runner.AuthorSkill("review-plus", hanReviewBrief())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, skillAuthorEnglishFallback) {
		t.Fatalf("expected english fallback description in manifest, got %q", text)
	}
	if containsHanForTest(text) {
		t.Fatalf("expected no Han text in fallback manifest, got %q", text)
	}
}

func containsHanForTest(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func hanReviewBrief() string {
	return string([]rune{0x7528, 0x4e8e, 0x4ee3, 0x7801, 0x8bc4, 0x5ba1})
}

func hanReviewBriefWithRisk() string {
	return string([]rune{
		0x7528, 0x4e8e, 0x4ee3, 0x7801, 0x8bc4, 0x5ba1,
		0xff0c,
		0x91cd, 0x70b9, 0x5173, 0x6ce8, 0x56de, 0x5f52, 0x98ce, 0x9669,
	})
}
