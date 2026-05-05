package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestClassifyBudgetBoundaries(t *testing.T) {
	warning := config.DefaultContextBudgetWarningRatio
	critical := config.DefaultContextBudgetCriticalRatio

	tests := []struct {
		name     string
		usage    float64
		expected budgetLevel
	}{
		{name: "84.99 percent", usage: 0.8499, expected: budgetNone},
		{name: "85 percent", usage: 0.85, expected: budgetWarning},
		{name: "94.99 percent", usage: 0.9499, expected: budgetWarning},
		{name: "95 percent", usage: 0.95, expected: budgetCritical},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyBudget(tc.usage, warning, critical)
			if got != tc.expected {
				t.Fatalf("unexpected budget level: got=%q want=%q", got, tc.expected)
			}
		})
	}
}

func containsToolUseID(messages []llm.Message, toolUseID string) bool {
	for i := range messages {
		message := messages[i]
		message.Normalize()
		for _, part := range message.Parts {
			if part.Type != llm.PartToolUse || part.ToolUse == nil {
				continue
			}
			if part.ToolUse.ID == toolUseID {
				return true
			}
		}
	}
	return false
}

// --- truncateRunes ---

func TestTruncateRunesLimitZero(t *testing.T) {
	if got := truncateRunes("hello", 0); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := truncateRunes("hello", -1); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestTruncateRunesEmptyValue(t *testing.T) {
	if got := truncateRunes("", 10); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := truncateRunes("   ", 10); got != "" {
		t.Fatalf("expected empty for whitespace, got %q", got)
	}
}

func TestTruncateRunesUnderLimit(t *testing.T) {
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Fatalf("expected \"hello\", got %q", got)
	}
}

func TestTruncateRunesExactLimit(t *testing.T) {
	if got := truncateRunes("hello", 5); got != "hello" {
		t.Fatalf("expected \"hello\", got %q", got)
	}
}

func TestTruncateRunesOverLimitWithEllipsis(t *testing.T) {
	got := truncateRunes("hello world", 8)
	if got != "hello..." {
		t.Fatalf("expected \"hello...\", got %q", got)
	}
}

func TestTruncateRunesLimitThreeOrLess(t *testing.T) {
	got := truncateRunes("hello", 3)
	if got != "hel" {
		t.Fatalf("expected \"hel\", got %q", got)
	}
	got = truncateRunes("hello", 2)
	if got != "he" {
		t.Fatalf("expected \"he\", got %q", got)
	}
	got = truncateRunes("hello", 1)
	if got != "h" {
		t.Fatalf("expected \"h\", got %q", got)
	}
}

func TestTruncateRunesPreservesUTF8(t *testing.T) {
	got := truncateRunes("你好世界测试", 4)
	// limit=4, ellipsis takes 3 chars, so 1 rune + "..."
	if got != "你..." {
		t.Fatalf("expected \"你...\", got %q", got)
	}
}

// --- cloneMessages ---

func TestCloneMessagesNil(t *testing.T) {
	if got := cloneMessages(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestCloneMessagesEmpty(t *testing.T) {
	if got := cloneMessages([]llm.Message{}); got != nil {
		t.Fatalf("expected nil for empty slice, got %v", got)
	}
}

func TestCloneMessagesNonEmpty(t *testing.T) {
	original := []llm.Message{
		llm.NewUserTextMessage("hello"),
		llm.NewAssistantTextMessage("world"),
	}
	cloned := cloneMessages(original)
	if len(cloned) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(cloned))
	}
	// Mutating clone should not affect original
	cloned[0].Role = "mutated"
	if original[0].Role == "mutated" {
		t.Fatalf("clone should not alias original")
	}
}

// --- isHumanUserMessage ---

func TestIsHumanUserMessageWithText(t *testing.T) {
	msg := llm.NewUserTextMessage("hello")
	if !isHumanUserMessage(msg) {
		t.Fatalf("expected user text message to be human")
	}
}

func TestIsHumanUserMessageWithImage(t *testing.T) {
	msg := llm.Message{Role: llm.RoleUser, Parts: []llm.Part{{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: "img-1"}}}}
	if !isHumanUserMessage(msg) {
		t.Fatalf("expected user image message to be human")
	}
}

