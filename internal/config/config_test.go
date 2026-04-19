package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUsesEnvOverrides(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	t.Setenv("BYTEMIND_MODEL", "override-model")
	t.Setenv("BYTEMIND_API_KEY", "secret")
	t.Setenv("BYTEMIND_TOKEN_QUOTA", "88000")
	t.Setenv("BYTEMIND_PROVIDER_TYPE", "anthropic")
	t.Setenv("BYTEMIND_PROVIDER_AUTO_DETECT_TYPE", "true")
	t.Setenv("BYTEMIND_STREAM", "false")
	t.Setenv("BYTEMIND_APPROVAL_MODE", "away")
	t.Setenv("BYTEMIND_AWAY_POLICY", "fail_fast")

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Model != "override-model" {
		t.Fatalf("expected override model, got %q", cfg.Provider.Model)
	}
	if cfg.Provider.Type != "anthropic" {
		t.Fatalf("expected anthropic provider, got %q", cfg.Provider.Type)
	}
	if !cfg.Provider.AutoDetectType {
		t.Fatalf("expected auto detect provider type from env")
	}
	if cfg.Stream {
		t.Fatalf("expected stream override to disable streaming")
	}
	if cfg.MaxIterations != 32 {
		t.Fatalf("expected default max iterations 32, got %d", cfg.MaxIterations)
	}
	if cfg.Provider.ResolveAPIKey() != "secret" {
		t.Fatalf("expected api key from env")
	}
	if cfg.TokenQuota != 88000 {
		t.Fatalf("expected token quota from env override, got %d", cfg.TokenQuota)
	}
	if cfg.ApprovalMode != "away" {
		t.Fatalf("expected approval mode from env override, got %q", cfg.ApprovalMode)
	}
	if cfg.AwayPolicy != "fail_fast" {
		t.Fatalf("expected away policy from env override, got %q", cfg.AwayPolicy)
	}
}

func TestLoadIgnoresInvalidTokenQuotaEnv(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	t.Setenv("BYTEMIND_API_KEY", "secret")
	t.Setenv("BYTEMIND_TOKEN_QUOTA", "not-a-number")

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenQuota != DefaultTokenQuota {
		t.Fatalf("expected invalid token quota to fall back to default %d, got %d", DefaultTokenQuota, cfg.TokenQuota)
	}
}

func TestLoadDefaultsUpdateCheckEnabled(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	t.Setenv("BYTEMIND_API_KEY", "secret")

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UpdateCheck.Enabled {
		t.Fatalf("expected update_check.enabled default true")
	}
}

func TestLoadUsesUpdateCheckEnvOverride(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	t.Setenv("BYTEMIND_API_KEY", "secret")
	t.Setenv("BYTEMIND_UPDATE_CHECK", "false")

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UpdateCheck.Enabled {
		t.Fatalf("expected BYTEMIND_UPDATE_CHECK=false to disable update checks")
	}
}

func TestLoadAppliesTokenUsageDefaults(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	t.Setenv("BYTEMIND_API_KEY", "secret")

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenUsage.StorageType != "file" {
		t.Fatalf("expected default token_usage.storage_type=file, got %q", cfg.TokenUsage.StorageType)
	}
	if cfg.TokenUsage.RetentionDays != 30 {
		t.Fatalf("expected default token_usage.retention_days=30, got %d", cfg.TokenUsage.RetentionDays)
	}
	if strings.TrimSpace(cfg.TokenUsage.BackupInterval) == "" {
		t.Fatalf("expected default token_usage.backup_interval to be set")
	}
}

func TestLoadAppliesContextBudgetDefaultsWhenMissing(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	t.Setenv("BYTEMIND_API_KEY", "secret")

	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "project-key"),
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ContextBudget.WarningRatio != DefaultContextBudgetWarningRatio {
		t.Fatalf("expected default warning ratio %v, got %v", DefaultContextBudgetWarningRatio, cfg.ContextBudget.WarningRatio)
	}
	if cfg.ContextBudget.CriticalRatio != DefaultContextBudgetCriticalRatio {
		t.Fatalf("expected default critical ratio %v, got %v", DefaultContextBudgetCriticalRatio, cfg.ContextBudget.CriticalRatio)
	}
	if cfg.ContextBudget.MaxReactiveRetry != DefaultContextBudgetMaxReactiveRetry {
		t.Fatalf("expected default max reactive retry %d, got %d", DefaultContextBudgetMaxReactiveRetry, cfg.ContextBudget.MaxReactiveRetry)
	}
}

