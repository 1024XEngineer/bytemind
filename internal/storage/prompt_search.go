package storage

import "strings"

// PromptSearchQuery is the normalized filter expression for prompt history.
type PromptSearchQuery struct {
	Tokens          []string
	WorkspaceFilter string
	SessionFilter   string
}

// ParsePromptSearchQuery parses free text into content tokens and scope filters.
func ParsePromptSearchQuery(raw string) PromptSearchQuery {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	tokens := make([]string, 0, len(fields))
	workspaceFilter := ""
	sessionFilter := ""

	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		switch {
		case strings.HasPrefix(field, "ws:"):
			workspaceFilter = strings.TrimSpace(strings.TrimPrefix(field, "ws:"))
		case strings.HasPrefix(field, "workspace:"):
			workspaceFilter = strings.TrimSpace(strings.TrimPrefix(field, "workspace:"))
		case strings.HasPrefix(field, "sid:"):
			sessionFilter = strings.TrimSpace(strings.TrimPrefix(field, "sid:"))
		case strings.HasPrefix(field, "session:"):
			sessionFilter = strings.TrimSpace(strings.TrimPrefix(field, "session:"))
		default:
			tokens = append(tokens, field)
		}
	}

	return PromptSearchQuery{
		Tokens:          tokens,
		WorkspaceFilter: workspaceFilter,
		SessionFilter:   sessionFilter,
	}
}

// FilterPromptEntries filters and ranks prompt history entries newest-first.
func FilterPromptEntries(entries []PromptEntry, rawQuery string, limit int) []PromptEntry {
	if limit <= 0 {
		limit = len(entries)
	}
	parsed := ParsePromptSearchQuery(rawQuery)
	matches := make([]PromptEntry, 0, minInt(len(entries), limit))

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		prompt := strings.TrimSpace(entry.Prompt)
		if prompt == "" {
			continue
		}

		workspaceValue := strings.ToLower(strings.TrimSpace(entry.Workspace))
		if parsed.WorkspaceFilter != "" && !strings.Contains(workspaceValue, parsed.WorkspaceFilter) {
			continue
		}

		sessionValue := strings.ToLower(strings.TrimSpace(string(entry.SessionID)))
		if parsed.SessionFilter != "" && !strings.Contains(sessionValue, parsed.SessionFilter) {
			continue
		}

		if !matchesAllTokens(strings.ToLower(prompt), parsed.Tokens) {
			continue
		}

		matches = append(matches, entry)
		if len(matches) >= limit {
			break
		}
	}

	return matches
}

func matchesAllTokens(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}
