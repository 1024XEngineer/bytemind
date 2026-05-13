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

	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

const (
	envBytemindHome            = "BYTEMIND_HOME"
	defaultHomeDir             = ".bytemind"
	defaultProviderID          = "deepseek"
	defaultModelID             = "deepseek-v4-flash"
	legacyOpenAIDefaultModelID = "gpt-5.4-mini"
	// EnvAllowAwayFullAccessCompat is a temporary migration gate that permits
	// approval_mode=away to map to full_access.
	EnvAllowAwayFullAccessCompat = "BYTEMIND_ALLOW_AWAY_FULL_ACCESS"
)

const (
	DefaultTokenQuota                    = 0
	DefaultMaxIterations                 = 64
	DefaultContextBudgetWarningRatio     = 0.85
	DefaultContextBudgetCriticalRatio    = 0.95
	DefaultContextBudgetMaxReactiveRetry = 1
	DefaultContextBudgetFallback         = 128000
	DefaultMCPSyncTTLSeconds             = 30
	DefaultMCPStartupTimeoutSeconds      = 20
	DefaultMCPCallTimeoutSeconds         = 60
	DefaultMCPMaxConcurrency             = 4
)

type Config struct {
	Provider          ProviderConfig        `json:"provider"`
	ProviderRuntime   ProviderRuntimeConfig `json:"provider_runtime"`
	ApprovalPolicy    string                `json:"approval_policy"`
	ApprovalMode      string                `json:"approval_mode"`
	AwayPolicy        string                `json:"away_policy"`
	SandboxEnabled    bool                  `json:"sandbox_enabled"`
	SystemSandboxMode string                `json:"system_sandbox_mode"`
	WritableRoots     []string              `json:"writable_roots"`
	ExecAllowlist     []ExecAllowRule       `json:"exec_allowlist"`
	NetworkAllowlist  []NetworkAllowRule    `json:"network_allowlist"`
	Notifications     NotificationsConfig   `json:"notifications"`
	MaxIterations     int                   `json:"max_iterations"`
	Stream            bool                  `json:"stream"`
	UpdateCheck       UpdateCheckConfig     `json:"update_check"`
	TokenQuota        int                   `json:"token_quota"`
	ContextBudget     ContextBudgetConfig   `json:"context_budget"`
	MCP               MCPConfig             `json:"-"`
}

type UpdateCheckConfig struct {
	Enabled bool `json:"enabled"`
}

