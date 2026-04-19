package agent

import (
	"fmt"
	"regexp"
	"strings"

	"bytemind/internal/llm"
)

type assistantTurnIntent string

const (
	turnIntentUnknown      assistantTurnIntent = ""
	turnIntentContinueWork assistantTurnIntent = "continue_work"
	turnIntentAskUser      assistantTurnIntent = "ask_user"
	turnIntentFinalize     assistantTurnIntent = "finalize"
)

const (
	defaultNoProgressTurnLimit = 3
)

var turnIntentTagPattern = regexp.MustCompile(`(?is)<turn_intent>\s*([a-z_]+)\s*</turn_intent>`)

func normalizeAssistantTurnIntent(raw string) assistantTurnIntent {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(turnIntentContinueWork):
		return turnIntentContinueWork
	case string(turnIntentAskUser):
		return turnIntentAskUser
	case string(turnIntentFinalize):
		return turnIntentFinalize
	default:
		return turnIntentUnknown
	}
}

func parseAssistantTurnIntent(reply llm.Message) (assistantTurnIntent, llm.Message, bool) {
	intent := turnIntentUnknown
	explicit := false
	if reply.Meta != nil {
		if raw, ok := reply.Meta["turn_intent"].(string); ok {
			intent = normalizeAssistantTurnIntent(raw)
			explicit = intent != turnIntentUnknown
		}
	}

	cleanedContent := strings.TrimSpace(reply.Content)
	if match := turnIntentTagPattern.FindStringSubmatch(cleanedContent); len(match) == 2 {
		if parsed := normalizeAssistantTurnIntent(match[1]); parsed != turnIntentUnknown {
			intent = parsed
			explicit = true
		}
	}
	cleanedContent = strings.TrimSpace(turnIntentTagPattern.ReplaceAllString(cleanedContent, ""))

	cleaned := reply
	if strings.TrimSpace(cleaned.Content) != cleanedContent {
		cleaned.Content = cleanedContent
		// Rebuild legacy-compatible parts from cleaned content/tool calls.
		cleaned.Parts = nil
		cleaned.Normalize()
	}
	return intent, cleaned, explicit
}

func inferAssistantTurnIntent(text string) assistantTurnIntent {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return turnIntentUnknown
	}

	if containsAnyToken(normalized,
		"please confirm", "if you agree", "do you want", "would you like", "can you confirm",
		"should i", "tell me if you want", "你要我", "请确认", "是否继续", "要不要我", "如果你同意",
	) {
		return turnIntentAskUser
	}

	if containsAnyToken(normalized,
		"i will", "i'll", "let me", "next i'll", "i am going to", "i'm going to",
		"我会", "我将", "我先", "接下来我", "然后我", "继续处理", "继续执行", "准备调用", "开始调用",
	) {
		return turnIntentContinueWork
	}
	return turnIntentUnknown
}

func maxSemanticRepairAttempts(maxReactiveRetry int) int {
	if maxReactiveRetry <= 0 {
		maxReactiveRetry = 1
	}
	// Keep one extra attempt beyond prompt-too-long retry budget to absorb malformed turns.
	return maxReactiveRetry + 1
}

func buildSemanticRepairInstruction(reply llm.Message, attempt, maxAttempts int) string {
	preview := strings.TrimSpace(reply.Content)
	if preview == "" {
		preview = "(empty assistant text)"
	}
	preview = truncateRunes(preview, 240)
	return strings.TrimSpace(fmt.Sprintf(
		`The previous assistant turn indicated ongoing work but returned no structured tool calls.
Attempt %d/%d.

Reply text preview:
%s

For this next turn:
1) If more execution is needed, emit structured tool calls directly.
2) If waiting for user input, include <turn_intent>ask_user</turn_intent> and ask clearly.
3) If task is complete, include <turn_intent>finalize</turn_intent> and provide final output.
4) Do not output proposal-only text with <turn_intent>continue_work</turn_intent> unless you also include tool calls in the same turn.`,
		attempt,
		maxAttempts,
		preview,
	))
}

type adaptiveTurnState struct {
	semanticRepairAttempts int
	maxSemanticRepairs     int
	noProgressTurns        int
	noProgressTurnLimit    int
	pendingSystemNote      string
}

func newAdaptiveTurnState(maxReactiveRetry int) *adaptiveTurnState {
	maxRepairs := maxSemanticRepairAttempts(maxReactiveRetry)
	noProgressLimit := defaultNoProgressTurnLimit
	if noProgressLimit < maxRepairs {
		noProgressLimit = maxRepairs
	}
	return &adaptiveTurnState{
		maxSemanticRepairs:  maxRepairs,
		noProgressTurnLimit: noProgressLimit,
	}
}

func (s *adaptiveTurnState) consumePendingSystemNote() string {
	if s == nil {
		return ""
	}
	note := strings.TrimSpace(s.pendingSystemNote)
	s.pendingSystemNote = ""
	return note
}

func (s *adaptiveTurnState) schedulePendingSystemNote(note string) {
	if s == nil {
		return
	}
	s.pendingSystemNote = strings.TrimSpace(note)
}

func (s *adaptiveTurnState) recordNoProgressTurn() {
	if s == nil {
		return
	}
	s.noProgressTurns++
}

func (s *adaptiveTurnState) recordProgress() {
	if s == nil {
		return
	}
	s.noProgressTurns = 0
	s.semanticRepairAttempts = 0
	s.pendingSystemNote = ""
}

func (s *adaptiveTurnState) recordSemanticRepairAttempt() int {
	if s == nil {
		return 0
	}
	s.semanticRepairAttempts++
	return s.semanticRepairAttempts
}

func (s *adaptiveTurnState) exceededSemanticRepairLimit() bool {
	if s == nil {
		return false
	}
	return s.semanticRepairAttempts > s.maxSemanticRepairs
}

func (s *adaptiveTurnState) exceededNoProgressLimit() bool {
	if s == nil {
		return false
	}
	return s.noProgressTurns >= s.noProgressTurnLimit
}
