package policy

import "strings"

// ExplicitWebLookupInstruction returns an extra system hint when the user
// explicitly asks for online/source-website lookup instead of local workspace
// inspection.
func ExplicitWebLookupInstruction(userInput string) string {
	text := strings.ToLower(strings.TrimSpace(userInput))
	if text == "" {
		return ""
	}

	webSignals := []string{
		"github", "gitlab", "bitbucket",
		"联网", "上网", "互联网", "网上",
		"源码", "源代码", "repo", "repository",
		"official website", "官网",
	}
	matched := false
	for _, signal := range webSignals {
		if strings.Contains(text, signal) {
			matched = true
			break
		}
	}
	if !matched {
		return ""
	}

	return "The user explicitly requested online or GitHub-source lookup. Use web_search/web_fetch first. Do not substitute local-workspace tools (list_files/read_file/search_text) for this request unless the user explicitly asks to inspect the current workspace repository."
}
