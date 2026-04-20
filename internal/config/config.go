package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	envBytemindHome = "BYTEMIND_HOME"
	defaultHomeDir  = ".bytemind"
	defaultModelID  = "gpt-5.4-mini"
)

const (
	DefaultTokenQuota                    = 300000
	DefaultContextBudgetWarningRatio     = 0.85
	DefaultContextBudgetCriticalRatio    = 0.95
	DefaultContextBudgetMaxReactiveRetry = 1
)

type Config struct {
	Provider         ProviderConfig        `json:"provider"`
	ProviderRuntime  ProviderRuntimeConfig `json:"provider_runtime"`
	ApprovalPolicy   string                `json:"approval_policy"`
	ApprovalMode     string                `json:"approval_mode"`
	AwayPolicy       string                `json:"away_policy"`
	SandboxEnabled   bool                  `json:"sandbox_enabled"`
	WritableRoots    []string              `json:"writable_roots"`
	ExecAllowlist    []ExecAllowRule       `json:"exec_allowlist"`
	NetworkAllowlist []NetworkAllowRule    `json:"network_allowlist"`
	MaxIterations    int                   `json:"max_iterations"`
	Stream           bool                  `json:"stream"`
	UpdateCheck      UpdateCheckConfig     `json:"update_check"`
	TokenQuota       int                   `json:"token_quota"`
	TokenUsage       TokenUsageConfig      `json:"token_usage"`
	ContextBudget    ContextBudgetConfig   `json:"context_budget"`
}

type UpdateCheckConfig struct {
	Enabled bool `json:"enabled"`
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

type ExecAllowRule struct {
	Command     string   `json:"command"`
	ArgsPattern []string `json:"args_pattern"`
}

type NetworkAllowRule struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Scheme string `json:"scheme"`
}

