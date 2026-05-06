package agent

import (
	"fmt"
	"strings"

	policypkg "github.com/1024XEngineer/bytemind/internal/policy"
)

func shouldRepairMissingRequiredWebLookup(requirement policypkg.WebLookupRequirement, executedTools *[]string) bool {
	if requirement != policypkg.WebLookupRequirementMust {
		return false
	}
	return !hasWebLookupToolActivity(executedToolNamesSnapshot(executedTools))
}

func webLookupToolsAvailable(availableTools []string) bool {
	return containsToolName(availableTools, "web_search") || containsToolName(availableTools, "web_fetch")
}

func hasWebLookupToolActivity(toolNames []string) bool {
	return containsToolName(toolNames, "web_search") || containsToolName(toolNames, "web_fetch")
}

func executedToolNamesSnapshot(executedTools *[]string) []string {
	if executedTools == nil || *executedTools == nil {
		return nil
	}
	return append([]string(nil), (*executedTools)...)
}

func buildRequiredWebLookupRepairInstruction(latestUser string, attempt, maxAttempts int, availableTools []string) string {
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
		`The previous assistant turn tried to answer without web_search/web_fetch, but this user request requires current or external web evidence.
Attempt %d/%d.

Latest user input:
%s

Available tools in this run:
%s

For this next turn:
1) Use web_fetch for explicit URLs, or web_search followed by web_fetch for current, volatile, provider, model, version, pricing, or official-source facts.
2) Do not finalize until at least one real web_search or web_fetch tool call has run in this request.
3) If web results are weak, unavailable, or policy-denied, state that clearly instead of asserting unsupported facts.`,
		attempt,
		maxAttempts,
		latestUser,
		toolList,
	))
}
