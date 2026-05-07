package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/history"
	"github.com/1024XEngineer/bytemind/internal/mention"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
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
	for _, want := range []string{"/help", "/session", "/agents"} {
		if !strings.Contains(commandView, want) {
			t.Fatalf("expected command palette to contain %q, got %q", want, commandView)
		}
	}

	m.mentionResults = []mention.Candidate{{Path: "tui/model.go", BaseName: "model.go", TypeTag: "go"}}
	mentionView := m.renderMentionPalette()
	if !strings.Contains(mentionView, "tui/model.go") {
		t.Fatalf("expected mention palette row with path, got %q", mentionView)
	}
}

func TestComponentFooterInfoRightModelAndHintPaths(t *testing.T) {
	hints := []footerShortcutHint{
		{Key: "tab", Label: "agents"},
		{Key: "/", Label: "commands"},
	}
	withModel := renderFooterInfoRight("GPT-5.4", hints, 40)
	if !strings.Contains(withModel, "GPT-5.4") {
		t.Fatalf("expected model text in footer right, got %q", withModel)
	}

	hintsOnly := renderFooterInfoRight("", hints, 20)
	if strings.TrimSpace(hintsOnly) == "" {
		t.Fatal("expected compacted hints when model is empty")
	}
}

func TestComponentFooterHintsShowEscInterruptOnlyWhenCancelable(t *testing.T) {
	m := model{
		busy:      true,
		runCancel: func() {},
	}

	hints := m.footerShortcutHints()
	foundEsc := false
	for _, hint := range hints {
		if hint.Key == "Esc" && hint.Label == "interrupt" {
			foundEsc = true
			break
		}
	}
	if !foundEsc {
		t.Fatalf("expected Esc interrupt hint while run is cancelable, got %#v", hints)
	}

	m.runCancel = nil
	hints = m.footerShortcutHints()
	for _, hint := range hints {
		if hint.Key == "Esc" {
			t.Fatalf("did not expect Esc hint when runCancel is nil, got %#v", hints)
		}
	}

	m.busy = false
	m.runCancel = func() {}
	hints = m.footerShortcutHints()
	for _, hint := range hints {
		if hint.Key == "Esc" {
			t.Fatalf("did not expect Esc hint when not busy, got %#v", hints)
		}
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

func TestRenderConversationRendersExpandableUserPasteBlocks(t *testing.T) {
	m := model{
		pastedContents: map[string]pastedContent{
			"1": {
				ID:      "1",
				Content: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12",
				Lines:   12,
			},
		},
		pastedOrder:      []string{"1"},
		pasteExpandLevel: map[string]int{},
	}
	m.viewport.Width = 100
	m.chatItems = []chatEntry{{Kind: "user", Title: "You", Body: "inspect [Paste #1 ~12 lines]"}}

	collapsed := stripANSI(m.renderConversation())
	if !strings.Contains(collapsed, "[Paste #1 ~12 lines]") {
		t.Fatalf("expected collapsed paste marker in conversation, got %q", collapsed)
	}
	if !strings.Contains(collapsed, "[click]") {
		t.Fatalf("expected collapsed paste marker hint, got %q", collapsed)
	}

	m.pasteExpandLevel["1"] = 1
	preview := stripANSI(m.renderConversation())
	for _, want := range []string{"[Paste #1 ~12 lines] [preview]", "line1", "line10", "... (2 more lines, click again for full, Ctrl+E expand all)"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("expected preview conversation to contain %q, got %q", want, preview)
		}
	}
	if strings.Contains(preview, "line11") || strings.Contains(preview, "line12") {
		t.Fatalf("expected preview not to show remaining lines, got %q", preview)
	}

	m.pasteExpandLevel["1"] = 2
	full := stripANSI(m.renderConversation())
	for _, want := range []string{"[Paste #1 ~12 lines] [full]", "line12", "click again to collapse"} {
		if !strings.Contains(full, want) {
			t.Fatalf("expected full conversation to contain %q, got %q", want, full)
		}
	}
}

func TestRenderBytemindRunCardCollapsesConsecutiveReadTools(t *testing.T) {
	entries := []chatEntry{
		{Kind: "assistant", Title: thinkingLabel, Body: "Inspecting files", Status: "thinking"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read server.py\nrange: 1-20", Status: "done", CompactBody: "server.py (1-20)"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read index.html\nrange: 1-40", Status: "done", CompactBody: "index.html (1-40)"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read README.md\nrange: 1-80", Status: "done", CompactBody: "README.md (1-80)"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read faq.md\nrange: 1-50", Status: "done", CompactBody: "faq.md (1-50)"},
	}

	collapsed := stripANSI(renderBytemindRunCard(entries, 80, false, true))
	if !strings.Contains(strings.ToLower(collapsed), "reading 4 files") {
		t.Fatalf("expected consecutive read tools to collapse into one summary row, got %q", collapsed)
	}
	if !strings.Contains(strings.ToLower(collapsed), "(ctrl+o to expand)") {
		t.Fatalf("expected collapsed view to include expand hint, got %q", collapsed)
	}
	if strings.Contains(collapsed, "└") {
		t.Fatalf("expected collapsed done view to hide detail hint rows, got %q", collapsed)
	}

	expanded := stripANSI(renderBytemindRunCard(entries, 80, true, true))
	if strings.Count(expanded, "└") != 4 {
		t.Fatalf("expected expanded view to show 4 detail rows, got %q", expanded)
	}
}
func TestRenderBytemindRunCardDoesNotCollapseSeparatedReadTools(t *testing.T) {
	view := stripANSI(renderBytemindRunCard([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read server.py", Status: "done", CompactBody: "server.py"},
		{Kind: "assistant", Title: assistantLabel, Body: "Using that result first", Status: "final"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read index.html", Status: "done", CompactBody: "index.html"},
	}, 80, false, true))

	if strings.Count(view, "●") != 2 {
		t.Fatalf("expected separated read tools to remain distinct with tool icons, got %q", view)
	}
}
func TestRenderBytemindRunCardOmitsDividerBetweenSections(t *testing.T) {
	view := stripANSI(renderBytemindRunCard([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("list_files"), Body: "Read 29 files, listed 31 directories", Status: "done", CompactBody: "29 files, 31 dirs"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read 2 files", Status: "error", CompactBody: "README.md"},
	}, 100, false, true))

	if strings.Contains(view, "-----") {
		t.Fatalf("expected run card sections to omit divider line, got %q", view)
	}
	if !strings.Contains(strings.ToLower(view), "reading 1 file, listing 1 path") {
		t.Fatalf("expected collapsed live inspect summary to include read/list counters, got %q", view)
	}
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Fatalf("expected error status to remain visible, got %q", view)
	}
}

func TestRenderConversationToolDetailsDefaultCollapsedAndExpandToggle(t *testing.T) {
	m := model{}
	m.viewport.Width = 80
	m.chatItems = []chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read a.go\nrange: 1-10", Status: "done", CompactBody: "a.go (1-10)"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read b.go\nrange: 1-20", Status: "done", CompactBody: "b.go (1-20)"},
	}

	collapsed := stripANSI(m.renderConversation())
	if strings.Contains(collapsed, "└a.go") || strings.Contains(collapsed, "└ b.go (1-20)") {
		t.Fatalf("expected collapsed tool details to hide detail hint rows by default, got %q", collapsed)
	}

	m.toolDetailExpanded = true
	expanded := stripANSI(m.renderConversation())
	for _, want := range []string{"└a.go (1-10)", "└b.go (1-20)"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expected expanded tool detail view to contain %q, got %q", want, expanded)
		}
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

