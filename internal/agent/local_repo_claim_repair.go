package agent

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

type localRepoClaimRepairKind int

const (
	localRepoClaimRepairNone localRepoClaimRepairKind = iota
	localRepoClaimRepairPathUnverified
	localRepoClaimRepairImplementationUnverified
)

type localRepoClaimEvidence struct {
	LatestUser             string
	ReplyPreview           string
	ReferencedPaths        []string
	WeakSignals            []string
	DirectConfirmations    []string
	ImplementationEvidence []string
}

type localRepoEvidenceTrace struct {
	evidence                localRepoClaimEvidence
	confirmedPaths          map[string]bool
	hasImplementationSignal bool
}

var localRepoPathPattern = regexp.MustCompile(`(?i)(?:[A-Za-z0-9._-]+[\\/])+[A-Za-z0-9._-]+`)

func evaluateLocalRepoClaimRepairTurn(runMode planpkg.AgentMode, latestUser string, reply llm.Message, messages []llm.Message) (localRepoClaimRepairKind, localRepoClaimEvidence) {
	evidence := newLocalRepoClaimEvidence(latestUser, reply.Content)
	if (runMode != planpkg.ModeBuild && runMode != planpkg.ModePlan) || len(reply.ToolCalls) > 0 {
		return localRepoClaimRepairNone, evidence
	}

	text := strings.TrimSpace(reply.Content)
	normalized := strings.ToLower(text)
	if normalized == "" || hasUnverifiedLocalRepoQualifier(normalized) {
		return localRepoClaimRepairNone, evidence
	}

	referencedPaths := extractLocalRepoPaths(text)
	evidence.ReferencedPaths = referencedPaths
	if !looksLikeConcreteLocalRepoClaim(normalized, referencedPaths) {
		return localRepoClaimRepairNone, evidence
	}

	index, normalizedLatestUser := latestHumanUserMessage(messages)
	if index < 0 {
		return localRepoClaimRepairNone, evidence
	}
	if evidence.LatestUser == "(empty user input)" && strings.TrimSpace(normalizedLatestUser) != "" {
		evidence.LatestUser = normalizedLatestUser
	}

	trace := inspectLocalRepoEvidence(messages[index+1:], evidence, referencedPaths)
	if len(referencedPaths) > 0 && !trace.hasDirectConfirmationForAll(referencedPaths) {
		return localRepoClaimRepairPathUnverified, trace.evidence
	}
	if looksLikeStrongRepoImplementationClaim(normalized) && !trace.hasImplementationSignal {
		return localRepoClaimRepairImplementationUnverified, trace.evidence
	}
	return localRepoClaimRepairNone, trace.evidence
}

func buildLocalRepoClaimRepairInstruction(kind localRepoClaimRepairKind, reply llm.Message, latestUser string, attempt, maxAttempts int, evidence localRepoClaimEvidence) string {
	if evidence.LatestUser == "" && evidence.ReplyPreview == "" {
		evidence = newLocalRepoClaimEvidence(latestUser, reply.Content)
	}

	switch kind {
	case localRepoClaimRepairPathUnverified:
		return strings.TrimSpace(fmt.Sprintf(
			`The previous assistant turn made a concrete local repository claim, but the referenced path or command target was not directly confirmed.
Attempt %d/%d.

Latest user request:
%s

Reply text preview:
%s

Referenced local paths or command targets:
%s

Weak signals observed so far:
%s

Direct confirmations observed so far:
%s

For this next turn:
1) Do not claim a local file, entrypoint, or runnable command exists based only on search hits, broad root listings, or README/docs mentions.
2) First confirm the exact path directly with list_files scoped to the parent directory, read_file on the file itself, or another equally direct local check.
3) If the path is still unconfirmed, say that clearly instead of presenting the command as runnable.
4) Keep the claim tied to actual tool evidence.`,
			attempt,
			maxAttempts,
			evidence.LatestUser,
			evidence.ReplyPreview,
			formatLocalRepoEvidence(evidence.ReferencedPaths),
			formatLocalRepoEvidence(evidence.WeakSignals),
			formatLocalRepoEvidence(evidence.DirectConfirmations),
		))
	case localRepoClaimRepairImplementationUnverified:
		return strings.TrimSpace(fmt.Sprintf(
			`The previous assistant turn concluded that the repository already contains a runnable implementation, but the evidence only covered documentation or path-level hints.
Attempt %d/%d.

Latest user request:
%s

Reply text preview:
%s

Referenced local paths or command targets:
%s

Direct confirmations observed so far:
%s

Implementation evidence observed so far:
%s

For this next turn:
1) Do not conclude "already implemented", "can directly run", or equivalent unless you inspected at least one non-documentation implementation file or verified the entrypoint more directly.
2) Treat README/docs/search hits as leads, not proof of implementation.
3) If you only confirmed documentation, say the repo documents this path but the implementation is not yet confirmed.
4) If implementation evidence exists after inspection, ground the claim in that specific file or result.`,
			attempt,
			maxAttempts,
			evidence.LatestUser,
			evidence.ReplyPreview,
			formatLocalRepoEvidence(evidence.ReferencedPaths),
			formatLocalRepoEvidence(evidence.DirectConfirmations),
			formatLocalRepoEvidence(evidence.ImplementationEvidence),
		))
	default:
		return ""
	}
}

