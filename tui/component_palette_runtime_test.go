package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/mention"
	"github.com/charmbracelet/bubbles/textarea"
)

type staticAgentSource struct {
	entries []mention.AgentEntry
}

func (s staticAgentSource) ListAgents() []mention.AgentEntry {
	return append([]mention.AgentEntry(nil), s.entries...)
}

func TestSyncMentionPaletteMergesAgentCandidatesAndTruncates(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mo")

	candidates := make([]mention.Candidate, 0, mentionSearchLimit+10)
	for i := 0; i < mentionSearchLimit+10; i++ {
		path := fmt.Sprintf("module/path_%02d.go", i)
		candidates = append(candidates, mention.Candidate{Path: path, BaseName: fmt.Sprintf("path_%02d.go", i)})
	}

	m := model{
		input:         input,
		mentionCursor: 99,
		mentionIndex:  mention.NewStaticWorkspaceFileIndex(candidates, 0, false),
		agentSource: staticAgentSource{
			entries: []mention.AgentEntry{
				{Name: "modeler", Description: "agent helper"},
				{Name: "zzz", Description: "should not match query"},
			},
		},
	}

	m.syncMentionPalette()
	if !m.mentionOpen {
		t.Fatal("expected mention palette to open for active @ token")
	}
	if m.mentionQuery != "mo" {
		t.Fatalf("expected mention query mo, got %q", m.mentionQuery)
	}
	if len(m.mentionResults) != mentionSearchLimit {
		t.Fatalf("expected results to be clamped to mentionSearchLimit=%d, got %d", mentionSearchLimit, len(m.mentionResults))
	}
	if m.mentionCursor != 0 {
		t.Fatalf("expected out-of-range mention cursor to reset to 0, got %d", m.mentionCursor)
	}

	foundAgent := false
	for _, item := range m.mentionResults {
		if item.Kind == "agent" && item.Path == "modeler" {
			foundAgent = true
		}
		if item.Path == "zzz" {
			t.Fatalf("expected non-matching agent candidate to be filtered out, got %+v", item)
		}
	}
	if !foundAgent {
		t.Fatalf("expected matching agent candidate to be merged into results, got %+v", m.mentionResults)
	}
}

func TestSyncMentionPaletteNoResultsResetsCursor(t *testing.T) {
	input := textarea.New()
	input.SetValue("@nomatch")

	m := model{
		input:         input,
		mentionCursor: 7,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "tui/model.go", BaseName: "model.go"},
		}, 0, false),
		agentSource: staticAgentSource{
			entries: []mention.AgentEntry{{Name: "reviewer"}},
		},
	}

	m.syncMentionPalette()
	if !m.mentionOpen {
		t.Fatal("expected mention palette to remain open with active token even when no results")
	}
	if len(m.mentionResults) != 0 {
		t.Fatalf("expected no mention matches, got %+v", m.mentionResults)
	}
	if m.mentionCursor != 0 {
		t.Fatalf("expected mention cursor reset to 0 for empty result set, got %d", m.mentionCursor)
	}
}

func TestAgentMatchScoreAndMatchesQueryBranches(t *testing.T) {
	tests := []struct {
		query string
		name  string
		want  float64
	}{
		{query: "", name: "explorer", want: -1},
		{query: "explorer", name: "explorer", want: 0.0},
		{query: "exp", name: "explorer", want: 0.05},
		{query: "plor", name: "explorer", want: 0.15},
		{query: "epr", name: "explorer", want: 0.3},
		{query: "zzz", name: "explorer", want: -1},
	}

	for _, tc := range tests {
		got := agentMatchScore(strings.ToLower(tc.query), strings.ToLower(tc.name))
		if got != tc.want {
			t.Fatalf("agentMatchScore(%q, %q) = %v, want %v", tc.query, tc.name, got, tc.want)
		}
	}

	if !matchesQuery("Explorer", "exp") {
		t.Fatal("expected matchesQuery to use case-insensitive matching")
	}
	if matchesQuery("Explorer", "zzz") {
		t.Fatal("expected non-matching query to return false")
	}
}
