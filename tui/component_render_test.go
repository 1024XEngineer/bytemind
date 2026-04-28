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

	settling := renderChatSection(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   "Wrapping up",
		Status: "settling",
	}, 60)
	if !strings.Contains(settling, "Finalizing") {
		t.Fatalf("expected settling assistant section to show finalizing badge, got %q", settling)
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

func TestRenderBytemindRunCardCollapsesConsecutiveReadTools(t *testing.T) {
	view := stripANSI(renderBytemindRunCard([]chatEntry{
		{Kind: "assistant", Title: thinkingLabel, Body: "Inspecting files", Status: "thinking"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read server.py\nrange: 1-20", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read index.html\nrange: 1-40", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read README.md\nrange: 1-80", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read faq.md\nrange: 1-50", Status: "done"},
	}, 80))

	if strings.Count(view, "READ x") != 1 {
		t.Fatalf("expected consecutive read tools to collapse into one section, got %q", view)
	}
	if !strings.Contains(view, "READ x 4") {
		t.Fatalf("expected collapsed read header with count, got %q", view)
	}
	if !strings.Contains(view, "Read 4 files: server.py, index.html, README.md +1") {
		t.Fatalf("expected collapsed read summary, got %q", view)
	}
}

func TestRenderBytemindRunCardDoesNotCollapseSeparatedReadTools(t *testing.T) {
	view := stripANSI(renderBytemindRunCard([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read server.py", Status: "done"},
		{Kind: "assistant", Title: assistantLabel, Body: "Using that result first", Status: "final"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read index.html", Status: "done"},
	}, 80))

	if strings.Count(view, "READ  ") != 2 {
		t.Fatalf("expected separated read tools to remain distinct, got %q", view)
	}
}

func TestCollapseRunSectionGroupsKeepsNonReadAndSplitsReadRuns(t *testing.T) {
	items := []chatEntry{
		{Kind: "assistant", Title: assistantLabel, Body: "Thinking", Status: "final"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read a.go", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read b.go", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("list_files"), Body: "Listed files", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read c.go", Status: "done"},
	}

	groups := collapseRunSectionGroups(items)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups, got %d: %#v", len(groups), groups)
	}
	if len(groups[0]) != 1 || groups[0][0].Kind != "assistant" {
		t.Fatalf("expected first group to keep non-tool item intact, got %#v", groups[0])
	}
	if len(groups[1]) != 2 {
		t.Fatalf("expected adjacent read tools to collapse together, got %#v", groups[1])
	}
	if len(groups[2]) != 1 || !strings.Contains(groups[2][0].Title, "list_files") {
		t.Fatalf("expected non-read tool to stay separate, got %#v", groups[2])
	}
	if len(groups[3]) != 1 || !strings.Contains(groups[3][0].Body, "Read c.go") {
		t.Fatalf("expected trailing read tool to become its own group, got %#v", groups[3])
	}
}

func TestCollapsibleParallelToolNameOnlyAcceptsReadTools(t *testing.T) {
	tests := []struct {
		name string
		item chatEntry
		ok   bool
		want string
	}{
		{
			name: "read tool",
			item: chatEntry{Kind: "tool", Title: toolEntryTitle("read_file")},
			ok:   true,
			want: "read_file",
		},
		{
			name: "non tool",
			item: chatEntry{Kind: "assistant", Title: toolEntryTitle("read_file")},
			ok:   false,
		},
		{
			name: "empty tool name",
			item: chatEntry{Kind: "tool", Title: "READ | "},
			ok:   false,
		},
		{
			name: "non read tool",
			item: chatEntry{Kind: "tool", Title: toolEntryTitle("list_files")},
			ok:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := collapsibleParallelToolName(tc.item)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("expected (%q, %v), got (%q, %v)", tc.want, tc.ok, got, ok)
			}
		})
	}
}