func newLocalRepoClaimEvidence(latestUser, replyText string) localRepoClaimEvidence {
	latestUser = strings.TrimSpace(latestUser)
	if latestUser == "" {
		latestUser = "(empty user input)"
	}
	preview := strings.TrimSpace(replyText)
	if preview == "" {
		preview = "(empty assistant text)"
	}
	return localRepoClaimEvidence{
		LatestUser:   latestUser,
		ReplyPreview: truncateRunes(preview, 240),
	}
}

func inspectLocalRepoEvidence(messages []llm.Message, evidence localRepoClaimEvidence, referencedPaths []string) localRepoEvidenceTrace {
	trace := localRepoEvidenceTrace{
		evidence:       evidence,
		confirmedPaths: make(map[string]bool, len(referencedPaths)),
	}
	for _, ref := range referencedPaths {
		trace.confirmedPaths[normalizeLocalRepoPath(ref)] = false
	}
	for _, msg := range messages {
		for _, call := range msg.ToolCalls {
			trace.observe(call, referencedPaths)
		}
	}
	return trace
}

func (t *localRepoEvidenceTrace) observe(call llm.ToolCall, referencedPaths []string) {
	if t == nil {
		return
	}
	name := strings.ToLower(strings.TrimSpace(call.Function.Name))
	switch name {
	case "search_text":
		query, searchPath := extractSearchTextArgs(call.Function.Arguments)
		signal := "search_text"
		if query != "" {
			signal += " query=" + truncateRunes(query, 80)
		}
		if searchPath != "" {
			signal += " path=" + searchPath
		}
		t.evidence.WeakSignals = appendUniqueEvidenceItem(t.evidence.WeakSignals, signal)
	case "list_files":
		listPath := extractPathArg(call.Function.Arguments)
		if listPath == "" {
			listPath = "."
		}
		t.evidence.WeakSignals = appendUniqueEvidenceItem(t.evidence.WeakSignals, "list_files "+listPath)
		for _, ref := range referencedPaths {
			if confirmsReferencedPathByListing(ref, listPath) {
				normalized := normalizeLocalRepoPath(ref)
				t.confirmedPaths[normalized] = true
				t.evidence.DirectConfirmations = appendUniqueEvidenceItem(t.evidence.DirectConfirmations, "list_files "+listPath)
			}
		}
	case "read_file":
		readPath := extractPathArg(call.Function.Arguments)
		if readPath == "" {
			return
		}
		t.evidence.DirectConfirmations = appendUniqueEvidenceItem(t.evidence.DirectConfirmations, "read_file "+readPath)
		for _, ref := range referencedPaths {
			if normalizeLocalRepoPath(readPath) == normalizeLocalRepoPath(ref) {
				t.confirmedPaths[normalizeLocalRepoPath(ref)] = true
			}
		}
		if !isDocumentationPath(readPath) {
			t.hasImplementationSignal = true
			t.evidence.ImplementationEvidence = appendUniqueEvidenceItem(t.evidence.ImplementationEvidence, "read_file "+readPath)
		}
	case "run_shell":
		command := extractRunShellCommand(call.Function.Arguments)
		if command == "" {
			return
		}
		for _, ref := range referencedPaths {
			if strings.Contains(strings.ToLower(command), strings.ToLower(normalizeLocalRepoPath(ref))) {
				t.confirmedPaths[normalizeLocalRepoPath(ref)] = true
				t.evidence.DirectConfirmations = appendUniqueEvidenceItem(t.evidence.DirectConfirmations, "run_shell "+truncateRunes(command, 100))
				t.hasImplementationSignal = true
				t.evidence.ImplementationEvidence = appendUniqueEvidenceItem(t.evidence.ImplementationEvidence, "run_shell "+truncateRunes(command, 100))
			}
		}
	}
}

