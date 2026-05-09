package agent

import (
	"strings"
	"testing"
)

func TestExtractAgentMentionsFiltersUnknownAndDeduplicates(t *testing.T) {
	known := map[string]struct{}{
		"explorer":   {},
		"review-bot": {},
	}
	input := "Please ask @explorer to scan, then @review-bot to summarize, and @explorer again."

	got := extractAgentMentions(input, known)
	if len(got) != 2 {
		t.Fatalf("expected 2 known unique mentions, got %#v", got)
	}
	if got[0].Name != "explorer" || got[1].Name != "review-bot" {
		t.Fatalf("expected mention order to be preserved, got %#v", got)
	}
}

func TestBuildAgentMentionReminderIncludesDescriptionsAndFallback(t *testing.T) {
	if got := buildAgentMentionReminder(nil, map[string]string{}); got != "" {
		t.Fatalf("expected empty reminder for no mentions, got %q", got)
	}

	reminder := buildAgentMentionReminder(
		[]AgentMention{{Name: "explorer"}, {Name: "review"}},
		map[string]string{"explorer": "Read-only codebase exploration."},
	)

	for _, want := range []string{
		"The user has expressed a desire to invoke the following agent(s):",
		"- explorer: Read-only codebase exploration.",
		"- review",
		"Use the delegate_subagent tool if appropriate",
	} {
		if !strings.Contains(reminder, want) {
			t.Fatalf("expected reminder to contain %q, got %q", want, reminder)
		}
	}
}

func TestEnhanceUserMessageWithAgentMentionsWrapsReminderOnlyWhenMentioned(t *testing.T) {
	known := map[string]struct{}{"explorer": {}}
	descs := map[string]string{"explorer": "Scan relevant files and summarize findings."}

	original := "Please proceed without delegation."
	if got := enhanceUserMessageWithAgentMentions(original, known, descs); got != original {
		t.Fatalf("expected untouched message without mentions, got %q", got)
	}

	withMention := "Can @explorer inspect the failure path?"
	enhanced := enhanceUserMessageWithAgentMentions(withMention, known, descs)
	for _, want := range []string{
		withMention,
		"<system-reminder>",
		"- explorer: Scan relevant files and summarize findings.",
		"</system-reminder>",
	} {
		if !strings.Contains(enhanced, want) {
			t.Fatalf("expected enhanced message to contain %q, got %q", want, enhanced)
		}
	}
}
