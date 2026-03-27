package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/session"
	"bytemind/internal/skills"

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

func TestHelpTextMentionsSkillCommands(t *testing.T) {
	text := model{}.helpText()
	for _, want := range []string{"/skill-author", "/skills", "/clear-skill", "-skill <name>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %q", want)
		}
	}
}

func TestCommandPaletteListsSkillAuthorCommand(t *testing.T) {
	found := false
	for _, item := range commandItems {
		if item.Name == "/skill-author" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected command palette to include /skill-author")
	}
}

func TestAvailableCommandItemsDoNotShowSkillsByDefault(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("name: review\ndescription: review code\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := skills.NewManager(workspace)
	if err := manager.Load(); err != nil {
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

	m := newModel(Options{
		Runner:    agent.NewRunner(agent.Options{Skills: manager}),
		Store:     store,
		Session:   sess,
		Config:    config.Default(workspace),
		Workspace: workspace,
	})

	m.setInputValue("/")
	for _, item := range m.availableCommandItems() {
		if item.Name == "/review" {
			t.Fatal("did not expect workspace skill in default available commands")
		}
	}
}

func TestFilteredCommandsShowMatchingSkillsByPrefix(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("name: review\ndescription: review code\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := skills.NewManager(workspace)
	if err := manager.Load(); err != nil {
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

	m := newModel(Options{
		Runner:    agent.NewRunner(agent.Options{Skills: manager}),
		Store:     store,
		Session:   sess,
		Config:    config.Default(workspace),
		Workspace: workspace,
	})

	m.setInputValue("/re")
	found := false
	for _, item := range m.filteredCommands() {
		if item.Name == "/review" {
			found = true
			if !strings.Contains(item.Description, "使用技能") {
				t.Fatalf("expected chinese skill description, got %q", item.Description)
			}
		}
	}
	if !found {
		t.Fatal("expected matching skill to appear for prefixed input")
	}
}

func TestHandleSlashCommandActivatesSkill(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("name: review\ndescription: review code\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := skills.NewManager(workspace)
	if err := manager.Load(); err != nil {
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

	m := newModel(Options{
		Runner:    agent.NewRunner(agent.Options{Skills: manager}),
		Store:     store,
		Session:   sess,
		Config:    config.Default(workspace),
		Workspace: workspace,
	})

	if err := m.handleSlashCommand("/review"); err != nil {
		t.Fatal(err)
	}
	if m.sess.ActiveSkill != "review" {
		t.Fatalf("expected active skill to be review, got %q", m.sess.ActiveSkill)
	}
}

func TestHandleSlashCommandActivatesBuiltinSkillAuthor(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	m := newModel(Options{
		Runner:    agent.NewRunner(agent.Options{}),
		Store:     store,
		Session:   sess,
		Config:    config.Default(workspace),
		Workspace: workspace,
	})

	if err := m.handleSlashCommand("/skill-author"); err != nil {
		t.Fatal(err)
	}
	if m.sess.ActiveSkill != skills.BuiltinSkillAuthorName {
		t.Fatalf("expected active skill to be %q, got %q", skills.BuiltinSkillAuthorName, m.sess.ActiveSkill)
	}
}

func TestRunnerSkillReloadsNewlyCreatedProjectSkill(t *testing.T) {
	workspace := t.TempDir()
	manager := skills.NewManager(workspace)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}

	runner := agent.NewRunner(agent.Options{Skills: manager})
	if runner.Skill("review") != nil {
		t.Fatal("did not expect review skill before file creation")
	}

	skillDir := filepath.Join(workspace, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("name: review\ndescription: review code\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if runner.Skill("review") == nil {
		t.Fatal("expected runner to reload and resolve newly created skill")
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