func TestCollapseRunSectionGroupsForViewCollapsesLiveInspectFlow(t *testing.T) {
	items := []chatEntry{
		{Kind: "tool", Title: toolEntryTitle("search_text"), Status: "running", CompactBody: `"isMeaningfulThinking"`},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Status: "done", CompactBody: "tui/model.go"},
		{Kind: "tool", Title: toolEntryTitle("search_text"), Status: "running", CompactBody: `"finalizeAssistantTurnForTool"`},
	}

	collapsedGroups := collapseRunSectionGroupsForView(items, false)
	if len(collapsedGroups) != 1 {
		t.Fatalf("expected collapsed live inspect flow to merge into one group, got %#v", collapsedGroups)
	}

	expandedGroups := collapseRunSectionGroupsForView(items, true)
	if len(expandedGroups) != 3 {
		t.Fatalf("expected expanded view to keep tool groups split, got %#v", expandedGroups)
	}
}

func TestRenderBytemindRunCardCollapsedLiveInspectSummaryAcrossTools(t *testing.T) {
	entries := []chatEntry{
		{Kind: "tool", Title: toolEntryTitle("search_text"), Status: "running", CompactBody: `"isMeaningfulThinking"`},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Status: "done", CompactBody: "tui/model.go"},
		{Kind: "tool", Title: toolEntryTitle("search_text"), Status: "running", CompactBody: `"finalizeAssistantTurnForTool"`},
	}

	collapsed := stripANSI(renderBytemindRunCard(entries, 100, false, true))
	for _, want := range []string{
		"Searching for 2 patterns, reading 1 file...",
		"(ctrl+o to expand)",
		`└ "finalizeAssistantTurnForTool"`,
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("expected collapsed live inspect summary to contain %q, got %q", want, collapsed)
		}
	}
}

