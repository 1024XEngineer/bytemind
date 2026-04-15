package app

import "testing"

func TestCompleteSlashCommand(t *testing.T) {
	commands := DefaultSlashCommands()
	completed, suggestions := CompleteSlashCommand("/he", commands)
	if len(suggestions) != 0 {
		t.Fatalf("expected unique completion, got suggestions %#v", suggestions)
	}
	if completed != "/help" {
		t.Fatalf("expected /help, got %q", completed)
	}
}

func TestCompleteSlashCommandReturnsSuggestionsForAmbiguousPrefix(t *testing.T) {
	commands := DefaultSlashCommands()
	completed, suggestions := CompleteSlashCommand("/sess", commands)
	if completed != "/sess" {
		t.Fatalf("expected input to remain unchanged, got %q", completed)
	}
	if len(suggestions) != 2 || suggestions[0] != "/session" || suggestions[1] != "/sessions" {
		t.Fatalf("unexpected suggestions: %#v", suggestions)
	}
}

func TestCommandNames(t *testing.T) {
	commands := DefaultSlashCommands()
	names := CommandNames(commands)
	if len(names) != len(commands) {
		t.Fatalf("expected %d names, got %d", len(commands), len(names))
	}
	if names[0] != "/help" || names[len(names)-1] != "/quit" {
		t.Fatalf("unexpected command names: %#v", names)
	}
}
