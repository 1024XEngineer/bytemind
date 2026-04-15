package policy

import (
	"strings"
	"testing"
)

func TestExplicitWebLookupInstructionReturnsHintForSourceLookup(t *testing.T) {
	got := ExplicitWebLookupInstruction("Find implementation in GitHub repository")
	if !strings.Contains(got, "web_search/web_fetch") {
		t.Fatalf("expected web lookup instruction, got %q", got)
	}
}

func TestExplicitWebLookupInstructionSupportsChineseSignals(t *testing.T) {
	got := ExplicitWebLookupInstruction("请联网查一下这个功能的源码")
	if !strings.Contains(got, "web_search/web_fetch") {
		t.Fatalf("expected web lookup instruction for chinese signal, got %q", got)
	}
}

func TestExplicitWebLookupInstructionReturnsEmptyWhenLocalOnly(t *testing.T) {
	got := ExplicitWebLookupInstruction("Use search_text in current workspace")
	if got != "" {
		t.Fatalf("expected empty instruction, got %q", got)
	}
}