func TestRenderBytemindRunCardCollapsedDoneInspectGroupKeepsSingleLine(t *testing.T) {
	entries := []chatEntry{
		{Kind: "tool", Title: toolEntryTitle("search_text"), Status: "done", CompactBody: `17 matches for "toolDetailsExpanded"`},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Status: "done", CompactBody: "model.go (2350-2420)"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Status: "done", CompactBody: "model.go (2410-2470)"},
	}

	collapsed := stripANSI(renderBytemindRunCard(entries, 58, false, true))
	if strings.Contains(strings.ToLower(collapsed), "ctrl+o to \n expand") {
		t.Fatalf("expected collapsed done inspect group headline not to wrap unexpectedly, got %q", collapsed)
	}
	if !strings.Contains(strings.ToLower(collapsed), "searching for 1 pattern") {
		t.Fatalf("expected collapsed done inspect summary to include counters, got %q", collapsed)
	}
}

func TestToolStatusIndicatorRunningCanBlinkOff(t *testing.T) {
	visible := stripANSI(toolStatusIndicator("running", true))
	hidden := stripANSI(toolStatusIndicator("running", false))
	if strings.TrimSpace(visible) == "" {
		t.Fatalf("expected running indicator to be visible when blink frame is on, got %q", visible)
	}
	if strings.TrimSpace(hidden) != "" {
		t.Fatalf("expected running indicator to be blank when blink frame is off, got %q", hidden)
	}
}

