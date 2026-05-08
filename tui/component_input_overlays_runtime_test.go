package tui

import (
	"testing"

	"github.com/1024XEngineer/bytemind/internal/mention"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func TestShouldExecuteTypedSlashFromPaletteBranches(t *testing.T) {
	selected := commandItem{Name: "/agents", Usage: "/agents"}

	if shouldExecuteTypedSlashFromPalette(selected, "") {
		t.Fatal("expected empty typed value not to execute")
	}
	if shouldExecuteTypedSlashFromPalette(selected, "agents now") {
		t.Fatal("expected non-slash typed value not to execute")
	}
	if shouldExecuteTypedSlashFromPalette(commandItem{Name: "/agents", Usage: ""}, "/agents run") {
		t.Fatal("expected empty usage not to execute")
	}
	if shouldExecuteTypedSlashFromPalette(selected, "/agents") {
		t.Fatal("expected exact usage not to trigger typed execution")
	}
	if shouldExecuteTypedSlashFromPalette(selected, "/agents") {
		t.Fatal("expected exact name not to trigger typed execution")
	}
	if !shouldExecuteTypedSlashFromPalette(selected, "/agents scan runner") {
		t.Fatal("expected slash command with args to execute from palette")
	}
	if shouldExecuteTypedSlashFromPalette(selected, "/help") {
		t.Fatal("expected unrelated slash command not to execute selected command")
	}
}

func TestHandleMentionPaletteKeyPageNavigation(t *testing.T) {
	results := make([]mention.Candidate, 0, 10)
	for i := 0; i < 10; i++ {
		results = append(results, mention.Candidate{Path: "file", BaseName: "file"})
	}
	m := model{
		height:         10,
		mentionOpen:    true,
		mentionCursor:  5,
		mentionResults: results,
	}

	got, _ := m.handleMentionPaletteKey(tea.KeyMsg{Type: tea.KeyPgUp})
	up := got.(model)
	if up.mentionCursor != 0 {
		t.Fatalf("expected PgUp to move mention cursor to 0, got %d", up.mentionCursor)
	}

	got, _ = up.handleMentionPaletteKey(tea.KeyMsg{Type: tea.KeyPgDown})
	down := got.(model)
	if down.mentionCursor != 7 {
		t.Fatalf("expected PgDown to advance by mention page size, got %d", down.mentionCursor)
	}
}

func TestApplyMentionSelectionAgentBranch(t *testing.T) {
	input := textarea.New()
	input.SetValue("please @exp")
	token, ok := mention.FindActiveToken(input.Value())
	if !ok {
		t.Fatal("expected active mention token")
	}

	m := model{
		input:         input,
		mentionOpen:   true,
		mentionToken:  token,
		mentionCursor: 3,
		mentionResults: []mention.Candidate{
			{Path: "explorer", BaseName: "explorer", Kind: "agent", Description: "scan"},
		},
	}

	m.applyMentionSelection(m.mentionResults[0])
	if m.mentionOpen {
		t.Fatal("expected mention palette to close after selection")
	}
	if got := m.input.Value(); got != "please @explorer " {
		t.Fatalf("expected agent mention to be inserted, got %q", got)
	}
	if m.statusNote != "Inserted agent mention: @explorer" {
		t.Fatalf("expected agent insert status note, got %q", m.statusNote)
	}
}

func TestOpenCommandPaletteClosesModelPickerAndSeedsSlash(t *testing.T) {
	input := textarea.New()
	m := model{
		input:      input,
		modelsOpen: true,
		skillsOpen: true,
	}

	m.openCommandPalette()
	if !m.commandOpen {
		t.Fatal("expected command palette to be open")
	}
	if m.modelsOpen {
		t.Fatal("expected openCommandPalette to close model picker")
	}
	if m.skillsOpen {
		t.Fatal("expected openCommandPalette to close skills picker")
	}
	if m.input.Value() != "/" {
		t.Fatalf("expected command palette to seed slash input, got %q", m.input.Value())
	}
}