func TestLoadRejectsInvalidContextBudgetRatios(t *testing.T) {
	tests := []struct {
		name          string
		contextBudget map[string]any
	}{
		{
			name:          "warning not less than critical",
			contextBudget: contextBudgetDoc(DefaultContextBudgetCriticalRatio, DefaultContextBudgetCriticalRatio, DefaultContextBudgetMaxReactiveRetry),
		},
		{
			name:          "critical greater than one",
			contextBudget: contextBudgetDoc(DefaultContextBudgetWarningRatio, 1.1, DefaultContextBudgetMaxReactiveRetry),
		},
		{
			name:          "warning not positive",
			contextBudget: contextBudgetDoc(0.0, DefaultContextBudgetCriticalRatio, DefaultContextBudgetMaxReactiveRetry),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			t.Setenv("BYTEMIND_HOME", t.TempDir())
			if err := writeConfig(projectConfigPath(workspace), map[string]any{
				"provider":       minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
				"context_budget": tc.contextBudget,
			}); err != nil {
				t.Fatal(err)
			}

			_, err := Load(workspace, "")
			if err == nil {
				t.Fatalf("expected invalid context_budget to fail")
			}
			if !strings.Contains(err.Error(), "context_budget") {
				t.Fatalf("expected context_budget validation error, got %v", err)
			}
		})
	}
}

func TestLoadRejectsNegativeMaxReactiveRetry(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider":       minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"context_budget": contextBudgetDoc(DefaultContextBudgetWarningRatio, DefaultContextBudgetCriticalRatio, -1),
	}); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected negative max_reactive_retry to fail")
	}
	if !strings.Contains(err.Error(), "context_budget.max_reactive_retry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveConfigPathExplicit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveConfigPath(t.TempDir(), file)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected explicit path")
	}
}

func TestLoadMergesUserAndProjectConfigWithProjectPrecedence(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	t.Setenv("BYTEMIND_API_KEY", "")

	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(filepath.Join(home, "config.json"), map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "user-model",
			"api_key":  "user-key",
		},
		"approval_policy": "always",
		"approval_mode":   "interactive",
		"away_policy":     "auto_deny_continue",
		"max_iterations":  40,
		"stream":          true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "project-model",
			"api_key":  "project-key",
		},
		"approval_policy": "never",
		"approval_mode":   "away",
		"away_policy":     "fail_fast",
		"max_iterations":  16,
		"stream":          false,
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Model != "project-model" {
		t.Fatalf("expected project model precedence, got %q", cfg.Provider.Model)
	}
	if cfg.Provider.ResolveAPIKey() != "project-key" {
		t.Fatalf("expected project api key precedence, got %q", cfg.Provider.ResolveAPIKey())
	}
	if cfg.ApprovalPolicy != "never" {
		t.Fatalf("expected project approval policy precedence, got %q", cfg.ApprovalPolicy)
	}
	if cfg.ApprovalMode != "away" {
		t.Fatalf("expected project approval mode precedence, got %q", cfg.ApprovalMode)
	}
	if cfg.AwayPolicy != "fail_fast" {
		t.Fatalf("expected project away policy precedence, got %q", cfg.AwayPolicy)
	}
	if cfg.MaxIterations != 16 {
		t.Fatalf("expected project max iterations precedence, got %d", cfg.MaxIterations)
	}
	if cfg.Stream {
		t.Fatalf("expected project stream value false")
	}
}

