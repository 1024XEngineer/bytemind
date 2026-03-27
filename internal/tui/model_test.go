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

func TestCommandPaletteDoesNotListPlanCommands(t *testing.T) {
	for _, item := range visibleCommandItems() {
		if strings.HasPrefix(item.Name, "/plan") {
			t.Fatalf("did not expect command palette to include %q", item.Name)
		}
	}
}

func LegacyTestHelpTextDoesNotMentionSidebar(t *testing.T) {
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