type ProviderConfig struct {
	Type             string            `json:"type"`
	Family           string            `json:"family"`
	AutoDetectType   bool              `json:"auto_detect_type"`
	BaseURL          string            `json:"base_url"`
	APIPath          string            `json:"api_path"`
	Model            string            `json:"model"`
	Models           []string          `json:"models,omitempty"`
	APIKey           string            `json:"api_key"`
	APIKeyEnv        string            `json:"api_key_env"`
	AuthHeader       string            `json:"auth_header"`
	AuthScheme       string            `json:"auth_scheme"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	AnthropicVersion string            `json:"anthropic_version"`
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

type NotificationsConfig struct {
	Desktop DesktopNotificationConfig `json:"desktop"`
}

type DesktopNotificationConfig struct {
	Enabled            bool `json:"enabled"`
	OnApprovalRequired bool `json:"on_approval_required"`
	OnRunCompleted     bool `json:"on_run_completed"`
	OnRunFailed        bool `json:"on_run_failed"`
	OnRunCanceled      bool `json:"on_run_canceled"`
	CooldownSeconds    int  `json:"cooldown_seconds"`
}

type MCPConfig struct {
	Enabled        bool              `json:"enabled"`
	SyncTTLSeconds int               `json:"sync_ttl_s"`
	Servers        []MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	ID                    string                  `json:"id"`
	Name                  string                  `json:"name"`
	Enabled               *bool                   `json:"enabled,omitempty"`
	Transport             MCPTransportConfig      `json:"transport"`
	AutoStart             *bool                   `json:"auto_start,omitempty"`
	StartupTimeoutSeconds int                     `json:"startup_timeout_s"`
	CallTimeoutSeconds    int                     `json:"call_timeout_s"`
	MaxConcurrency        int                     `json:"max_concurrency"`
	ToolOverrides         []MCPToolOverrideConfig `json:"tool_overrides"`
	ProtocolVersion       string                  `json:"protocol_version"`
	ProtocolVersions      []string                `json:"protocol_versions"`
}

type MCPTransportConfig struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	CWD     string            `json:"cwd"`
}

type MCPToolOverrideConfig struct {
	ToolName        string   `json:"tool_name"`
	SafetyClass     string   `json:"safety_class,omitempty"`
	ReadOnly        *bool    `json:"read_only,omitempty"`
	Destructive     *bool    `json:"destructive,omitempty"`
	AllowedModes    []string `json:"allowed_modes,omitempty"`
	DefaultTimeoutS int      `json:"default_timeout_s,omitempty"`
	MaxTimeoutS     int      `json:"max_timeout_s,omitempty"`
	MaxResultChars  int      `json:"max_result_chars,omitempty"`
}

func (s MCPServerConfig) EnabledValue() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

func (s MCPServerConfig) AutoStartValue() bool {
	if s.AutoStart == nil {
		return true
	}
	return *s.AutoStart
}

func Default(workspace string) Config {
	return Config{
		Provider: ProviderConfig{
			Type:      "openai-compatible",
			BaseURL:   "https://api.deepseek.com",
			Model:     defaultModelID,
			Models:    []string{defaultModelID, "deepseek-v4-pro"},
			APIKeyEnv: "DEEPSEEK_API_KEY",
		},
		ProviderRuntime: ProviderRuntimeConfig{
			CurrentProvider: defaultProviderID,
			DefaultProvider: defaultProviderID,
			DefaultModel:    defaultModelID,
			Providers: map[string]ProviderConfig{
				defaultProviderID: {
					Type:      "openai-compatible",
					BaseURL:   "https://api.deepseek.com",
					Model:     defaultModelID,
					Models:    []string{defaultModelID, "deepseek-v4-pro"},
					APIKeyEnv: "DEEPSEEK_API_KEY",
				},
			},
		},
		ApprovalPolicy:    "on-request",
		ApprovalMode:      "interactive",
		AwayPolicy:        "auto_deny_continue",
		SandboxEnabled:    false,
		SystemSandboxMode: "off",
		WritableRoots:     []string{},
		ExecAllowlist:     []ExecAllowRule{},
		NetworkAllowlist:  []NetworkAllowRule{},
		Notifications: NotificationsConfig{
			Desktop: DesktopNotificationConfig{
				Enabled:            true,
				OnApprovalRequired: true,
				OnRunCompleted:     true,
				OnRunFailed:        true,
				OnRunCanceled:      false,
				CooldownSeconds:    3,
			},
		},
		MaxIterations: DefaultMaxIterations,
		Stream:        true,
		UpdateCheck: UpdateCheckConfig{
			Enabled: true,
		},
		TokenQuota: DefaultTokenQuota,
		ContextBudget: ContextBudgetConfig{
			WarningRatio:     DefaultContextBudgetWarningRatio,
			CriticalRatio:    DefaultContextBudgetCriticalRatio,
			MaxReactiveRetry: DefaultContextBudgetMaxReactiveRetry,
		},
		MCP: MCPConfig{
			Enabled:        false,
			SyncTTLSeconds: DefaultMCPSyncTTLSeconds,
			Servers:        nil,
		},
	}
}

func Load(workspace, configPath string) (Config, error) {
	return LoadWithMCPConfigPath(workspace, configPath, "")
}

func LoadWithMCPConfigPath(workspace, configPath, mcpConfigPath string) (Config, error) {
	cfg := Default(workspace)
	var userProviderOverride map[string]json.RawMessage

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
			userProviderOverride, err = loadUserProviderOverride(userConfig)
			if err != nil {
				return cfg, err
			}
		}

		if projectConfig := resolveProjectConfigPath(workspace); projectConfig != "" {
			if err := mergeConfigFromFile(projectConfig, &cfg); err != nil {
				return cfg, err
			}
		}

		if err := mergeProviderOverride(userProviderOverride, &cfg); err != nil {
			return cfg, err
		}
	}

	if strings.TrimSpace(mcpConfigPath) != "" {
		path, err := filepath.Abs(mcpConfigPath)
		if err != nil {
			return cfg, err
		}
		if err := mergeMCPConfigFromFile(path, &cfg.MCP); err != nil {
			return cfg, err
		}
	} else {
		if userMCPConfig, err := resolveUserMCPConfigPath(); err != nil {
			return cfg, err
		} else if userMCPConfig != "" {
			if err := mergeMCPConfigFromFile(userMCPConfig, &cfg.MCP); err != nil {
				return cfg, err
			}
		}
		if projectMCPConfig := resolveProjectMCPConfigPath(workspace); projectMCPConfig != "" {
			if err := mergeMCPConfigFromFile(projectMCPConfig, &cfg.MCP); err != nil {
				return cfg, err
			}
		}
	}

	overrideProvider := applyEnv(&cfg)
	if err := normalize(&cfg); err != nil {
		return cfg, err
	}
	if overrideProvider {
		applyProviderEnvOverrides(&cfg)
		cfg.ProviderRuntime = SyncProviderRuntimeWithProvider(cfg.ProviderRuntime, cfg.Provider)
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
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			return value
		}
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
			BaseURL:          "https://api.deepseek.com",
			Model:            defaultModelID,
			Models:           []string{defaultModelID, "deepseek-v4-pro"},
			APIKeyEnv:        "DEEPSEEK_API_KEY",
			AnthropicVersion: "2023-06-01",
		},
		ProviderRuntime: ProviderRuntimeConfig{
			CurrentProvider: defaultProviderID,
			DefaultProvider: defaultProviderID,
			DefaultModel:    defaultModelID,
			Providers: map[string]ProviderConfig{
				defaultProviderID: {
					Type:             "openai-compatible",
					BaseURL:          "https://api.deepseek.com",
					Model:            defaultModelID,
					Models:           []string{defaultModelID, "deepseek-v4-pro"},
					APIKeyEnv:        "DEEPSEEK_API_KEY",
					AnthropicVersion: "2023-06-01",
				},
			},
		},
		ApprovalPolicy:    "on-request",
		ApprovalMode:      "interactive",
		AwayPolicy:        "auto_deny_continue",
		SandboxEnabled:    false,
		SystemSandboxMode: "off",
		WritableRoots:     []string{},
		ExecAllowlist:     []ExecAllowRule{},
		NetworkAllowlist:  []NetworkAllowRule{},
		Notifications: NotificationsConfig{
			Desktop: DesktopNotificationConfig{
				Enabled:            true,
				OnApprovalRequired: true,
				OnRunCompleted:     true,
				OnRunFailed:        true,
				OnRunCanceled:      false,
				CooldownSeconds:    3,
			},
		},
		MaxIterations: DefaultMaxIterations,
		Stream:        true,
		UpdateCheck: UpdateCheckConfig{
			Enabled: true,
		},
		TokenQuota: DefaultTokenQuota,
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

func resolveProjectMCPConfigPath(workspace string) string {
	candidates := []string{
		filepath.Join(workspace, ".bytemind", "mcp.json"),
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

func resolveUserMCPConfigPath() (string, error) {
	home, err := ResolveHomeDir()
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(home, "mcp.json")
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
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, ok := raw["provider_runtime"]; ok {
		cfg.ProviderRuntime = ProviderRuntimeConfig{}
	} else if _, ok := raw["provider"]; ok {
		cfg.ProviderRuntime = ProviderRuntimeConfig{}
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return err
	}
	return nil
}

func loadUserProviderOverride(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if !hasProviderCredential(raw) {
		return nil, nil
	}
	return raw, nil
}

func mergeProviderOverride(raw map[string]json.RawMessage, cfg *Config) error {
	if raw == nil || cfg == nil {
		return nil
	}
	providerOverridden := false
	if providerRaw, ok := raw["provider"]; ok {
		if err := json.Unmarshal(providerRaw, &cfg.Provider); err != nil {
			return err
		}
		providerOverridden = true
	}
	if runtimeRaw, ok := raw["provider_runtime"]; ok {
		if err := json.Unmarshal(runtimeRaw, &cfg.ProviderRuntime); err != nil {
			return err
		}
	} else if providerOverridden {
		cfg.ProviderRuntime = ProviderRuntimeConfig{}
	}
	return nil
}

func hasProviderCredential(raw map[string]json.RawMessage) bool {
	if providerRaw, ok := raw["provider"]; ok && providerRawHasCredential(providerRaw) {
		return true
	}
	runtimeRaw, ok := raw["provider_runtime"]
	if !ok {
		return false
	}
	var runtime struct {
		Providers map[string]json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(runtimeRaw, &runtime); err != nil {
		return false
	}
	for _, providerRaw := range runtime.Providers {
		if providerRawHasCredential(providerRaw) {
			return true
		}
	}
	return false
}

func providerRawHasCredential(raw json.RawMessage) bool {
	var provider map[string]json.RawMessage
	if err := json.Unmarshal(raw, &provider); err != nil {
		return false
	}
	if valueRaw, ok := provider["api_key"]; ok {
		var value string
		if err := json.Unmarshal(valueRaw, &value); err == nil && strings.TrimSpace(value) != "" {
			return true
		}
	}
	if valueRaw, ok := provider["api_key_env"]; ok {
		var value string
		if err := json.Unmarshal(valueRaw, &value); err == nil && strings.TrimSpace(os.Getenv(strings.TrimSpace(value))) != "" {
			return true
		}
	}
	return false
}

func mergeMCPConfigFromFile(path string, cfg *MCPConfig) error {
	if cfg == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err == nil {
		if _, ok := root["mcp"]; ok {
			return errors.New("mcp config files must use top-level MCP fields; move mcp.* to the root")
		}
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return err
	}
	return nil
}

func applyEnv(cfg *Config) (providerOverride bool) {
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_PROVIDER_TYPE")); value != "" {
		cfg.Provider.Type = value
		providerOverride = true
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_PROVIDER_FAMILY")); value != "" {
		cfg.Provider.Family = value
		providerOverride = true
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_PROVIDER_AUTO_DETECT_TYPE")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Provider.AutoDetectType = parsed
			providerOverride = true
		}
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_BASE_URL")); value != "" {
		cfg.Provider.BaseURL = value
		providerOverride = true
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_MODEL")); value != "" {
		cfg.Provider.Model = value
		providerOverride = true
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_API_KEY")); value != "" {
		cfg.Provider.APIKey = value
		providerOverride = true
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_API_KEY_ENV")); value != "" {
		cfg.Provider.APIKeyEnv = value
		providerOverride = true
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
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_SYSTEM_SANDBOX_MODE")); value != "" {
		cfg.SystemSandboxMode = value
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
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_DESKTOP_NOTIFY")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Notifications.Desktop.Enabled = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_NOTIFY_APPROVAL")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Notifications.Desktop.OnApprovalRequired = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_NOTIFY_RUN_COMPLETED")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Notifications.Desktop.OnRunCompleted = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_NOTIFY_RUN_FAILED")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Notifications.Desktop.OnRunFailed = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_NOTIFY_RUN_CANCELED")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Notifications.Desktop.OnRunCanceled = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_NOTIFY_COOLDOWN_SECONDS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.Notifications.Desktop.CooldownSeconds = parsed
		}
	}
	return providerOverride
}

func applyProviderEnvOverrides(cfg *Config) {
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_PROVIDER_TYPE")); value != "" {
		cfg.Provider.Type = value
	}
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_PROVIDER_FAMILY")); value != "" {
		cfg.Provider.Family = value
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
	cfg.Provider.Type = normalizeProviderType(cfg.Provider.Type)
	cfg.Provider.Family = normalizeProviderFamily(cfg.Provider.Family)
}

// NormalizeApprovalMode validates and normalizes approval_mode values.
// By default, legacy away mode is blocked to prevent silent privilege
// escalation to full_access. Operators can temporarily enable migration by
// setting BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true.
func NormalizeApprovalMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "interactive":
		return "interactive", nil
	case "full_access":
		return "full_access", nil
	case "away":
		if allowAwayFullAccessCompat() {
			return "full_access", nil
		}
		return "", fmt.Errorf("approval_mode=away is blocked by default to prevent silent privilege escalation; migrate to approval_mode=interactive or approval_mode=full_access, or set %s=true for temporary migration", EnvAllowAwayFullAccessCompat)
	default:
		return "", errors.New("approval_mode must be one of interactive, full_access")
	}
}

func allowAwayFullAccessCompat() bool {
	raw := strings.TrimSpace(os.Getenv(EnvAllowAwayFullAccessCompat))
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return enabled
}

func normalize(cfg *Config) error {
	explicitCurrentProvider := strings.TrimSpace(cfg.ProviderRuntime.CurrentProvider) != ""
	cfg.Provider.Type = normalizeProviderType(cfg.Provider.Type)
	cfg.Provider.Family = normalizeProviderFamily(cfg.Provider.Family)
	if cfg.Provider.Type == "" {
		if cfg.Provider.AutoDetectType {
			cfg.Provider.Type = detectProviderType(cfg.Provider)
		} else {
			cfg.Provider.Type = "openai-compatible"
		}
	}
	if cfg.Provider.BaseURL == "" || usesOpenAIDefaultBaseURLForNativeProvider(cfg.Provider.Type, cfg.Provider.BaseURL) {
		cfg.Provider.BaseURL = defaultBaseURL(cfg.Provider.Type)
	}
	if strings.TrimSpace(cfg.Provider.BaseURL) == "" {
		return errors.New("provider.base_url is required")
	}
	if strings.TrimSpace(cfg.Provider.Model) == "" || usesOpenAIDefaultModelForNativeProvider(cfg.Provider.Type, cfg.Provider.Model) {
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
	cfg.Provider.Models = normalizeStringList(cfg.Provider.Models)
	if len(cfg.ProviderRuntime.Providers) == 0 {
		legacy := LegacyProviderRuntimeConfig(cfg.Provider)
		if strings.TrimSpace(cfg.ProviderRuntime.CurrentProvider) == "" && strings.TrimSpace(cfg.ProviderRuntime.DefaultProvider) == "" {
			cfg.ProviderRuntime.CurrentProvider = legacy.CurrentProvider
		}
		if strings.TrimSpace(cfg.ProviderRuntime.DefaultProvider) == "" {
			cfg.ProviderRuntime.DefaultProvider = legacy.DefaultProvider
		}
		if strings.TrimSpace(cfg.ProviderRuntime.DefaultModel) == "" {
			cfg.ProviderRuntime.DefaultModel = legacy.DefaultModel
		}
		cfg.ProviderRuntime.Providers = legacy.Providers
	}
	cfg.ProviderRuntime.CurrentProvider = strings.ToLower(strings.TrimSpace(cfg.ProviderRuntime.CurrentProvider))
	cfg.ProviderRuntime.DefaultProvider = strings.ToLower(strings.TrimSpace(cfg.ProviderRuntime.DefaultProvider))
	if cfg.ProviderRuntime.CurrentProvider == "" {
		cfg.ProviderRuntime.CurrentProvider = cfg.ProviderRuntime.DefaultProvider
	}
	if cfg.ProviderRuntime.DefaultProvider == "" {
		cfg.ProviderRuntime.DefaultProvider = cfg.ProviderRuntime.CurrentProvider
	}
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
		providerCfg.Family = normalizeProviderFamily(providerCfg.Family)
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
		providerCfg.Models = normalizeStringList(providerCfg.Models)
		normalizedProviders[normalizedID] = providerCfg
		normalizedSources[normalizedID] = id
	}
	cfg.ProviderRuntime.Providers = normalizedProviders
	if explicitCurrentProvider {
		if _, ok := cfg.ProviderRuntime.Providers[cfg.ProviderRuntime.CurrentProvider]; !ok {
			return fmt.Errorf("provider_runtime.current_provider %q is not configured", cfg.ProviderRuntime.CurrentProvider)
		}
	}
	if providerID, providerCfg, ok := SelectedProviderConfig(cfg.ProviderRuntime); ok {
		cfg.ProviderRuntime.CurrentProvider = providerID
		cfg.ProviderRuntime.DefaultProvider = providerID
		if model := strings.TrimSpace(providerCfg.Model); model != "" {
			cfg.ProviderRuntime.DefaultModel = model
		}
		cfg.Provider = providerCfg
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
		cfg.MaxIterations = DefaultMaxIterations
	}
	if !isSupportedProviderType(cfg.Provider.Type) {
		return errors.New("provider.type must be one of openai-compatible, openai, anthropic, gemini (or leave it empty with provider.auto_detect_type=true)")
	}
	switch cfg.ApprovalPolicy {
	case "", "on-request":
		cfg.ApprovalPolicy = "on-request"
	case "always", "never":
	default:
		return errors.New("approval_policy must be one of always, on-request, never")
	}
	normalizedApprovalMode, err := NormalizeApprovalMode(cfg.ApprovalMode)
	if err != nil {
		return err
	}
	cfg.ApprovalMode = normalizedApprovalMode
	switch strings.TrimSpace(cfg.AwayPolicy) {
	case "", "auto_deny_continue":
		cfg.AwayPolicy = "auto_deny_continue"
	case "fail_fast":
	default:
		return errors.New("away_policy must be one of auto_deny_continue, fail_fast")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.SystemSandboxMode)) {
	case "", "off":
		cfg.SystemSandboxMode = "off"
	case "best_effort":
		cfg.SystemSandboxMode = "best_effort"
	case "required":
		cfg.SystemSandboxMode = "required"
	default:
		return errors.New("system_sandbox_mode must be one of off, best_effort, required")
	}
	if cfg.SystemSandboxMode != "off" && !cfg.SandboxEnabled {
		return errors.New("system_sandbox_mode requires sandbox_enabled=true")
	}
	if err := normalizeSandboxPolicy(cfg); err != nil {
		return err
	}
	if cfg.TokenQuota < 0 {
		cfg.TokenQuota = 0
	}
	if cfg.Notifications.Desktop.CooldownSeconds < 0 {
		return errors.New("notifications.desktop.cooldown_seconds must be >= 0")
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
	if err := normalizeMCPConfig(&cfg.MCP); err != nil {
		return err
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
	case "gemini", "google", "google-gemini":
		return "gemini"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeProviderFamily(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "moonshot-ai":
		return "moonshot"
	case "z-ai", "z.ai", "zhipu-ai":
		return "zai"
	default:
		return value
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
	if authHeader == "x-goog-api-key" || hasHeaderName(cfg.ExtraHeaders, "x-goog-api-key") {
		return "gemini"
	}
	if strings.Contains(baseURL, "generativelanguage.googleapis.com") && !strings.Contains(baseURL, "/openai") {
		return "gemini"
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
	case "openai-compatible", "openai", "anthropic", "gemini":
		return true
	default:
		return false
	}
}

func defaultBaseURL(providerType string) string {
	switch normalizeProviderType(providerType) {
	case "anthropic":
		return "https://api.anthropic.com"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta"
	default:
		return "https://api.openai.com/v1"
	}
}

func usesOpenAIDefaultBaseURLForNativeProvider(providerType, baseURL string) bool {
	switch normalizeProviderType(providerType) {
	case "anthropic", "gemini":
		baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		return strings.EqualFold(baseURL, "https://api.openai.com/v1") ||
			strings.EqualFold(baseURL, "https://api.deepseek.com")
	default:
		return false
	}
}

func usesOpenAIDefaultModelForNativeProvider(providerType, model string) bool {
	switch normalizeProviderType(providerType) {
	case "anthropic", "gemini":
		model = strings.TrimSpace(model)
		return model == defaultModelID || model == legacyOpenAIDefaultModelID
	default:
		return false
	}
}

func defaultModel(providerType, baseURL string) string {
	switch normalizeProviderType(providerType) {
	case "openai-compatible", "openai", "":
		if strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), "deepseek.com") {
			return defaultModelID
		}
		return legacyOpenAIDefaultModelID
	case "gemini":
		return "gemini-2.5-flash"
	default:
		return ""
	}
}

func normalizeMCPConfig(cfg *MCPConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.SyncTTLSeconds <= 0 {
		cfg.SyncTTLSeconds = DefaultMCPSyncTTLSeconds
	}
	if len(cfg.Servers) == 0 {
		cfg.Servers = nil
		return nil
	}
	normalized := make([]MCPServerConfig, 0, len(cfg.Servers))
	normalizedSources := make(map[string]string, len(cfg.Servers))
	for _, server := range cfg.Servers {
		originalID := strings.TrimSpace(server.ID)
		server.ID = normalizeMCPServerID(server.ID)
		if server.ID == "" {
			return errors.New("mcp.servers contains an empty server id")
		}
		if existingSource, exists := normalizedSources[server.ID]; exists {
			return fmt.Errorf("mcp.servers has duplicate server id after normalization: %q (from %q and %q)", server.ID, existingSource, originalID)
		}
		normalizedSources[server.ID] = originalID
		server.Name = strings.TrimSpace(server.Name)
		if server.Name == "" {
			server.Name = server.ID
		}
		if server.Enabled == nil {
			value := true
			server.Enabled = &value
		}
		if server.AutoStart == nil {
			value := true
			server.AutoStart = &value
		}
		server.Transport.Type = strings.ToLower(strings.TrimSpace(server.Transport.Type))
		if server.Transport.Type == "" {
			server.Transport.Type = "stdio"
		}
		if server.Transport.Type != "stdio" {
			return fmt.Errorf("mcp server %q uses unsupported transport.type %q (expected stdio)", server.ID, server.Transport.Type)
		}
		server.Transport.Command = strings.TrimSpace(server.Transport.Command)
		server.Transport.CWD = strings.TrimSpace(server.Transport.CWD)
		if server.EnabledValue() && server.Transport.Command == "" {
			return fmt.Errorf("mcp server %q requires transport.command for stdio mode", server.ID)
		}
		if server.Transport.Args == nil {
			server.Transport.Args = []string{}
		}
		for i, arg := range server.Transport.Args {
			server.Transport.Args[i] = strings.TrimSpace(arg)
		}
		cleanEnv := map[string]string{}
		for key, value := range server.Transport.Env {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			cleanEnv[key] = strings.TrimSpace(value)
		}
		server.Transport.Env = cleanEnv
		if server.StartupTimeoutSeconds <= 0 {
			server.StartupTimeoutSeconds = DefaultMCPStartupTimeoutSeconds
		}
		if server.CallTimeoutSeconds <= 0 {
			server.CallTimeoutSeconds = DefaultMCPCallTimeoutSeconds
		}
		if server.MaxConcurrency < 1 {
			server.MaxConcurrency = DefaultMCPMaxConcurrency
		}
		server.ProtocolVersion = strings.TrimSpace(server.ProtocolVersion)
		server.ProtocolVersions = normalizeMCPProtocolVersions(server.ProtocolVersion, server.ProtocolVersions)
		normalizedOverrides, err := normalizeMCPToolOverrides(server.ID, server.ToolOverrides)
		if err != nil {
			return err
		}
		server.ToolOverrides = normalizedOverrides
		normalized = append(normalized, server)
	}
	cfg.Servers = normalized
	return nil
}

func normalizeMCPServerID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	raw = replacer.Replace(raw)
	raw = strings.Trim(raw, "-_")
	return raw
}

func normalizeMCPProtocolVersions(primary string, extras []string) []string {
	versions := make([]string, 0, 1+len(extras))
	if strings.TrimSpace(primary) != "" {
		versions = append(versions, strings.TrimSpace(primary))
	}
	versions = append(versions, extras...)
	normalized := make([]string, 0, len(versions))
	seen := map[string]struct{}{}
	for _, version := range versions {
		version = strings.TrimSpace(version)
		if version == "" {
			continue
		}
		if _, ok := seen[version]; ok {
			continue
		}
		seen[version] = struct{}{}
		normalized = append(normalized, version)
	}
	return normalized
}

func normalizeMCPToolOverrides(serverID string, overrides []MCPToolOverrideConfig) ([]MCPToolOverrideConfig, error) {
	if len(overrides) == 0 {
		return nil, nil
	}
	normalized := make([]MCPToolOverrideConfig, 0, len(overrides))
	seen := map[string]struct{}{}
	for _, override := range overrides {
		override.ToolName = strings.TrimSpace(override.ToolName)
		if override.ToolName == "" {
			return nil, fmt.Errorf("mcp server %q has tool override with empty tool_name", serverID)
		}
		toolKey := strings.ToLower(override.ToolName)
		if _, exists := seen[toolKey]; exists {
			return nil, fmt.Errorf("mcp server %q has duplicate tool override for %q", serverID, override.ToolName)
		}
		seen[toolKey] = struct{}{}
		override.SafetyClass = strings.ToLower(strings.TrimSpace(override.SafetyClass))
		normalizedModes := make([]string, 0, len(override.AllowedModes))
		modeSeen := map[planpkg.AgentMode]struct{}{}
		for _, mode := range override.AllowedModes {
			normalizedMode := planpkg.NormalizeMode(strings.TrimSpace(mode))
			if normalizedMode == "" {
				return nil, fmt.Errorf("mcp server %q has tool override %q with invalid allowed mode %q", serverID, override.ToolName, mode)
			}
			if _, exists := modeSeen[normalizedMode]; exists {
				continue
			}
			modeSeen[normalizedMode] = struct{}{}
			normalizedModes = append(normalizedModes, string(normalizedMode))
		}
		override.AllowedModes = normalizedModes
		if override.DefaultTimeoutS < 0 || override.MaxTimeoutS < 0 || override.MaxResultChars < 0 {
			return nil, fmt.Errorf("mcp server %q has tool override %q with negative timeout/result limits", serverID, override.ToolName)
		}
		if err := validateMCPToolOverride(override); err != nil {
			return nil, fmt.Errorf("mcp server %q tool override %q is invalid: %w", serverID, override.ToolName, err)
		}
		normalized = append(normalized, override)
	}
	return normalized, nil
}

func validateMCPToolOverride(override MCPToolOverrideConfig) error {
	switch override.SafetyClass {
	case "", "safe", "moderate", "sensitive", "destructive":
	default:
		return fmt.Errorf("unsupported safety_class %q", override.SafetyClass)
	}

	if override.ReadOnly != nil && override.Destructive != nil && *override.ReadOnly && *override.Destructive {
		return fmt.Errorf("read_only and destructive cannot both be true")
	}
	if override.DefaultTimeoutS > 0 && override.MaxTimeoutS > 0 && override.DefaultTimeoutS > override.MaxTimeoutS {
		return fmt.Errorf("default_timeout_s must be <= max_timeout_s")
	}
	return nil
}