func TestCollapsibleParallelToolNameAcceptsAllTools(t *testing.T) {
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
			ok:   true,
			want: "READ |",
		},
		{
			name: "list tool",
			item: chatEntry{Kind: "tool", Title: toolEntryTitle("list_files")},
			ok:   true,
			want: "list_files",
		},
		{
			name: "shell tool",
			item: chatEntry{Kind: "tool", Title: toolEntryTitle("run_shell")},
			ok:   true,
			want: "run_shell",
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
	if got := renderRunSectionGroup(nil, 60, false, true); got != "" {
		t.Fatalf("expected empty group to render empty string, got %q", got)
	}

	singleCollapsed := renderRunSectionGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read one.go", Status: "done", CompactBody: "one.go", DetailLines: []string{"range: 1-10", "path: one.go"}},
	}, 60, false, true)
	if !strings.Contains(stripANSI(singleCollapsed), "one.go") {
		t.Fatalf("expected single group to render compact body, got %q", singleCollapsed)
	}
	if strings.Contains(strings.ToLower(stripANSI(singleCollapsed)), "done") {
		t.Fatalf("expected done status text to stay hidden for cleaner render, got %q", singleCollapsed)
	}
	if strings.Contains(stripANSI(singleCollapsed), "range: 1-10") {
		t.Fatalf("expected single collapsed group to hide details, got %q", singleCollapsed)
	}

	singleExpanded := renderRunSectionGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read one.go", Status: "done", CompactBody: "one.go", DetailLines: []string{"range: 1-10", "path: one.go"}},
	}, 60, true, true)
	for _, want := range []string{"one.go", "range: 1-10", "path: one.go"} {
		if !strings.Contains(stripANSI(singleExpanded), want) {
			t.Fatalf("expected single expanded group to contain %q, got %q", want, singleExpanded)
		}
	}

	multiReadCollapsed := stripANSI(renderRunSectionGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read one.go", Status: "done", CompactBody: "one.go"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read two.go", Status: "running", CompactBody: "two.go"},
	}, 80, false, true))
	for _, want := range []string{"reading 2 files", "(ctrl+o to expand)", "two.go"} {
		if !strings.Contains(strings.ToLower(multiReadCollapsed), strings.ToLower(want)) {
			t.Fatalf("expected grouped read collapsed render to contain %q, got %q", want, multiReadCollapsed)
		}
	}

	multiReadExpanded := stripANSI(renderRunSectionGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read one.go", Status: "done", CompactBody: "one.go"},
		{Kind: "tool", Title: toolEntryTitle("read_file"), Body: "Read two.go", Status: "running", CompactBody: "two.go"},
	}, 80, true, true))
	for _, want := range []string{"read 2 files", "one.go", "two.go", "running"} {
		if !strings.Contains(strings.ToLower(multiReadExpanded), strings.ToLower(want)) {
			t.Fatalf("expected grouped read expanded render to contain %q, got %q", want, multiReadExpanded)
		}
	}

	multiOther := stripANSI(renderRunSectionGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("list_files"), Body: "files", Status: "warn", CompactBody: "10 files, 5 dirs"},
		{Kind: "tool", Title: toolEntryTitle("list_files"), Body: "more files", Status: "done", CompactBody: "20 files, 3 dirs"},
	}, 80, false, true))
	if !strings.Contains(strings.ToLower(multiOther), "listing 2 paths") {
		t.Fatalf("expected live inspect summary for list tools, got %q", multiOther)
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
	if strings.Contains(got, "─") {
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

func TestRenderMentionPaletteEmptyAndAgentRecentMarkers(t *testing.T) {
	empty := model{width: 90}
	emptyView := stripANSI(empty.renderMentionPalette())
	if !strings.Contains(emptyView, "No matching results.") {
		t.Fatalf("expected empty mention palette state, got %q", emptyView)
	}

	m := model{
		width:  90,
		height: 10,
		mentionResults: []mention.Candidate{
			{Path: "explorer", BaseName: "explorer", Kind: "agent", Description: "scan code paths"},
			{Path: "tui/model.go", BaseName: "model.go", Kind: "file"},
		},
		mentionRecent: map[string]int{
			"explorer":     2,
			"tui/model.go": 1,
		},
	}
	view := stripANSI(m.renderMentionPalette())
	for _, want := range []string{
		"* * explorer  scan code paths",
		"* + tui/model.go",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected mention palette to contain %q, got %q", want, view)
		}
	}
}

func TestCollapsibleParallelToolNameUsesAgentIDForDelegateSubagent(t *testing.T) {
	withAgent, ok := collapsibleParallelToolName(chatEntry{
		Kind:    "tool",
		Title:   toolEntryTitle("delegate_subagent"),
		AgentID: "explorer",
	})
	if !ok || withAgent != "delegate_subagent:explorer" {
		t.Fatalf("expected delegate_subagent key with agent id, got key=%q ok=%v", withAgent, ok)
	}

	withoutAgent, ok := collapsibleParallelToolName(chatEntry{
		Kind:  "tool",
		Title: toolEntryTitle("delegate_subagent"),
	})
	if !ok || withoutAgent != "delegate_subagent" {
		t.Fatalf("expected delegate_subagent key without suffix, got key=%q ok=%v", withoutAgent, ok)
	}
}

func TestRenderSubAgentBlockThreeStateModes(t *testing.T) {
	running := chatEntry{
		Status: "running",
		SubAgentTools: []SubAgentToolCall{
			{ToolName: "read_file", CompactBody: "a.go", Status: "done"},
			{ToolName: "search_text", CompactBody: `"token"`, Status: "done"},
			{ToolName: "run_shell", CompactBody: "go test ./...", Status: "done"},
			{ToolName: "write_file", CompactBody: "b.go", Status: "running"},
		},
	}
	runningView := stripANSI(renderSubAgentBlock(running, "explorer", "scan auth flow", 120, false, true))
	for _, want := range []string{"explorer", "scan auth flow", "+4 tool uses", "(1 running)", "(ctrl+o to expand)"} {
		if !strings.Contains(runningView, want) {
			t.Fatalf("expected running subagent block to contain %q, got %q", want, runningView)
		}
	}

	done := chatEntry{
		Status:         "done",
		TotalToolCalls: 5,
	}
	doneView := stripANSI(renderSubAgentBlock(done, "reviewer", "summarize failures", 120, false, true))
	for _, want := range []string{"reviewer", "Done (5 tool uses)", "(ctrl+o to expand)"} {
		if !strings.Contains(doneView, want) {
			t.Fatalf("expected completed subagent block to contain %q, got %q", want, doneView)
		}
	}

	expanded := chatEntry{
		Status:     "done",
		TaskPrompt: "investigate flaky tests",
		SubAgentTools: []SubAgentToolCall{
			{ToolName: "read_file", CompactBody: "main_test.go", Status: "done"},
			{ToolName: "search_text", Summary: "found unstable timing assertion in 3 places", Status: "running"},
		},
	}
	expandedView := stripANSI(renderSubAgentBlock(expanded, "planner", "", 120, true, true))
	for _, want := range []string{"planner", "Prompt:", "investigate flaky tests", "read_file(main_test.go)", "search_text("} {
		if !strings.Contains(expandedView, want) {
			t.Fatalf("expected expanded subagent block to contain %q, got %q", want, expandedView)
		}
	}
}

func TestRenderRunSectionGroupDelegateSubagentAggregation(t *testing.T) {
	group := []chatEntry{
		{
			Kind:        "tool",
			Title:       toolEntryTitle("delegate_subagent"),
			Status:      "running",
			AgentID:     "explorer",
			CompactBody: "scan service wiring",
			DetailLines: []string{"prompt: scan service wiring"},
			SubAgentTools: []SubAgentToolCall{
				{ToolName: "read_file", CompactBody: "service.go", Status: "done"},
				{ToolName: "search_text", CompactBody: `"wire.NewSet"`, Status: "running"},
			},
		},
		{
			Kind:        "tool",
			Title:       toolEntryTitle("delegate_subagent"),
			Status:      "done",
			AgentID:     "explorer",
			CompactBody: "verify tests",
			DetailLines: []string{"prompt: verify tests"},
			SubAgentTools: []SubAgentToolCall{
				{ToolName: "run_shell", CompactBody: "go test ./...", Status: "done"},
			},
		},
	}

	collapsed := stripANSI(renderRunSectionGroup(group, 140, false, true))
	for _, want := range []string{"2 x explorer", "scan service wiring", "verify tests", "(ctrl+o to expand)"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("expected collapsed delegate group to contain %q, got %q", want, collapsed)
		}
	}

	expanded := stripANSI(renderRunSectionGroup(group, 140, true, true))
	for _, want := range []string{"2 x explorer", "prompt: scan service wiring", "read_file(service.go)", "run_shell(go test ./...)"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expected expanded delegate group to contain %q, got %q", want, expanded)
		}
	}
}

