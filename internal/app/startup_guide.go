package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/provider"
	"github.com/1024XEngineer/bytemind/tui"
)

func StartupIssueHint(check provider.Availability) string {
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
		return CompactLine(check.Reason, 120)
	}
}

func ConfigPathHint(workspace, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		if abs, err := filepath.Abs(explicit); err == nil {
			return abs
		}
		return explicit
	}

	candidates := []string{
		filepath.Join(workspace, "config.json"),
		filepath.Join(workspace, ".bytemind", "config.json"),
		filepath.Join(workspace, "bytemind.config.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	home, err := config.ResolveHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "config.json")
}

func CompactLine(raw string, limit int) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\n", " "))
	if len(raw) <= limit {
		return raw
	}
	if limit <= 3 {
		return raw[:limit]
	}
	return raw[:limit-3] + "..."
}

func BuildStartupGuide(cfg config.Config, check provider.Availability, workspace, explicitConfigPath string) tui.StartupGuide {
	path := ConfigPathHint(workspace, explicitConfigPath)
	envName := strings.TrimSpace(cfg.Provider.APIKeyEnv)
	if envName == "" {
		envName = "BYTEMIND_API_KEY"
	}
	lines := []string{
		"Paste your API key in the input box below and press Enter.",
		"Bytemind will verify it and save it automatically.",
		"Default OpenAI setup only needs API key.",
		"For other providers, set provider.base_url and provider.model too.",
		"Optional: model=<name>  base_url=<url>  provider=<openai-compatible|anthropic>",
		"You can still use /help and /quit commands.",
		"Env fallback: " + envName,
	}
	if path != "" {
		lines = append(lines, "Config file: "+path)
	}
	lines = append(lines, "Issue: "+StartupIssueHint(check))

	return tui.StartupGuide{
		Active:       true,
		Title:        "Let's finish setup",
		Status:       "Bytemind will guide you through provider, base_url, model, and API key.",
		Lines:        lines,
		ConfigPath:   path,
		CurrentField: "type",
	}
}
