package notify

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	reHeaderAuthorization = regexp.MustCompile(`(?i)\bauthorization\b\s*:\s*[^\r\n]+`)
	reBearerToken         = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._+\-=/]+`)
	reKeyValueSecret      = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\b\s*[:=]\s*(\"[^\"]*\"|'[^']*'|[^\s,;|]+)`)
	reQuerySecret         = regexp.MustCompile(`(?i)([?&](?:api[_-]?key|token|secret|password)=)([^&#\s]+)`)
	reFlagSecret          = regexp.MustCompile(`(?i)(--?(?:api[_-]?key|token|secret|password)\s+)([^\s]+)`)
	reLongHex             = regexp.MustCompile(`\b[0-9A-Fa-f]{24,}\b`)
	reBase64Like          = regexp.MustCompile(`\b[A-Za-z0-9+/=]{40,}\b`)
)

func sanitizeNotificationText(input string) string {
	text := strings.TrimSpace(input)
	if text == "" {
		return ""
	}

	text = reHeaderAuthorization.ReplaceAllString(text, "authorization: [REDACTED]")
	text = reBearerToken.ReplaceAllString(text, "bearer [REDACTED]")
	text = reKeyValueSecret.ReplaceAllString(text, "$1=[REDACTED]")
	text = reQuerySecret.ReplaceAllString(text, "$1[REDACTED]")
	text = reFlagSecret.ReplaceAllString(text, "$1[REDACTED]")
	text = reLongHex.ReplaceAllString(text, "[REDACTED]")
	text = reBase64Like.ReplaceAllString(text, "[REDACTED]")
	return text
}

func truncate(input string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(input) <= limit {
		return input
	}
	if limit <= 3 {
		return strings.Repeat(".", limit)
	}
	runes := []rune(input)
	return string(runes[:limit-3]) + "..."
}

func normalizeForKey(input string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(input)))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}