func TestSummarizeDelegateSubAgentHandlesInvalidPayload(t *testing.T) {
	agentName, task := summarizeDelegateSubAgent(`{"agent"`)
	if agentName != "" || task != "" {
		t.Fatalf("expected invalid payload to return empty summary, got %q %q", agentName, task)
	}
}

func TestRenderToolTreeItemSubAgentToolsExpandedAndCollapsedBranches(t *testing.T) {
	longSummary := strings.Repeat("S", 120)
	item := chatEntry{
		Kind:        "tool",
		Title:       toolEntryTitle("run_shell"),
		Status:      "queued",
		CompactBody: "run very long command with lots of detail to force truncation behavior",
		DetailLines: []string{
			"first detail",
			"   ",
			"second detail",
		},
		SubAgentTools: []SubAgentToolCall{
			{ToolName: "read_file", CompactBody: "a.go", Status: "done"},
			{ToolName: "search_text", CompactBody: "\"needle\"", Status: "done"},
			{ToolName: "run_shell", CompactBody: "go test ./...", Status: "running"},
			{ToolName: "list_files", Summary: longSummary, Status: "done"},
			{ToolName: "write_file", CompactBody: "b.go", Status: "done"},
			{ToolName: "replace_in_file", CompactBody: "c.go", Status: "done"},
		},
	}

	expanded := stripANSI(renderToolTreeItem(item, 48, true, true))
	for _, want := range []string{"first detail", "second detail", "+1 more", "run_shell: go test ./..."} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expected expanded render to contain %q, got %q", want, expanded)
		}
	}
	if strings.Contains(expanded, longSummary) {
		t.Fatalf("expected long summary to be truncated in expanded render, got %q", expanded)
	}

	collapsed := stripANSI(renderToolTreeItem(item, 48, false, true))
	if !strings.Contains(collapsed, "(ctrl+o to expand)") {
		t.Fatalf("expected collapsed render to include expand hint, got %q", collapsed)
	}
}

