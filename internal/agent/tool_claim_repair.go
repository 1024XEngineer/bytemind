package agent

import (
	"fmt"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

func shouldRepairUnexecutedToolClaimTurn(runMode planpkg.AgentMode, reply llm.Message, messages []llm.Message, availableTools []string) bool {
	if runMode != planpkg.ModeBuild || len(reply.ToolCalls) > 0 {
		return false
	}
	if !containsToolName(availableTools, "run_shell") || hasToolActivitySinceLatestHumanUser(messages) {
		return false
	}

	text := strings.ToLower(strings.TrimSpace(reply.Content))
	if text == "" {
		return false
	}
	return looksLikeUnavailableOrTimedOutRunShellClaim(text)
}

func buildUnexecutedToolClaimRepairInstruction(reply llm.Message, latestUser string, attempt, maxAttempts int, availableTools []string) string {
	preview := strings.TrimSpace(reply.Content)
	if preview == "" {
		preview = "(empty assistant text)"
	}
	preview = truncateRunes(preview, 240)

	latestUser = strings.TrimSpace(latestUser)
	if latestUser == "" {
		latestUser = "(empty user input)"
	}

	toolList := strings.TrimSpace(strings.Join(availableTools, ", "))
	if toolList == "" {
		toolList = "(none)"
	}
	toolList = truncateRunes(toolList, 240)

	return strings.TrimSpace(fmt.Sprintf(
		`The previous assistant turn claimed the shell tool was unavailable or timed out, but there was no structured tool call or tool result since the latest user request.
Attempt %d/%d.

Latest user input:
%s

Reply text preview:
%s

Available tools in this run:
%s

For this next turn:
1) Do not claim run_shell is unavailable or timed out unless a real structured tool result or policy denial shows that.
2) If shell execution is needed, emit a structured run_shell call directly.
3) If approval or sandbox policy blocks the action, rely on the actual tool approval/result flow instead of narrating a hypothetical limitation.
4) If shell execution is unnecessary, use the right available tool or finalize clearly.`,
		attempt,
		maxAttempts,
		latestUser,
		preview,
		toolList,
	))
}

func containsToolName(toolNames []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, name := range toolNames {
		if strings.ToLower(strings.TrimSpace(name)) == want {
			return true
		}
	}
	return false
}

func looksLikeUnavailableOrTimedOutRunShellClaim(text string) bool {
	if !containsAnyToken(text,
		"run_shell",
		"shell tool",
		"shell 工具",
		"shell 命令",
		"terminal",
		"终端",
		"命令行",
	) {
		return false
	}
	if containsAnyToken(text,
		"timed out",
		"timeout",
		"time out",
		"超时",
	) {
		return true
	}
	return containsAnyToken(text,
		"unavailable",
		"not available",
		"don't have access",
		"do not have access",
		"can't access",
		"cannot access",
		"unable to access",
		"can't use",
		"cannot use",
		"unable to use",
		"don't have",
		"do not have",
		"no access",
		"not supported",
		"unsupported",
		"can't run",
		"cannot run",
		"unable to run",
		"无法",
		"不能",
		"没有",
		"不可用",
		"不支持",
	)
}
