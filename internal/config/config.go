package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	envBytemindHome = "BYTEMIND_HOME"
	defaultHomeDir  = ".bytemind"
)

const (
	DefaultContextBudgetWarningRatio     = 0.85
	DefaultContextBudgetCriticalRatio    = 0.95
	DefaultContextBudgetMaxReactiveRetry = 1
)

type Config struct {
	Provider       ProviderConfig      `json:"provider"`
	ApprovalPolicy string              `json:"approval_policy"`
	MaxIterations  int                 `json:"max_iterations"`
	Stream         bool                `json:"stream"`
	TokenQuota     int                 `json:"token_quota"`
	TokenUsage     TokenUsageConfig    `json:"token_usage"`
	ContextBudget  ContextBudgetConfig `json:"context_budget"`
}

type ProviderConfig struct {
	Type             string            `json:"type"`
	AutoDetectType   bool              `json:"auto_detect_type"`
	BaseURL          string            `json:"base_url"`
	APIPath          string            `json:"api_path"`
	Model            string            `json:"model"`
	APIKey           string            `json:"api_key"`
	APIKeyEnv        string            `json:"api_key_env"`
	AuthHeader       string            `json:"auth_header"`
	AuthScheme       string            `json:"auth_scheme"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	AnthropicVersion string            `json:"anthropic_version"`
}

type TokenUsageConfig struct {
	StorageType     string `json:"storage_type"`
	StoragePath     string `json:"storage_path"`
	BackupInterval  string `json:"backup_interval"`
	MaxSessions     int    `json:"max_sessions"`
	AlertThreshold  int64  `json:"alert_threshold"`
	EnableRealtime  bool   `json:"enable_realtime"`
	RetentionDays   int    `json:"retention_days"`
	MonitorInterval string `json:"monitor_interval"`
	DatabaseDriver  string `json:"database_driver"`
}

type ContextBudgetConfig struct {
	WarningRatio     float64 `json:"warning_ratio"`
	CriticalRatio    float64 `json:"critical_ratio"`
	MaxReactiveRetry int     `json:"max_reactive_retry"`
}

func Default(workspace string) Config {
	return Config{
		Provider: ProviderConfig{
			Type:      "openai-compatible",
			BaseURL:   "https://api.openai.com/v1",
			Model:     "GPT-5.4",
			APIKeyEnv: "BYTEMIND_API_KEY",
		},
		ApprovalPolicy: "on-request",
		MaxIterations:  32,
		Stream:         true,
		TokenQuota:     5000,
		TokenUsage: TokenUsageConfig{
			StorageType:     "file",
			StoragePath:     ".bytemind/token_usage.json",
			BackupInterval:  "1m",
			MaxSessions:     10000,
			AlertThreshold:  1000000,
			EnableRealtime:  true,
			RetentionDays:   30,
			MonitorInterval: "30s",
			DatabaseDriver:  "sqlite3",
		},
		ContextBudget: ContextBudgetConfig{
			WarningRatio:     DefaultContextBudgetWarningRatio,
			CriticalRatio:    DefaultContextBudgetCriticalRatio,
			MaxReactiveRetry: DefaultContextBudgetMaxReactiveRetry,
		},
	}
}

func Load(workspace, configPath string) (Config, error) {
	cfg := Default(workspace)

	if strings.TrimSpace(configPath) != "" {
		path, err := resolveConfigPath(workspace, configPath)
		if err != nil {
			return cfg, err
		}
		if err := mergeConfigFromFile(path, &cfg); err != nil {
			return cfg, err
		}
	} else {
		if userConfig, err := resolveUserConfigPath(); err != nil {
			return cfg, err
		} else if userConfig != "" {
			if err := mergeConfigFromFile(userConfig, &cfg); err != nil {
				return cfg, err
			}
		}

		if projectConfig := resolveProjectConfigPath(workspace); projectConfig != "" {
			if err := mergeConfigFromFile(projectConfig, &cfg); err != nil {
				return cfg, err
			}
		}
	}

	applyEnv(&cfg)
	if err := normalize(&cfg); err != nil {
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

func ResolveHomeDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv(envBytemindHome)); override != "" {
		return filepath.Abs(override)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultHomeDir), nil
}

func EnsureHomeLayout() (string, error) {
	home, err := ResolveHomeDir()
	if err != nil {
		return "", err
	}

	dirs := []string{
		home,
		filepath.Join(home, "sessions"),
		filepath.Join(home, "logs"),
		filepath.Join(home, "cache"),
		filepath.Join(home, "auth"),
		filepath.Join(home, "migrations"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := ensureDefaultConfigFile(home); err != nil {
		return "", err
	}
	return home, nil
}

func ensureDefaultConfigFile(home string) error {
	path := filepath.Join(home, "config.json")
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return errors.New("home config path is a directory")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	cfg := Config{
		Provider: ProviderConfig{
			Type:             "openai-compatible",
			BaseURL:          "https://api.openai.com/v1",
			Model:            "GPT-5.4",
			APIKeyEnv:        "BYTEMIND_API_KEY",
			AnthropicVersion: "2023-06-01",
		},
		ApprovalPolicy: "on-request",
		MaxIterations:  32,
		Stream:         true,
		TokenQuota:     5000,
		TokenUsage: TokenUsageConfig{
			StorageType:     "file",
			StoragePath:     ".bytemind/token_usage.json",
			BackupInterval:  "1m",
			MaxSessions:     10000,
			AlertThreshold:  1000000,
			EnableRealtime:  true,
			RetentionDays:   30,
			MonitorInterval: "30s",
			DatabaseDriver:  "sqlite3",
		},
		ContextBudget: ContextBudgetConfig{
			WarningRatio:     DefaultContextBudgetWarningRatio,
			CriticalRatio:    DefaultContextBudgetCriticalRatio,
			MaxReactiveRetry: DefaultContextBudgetMaxReactiveRetry,
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func resolveConfigPath(workspace, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}

	if project := resolveProjectConfigPath(workspace); project != "" {
		return project, nil
	}
	if user, err := resolveUserConfigPath(); err == nil && user != "" {
		return user, nil
	}
	return "", nil
}

func resolveProjectConfigPath(workspace string) string {
	candidates := []string{
		filepath.Join(workspace, ".bytemind", "config.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func resolveUserConfigPath() (string, error) {
	home, err := ResolveHomeDir()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(home, "config.json")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", nil
}

func mergeConfigFromFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return err
	}
	return nil
}

func applyEnv(cfg *Config) {
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_PROVIDER_TYPE")); value != "" {
		cfg.Provider.Type = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_PROVIDER_AUTO_DETECT_TYPE")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Provider.AutoDetectType = parsed
		}
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
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_TOKEN_QUOTA")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.TokenQuota = parsed
		}
	}
}

func normalize(cfg *Config) error {
	cfg.Provider.Type = normalizeProviderType(cfg.Provider.Type)
	if cfg.Provider.Type == "" {
		if cfg.Provider.AutoDetectType {
			cfg.Provider.Type = detectProviderType(cfg.Provider)
		} else {
			cfg.Provider.Type = "openai-compatible"
		}
	}
	if cfg.Provider.BaseURL == "" {
		cfg.Provider.BaseURL = defaultBaseURL(cfg.Provider.Type)
	}
	if strings.TrimSpace(cfg.Provider.BaseURL) == "" {
		return errors.New("provider.base_url is required")
	}
	if strings.TrimSpace(cfg.Provider.Model) == "" {
		cfg.Provider.Model = defaultModel(cfg.Provider.Type)
		if strings.TrimSpace(cfg.Provider.Model) == "" {
			return errors.New("provider.model is required")
		}
	}
	if cfg.Provider.APIKeyEnv == "" {
		cfg.Provider.APIKeyEnv = "BYTEMIND_API_KEY"
	}
	cfg.Provider.APIPath = strings.TrimSpace(cfg.Provider.APIPath)
	cfg.Provider.AuthHeader = strings.TrimSpace(cfg.Provider.AuthHeader)
	cfg.Provider.AuthScheme = strings.TrimSpace(cfg.Provider.AuthScheme)
	cfg.Provider.AnthropicVersion = strings.TrimSpace(cfg.Provider.AnthropicVersion)
	if cfg.Provider.Type == "anthropic" && cfg.Provider.AnthropicVersion == "" {
		cfg.Provider.AnthropicVersion = "2023-06-01"
	}
	if cfg.Provider.ExtraHeaders == nil {
		cfg.Provider.ExtraHeaders = map[string]string{}
	}
	for key, value := range cfg.Provider.ExtraHeaders {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			delete(cfg.Provider.ExtraHeaders, key)
			continue
		}
		if trimmedKey != key {
			delete(cfg.Provider.ExtraHeaders, key)
			cfg.Provider.ExtraHeaders[trimmedKey] = trimmedValue
			continue
		}
		cfg.Provider.ExtraHeaders[key] = trimmedValue
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 32
	}
	if !isSupportedProviderType(cfg.Provider.Type) {
		return errors.New("provider.type must be one of openai-compatible, openai, anthropic (or leave it empty with provider.auto_detect_type=true)")
	}
	switch cfg.ApprovalPolicy {
	case "", "on-request":
		cfg.ApprovalPolicy = "on-request"
	case "always", "never":
	default:
		return errors.New("approval_policy must be one of always, on-request, never")
	}
	if cfg.TokenQuota < 1 {
		cfg.TokenQuota = 5000
	}
	if strings.TrimSpace(cfg.TokenUsage.StorageType) == "" {
		cfg.TokenUsage.StorageType = "file"
	}
	if strings.TrimSpace(cfg.TokenUsage.StoragePath) == "" {
		cfg.TokenUsage.StoragePath = ".bytemind/token_usage.json"
	}
	if strings.TrimSpace(cfg.TokenUsage.BackupInterval) == "" {
		cfg.TokenUsage.BackupInterval = "1m"
	}
	if strings.TrimSpace(cfg.TokenUsage.MonitorInterval) == "" {
		cfg.TokenUsage.MonitorInterval = "30s"
	}
	if strings.TrimSpace(cfg.TokenUsage.DatabaseDriver) == "" {
		cfg.TokenUsage.DatabaseDriver = "sqlite3"
	}
	if cfg.TokenUsage.MaxSessions < 1 {
		cfg.TokenUsage.MaxSessions = 10000
	}
	if cfg.TokenUsage.RetentionDays < 1 {
		cfg.TokenUsage.RetentionDays = 30
	}
	if cfg.TokenUsage.AlertThreshold < 1 {
		cfg.TokenUsage.AlertThreshold = 1000000
	}
	if cfg.ContextBudget.WarningRatio <= 0 {
		return errors.New("context_budget.warning_ratio must be > 0")
	}
	if cfg.ContextBudget.CriticalRatio <= 0 || cfg.ContextBudget.CriticalRatio > 1 {
		return errors.New("context_budget.critical_ratio must be > 0 and <= 1")
	}
	if cfg.ContextBudget.WarningRatio >= cfg.ContextBudget.CriticalRatio {
		return errors.New("context_budget.warning_ratio must be < context_budget.critical_ratio")
	}
	if cfg.ContextBudget.MaxReactiveRetry < 0 {
		return errors.New("context_budget.max_reactive_retry must be >= 0")
	}
	return nil
}

func normalizeProviderType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "openai-compatible", "openai_compatible":
		return "openai-compatible"
	case "openai":
		return "openai"
	case "anthropic":
		return "anthropic"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func detectProviderType(cfg ProviderConfig) string {
	baseURL := strings.ToLower(strings.TrimSpace(cfg.BaseURL))
	apiPath := strings.ToLower(strings.TrimSpace(cfg.APIPath))
	authHeader := strings.ToLower(strings.TrimSpace(cfg.AuthHeader))

	if strings.Contains(apiPath, "/v1/messages") || strings.HasSuffix(strings.TrimRight(apiPath, "/"), "messages") {
		return "anthropic"
	}
	if strings.Contains(baseURL, "/v1/messages") || strings.HasSuffix(strings.TrimRight(baseURL, "/"), "/messages") {
		return "anthropic"
	}
	if hasHeaderName(cfg.ExtraHeaders, "anthropic-version") {
		return "anthropic"
	}
	if authHeader == "x-api-key" && strings.Contains(apiPath, "messages") {
		return "anthropic"
	}
	return "openai-compatible"
}

func hasHeaderName(headers map[string]string, target string) bool {
	for key := range headers {
		if strings.EqualFold(strings.TrimSpace(key), target) {
			return true
		}
	}
	return false
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

func defaultModel(providerType string) string {
	switch normalizeProviderType(providerType) {
	case "openai-compatible", "openai", "":
		return "GPT-5.4"
	default:
		return ""
	}
}