func TestSubAgentGroupingAndCollapsedToolHelpers(t *testing.T) {
	if isSubAgentGroup([]chatEntry{{AgentID: "explorer"}}) {
		t.Fatal("expected one-item group not to be treated as subagent aggregate")
	}
	if isSubAgentGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("delegate_subagent"), AgentID: "", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("delegate_subagent"), AgentID: "", Status: "done"},
	}) {
		t.Fatal("expected empty agent id group not to aggregate")
	}
	if isSubAgentGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("delegate_subagent"), AgentID: "explorer", Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("delegate_subagent"), AgentID: "review", Status: "done"},
	}) {
		t.Fatal("expected mixed agent ids not to aggregate")
	}
	if !isSubAgentGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("delegate_subagent"), AgentID: "explorer", Status: "running"},
		{Kind: "tool", Title: toolEntryTitle("delegate_subagent"), AgentID: "explorer", Status: "done"},
	}) {
		t.Fatal("expected same-agent delegate group to aggregate")
	}

	if got := renderSubAgentToolsCollapsed(nil, lipgloss.NewStyle(), "  "); got != "" {
		t.Fatalf("expected empty collapsed tools output, got %q", got)
	}
}

func TestRenderSubAgentBlockCoversFallbackDetailsAndNonDoneStates(t *testing.T) {
	longSummary := strings.Repeat("L", 120)
	expandedItem := chatEntry{
		Status:      "running",
		DetailLines: []string{"  ", "prompt line from details"},
		SubAgentTools: []SubAgentToolCall{
			{ToolName: "search_text", Summary: longSummary, Status: "done"},
		},
	}
	expanded := stripANSI(renderSubAgentBlock(expandedItem, "explorer", "", 64, true, true))
	if !strings.Contains(expanded, "prompt line from details") {
		t.Fatalf("expected detail fallback line in expanded block, got %q", expanded)
	}
	if strings.Contains(expanded, longSummary) {
		t.Fatalf("expected long subagent summary to truncate in expanded block, got %q", expanded)
	}

	doneWithTools := chatEntry{
		Status: "completed",
		SubAgentTools: []SubAgentToolCall{
			{ToolName: "run_shell", CompactBody: "go test ./...", Status: "done"},
		},
	}
	doneView := stripANSI(renderSubAgentBlock(doneWithTools, "review", "verify regressions", 100, false, true))
	if !strings.Contains(doneView, "(ctrl+o to expand)") {
		t.Fatalf("expected completed collapsed block with tools to include expand hint, got %q", doneView)
	}

	errorState := chatEntry{
		Status: "failed",
		SubAgentTools: []SubAgentToolCall{
			{ToolName: "read_file", CompactBody: "model.go", Status: "done"},
		},
	}
	errorView := stripANSI(renderSubAgentBlock(errorState, "review", "inspect failure", 100, false, true))
	if !strings.Contains(errorView, "read_file(model.go)") {
		t.Fatalf("expected failed state to show collapsed tool list, got %q", errorView)
	}
}

