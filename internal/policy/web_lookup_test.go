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

func TestEvaluateWebLookupRequirementRequiresWebForModelReality(t *testing.T) {
	got := EvaluateWebLookupRequirement("gpt-5.4-mini 是否存在？")
	if got.Requirement != WebLookupRequirementMust {
		t.Fatalf("expected must web lookup requirement, got %#v", got)
	}
	if got.Instruction == "" {
		t.Fatalf("expected web lookup instruction, got %#v", got)
	}
}

func TestEvaluateWebLookupRequirementRequiresWebForURL(t *testing.T) {
	got := EvaluateWebLookupRequirement("https://example.com 这个页面有没有不合理的地方")
	if got.Requirement != WebLookupRequirementMust {
		t.Fatalf("expected must web lookup requirement for URL, got %#v", got)
	}
}

func TestExplicitWebLookupInstructionSupportsChineseSignals(t *testing.T) {
	got := ExplicitWebLookupInstruction("请联网查一下这个功能的源码")
	if !strings.Contains(got, "web_search/web_fetch") {
		t.Fatalf("expected web lookup instruction for chinese signal, got %q", got)
	}
}

func TestExplicitWebLookupInstructionReturnsEmptyForLocalRepoLanguage(t *testing.T) {
	got := ExplicitWebLookupInstruction("inspect repo")
	if got != "" {
		t.Fatalf("expected empty instruction for local repo wording, got %q", got)
	}
}

func TestExplicitWebLookupInstructionReturnsEmptyWhenLocalOnly(t *testing.T) {
	got := ExplicitWebLookupInstruction("Use search_text in current workspace")
	if got != "" {
		t.Fatalf("expected empty instruction, got %q", got)
	}
}

func TestEvaluateWebLookupRequirementReturnsNoneForLocalOnly(t *testing.T) {
	got := EvaluateWebLookupRequirement("Use search_text in current workspace")
	if got.Requirement != WebLookupRequirementNone {
		t.Fatalf("expected no web lookup requirement, got %#v", got)
	}
}
