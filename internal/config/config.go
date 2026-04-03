package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Provider       ProviderConfig `json:"provider"`
	ApprovalPolicy string         `json:"approval_policy"`
	MaxIterations  int            `json:"max_iterations"`
	SessionDir     string         `json:"session_dir"`
	Stream         bool           `json:"stream"`
}

type ProviderConfig struct {
	Type             string `json:"type"`
	BaseURL          string `json:"base_url"`
	Model            string `json:"model"`
	APIKey           string `json:"api_key"`
	APIKeyEnv        string `json:"api_key_env"`
	AnthropicVersion string `json:"anthropic_version"`
}

func Default(workspace string) Config {
	return Config{
		Provider: ProviderConfig{
			Type:             "openai-compatible",
			BaseURL:          "https://api.openai.com/v1",
			Model:            "GPT-5.4",
			APIKeyEnv:        "BYTEMIND_API_KEY",
			AnthropicVersion: "2023-06-01",
		},
		ApprovalPolicy: "on-request",
		MaxIterations:  32,
		SessionDir:     filepath.Join(workspace, ".bytemind", "sessions"),
		Stream:         true,
	}
}

func Load(workspace, configPath string) (Config, error) {
	cfg := Default(workspace)

	path, err := resolveConfigPath(workspace, configPath)
	if err != nil {
		return cfg, err
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}

	applyEnv(&cfg)
	if err := normalize(workspace, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (p ProviderConfig) ResolveAPIKey() string {
	if strings.TrimSpace(p.APIKey) != "" {
		return strings.TrimSpace(p.APIKey)
	}
	if env := strings.TrimSpace(p.APIKeyEnv); env != "" {
		return strings.TrimSpace(os.Getenv(env))
	}
	return strings.TrimSpace(os.Getenv("BYTEMIND_API_KEY"))
}

func resolveConfigPath(workspace, explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}

	candidates := []string{
		filepath.Join(workspace, "config.json"),
		filepath.Join(workspace, ".bytemind", "config.json"),
		filepath.Join(workspace, "bytemind.config.json"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".bytemind", "config.json"))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}

func applyEnv(cfg *Config) {
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_PROVIDER_TYPE")); value != "" {
		cfg.Provider.Type = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_BASE_URL")); value != "" {
		cfg.Provider.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_MODEL")); value != "" {
		cfg.Provider.Model = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_API_KEY")); value != "" {
		cfg.Provider.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_API_KEY_ENV")); value != "" {
		cfg.Provider.APIKeyEnv = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_APPROVAL_POLICY")); value != "" {
		cfg.ApprovalPolicy = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_STREAM")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Stream = parsed
		}
	}
}

func normalize(workspace string, cfg *Config) error {
	cfg.Provider.Type = normalizeProviderType(cfg.Provider.Type)
	if cfg.Provider.BaseURL == "" {
		cfg.Provider.BaseURL = defaultBaseURL(cfg.Provider.Type)
	}
	if strings.TrimSpace(cfg.Provider.BaseURL) == "" {
		return errors.New("provider.base_url is required")
	}
	if strings.TrimSpace(cfg.Provider.Model) == "" {
		return errors.New("provider.model is required")
	}
	if cfg.Provider.APIKeyEnv == "" {
		cfg.Provider.APIKeyEnv = "BYTEMIND_API_KEY"
	}
	if cfg.Provider.AnthropicVersion == "" {
		cfg.Provider.AnthropicVersion = "2023-06-01"
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 32
	}
	if !isSupportedProviderType(cfg.Provider.Type) {
		return errors.New("provider.type must be one of openai-compatible, openai, anthropic")
	}
	switch cfg.ApprovalPolicy {
	case "", "on-request":
		cfg.ApprovalPolicy = "on-request"
	case "always", "never":
	default:
		return errors.New("approval_policy must be one of always, on-request, never")
	}
	if cfg.SessionDir == "" {
		cfg.SessionDir = filepath.Join(workspace, ".bytemind", "sessions")
	}
	if !filepath.IsAbs(cfg.SessionDir) {
		cfg.SessionDir = filepath.Join(workspace, cfg.SessionDir)
	}
	return nil
}

func normalizeProviderType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "openai-compatible", "openai_compatible", "openai":
		if strings.EqualFold(strings.TrimSpace(value), "openai") {
			return "openai"
		}
		return "openai-compatible"
	case "anthropic":
		return "anthropic"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func isSupportedProviderType(value string) bool {
	switch value {
	case "openai-compatible", "openai", "anthropic":
		return true
	default:
		return false
	}
}

func defaultBaseURL(providerType string) string {
	switch normalizeProviderType(providerType) {
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return "https://api.openai.com/v1"
	}
}