func TestLiveInspectAndStatusHelpersAdditionalBranches(t *testing.T) {
	fallbackSummary := summarizeLiveInspectGroup([]chatEntry{
		{Kind: "tool", Title: toolEntryTitle("run_shell"), Status: "done"},
		{Kind: "tool", Title: toolEntryTitle("apply_patch"), Status: "done"},
	})
	if fallbackSummary != "Running 2 tool calls" {
		t.Fatalf("expected generic summary fallback, got %q", fallbackSummary)
	}

	hint := latestLiveInspectHint([]chatEntry{
		{Kind: "tool", Status: "done", CompactBody: "older.go"},
		{Kind: "tool", Status: "running", CompactBody: "  ", Body: "  "},
	})
	if hint != "older.go" {
		t.Fatalf("expected hint fallback from non-running entry, got %q", hint)
	}

	if got := compactToolHint(chatEntry{CompactBody: " ", Body: " \n\nline from body"}); got != "line from body" {
		t.Fatalf("expected compactToolHint body fallback, got %q", got)
	}
	if got := compactToolHint(chatEntry{CompactBody: " ", Body: " "}); got != "" {
		t.Fatalf("expected compactToolHint empty fallback, got %q", got)
	}

	if got := aggregateToolGroupStatus([]chatEntry{{Status: "queued"}, {Status: "done"}}); got != "queued" {
		t.Fatalf("expected queued aggregate status, got %q", got)
	}

	if got := renderToolTag("", "queued"); got != "" {
		t.Fatalf("expected empty tool tag text to render empty string, got %q", got)
	}
	queuedTag := stripANSI(renderToolTag("queued", "queued"))
	if !strings.Contains(strings.ToLower(queuedTag), "queued") {
		t.Fatalf("expected queued tag text to be rendered, got %q", queuedTag)
	}

	queuedVisible := stripANSI(toolStatusIndicator("queued", true))
	queuedHidden := stripANSI(toolStatusIndicator("queued", false))
	if strings.TrimSpace(queuedVisible) == "" {
		t.Fatalf("expected queued indicator to be visible when blinking is on, got %q", queuedVisible)
	}
	if strings.TrimSpace(queuedHidden) != "" {
		t.Fatalf("expected queued indicator to be blank when blinking is off, got %q", queuedHidden)
	}

	thinking := stripANSI((model{stalled: true}).renderThinkingHeadline("thinking"))
	if !strings.Contains(thinking, "thinking") {
		t.Fatalf("expected stalled thinking headline text, got %q", thinking)
	}
}
