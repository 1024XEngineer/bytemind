package tui

import (
	"strings"
	"testing"

	"bytemind/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleMouseScrollsViewport(t *testing.T) {
	m := model{
		screen: screenChat,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			return vp
		}(),
	}

	got, _ := m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	updated := got.(model)
	if updated.viewport.YOffset == 0 {
		t.Fatalf("expected viewport to scroll down, got offset %d", updated.viewport.YOffset)
	}
}

func TestHelpTextOnlyMentionsSupportedEntryPoints(t *testing.T) {
	text := model{}.helpText()

	for _, unwanted := range []string{
		"scripts\\install.ps1",
		"aicoding chat",
		"aicoding run",
		"当前版本还没实现",
	} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("help text should not mention %q", unwanted)
		}
	}

	for _, wanted := range []string{
		"go run ./cmd/bytemind chat",
		"go run ./cmd/bytemind run -prompt",
		"/quit",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("help text should mention %q", wanted)
		}
	}
}

func TestRenderFooterDoesNotAdvertiseHistory(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 120,
		input: input,
	}

	footer := m.renderFooter()
	if strings.Contains(footer, "Up/Down history") {
		t.Fatalf("footer should not advertise history navigation")
	}
	if !strings.Contains(footer, "? help") {
		t.Fatalf("footer should advertise help shortcut")
	}
}

func TestCommandPaletteListsQuitCommand(t *testing.T) {
	found := false
	for _, item := range commandItems {
		if item.Name == "/quit" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected command palette to include /quit")
	}
}

func TestCommandPaletteDoesNotListExitAlias(t *testing.T) {
	for _, item := range commandItems {
		if item.Name == "/exit" {
			t.Fatalf("did not expect command palette to include /exit")
		}
	}
}

func TestSessionTextShowsSessionDetails(t *testing.T) {
	sess := session.New("E:\\bytemind")

	m := model{sess: sess}
	text := m.sessionText()

	for _, want := range []string{"Session ID:", "Workspace:", "Updated:", "Messages:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected session text to contain %q", want)
		}
	}
}

func TestPlanTextShowsSavedPlanItems(t *testing.T) {
	m := model{
		plan: []session.PlanItem{
			{Step: "Inspect current TUI behavior", Status: "completed"},
			{Step: "Align visible features with code", Status: "in_progress"},
		},
	}

	text := m.planText()
	for _, want := range []string{
		"Current plan (2 step(s)):",
		"1. [completed] Inspect current TUI behavior",
		"2. [in_progress] Align visible features with code",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected plan text to contain %q", want)
		}
	}
}

func TestHelpTextDoesNotMentionSidebar(t *testing.T) {
	text := model{}.helpText()
	if strings.Contains(text, "右侧状态栏") {
		t.Fatalf("help text should not mention sidebar")
	}
	if !strings.Contains(text, "主界面只显示用户消息和助手回复") {
		t.Fatalf("help text should describe the actual single-panel chat layout")
	}
	if strings.Contains(text, "/exit") {
		t.Fatalf("help text should not mention /exit")
	}
	if !strings.Contains(text, "/quit: 退出 TUI。") {
		t.Fatalf("help text should mention /quit as the only exit command")
	}
}

func TestAssistantChatBubbleUsesFullAvailableWidth(t *testing.T) {
	width := 80
	assistantWidth := chatBubbleWidth(chatEntry{Kind: "assistant"}, width)
	if assistantWidth != width {
		t.Fatalf("expected assistant bubble width %d, got %d", width, assistantWidth)
	}

	userWidth := chatBubbleWidth(chatEntry{Kind: "user"}, width)
	if userWidth >= width {
		t.Fatalf("expected user bubble to stay narrower than the full width, got %d", userWidth)
	}
}

func TestApprovalBannerRendersAboveInput(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 120,
		input: input,
		approval: &approvalPrompt{
			Command: "go test ./internal/tui",
			Reason:  "run tests",
		},
	}

	footer := m.renderFooter()
	for _, want := range []string{
		"需要你的确认",
		"原因: run tests",
		"go test ./internal/tui",
		"Y / Enter 同意    N / Esc 拒绝",
	} {
		if !strings.Contains(footer, want) {
			t.Fatalf("expected approval banner to contain %q", want)
		}
	}
	if strings.Contains(footer, "审批请求") {
		t.Fatalf("did not expect old centered approval modal title in footer")
	}
}

func TestFormatChatBodyPreservesExplicitBlankLines(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "第一段\n\n第二段",
	}

	got := formatChatBody(item, 80)
	if !strings.Contains(got, "\n\n第二段") {
		t.Fatalf("expected explicit blank line to be preserved, got %q", got)
	}
}

func TestFormatChatBodySeparatesParagraphAndList(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "这里是说明\n- 第一项\n- 第二项",
	}

	got := formatChatBody(item, 80)
	if !strings.Contains(got, "这里是说明\n\n- 第一项") {
		t.Fatalf("expected list to be separated from paragraph, got %q", got)
	}
}

func TestFormatChatBodyRendersMarkdownHeadingWithoutHashes(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "# 一级标题\n正文",
	}

	got := formatChatBody(item, 80)
	if strings.Contains(got, "# 一级标题") {
		t.Fatalf("expected heading marker to be stripped, got %q", got)
	}
	if !strings.Contains(got, "一级标题") {
		t.Fatalf("expected heading text to remain, got %q", got)
	}
}

func TestFormatChatBodyRendersCodeBlockWithoutFences(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "```go\nfmt.Println(\"hi\")\n```",
	}

	got := formatChatBody(item, 80)
	if strings.Contains(got, "```") {
		t.Fatalf("expected code fences to be removed, got %q", got)
	}
	if !strings.Contains(got, "fmt.Println(\"hi\")") {
		t.Fatalf("expected code contents to remain, got %q", got)
	}
}