func TestIsHumanUserMessageWithOnlyToolResult(t *testing.T) {
	msg := llm.NewToolResultMessage("call-1", "result")
	if isHumanUserMessage(msg) {
		t.Fatalf("expected user message with only tool_result to not be human")
	}
}

func TestIsHumanUserMessageAssistantRole(t *testing.T) {
	msg := llm.NewAssistantTextMessage("hello")
	if isHumanUserMessage(msg) {
		t.Fatalf("expected assistant message to not be human")
	}
}

func TestIsHumanUserMessageSystemRole(t *testing.T) {
	msg := llm.NewTextMessage(llm.RoleSystem, "system prompt")
	if isHumanUserMessage(msg) {
		t.Fatalf("expected system message to not be human")
	}
}

// --- firstUserGoal ---

func TestFirstUserGoalEmptyMessages(t *testing.T) {
	if got := firstUserGoal(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := firstUserGoal([]llm.Message{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFirstUserGoalFindsFirstUserText(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleSystem, "system"),
		llm.NewUserTextMessage("fix the bug"),
		llm.NewAssistantTextMessage("ok"),
	}
	if got := firstUserGoal(messages); got != "fix the bug" {
		t.Fatalf("expected \"fix the bug\", got %q", got)
	}
}

func TestFirstUserGoalSkipsAssistantMessages(t *testing.T) {
	messages := []llm.Message{
		llm.NewAssistantTextMessage("hello"),
		llm.NewUserTextMessage("do something"),
	}
	if got := firstUserGoal(messages); got != "do something" {
		t.Fatalf("expected \"do something\", got %q", got)
	}
}

func TestFirstUserGoalNoUserMessages(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleSystem, "system"),
		llm.NewAssistantTextMessage("hello"),
	}
	if got := firstUserGoal(messages); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// --- compactForCompaction ---

func TestCompactForCollapsesWhitespace(t *testing.T) {
	got := compactForCompaction("  hello   world  \n  foo  ")
	if got != "hello world foo" {
		t.Fatalf("expected \"hello world foo\", got %q", got)
	}
}

func TestCompactForEmpty(t *testing.T) {
	if got := compactForCompaction(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := compactForCompaction("   "); got != "" {
		t.Fatalf("expected empty for whitespace, got %q", got)
	}
}

// --- formatCompactionMessage ---

func TestFormatCompactionMessageTextPart(t *testing.T) {
	msg := llm.NewUserTextMessage("hello world")
	got := formatCompactionMessage(1, msg)
	if got == "" {
		t.Fatalf("expected non-empty output")
	}
	if got != "001 user: hello world" {
		t.Fatalf("unexpected format: %q", got)
	}
}

func TestFormatCompactionMessageMultipleParts(t *testing.T) {
	msg := llm.Message{
		Role: llm.RoleAssistant,
		Parts: []llm.Part{
			{Type: llm.PartText, Text: &llm.TextPart{Value: "thinking about it"}},
			{Type: llm.PartToolUse, ToolUse: &llm.ToolUsePart{ID: "call-1", Name: "read_file", Arguments: `{"path":"main.go"}`}},
		},
	}
	got := formatCompactionMessage(2, msg)
	if got == "" {
		t.Fatalf("expected non-empty output")
	}
	if !strings.Contains(got, "002 assistant:") {
		t.Fatalf("expected prefix, got %q", got)
	}
	if !strings.Contains(got, "tool_use") {
		t.Fatalf("expected tool_use in output, got %q", got)
	}
}

func TestFormatCompactionMessageToolResult(t *testing.T) {
	msg := llm.NewToolResultMessage("call-1", "file contents here")
	got := formatCompactionMessage(3, msg)
	if got == "" {
		t.Fatalf("expected non-empty output")
	}
	if !strings.Contains(got, "tool_result") {
		t.Fatalf("expected tool_result in output, got %q", got)
	}
}

func TestFormatCompactionMessageEmptyParts(t *testing.T) {
	msg := llm.Message{Role: llm.RoleUser, Parts: nil}
	msg.Normalize()
	got := formatCompactionMessage(4, msg)
	if got != "" {
		t.Fatalf("expected empty for message with no parts, got %q", got)
	}
}

func TestFormatCompactionMessageThinkingPart(t *testing.T) {
	msg := llm.Message{
		Role: llm.RoleAssistant,
		Parts: []llm.Part{
			{Type: llm.PartThinking, Thinking: &llm.ThinkingPart{Value: "reasoning step"}},
		},
	}
	got := formatCompactionMessage(5, msg)
	if !strings.Contains(got, "thinking: reasoning step") {
		t.Fatalf("expected thinking prefix, got %q", got)
	}
}

func TestFormatCompactionMessageImageRef(t *testing.T) {
	msg := llm.Message{
		Role: llm.RoleUser,
		Parts: []llm.Part{
			{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: "asset-42"}},
		},
	}
	got := formatCompactionMessage(6, msg)
	if !strings.Contains(got, "image_ref asset-42") {
		t.Fatalf("expected image_ref in output, got %q", got)
	}
}

// --- buildCompactionTranscript ---

func TestBuildCompactionTranscriptEmpty(t *testing.T) {
	if got := buildCompactionTranscript(nil, 1000); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := buildCompactionTranscript([]llm.Message{}, 1000); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestBuildCompactionTranscriptLimitZero(t *testing.T) {
	messages := []llm.Message{llm.NewUserTextMessage("hello")}
	if got := buildCompactionTranscript(messages, 0); got != "" {
		t.Fatalf("expected empty for limit=0, got %q", got)
	}
}

func TestBuildCompactionTranscriptSingleMessage(t *testing.T) {
	messages := []llm.Message{llm.NewUserTextMessage("hello")}
	got := buildCompactionTranscript(messages, 1000)
	if got != "001 user: hello" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestBuildCompactionTranscriptMultipleMessagesWithSeparators(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage("hello"),
		llm.NewAssistantTextMessage("world"),
	}
	got := buildCompactionTranscript(messages, 1000)
	if !strings.Contains(got, "\n\n") {
		t.Fatalf("expected double newline separator between messages, got %q", got)
	}
}

func TestBuildCompactionTranscriptTruncation(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage(strings.Repeat("a", 500)),
		llm.NewAssistantTextMessage(strings.Repeat("b", 500)),
		llm.NewUserTextMessage(strings.Repeat("c", 500)),
	}
	got := buildCompactionTranscript(messages, 600)
	if !strings.Contains(got, "[...older details omitted...]") {
		t.Fatalf("expected omission marker, got %q", got)
	}
}

func TestBuildCompactionTranscriptSkipsEmptyMessages(t *testing.T) {
	messages := []llm.Message{
		llm.NewUserTextMessage("hello"),
		{Role: llm.RoleUser, Parts: nil}, // empty message after normalize
		llm.NewAssistantTextMessage("world"),
	}
	got := buildCompactionTranscript(messages, 1000)
	if strings.Contains(got, "002") {
		t.Fatalf("expected empty message to be skipped, got %q", got)
	}
}

// --- fallbackCompactionSummary ---

func TestFallbackCompactionSummaryEmptyHistory(t *testing.T) {
	if got := fallbackCompactionSummary(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFallbackCompactionSummaryWithGoal(t *testing.T) {
	history := []llm.Message{
		llm.NewUserTextMessage("fix the login bug"),
		llm.NewAssistantTextMessage("I'll look into it"),
	}
	got := fallbackCompactionSummary(history)
	if !strings.Contains(got, "fix the login bug") {
		t.Fatalf("expected goal in summary, got %q", got)
	}
	if !strings.Contains(got, "Compaction fallback summary") {
		t.Fatalf("expected fallback header, got %q", got)
	}
}

func TestFallbackCompactionSummaryNoGoal(t *testing.T) {
	history := []llm.Message{
		llm.NewAssistantTextMessage("hello"),
	}
	got := fallbackCompactionSummary(history)
	if !strings.Contains(got, "(unknown)") {
		t.Fatalf("expected (unknown) goal, got %q", got)
	}
}

func TestFallbackCompactionSummaryLimitsRecentMessages(t *testing.T) {
	history := make([]llm.Message, 10)
	for i := range history {
		history[i] = llm.NewUserTextMessage(fmt.Sprintf("message %d", i))
	}
	got := fallbackCompactionSummary(history)
	if !strings.Contains(got, "message 9") {
		t.Fatalf("expected most recent message in summary")
	}
	// The goal comes from firstUserGoal which finds "message 0",
	// but the recent context section (last 6) should not include messages 0-3.
	// "message 0" appears only as the goal, not in the "- message N:" lines.
	if !strings.Contains(got, "User goal: message 0") {
		t.Fatalf("expected first message as goal, got %q", got)
	}
}

// --- contextBudgetRatios ---

func TestContextBudgetRatiosDefaults(t *testing.T) {
	r := &Runner{config: config.Config{}}
	warning, critical := r.contextBudgetRatios()
	if warning != config.DefaultContextBudgetWarningRatio {
		t.Fatalf("expected default warning ratio, got %f", warning)
	}
	if critical != config.DefaultContextBudgetCriticalRatio {
		t.Fatalf("expected default critical ratio, got %f", critical)
	}
}

func TestContextBudgetRatiosCriticalClampedToOne(t *testing.T) {
	r := &Runner{config: config.Config{
		ContextBudget: config.ContextBudgetConfig{
			WarningRatio:  0.5,
			CriticalRatio: 1.5,
		},
	}}
	warning, critical := r.contextBudgetRatios()
	if critical != 1.0 {
		t.Fatalf("expected critical clamped to 1.0, got %f", critical)
	}
	if warning != 0.5 {
		t.Fatalf("expected warning 0.5, got %f", warning)
	}
}

func TestContextBudgetRatiosWarningGECriticalResetsToDefaults(t *testing.T) {
	r := &Runner{config: config.Config{
		ContextBudget: config.ContextBudgetConfig{
			WarningRatio:  0.95,
			CriticalRatio: 0.85,
		},
	}}
	warning, critical := r.contextBudgetRatios()
	if warning != config.DefaultContextBudgetWarningRatio {
		t.Fatalf("expected default warning after reset, got %f", warning)
	}
	if critical != config.DefaultContextBudgetCriticalRatio {
		t.Fatalf("expected default critical after reset, got %f", critical)
	}
}

func TestContextBudgetRatiosValidCustom(t *testing.T) {
	r := &Runner{config: config.Config{
		ContextBudget: config.ContextBudgetConfig{
			WarningRatio:  0.70,
			CriticalRatio: 0.90,
		},
	}}
	warning, critical := r.contextBudgetRatios()
	if warning != 0.70 {
		t.Fatalf("expected 0.70, got %f", warning)
	}
	if critical != 0.90 {
		t.Fatalf("expected 0.90, got %f", critical)
	}
}

// --- contextBudgetQuota ---

func TestContextBudgetQuotaDefault(t *testing.T) {
	r := &Runner{config: config.Config{}}
	if got := r.contextBudgetQuota(); got != 5000 {
		t.Fatalf("expected 5000, got %d", got)
	}
}

func TestContextBudgetQuotaCustom(t *testing.T) {
	r := &Runner{config: config.Config{TokenQuota: 10000}}
	if got := r.contextBudgetQuota(); got != 10000 {
		t.Fatalf("expected 10000, got %d", got)
	}
}

func TestContextBudgetQuotaNegative(t *testing.T) {
	r := &Runner{config: config.Config{TokenQuota: -1}}
	if got := r.contextBudgetQuota(); got != 5000 {
		t.Fatalf("expected default 5000 for negative, got %d", got)
	}
}

// --- contextBudgetMaxReactiveRetry ---

func TestContextBudgetMaxReactiveRetryDefault(t *testing.T) {
	r := &Runner{config: config.Config{}}
	if got := r.contextBudgetMaxReactiveRetry(); got != config.DefaultContextBudgetMaxReactiveRetry {
		t.Fatalf("expected default max retry, got %d", got)
	}
}

func TestContextBudgetMaxReactiveRetryCustom(t *testing.T) {
	r := &Runner{config: config.Config{ContextBudget: config.ContextBudgetConfig{MaxReactiveRetry: 3}}}
	if got := r.contextBudgetMaxReactiveRetry(); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func containsToolResultID(messages []llm.Message, toolUseID string) bool {
	for i := range messages {
		message := messages[i]
		message.Normalize()
		for _, part := range message.Parts {
			if part.Type != llm.PartToolResult || part.ToolResult == nil {
				continue
			}
			if part.ToolResult.ToolUseID == toolUseID {
				return true
			}
		}
	}
	return false
}