func TestRenderRunSectionGroupSummariesAndStatuses(t *testing.T) {
	if got := renderRunSectionGroup(nil, 60); got != "" {
		t.Fatalf("expected empty group to render empty string, got %q", got)
	}

	single := renderRunSectionGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read one.go", Status: "done"},
	}, 60)
	if !strings.Contains(stripANSI(single), "Read one.go") {
		t.Fatalf("expected single group to render original section, got %q", single)
	}

	multiRead := stripANSI(renderRunSectionGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read one.go", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read two.go", Status: "running"},
	}, 80))
	for _, want := range []string{"READ x 2", "Read 2 files: one.go, two.go", "running"} {
		if !strings.Contains(strings.ToLower(multiRead), strings.ToLower(want)) {
			t.Fatalf("expected grouped read render to contain %q, got %q", want, multiRead)
		}
	}

	multiOther := stripANSI(renderRunSectionGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("list_files"), Body: "files", Status: "warn"},
		{Kind: "tool", Title: toolEntryTitle("list_files"), Body: "more files", Status: "done"},
	}, 80))
	if !strings.Contains(multiOther, "2 parallel list calls") {
		t.Fatalf("expected generic parallel summary, got %q", multiOther)
	}
	if !strings.Contains(strings.ToLower(multiOther), "warn") {
		t.Fatalf("expected grouped status to prefer warn, got %q", multiOther)
	}
}

func TestSummarizeParallelReadGroupAndAggregateStatusFallbacks(t *testing.T) {
	if got := summarizeParallelReadGroup([]chatEntry{
		{Kind: "tool", Body: ""},
		{Kind: "tool", Body: "   "},
	}); got != "Read 2 files" {
		t.Fatalf("expected fallback read summary, got %q", got)
	}

	if got := summarizeParallelReadGroup([]chatEntry{
		{Kind: "tool", Body: "Read a.go"},
		{Kind: "tool", Body: "Read b.go"},
		{Kind: "tool", Body: "Read c.go"},
		{Kind: "tool", Body: "Read d.go"},
	}); got != "Read 4 files: a.go, b.go, c.go +1" {
		t.Fatalf("expected preview read summary, got %q", got)
	}

	if got := summarizeParallelToolGroup(nil, "read_file"); got != "" {
		t.Fatalf("expected empty group summary to be empty, got %q", got)
	}

	if got := aggregateToolGroupStatus([]chatEntry{
		{Status: "done"},
		{Status: "failed"},
	}); got != "error" {
		t.Fatalf("expected failed tool group to map to error, got %q", got)
	}

	if got := aggregateToolGroupStatus([]chatEntry{
		{Status: "active"},
		{Status: "done"},
	}); got != "running" {
		t.Fatalf("expected active tool group to map to running, got %q", got)
	}

	if got := aggregateToolGroupStatus([]chatEntry{
		{Status: "custom"},
	}); got != "custom" {
		t.Fatalf("expected unknown status to fall back to first entry status, got %q", got)
	}
}

func TestRenderRunSectionDividerUsesAsciiHyphen(t *testing.T) {
	if got := renderRunSectionDivider(0); got != "" {
		t.Fatalf("expected zero-width divider to be empty, got %q", got)
	}

	got := stripANSI(renderRunSectionDivider(5))
	if !strings.Contains(got, "-----") {
		t.Fatalf("expected divider to use ascii hyphens, got %q", got)
	}
	if strings.Contains(got, "鈹€") {
		t.Fatalf("expected divider not to use box-drawing glyphs, got %q", got)
	}
}

func TestRenderRunSectionDividerLegacyUsesPreviousGlyph(t *testing.T) {
	if got := renderRunSectionDividerLegacy(0); got != "" {
		t.Fatalf("expected zero-width legacy divider to be empty, got %q", got)
	}

	got := stripANSI(renderRunSectionDividerLegacy(5))
	if strings.Contains(got, "-----") {
		t.Fatalf("expected legacy divider to differ from ascii fallback, got %q", got)
	}
}
