package tui

import (
	"strings"

	"bytemind/internal/config"
	"bytemind/internal/provider"
	tuiruntime "bytemind/internal/tui/runtime"
)

func parseStartupConfigInput(raw string) (field, value string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if lower == "" {
		return "", "", false
	}

	parse := func(alias, normalized string) (string, string, bool) {
		for _, sep := range []string{"=", ":"} {
			prefix := alias + sep
			if strings.HasPrefix(lower, prefix) {
				val := strings.TrimSpace(trimmed[len(prefix):])
				return normalized, val, true
			}
		}
		return "", "", false
	}

	for _, candidate := range []struct {
		alias      string
		normalized string
	}{
		{alias: "model", normalized: "model"},
		{alias: "base_url", normalized: "base_url"},
		{alias: "baseurl", normalized: "base_url"},
		{alias: "base-url", normalized: "base_url"},
		{alias: "provider", normalized: "type"},
		{alias: "type", normalized: "type"},
		{alias: "provider_type", normalized: "type"},
		{alias: "api_key", normalized: "api_key"},
		{alias: "apikey", normalized: "api_key"},
		{alias: "key", normalized: "api_key"},
	} {
		if field, value, ok := parse(candidate.alias, candidate.normalized); ok {
			return field, value, true
		}
	}

	return "", "", false
}

func sanitizeAPIKeyInput(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, "\"'")
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "authorization: bearer ") {
		value = strings.TrimSpace(value[len("authorization: bearer "):])
	}
	if strings.HasPrefix(lower, "bearer ") {
		value = strings.TrimSpace(value[len("bearer "):])
	}
	return strings.TrimSpace(value)
}

func normalizeStartupProviderType(value string) (string, bool) {
	return tuiruntime.NormalizeProviderType(value)
}

func isStartupGuideField(field string) bool {
	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel, startupFieldAPIKey:
		return true
	default:
		return false
	}
}

func startupNextField(current string) string {
	for i, field := range startupFieldOrder {
		if field == current {
			if i+1 >= len(startupFieldOrder) {
				return ""
			}
			return startupFieldOrder[i+1]
		}
	}
	return startupFieldType
}

func startupFieldStep(field string) (int, int) {
	for i, item := range startupFieldOrder {
		if item == field {
			return i + 1, len(startupFieldOrder)
		}
	}
	return 1, len(startupFieldOrder)
}

func startupFieldName(field string) string {
	switch field {
	case startupFieldType:
		return "provider"
	case startupFieldBaseURL:
		return "base_url"
	case startupFieldModel:
		return "model"
	case startupFieldAPIKey:
		return "api_key"
	default:
		return field
	}
}

func startupProviderDefaultBaseURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return "https://api.openai.com/v1"
	}
}

func startupProviderDefaultModel(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "anthropic":
		return ""
	default:
		return "GPT-5.4"
	}
}

func (m model) startupCurrentValue(field string) string {
	switch field {
	case startupFieldType:
		return strings.TrimSpace(m.cfg.Provider.Type)
	case startupFieldBaseURL:
		return strings.TrimSpace(m.cfg.Provider.BaseURL)
	case startupFieldModel:
		return strings.TrimSpace(m.cfg.Provider.Model)
	default:
		return ""
	}
}

func startupGuideStepLines(field string, cfg config.Config, configPath, issue string) []string {
	lines := make([]string, 0, 8)
	switch field {
	case startupFieldType:
		lines = append(lines, "Enter provider: openai-compatible or anthropic.")
	case startupFieldBaseURL:
		lines = append(lines, "Enter provider base_url.")
		lines = append(lines, "Example: https://api.deepseek.com")
	case startupFieldModel:
		lines = append(lines, "Enter model name.")
		lines = append(lines, "Example: deepseek-chat or GPT-5.4")
	case startupFieldAPIKey:
		lines = append(lines, "Paste API key and press Enter.")
		lines = append(lines, "Bytemind will verify it automatically.")
	}

	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel:
		current := ""
		switch field {
		case startupFieldType:
			current = strings.TrimSpace(cfg.Provider.Type)
		case startupFieldBaseURL:
			current = strings.TrimSpace(cfg.Provider.BaseURL)
		case startupFieldModel:
			current = strings.TrimSpace(cfg.Provider.Model)
		}
		if current == "" {
			lines = append(lines, "Press Enter to use default.")
		} else {
			lines = append(lines, "Press Enter to keep current: "+current)
		}
	}
	if strings.TrimSpace(issue) != "" {
		lines = append(lines, "Issue: "+issue)
	}
	if strings.TrimSpace(configPath) != "" {
		lines = append(lines, "Config file: "+configPath)
	}
	return lines
}

func startupGuideIssueHint(check provider.Availability) string {
	reason := strings.ToLower(strings.TrimSpace(check.Reason))
	switch {
	case strings.Contains(reason, "missing api key"):
		return "No API key is configured yet."
	case strings.Contains(reason, "unauthorized"):
		return "The API key was rejected by the provider."
	case strings.Contains(reason, "failed to reach"):
		return "Cannot reach provider endpoint. Check proxy or network."
	case strings.Contains(reason, "not found"):
		return "Provider endpoint path looks incorrect."
	default:
		if strings.TrimSpace(check.Reason) == "" {
			return "Provider check failed."
		}
		return compact(strings.TrimSpace(check.Reason), 90)
	}
}