func TestLoadIgnoresLegacyBytemindConfigJSON(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	t.Setenv("BYTEMIND_API_KEY", "")

	if err := writeConfig(filepath.Join(home, "config.json"), map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "user-model",
			"api_key":  "user-key",
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := writeConfig(filepath.Join(workspace, "bytemind.config.json"), map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "legacy-project-model",
			"api_key":  "legacy-project-key",
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Model != "user-model" {
		t.Fatalf("expected legacy bytemind.config.json to be ignored, got %q", cfg.Provider.Model)
	}
	if cfg.Provider.ResolveAPIKey() != "user-key" {
		t.Fatalf("expected user config api key when legacy project config exists, got %q", cfg.Provider.ResolveAPIKey())
	}
}

func TestLoadAcceptsLegacySessionDirField(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"session_dir": "tmp/sessions",
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Model != "gpt-5.4-mini" {
		t.Fatalf("expected provider model to load normally, got %q", cfg.Provider.Model)
	}
}

func TestLoadDefaultsModelFromProviderEndpointWhenMissing(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.deepseek.com",
			"model":    "",
			"api_key":  "test-key",
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Model != "deepseek-chat" {
		t.Fatalf("expected default model deepseek-chat, got %q", cfg.Provider.Model)
	}
}

func TestLoadRejectsUnsupportedProviderType(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	path := projectConfigPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{
  "provider": {
    "type": "unsupported",
    "base_url": "https://example.com",
    "model": "test-model",
    "api_key": "secret"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected unsupported provider type error")
	}
	if !strings.Contains(err.Error(), "provider.type must be one of") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidApprovalPolicy(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	path := projectConfigPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  },
  "approval_policy": "sometimes"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected invalid approval policy error")
	}
	if !strings.Contains(err.Error(), "approval_policy must be one of") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidApprovalMode(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	path := projectConfigPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  },
  "approval_mode": "nightly"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected invalid approval mode error")
	}
	if !strings.Contains(err.Error(), "approval_mode must be one of") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidAwayPolicy(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	path := projectConfigPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  },
  "away_policy": "retry_later"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected invalid away policy error")
	}
	if !strings.Contains(err.Error(), "away_policy must be one of") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsMalformedConfigJSON(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	configPath := projectConfigPath(workspace)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"provider":`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected malformed json error")
	}
	if !strings.Contains(err.Error(), "unexpected end of JSON input") && !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureHomeLayoutCreatesStandardDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)

	resolved, err := EnsureHomeLayout()
	if err != nil {
		t.Fatal(err)
	}
	if resolved != home {
		t.Fatalf("expected resolved home %q, got %q", home, resolved)
	}

	for _, name := range []string{"sessions", "logs", "cache", "auth", "migrations"} {
		if stat, err := os.Stat(filepath.Join(home, name)); err != nil || !stat.IsDir() {
			t.Fatalf("expected directory %q to be created", name)
		}
	}

	configPath := filepath.Join(home, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected default config.json to be created: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("expected default config.json to be valid json: %v", err)
	}
	if strings.Contains(string(data), "\"session_dir\"") {
		t.Fatalf("expected default config.json not to include legacy session_dir")
	}
	if strings.TrimSpace(cfg.Provider.Model) == "" {
		t.Fatalf("expected default provider model to be present")
	}
	if cfg.ApprovalMode != "interactive" {
		t.Fatalf("expected default approval_mode=interactive, got %q", cfg.ApprovalMode)
	}
	if cfg.AwayPolicy != "auto_deny_continue" {
		t.Fatalf("expected default away_policy=auto_deny_continue, got %q", cfg.AwayPolicy)
	}
	if cfg.TokenUsage.StorageType == "" || cfg.TokenUsage.BackupInterval == "" {
		t.Fatalf("expected default token_usage config to be present, got %#v", cfg.TokenUsage)
	}
	if !cfg.UpdateCheck.Enabled {
		t.Fatalf("expected default update_check.enabled=true in generated config")
	}
}

func TestUpsertProviderAPIKeyUpdatesExistingConfig(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "GPT-5.4",
    "api_key": "old-key",
    "api_key_env": "BYTEMIND_API_KEY"
  },
  "custom": "keep-me"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	writtenPath, err := UpsertProviderAPIKey(configPath, "new-key")
	if err != nil {
		t.Fatal(err)
	}
	if writtenPath == "" {
		t.Fatal("expected written path")
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	providerDoc, ok := doc["provider"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider object, got %#v", doc["provider"])
	}
	if providerDoc["api_key"] != "new-key" {
		t.Fatalf("expected api_key to be updated, got %#v", providerDoc["api_key"])
	}
	if doc["custom"] != "keep-me" {
		t.Fatalf("expected custom field to be preserved, got %#v", doc["custom"])
	}
}

func TestUpsertProviderAPIKeyCreatesConfigWhenMissing(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "nested", "config.json")

	writtenPath, err := UpsertProviderAPIKey(configPath, "new-key")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(writtenPath) != filepath.Clean(configPath) {
		t.Fatalf("expected written path %q, got %q", configPath, writtenPath)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.ResolveAPIKey() != "new-key" {
		t.Fatalf("expected api key to be written, got %q", cfg.Provider.ResolveAPIKey())
	}
	if cfg.Provider.Model == "" || cfg.Provider.BaseURL == "" {
		t.Fatalf("expected defaults to be present, got %#v", cfg.Provider)
	}
}

func TestUpsertProviderFieldUpdatesModelAndBaseURL(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "GPT-5.4",
    "api_key": "test-key"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := UpsertProviderField(configPath, "model", "deepseek-chat"); err != nil {
		t.Fatal(err)
	}
	if _, err := UpsertProviderField(configPath, "base_url", "https://api.deepseek.com"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Model != "deepseek-chat" {
		t.Fatalf("expected updated model, got %q", cfg.Provider.Model)
	}
	if cfg.Provider.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("expected updated base url, got %q", cfg.Provider.BaseURL)
	}
}

func TestUpsertProviderFieldBackfillsModelForDeepseekEndpointWhenMissing(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.deepseek.com",
    "model": "",
    "api_key": "test-key"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := UpsertProviderField(configPath, "base_url", "https://api.deepseek.com"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Model != "deepseek-chat" {
		t.Fatalf("expected deepseek model fallback, got %q", cfg.Provider.Model)
	}
}

func TestUpsertProviderFieldRejectsUnsupportedField(t *testing.T) {
	_, err := UpsertProviderField(filepath.Join(t.TempDir(), "config.json"), "foo", "bar")
	if err == nil {
		t.Fatal("expected unsupported field error")
	}
	if !strings.Contains(err.Error(), "unsupported provider field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeConfig(path string, cfg map[string]any) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func projectConfigPath(workspace string) string {
	return filepath.Join(workspace, ".bytemind", "config.json")
}

func minimalProviderConfigDoc(model, apiKey string) map[string]any {
	return map[string]any{
		"type":     "openai-compatible",
		"base_url": "https://api.openai.com/v1",
		"model":    strings.TrimSpace(model),
		"api_key":  strings.TrimSpace(apiKey),
	}
}

func contextBudgetDoc(warningRatio, criticalRatio float64, maxReactiveRetry int) map[string]any {
	return map[string]any{
		"warning_ratio":      warningRatio,
		"critical_ratio":     criticalRatio,
		"max_reactive_retry": maxReactiveRetry,
	}
}
