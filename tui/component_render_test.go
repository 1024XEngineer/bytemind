package tui

import (
	"strings"
	"testing"
	"time"

	"bytemind/internal/history"
	"bytemind/internal/mention"
	planpkg "bytemind/internal/plan"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
)

func TestComponentPromptSearchPaletteRendersEmptyAndResultStates(t *testing.T) {
	empty := model{width: 100}
	empty.promptSearchMode = promptSearchModeQuick
	empty.promptSearchQuery = ""
	emptyView := empty.renderPromptSearchPalette()
	if !strings.Contains(emptyView, "Prompt history search") || !strings.Contains(emptyView, "No matching prompts.") {
		t.Fatalf("expected empty prompt search view, got %q", emptyView)
	}

	withResult := model{width: 100}
	withResult.promptSearchMode = promptSearchModePanel
	withResult.promptSearchQuery = "bug"
	withResult.promptSearchMatches = []history.PromptEntry{{
		Timestamp: time.Now(),
		Workspace: "E:/bytemind",
		SessionID: "session-123",
		Prompt:    "fix rendering bug",
	}}
	resultView := withResult.renderPromptSearchPalette()
	for _, want := range []string{"fix rendering bug", "session-123", "panel  query:bug"} {
		if !strings.Contains(resultView, want) {
			t.Fatalf("expected prompt search result to contain %q, got %q", want, resultView)
		}
	}
}

func TestComponentCommandAndMentionPaletteRenderStates(t *testing.T) {
	input := textarea.New()
	input.SetValue("/definitely-not-found")
	m := model{width: 90, input: input}
	if got := m.renderCommandPalette(); !strings.Contains(got, "No matching commands.") {
		t.Fatalf("expected empty command palette state, got %q", got)
	}

	m.input.SetValue("/")
	m.syncCommandPalette()
	commandView := m.renderCommandPalette()
	for _, want := range []string{"/help", "/session", "/skills-select"} {
		if !strings.Contains(commandView, want) {
			t.Fatalf("expected command palette to contain %q, got %q", want, commandView)
		}
	}

	m.mentionResults = []mention.Candidate{{Path: "tui/model.go", BaseName: "model.go", TypeTag: "go"}}
	mentionView := m.renderMentionPalette()
	if !strings.Contains(mentionView, "[go] model.go") || !strings.Contains(mentionView, "tui/model.go") {
		t.Fatalf("expected mention palette row with metadata, got %q", mentionView)
	}
}

func TestComponentFooterInfoRightModelAndHintPaths(t *testing.T) {
	withModel := renderFooterInfoRight("GPT-5.4", 40)
	if !strings.Contains(withModel, "GPT-5.4") {
		t.Fatalf("expected model text in footer right, got %q", withModel)
	}

	hintsOnly := renderFooterInfoRight("", 20)
	if strings.TrimSpace(hintsOnly) == "" {
		t.Fatal("expected compacted hints when model is empty")
	}
}

func TestComponentPlanPanelContentAndStepRender(t *testing.T) {
	m := model{
		width:    120,
		mode:     modePlan,
		planView: viewport.New(10, 5),
		plan: planpkg.State{
			Goal:       "Finish componentization",
			Summary:    "Extract plan panel",
			Phase:      planpkg.PhaseExecuting,
			NextAction: "Open follow-up PR",
			Steps: []planpkg.Step{{
				Title:       "Extract renderPlanPanel",
				Description: "Move plan rendering into component file",
				Status:      planpkg.StepInProgress,
				Files:       []string{"tui/component_plan_panel.go"},
				Verify:      []string{"go test ./tui -run Plan"},
				Risk:        planpkg.RiskLow,
			}},
		},
	}

	content := m.planPanelContent(48)
	for _, want := range []string{"PLAN", "Phase: executing", "Goal", "Steps", "Next Action", "Risk: low"} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected plan panel content to contain %q, got %q", want, content)
		}
	}

	m.planView.SetContent("plan viewport")
	panel := m.renderPlanPanel(36)
	if strings.TrimSpace(panel) == "" {
		t.Fatal("expected non-empty rendered plan panel")
	}

	height := m.planPanelRenderHeight()
	if height != 0 {
		t.Fatalf("expected zero plan panel render height when panel is disabled, got %d", height)
	}
}

func TestRenderChatSectionShowsSimpleAssistantStateLabels(t *testing.T) {
	streaming := renderChatSection(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   "Streaming partial answer",
		Status: "streaming",
	}, 60)
	if !strings.Contains(streaming, "Generating") {
		t.Fatalf("expected streaming assistant section to show generating label, got %q", streaming)
	}

	final := renderChatSection(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   "Completed answer",
		Status: "final",
	}, 60)
	if !strings.Contains(final, "Answer") {
		t.Fatalf("expected final assistant section to show answer label, got %q", final)
	}
	if strings.Contains(final, "Generating") {
		t.Fatalf("expected final assistant section not to look in-progress, got %q", final)
	}
}

func TestRenderConversationKeepsProgressBlueAndFinalNeutral(t *testing.T) {
	m := model{}
	m.viewport.Width = 80
	m.chatItems = []chatEntry{
		{Kind: "user", Title: "You", Body: "Help me improve the UI"},
		{Kind: "assistant", Title: assistantLabel, Body: "Still working", Status: "streaming"},
		{Kind: "assistant", Title: assistantLabel, Body: "Updated the UI distinction.", Status: "final"},
	}

	view := m.renderConversation()
	for _, want := range []string{"Generating", "Answer", "Updated the UI distinction."} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected conversation to contain %q, got %q", want, view)
		}
	}
}
