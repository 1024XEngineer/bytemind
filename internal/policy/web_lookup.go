package policy

import (
	"regexp"
	"strings"
)

type WebLookupRequirement string

const (
	WebLookupRequirementNone   WebLookupRequirement = "none"
	WebLookupRequirementShould WebLookupRequirement = "should"
	WebLookupRequirementMust   WebLookupRequirement = "must"
)

type WebLookupRequirementResult struct {
	Requirement WebLookupRequirement
	Reason      string
	Instruction string
}

var (
	webURLPattern     = regexp.MustCompile(`(?i)\bhttps?://|www\.`)
	llmModelIDPattern = regexp.MustCompile(`(?i)\b(gpt|claude|gemini|deepseek|qwen|llama|mistral|o[0-9])[-._[:alnum:]]*\b`)
)

// ExplicitWebLookupInstruction returns an extra system hint when the user
// explicitly asks for online/source-website lookup instead of local workspace
// inspection.
func ExplicitWebLookupInstruction(userInput string) string {
	return EvaluateWebLookupRequirement(userInput).Instruction
}

func EvaluateWebLookupRequirement(userInput string) WebLookupRequirementResult {
	text := strings.ToLower(strings.TrimSpace(userInput))
	if text == "" {
		return WebLookupRequirementResult{
			Requirement: WebLookupRequirementNone,
			Reason:      "empty user input",
		}
	}

	if webURLPattern.MatchString(text) {
		return requiredWebLookup("user included an explicit web URL")
	}

	directSignals := []string{
		"github", "gitlab", "bitbucket",
		"联网", "上网", "互联网", "网上",
		"源码", "源代码",
		"official website", "official docs", "search web", "browse web",
		"online", "web_search", "web fetch", "官网", "官方文档",
	}
	for _, signal := range directSignals {
		if strings.Contains(text, signal) {
			return requiredWebLookup("user explicitly requested online or source-website lookup")
		}
	}

	hasRepoWord := strings.Contains(text, "repo") || strings.Contains(text, "repository")
	hasRemoteQualifier := strings.Contains(text, "github") || strings.Contains(text, "gitlab") || strings.Contains(text, "bitbucket") || strings.Contains(text, "online") || strings.Contains(text, "remote")
	if hasRepoWord && hasRemoteQualifier {
		return requiredWebLookup("user explicitly requested a remote repository lookup")
	}

	if looksLocalWorkspaceOnly(text) && !llmModelIDPattern.MatchString(text) {
		return WebLookupRequirementResult{
			Requirement: WebLookupRequirementNone,
			Reason:      "user requested local workspace inspection",
		}
	}

	if containsFreshnessSignal(text) {
		return requiredWebLookup("user asked for current or time-sensitive information")
	}
	if llmModelIDPattern.MatchString(text) && containsModelRealitySignal(text) {
		return requiredWebLookup("user asked about a current model or provider fact")
	}

	return WebLookupRequirementResult{
		Requirement: WebLookupRequirementNone,
		Reason:      "no web lookup requirement detected",
	}
}

func requiredWebLookup(reason string) WebLookupRequirementResult {
	return WebLookupRequirementResult{
		Requirement: WebLookupRequirementMust,
		Reason:      strings.TrimSpace(reason),
		Instruction: "The user request requires current or external web evidence. Use web_search/web_fetch before finalizing, ground volatile claims in fetched sources, and clearly state when web results are weak or unavailable instead of asserting unsupported facts.",
	}
}

func containsFreshnessSignal(text string) bool {
	return containsAnyText(text,
		"latest", "current", "currently", "today", "now", "recent", "newest", "up-to-date", "as of",
		"最新", "当前", "现在", "今天", "目前", "近期", "最近", "现行", "最新版",
	)
}

func containsModelRealitySignal(text string) bool {
	return containsAnyText(text,
		"exists", "exist", "available", "availability", "supported", "support", "real model", "model id",
		"是否存在", "存在", "可用", "支持", "真实", "模型",
	)
}

func looksLocalWorkspaceOnly(text string) bool {
	return containsAnyText(text,
		"current workspace", "local workspace", "current repo", "local repo", "search_text", "read_file", "list_files",
		"recent commit", "latest commit", "commit", "diff", "staged", "unstaged", "/review",
		"当前工作区", "本地工作区", "当前仓库", "本地仓库", "当前项目", "最近提交", "本地提交",
	)
}

func containsAnyText(text string, tokens ...string) bool {
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}