func Default(workspace string) Config {
	return Config{
		Provider: ProviderConfig{
			Type:      "openai-compatible",
			BaseURL:   "https://api.openai.com/v1",
			Model:     defaultModelID,
			APIKeyEnv: "BYTEMIND_API_KEY",
		},
		ApprovalPolicy:   "on-request",
		ApprovalMode:     "interactive",
		AwayPolicy:       "auto_deny_continue",
		SandboxEnabled:   false,
		WritableRoots:    []string{},
		ExecAllowlist:    []ExecAllowRule{},
		NetworkAllowlist: []NetworkAllowRule{},
		MaxIterations:    32,
		Stream:           true,
		UpdateCheck: UpdateCheckConfig{
			Enabled: true,
		},
		TokenQuota: DefaultTokenQuota,
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
	if err := normalizeWritableRoots(workspace, &cfg); err != nil {
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
			Model:            defaultModelID,
			APIKeyEnv:        "BYTEMIND_API_KEY",
			AnthropicVersion: "2023-06-01",
		},
		ApprovalPolicy:   "on-request",
		ApprovalMode:     "interactive",
		AwayPolicy:       "auto_deny_continue",
		SandboxEnabled:   false,
		WritableRoots:    []string{},
		ExecAllowlist:    []ExecAllowRule{},
		NetworkAllowlist: []NetworkAllowRule{},
		MaxIterations:    32,
		Stream:           true,
		UpdateCheck: UpdateCheckConfig{
			Enabled: true,
		},
		TokenQuota: DefaultTokenQuota,
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
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_APPROVAL_MODE")); value != "" {
		cfg.ApprovalMode = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_AWAY_POLICY")); value != "" {
		cfg.AwayPolicy = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_SANDBOX_ENABLED")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.SandboxEnabled = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_WRITABLE_ROOTS")); value != "" {
		cfg.WritableRoots = splitPathList(value)
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_STREAM")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Stream = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_UPDATE_CHECK")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.UpdateCheck.Enabled = parsed
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
		cfg.Provider.Model = defaultModel(cfg.Provider.Type, cfg.Provider.BaseURL)
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
	if len(cfg.ProviderRuntime.Providers) == 0 {
		legacy := LegacyProviderRuntimeConfig(cfg.Provider)
		if strings.TrimSpace(cfg.ProviderRuntime.DefaultProvider) == "" {
			cfg.ProviderRuntime.DefaultProvider = legacy.DefaultProvider
		}
		if strings.TrimSpace(cfg.ProviderRuntime.DefaultModel) == "" {
			cfg.ProviderRuntime.DefaultModel = legacy.DefaultModel
		}
		cfg.ProviderRuntime.Providers = legacy.Providers
	}
	cfg.ProviderRuntime.DefaultProvider = strings.ToLower(strings.TrimSpace(cfg.ProviderRuntime.DefaultProvider))
	cfg.ProviderRuntime.DefaultModel = strings.TrimSpace(cfg.ProviderRuntime.DefaultModel)
	if cfg.ProviderRuntime.DefaultModel == "" {
		cfg.ProviderRuntime.DefaultModel = cfg.Provider.Model
	}
	if cfg.ProviderRuntime.Providers == nil {
		cfg.ProviderRuntime.Providers = map[string]ProviderConfig{}
	}
	normalizedProviders := make(map[string]ProviderConfig, len(cfg.ProviderRuntime.Providers))
	normalizedSources := make(map[string]string, len(cfg.ProviderRuntime.Providers))
	for id, providerCfg := range cfg.ProviderRuntime.Providers {
		normalizedID := strings.ToLower(strings.TrimSpace(id))
		if normalizedID == "" {
			return errors.New("provider_runtime.providers contains an empty provider id")
		}
		if existingSource, exists := normalizedSources[normalizedID]; exists {
			return fmt.Errorf("provider_runtime.providers has duplicate provider id after normalization: %q (from %q and %q)", normalizedID, existingSource, id)
		}
		providerCfg.Type = normalizeProviderType(providerCfg.Type)
		if providerCfg.Type == "" {
			if providerCfg.AutoDetectType {
				providerCfg.Type = detectProviderType(providerCfg)
			} else {
				providerCfg.Type = "openai-compatible"
			}
		}
		if strings.TrimSpace(providerCfg.BaseURL) == "" {
			providerCfg.BaseURL = defaultBaseURL(providerCfg.Type)
		}
		if strings.TrimSpace(providerCfg.Model) == "" {
			providerCfg.Model = cfg.ProviderRuntime.DefaultModel
		}
		if providerCfg.APIKeyEnv == "" {
			providerCfg.APIKeyEnv = cfg.Provider.APIKeyEnv
		}
		if strings.TrimSpace(providerCfg.APIKey) == "" {
			providerCfg.APIKey = cfg.Provider.APIKey
		}
		providerCfg.APIPath = strings.TrimSpace(providerCfg.APIPath)
		providerCfg.AuthHeader = strings.TrimSpace(providerCfg.AuthHeader)
		providerCfg.AuthScheme = strings.TrimSpace(providerCfg.AuthScheme)
		providerCfg.AnthropicVersion = strings.TrimSpace(providerCfg.AnthropicVersion)
		if providerCfg.Type == "anthropic" && providerCfg.AnthropicVersion == "" {
			providerCfg.AnthropicVersion = "2023-06-01"
		}
		if providerCfg.ExtraHeaders == nil {
			providerCfg.ExtraHeaders = map[string]string{}
		}
		normalizedProviders[normalizedID] = providerCfg
		normalizedSources[normalizedID] = id
	}
	cfg.ProviderRuntime.Providers = normalizedProviders
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
	switch strings.TrimSpace(cfg.ApprovalMode) {
	case "", "interactive":
		cfg.ApprovalMode = "interactive"
	case "away":
	default:
		return errors.New("approval_mode must be one of interactive, away")
	}
	switch strings.TrimSpace(cfg.AwayPolicy) {
	case "", "auto_deny_continue":
		cfg.AwayPolicy = "auto_deny_continue"
	case "fail_fast":
	default:
		return errors.New("away_policy must be one of auto_deny_continue, fail_fast")
	}
	if err := normalizeSandboxPolicy(cfg); err != nil {
		return err
	}
	if cfg.TokenQuota < 1 {
		cfg.TokenQuota = DefaultTokenQuota
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

func normalizeWritableRoots(workspace string, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	absWorkspace, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil {
		return err
	}
	absWorkspace = filepath.Clean(absWorkspace)
	workspaceKey := normalizePathKey(absWorkspace)

	if len(cfg.WritableRoots) == 0 {
		cfg.WritableRoots = []string{}
		return nil
	}
	seen := map[string]struct{}{workspaceKey: {}}
	normalized := make([]string, 0, len(cfg.WritableRoots))
	for _, root := range cfg.WritableRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(absWorkspace, root)
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		absRoot = filepath.Clean(absRoot)
		key := normalizePathKey(absRoot)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, absRoot)
	}
	cfg.WritableRoots = normalized
	return nil
}