func (t localRepoEvidenceTrace) hasDirectConfirmationForAll(referencedPaths []string) bool {
	if len(referencedPaths) == 0 {
		return true
	}
	for _, ref := range referencedPaths {
		if !t.confirmedPaths[normalizeLocalRepoPath(ref)] {
			return false
		}
	}
	return true
}

func extractLocalRepoPaths(text string) []string {
	matches := localRepoPathPattern.FindAllString(text, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		normalized := normalizeLocalRepoPath(match)
		if normalized == "" || strings.Contains(normalized, "://") {
			continue
		}
		paths = appendUniqueEvidenceItem(paths, normalized)
	}
	return paths
}

func looksLikeConcreteLocalRepoClaim(text string, referencedPaths []string) bool {
	if len(referencedPaths) > 0 &&
		containsAnyToken(text,
			"already",
			"exists",
			"implemented",
			"runnable",
			"run ",
			"entrypoint",
			"server.py",
			"\u5df2\u7ecf",
			"\u5df2\u5b9e\u73b0",
			"\u53ef\u4ee5\u76f4\u63a5\u8fd0\u884c",
			"\u53ef\u4ee5\u76f4\u63a5\u8dd1",
			"\u5c31\u5728",
			"\u76f4\u63a5\u6267\u884c",
		) {
		return true
	}
	return looksLikeStrongRepoImplementationClaim(text)
}

func looksLikeStrongRepoImplementationClaim(text string) bool {
	return containsAnyToken(text,
		"already implemented",
		"already has",
		"can directly run",
		"ready to run",
		"runnable",
		"fully working",
		"minimal implementation already exists",
		"implementation is already there",
		"\u6700\u5c0f\u95ed\u73af\u5b9e\u73b0",
		"\u5df2\u7ecf\u6709",
		"\u5df2\u5b9e\u73b0",
		"\u53ef\u4ee5\u76f4\u63a5\u672c\u5730\u8fd0\u884c",
		"\u53ef\u4ee5\u76f4\u63a5\u8fd0\u884c",
		"\u53ef\u4ee5\u76f4\u63a5\u8dd1",
	)
}

func hasUnverifiedLocalRepoQualifier(text string) bool {
	return containsAnyToken(text,
		"not confirmed",
		"unconfirmed",
		"haven't confirmed",
		"have not confirmed",
		"did not confirm",
		"didn't confirm",
		"not yet confirmed",
		"not yet verified",
		"readme mentions",
		"docs mention",
		"documented but not confirmed",
		"appears documented",
		"\u672a\u786e\u8ba4",
		"\u6ca1\u6709\u786e\u8ba4",
		"\u5c1a\u672a\u786e\u8ba4",
		"\u8fd8\u6ca1\u786e\u8ba4",
		"\u53ea\u786e\u8ba4\u5230",
		"\u53ea\u80fd\u786e\u8ba4\u5230",
		"\u6587\u6863\u63d0\u5230",
		"\u4f46\u5b9e\u73b0\u672a\u786e\u8ba4",
	)
}

func extractPathArg(arguments string) string {
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return ""
	}
	return normalizeLocalRepoPath(payload.Path)
}

func extractSearchTextArgs(arguments string) (string, string) {
	var payload struct {
		Query string `json:"query"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return "", ""
	}
	return strings.TrimSpace(payload.Query), normalizeLocalRepoPath(payload.Path)
}

func extractRunShellCommand(arguments string) string {
	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Command)
}

func confirmsReferencedPathByListing(referencePath, listPath string) bool {
	referencePath = normalizeLocalRepoPath(referencePath)
	listPath = normalizeLocalRepoPath(listPath)
	if referencePath == "" {
		return false
	}
	if listPath == "" || listPath == "." {
		return path.Dir(referencePath) == "." || path.Dir(referencePath) == "/"
	}
	parent := path.Dir(referencePath)
	return listPath == referencePath || listPath == parent
}

func normalizeLocalRepoPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`'\"()[]{}<>.,:;")
	value = strings.ReplaceAll(value, `\`, "/")
	if value == "" {
		return ""
	}
	clean := path.Clean(value)
	if clean == "." {
		return value
	}
	return clean
}

func isDocumentationPath(value string) bool {
	value = strings.ToLower(normalizeLocalRepoPath(value))
	if value == "" {
		return false
	}
	base := path.Base(value)
	if strings.HasPrefix(base, "readme") {
		return true
	}
	return strings.HasSuffix(base, ".md") ||
		strings.HasSuffix(base, ".txt") ||
		strings.HasSuffix(base, ".rst") ||
		strings.HasSuffix(base, ".adoc")
}

func appendUniqueEvidenceItem(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func formatLocalRepoEvidence(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return truncateRunes(strings.Join(items, ", "), 240)
}
