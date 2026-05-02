package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
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

	if hasAskUserSignal(normalized) {
		return turnIntentAskUser
	}
	if hasFinalizeSignal(normalized) {
		return turnIntentFinalize
	}

	// Keep untagged continue_work fallback strict:
	// require both a strong "I will do X" signal and a concrete execution marker.
	if hasContinueStrongSignal(normalized) && hasExecutionActionMarker(normalized) {
		return turnIntentContinueWork
	}
	return turnIntentUnknown
}

func hasAskUserSignal(text string) bool {
	return containsAnyToken(text,
		"please confirm",
		"if you agree",
		"do you want",
		"would you like",
		"can you confirm",
		"should i",
		"tell me if you want",
		"let me know if you want",
		"\u8bf7\u786e\u8ba4",             // 请确认
		"\u662f\u5426\u7ee7\u7eed",       // 是否继续
		"\u8981\u4e0d\u8981\u6211",       // 要不要我
		"\u5982\u679c\u4f60\u540c\u610f", // 如果你同意
	)
}

func hasFinalizeSignal(text string) bool {
	return containsAnyToken(text,
		"all done",
		"task completed",
		"work completed",
		"final answer",
		"final response",
		"\u5df2\u5b8c\u6210", // 已完成
		"\u5b8c\u6210\u4e86", // 完成了
		"\u603b\u7ed3",       // 总结
	)
}

func hasContinueStrongSignal(text string) bool {
	return containsAnyToken(text,
		"i will",
		"i'll",
		"let me",
		"next i'll",
		"i am going to",
		"i'm going to",
		"now i'll",
		"\u6211\u4f1a",             // 我会
		"\u6211\u5c06",             // 我将
		"\u6211\u5148",             // 我先
		"\u63a5\u4e0b\u6765\u6211", // 接下来我
		"\u6211\u73b0\u5728",       // 我现在
	)
}

func hasExecutionActionMarker(text string) bool {
	return containsAnyToken(text,
		"run",
		"execute",
		"call",
		"invoke",
		"search",
		"inspect",
		"read",
		"open",
		"edit",
		"write",
		"create",
		"apply",
		"test",
		"build",
		"compile",
		"tool",
		"command",
		"file",
		"\u8c03\u7528", // 调用
		"\u6267\u884c", // 执行
		"\u8fd0\u884c", // 运行
		"\u67e5\u770b", // 查看
		"\u68c0\u67e5", // 检查
		"\u8bfb\u53d6", // 读取
		"\u4fee\u6539", // 修改
		"\u7f16\u8f91", // 编辑
		"\u7f16\u5199", // 编写
		"\u6d4b\u8bd5", // 测试
		"\u67e5\u627e", // 查找
	)
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
	pendingControlNote     string
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

func (s *adaptiveTurnState) consumePendingControlNote() string {
	if s == nil {
		return ""
	}
	note := strings.TrimSpace(s.pendingControlNote)
	s.pendingControlNote = ""
	return note
}

func (s *adaptiveTurnState) schedulePendingControlNote(note string) {
	if s == nil {
		return
	}
	s.pendingControlNote = strings.TrimSpace(note)
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
	s.pendingControlNote = ""
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