func normalizeSandboxPolicy(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	normalizedExec := make([]ExecAllowRule, 0, len(cfg.ExecAllowlist))
	seenExec := make(map[string]struct{}, len(cfg.ExecAllowlist))
	for _, rule := range cfg.ExecAllowlist {
		commandTokens := strings.Fields(strings.TrimSpace(rule.Command))
		if len(commandTokens) == 0 {
			return errors.New("exec_allowlist.command cannot be empty")
		}
		command := commandTokens[0]
		patternInputs := make([]string, 0, len(commandTokens)-1+len(rule.ArgsPattern))
		if len(commandTokens) > 1 {
			patternInputs = append(patternInputs, commandTokens[1:]...)
		}
		patternInputs = append(patternInputs, rule.ArgsPattern...)
		patterns := normalizeStringList(patternInputs)
		key := strings.ToLower(command) + "\x00" + strings.Join(patterns, "\x00")
		if _, exists := seenExec[key]; exists {
			continue
		}
		seenExec[key] = struct{}{}
		normalizedExec = append(normalizedExec, ExecAllowRule{
			Command:     command,
			ArgsPattern: patterns,
		})
	}
	sort.Slice(normalizedExec, func(i, j int) bool {
		a := strings.ToLower(normalizedExec[i].Command)
		b := strings.ToLower(normalizedExec[j].Command)
		if a != b {
			return a < b
		}
		return strings.Join(normalizedExec[i].ArgsPattern, "\x00") < strings.Join(normalizedExec[j].ArgsPattern, "\x00")
	})
	cfg.ExecAllowlist = normalizedExec

	normalizedNetwork := make([]NetworkAllowRule, 0, len(cfg.NetworkAllowlist))
	seenNetwork := make(map[string]struct{}, len(cfg.NetworkAllowlist))
	for _, rule := range cfg.NetworkAllowlist {
		host := strings.ToLower(strings.TrimSpace(rule.Host))
		scheme := strings.ToLower(strings.TrimSpace(rule.Scheme))
		if host == "" {
			return errors.New("network_allowlist.host cannot be empty")
		}
		if scheme == "" {
			return errors.New("network_allowlist.scheme cannot be empty")
		}
		if rule.Port < 1 || rule.Port > 65535 {
			return errors.New("network_allowlist.port must be between 1 and 65535")
		}
		key := host + "\x00" + strconv.Itoa(rule.Port) + "\x00" + scheme
		if _, exists := seenNetwork[key]; exists {
			continue
		}
		seenNetwork[key] = struct{}{}
		normalizedNetwork = append(normalizedNetwork, NetworkAllowRule{
			Host:   host,
			Port:   rule.Port,
			Scheme: scheme,
		})
	}
	sort.Slice(normalizedNetwork, func(i, j int) bool {
		a := normalizedNetwork[i]
		b := normalizedNetwork[j]
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		if a.Port != b.Port {
			return a.Port < b.Port
		}
		return a.Scheme < b.Scheme
	})
	cfg.NetworkAllowlist = normalizedNetwork
	return nil
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func splitPathList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func normalizePathKey(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
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

func defaultModel(providerType, baseURL string) string {
	switch normalizeProviderType(providerType) {
	case "openai-compatible", "openai", "":
		if strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), "deepseek.com") {
			return "deepseek-chat"
		}
		return defaultModelID
	default:
		return ""
	}
}
