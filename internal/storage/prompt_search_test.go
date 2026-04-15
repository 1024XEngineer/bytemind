package storage

import "testing"

func TestParsePromptSearchQuery(t *testing.T) {
	parsed := ParsePromptSearchQuery("fix bug ws:repo-a sid:alpha")
	if len(parsed.Tokens) != 2 || parsed.Tokens[0] != "fix" || parsed.Tokens[1] != "bug" {
		t.Fatalf("unexpected tokens: %#v", parsed.Tokens)
	}
	if parsed.WorkspaceFilter != "repo-a" {
		t.Fatalf("unexpected workspace filter: %q", parsed.WorkspaceFilter)
	}
	if parsed.SessionFilter != "alpha" {
		t.Fatalf("unexpected session filter: %q", parsed.SessionFilter)
	}
}

func TestFilterPromptEntriesNewestFirstWithLimit(t *testing.T) {
	entries := []PromptEntry{
		{Workspace: "repo-a", SessionID: "alpha", Prompt: "first prompt"},
		{Workspace: "repo-a", SessionID: "alpha", Prompt: "second prompt"},
		{Workspace: "repo-a", SessionID: "alpha", Prompt: "third prompt"},
	}

	matches := FilterPromptEntries(entries, "", 2)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Prompt != "third prompt" || matches[1].Prompt != "second prompt" {
		t.Fatalf("unexpected order: %#v", matches)
	}
}

func TestFilterPromptEntriesAppliesScopeAndTokenFilters(t *testing.T) {
	entries := []PromptEntry{
		{Workspace: "repo-a", SessionID: "alpha", Prompt: "fix login bug"},
		{Workspace: "repo-a", SessionID: "beta", Prompt: "fix api bug"},
		{Workspace: "repo-b", SessionID: "alpha", Prompt: "fix login bug"},
		{Workspace: "repo-a", SessionID: "alpha", Prompt: "add tests"},
	}

	matches := FilterPromptEntries(entries, "fix ws:repo-a sid:alpha", 20)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Prompt != "fix login bug" {
		t.Fatalf("unexpected prompt: %q", matches[0].Prompt)
	}
}

func TestFilterPromptEntriesSkipsEmptyPrompt(t *testing.T) {
	entries := []PromptEntry{
		{Workspace: "repo-a", SessionID: "alpha", Prompt: "   "},
		{Workspace: "repo-a", SessionID: "alpha", Prompt: "valid prompt"},
	}

	matches := FilterPromptEntries(entries, "", 10)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Prompt != "valid prompt" {
		t.Fatalf("unexpected prompt: %q", matches[0].Prompt)
	}
}
