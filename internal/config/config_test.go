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

func TestLoadNormalizesWritableRootsFromConfig(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	external := filepath.Join(t.TempDir(), "external-root")
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider":       minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"writable_roots": []any{"sandbox-output", external, "sandbox-output", "  "},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WritableRoots) != 2 {
		t.Fatalf("expected two normalized writable roots, got %#v", cfg.WritableRoots)
	}

	expectedA := filepath.Clean(filepath.Join(workspace, "sandbox-output"))
	expectedB := filepath.Clean(external)
	got := map[string]struct{}{}
	for _, root := range cfg.WritableRoots {
		got[normalizePathKey(root)] = struct{}{}
	}
	if _, ok := got[normalizePathKey(expectedA)]; !ok {
		t.Fatalf("expected relative writable root %q to be normalized into config, got %#v", expectedA, cfg.WritableRoots)
	}
	if _, ok := got[normalizePathKey(expectedB)]; !ok {
		t.Fatalf("expected absolute writable root %q to be preserved, got %#v", expectedB, cfg.WritableRoots)
	}
}

func TestLoadParsesWritableRootsFromEnv(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	t.Setenv("BYTEMIND_API_KEY", "env-key")
	external := filepath.Join(t.TempDir(), "env-external")
	t.Setenv("BYTEMIND_WRITABLE_ROOTS", "env-out"+string(os.PathListSeparator)+external)

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WritableRoots) != 2 {
		t.Fatalf("expected two writable roots from env, got %#v", cfg.WritableRoots)
	}

	expectedA := filepath.Clean(filepath.Join(workspace, "env-out"))
	expectedB := filepath.Clean(external)
	got := map[string]struct{}{}
	for _, root := range cfg.WritableRoots {
		got[normalizePathKey(root)] = struct{}{}
	}
	if _, ok := got[normalizePathKey(expectedA)]; !ok {
		t.Fatalf("expected env relative writable root %q, got %#v", expectedA, cfg.WritableRoots)
	}
	if _, ok := got[normalizePathKey(expectedB)]; !ok {
		t.Fatalf("expected env absolute writable root %q, got %#v", expectedB, cfg.WritableRoots)
	}
}

func TestDefaultIncludesSandboxPolicyFields(t *testing.T) {
	cfg := Default(t.TempDir())
	if cfg.SandboxEnabled {
		t.Fatal("expected sandbox_enabled to default to false")
	}
	if cfg.SystemSandboxMode != "off" {
		t.Fatalf("expected system_sandbox_mode to default to off, got %q", cfg.SystemSandboxMode)
	}
	if cfg.ExecAllowlist == nil || len(cfg.ExecAllowlist) != 0 {
		t.Fatalf("expected empty exec_allowlist default, got %#v", cfg.ExecAllowlist)
	}
	if cfg.NetworkAllowlist == nil || len(cfg.NetworkAllowlist) != 0 {
		t.Fatalf("expected empty network_allowlist default, got %#v", cfg.NetworkAllowlist)
	}
}

func TestLoadNormalizesSandboxAllowlistsFromConfig(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider":        minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"sandbox_enabled": true,
		"exec_allowlist": []any{
			map[string]any{"command": "  go test  ", "args_pattern": []any{"./...", "  ", "./..."}},
			map[string]any{"command": "go test", "args_pattern": []any{"./..."}},
			map[string]any{"command": "python", "args_pattern": []any{"pytest", "-m"}},
		},
		"network_allowlist": []any{
			map[string]any{"host": " Example.COM ", "port": 443, "scheme": " HTTPS "},
			map[string]any{"host": "example.com", "port": 443, "scheme": "https"},
			map[string]any{"host": "api.openai.com", "port": 443, "scheme": "https"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SandboxEnabled {
		t.Fatal("expected sandbox_enabled=true from config")
	}
	if len(cfg.ExecAllowlist) != 3 {
		t.Fatalf("expected normalized exec_allowlist with order-preserved args, got %#v", cfg.ExecAllowlist)
	}
	if cfg.ExecAllowlist[0].Command != "go" {
		t.Fatalf("expected first exec rule command to be normalized as go, got %#v", cfg.ExecAllowlist)
	}
	if got := strings.Join(cfg.ExecAllowlist[0].ArgsPattern, ","); got != "test,./..." {
		t.Fatalf("expected first go args pattern to preserve order, got %q", got)
	}
	if cfg.ExecAllowlist[1].Command != "go" {
		t.Fatalf("expected second exec rule command go, got %#v", cfg.ExecAllowlist)
	}
	if got := strings.Join(cfg.ExecAllowlist[1].ArgsPattern, ","); got != "test,./...,./..." {
		t.Fatalf("expected second go args pattern to preserve duplicates, got %q", got)
	}
	if cfg.ExecAllowlist[2].Command != "python" {
		t.Fatalf("expected third exec rule command python, got %#v", cfg.ExecAllowlist)
	}
	if got := strings.Join(cfg.ExecAllowlist[2].ArgsPattern, ","); got != "pytest,-m" {
		t.Fatalf("expected python args order to be preserved, got %q", got)
	}

	if len(cfg.NetworkAllowlist) != 2 {
		t.Fatalf("expected deduplicated network_allowlist, got %#v", cfg.NetworkAllowlist)
	}
	if cfg.NetworkAllowlist[0].Host != "api.openai.com" || cfg.NetworkAllowlist[0].Port != 443 || cfg.NetworkAllowlist[0].Scheme != "https" {
		t.Fatalf("unexpected first normalized network rule: %#v", cfg.NetworkAllowlist[0])
	}
	if cfg.NetworkAllowlist[1].Host != "example.com" || cfg.NetworkAllowlist[1].Port != 443 || cfg.NetworkAllowlist[1].Scheme != "https" {
		t.Fatalf("unexpected second normalized network rule: %#v", cfg.NetworkAllowlist[1])
	}
}

func TestLoadRejectsInvalidExecAllowlistRule(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"exec_allowlist": []any{
			map[string]any{"command": "   ", "args_pattern": []any{"./..."}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected invalid exec_allowlist error")
	}
	if !strings.Contains(err.Error(), "exec_allowlist.command cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidNetworkAllowlistRule(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"network_allowlist": []any{
			map[string]any{"host": "example.com", "port": 70000, "scheme": "https"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected invalid network_allowlist error")
	}
	if !strings.Contains(err.Error(), "network_allowlist.port must be between 1 and 65535") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadParsesSandboxEnabledFromEnv(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	t.Setenv("BYTEMIND_API_KEY", "env-key")
	t.Setenv("BYTEMIND_SANDBOX_ENABLED", "true")

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SandboxEnabled {
		t.Fatalf("expected sandbox_enabled=true from env override")
	}
}

func TestLoadParsesSystemSandboxModeFromEnv(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	t.Setenv("BYTEMIND_API_KEY", "env-key")
	t.Setenv("BYTEMIND_SANDBOX_ENABLED", "true")
	t.Setenv("BYTEMIND_SYSTEM_SANDBOX_MODE", "required")

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SystemSandboxMode != "required" {
		t.Fatalf("expected system_sandbox_mode=required from env override, got %q", cfg.SystemSandboxMode)
	}
}

func TestLoadRejectsSystemSandboxModeWithoutSandboxEnabled(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider":            minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"sandbox_enabled":     false,
		"system_sandbox_mode": "required",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected sandbox_enabled requirement error")
	}
	if !strings.Contains(err.Error(), "system_sandbox_mode requires sandbox_enabled=true") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsInvalidSystemSandboxMode(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider":            minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"system_sandbox_mode": "strictest",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected invalid system_sandbox_mode error")
	}
	if !strings.Contains(err.Error(), "system_sandbox_mode must be one of off, best_effort, required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadSortsNetworkAllowlistByPortWhenHostMatches(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"network_allowlist": []any{
			map[string]any{"host": "example.com", "port": 8443, "scheme": "https"},
			map[string]any{"host": "example.com", "port": 443, "scheme": "https"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.NetworkAllowlist) != 2 {
		t.Fatalf("expected two network rules, got %#v", cfg.NetworkAllowlist)
	}
	if cfg.NetworkAllowlist[0].Port != 443 || cfg.NetworkAllowlist[1].Port != 8443 {
		t.Fatalf("expected network allowlist sorted by port for same host, got %#v", cfg.NetworkAllowlist)
	}
}

func TestLoadAppliesMCPDefaultsAndNormalization(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "  Local Server ",
					"transport": map[string]any{
						"command": "node",
						"args":    []string{"./server.js"},
					},
					"protocol_version":  "2025-06-18",
					"protocol_versions": []string{" 2025-06-18 ", " 2024-11-05 "},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.MCP.Enabled {
		t.Fatal("expected mcp.enabled=true")
	}
	if cfg.MCP.SyncTTLSeconds != DefaultMCPSyncTTLSeconds {
		t.Fatalf("expected default mcp.sync_ttl_s=%d, got %d", DefaultMCPSyncTTLSeconds, cfg.MCP.SyncTTLSeconds)
	}
	if len(cfg.MCP.Servers) != 1 {
		t.Fatalf("expected one mcp server, got %d", len(cfg.MCP.Servers))
	}
	server := cfg.MCP.Servers[0]
	if server.ID != "local-server" {
		t.Fatalf("expected normalized server id local-server, got %q", server.ID)
	}
	if !server.EnabledValue() {
		t.Fatal("expected mcp server enabled by default")
	}
	if !server.AutoStartValue() {
		t.Fatal("expected mcp server auto_start enabled by default")
	}
	if server.Transport.Type != "stdio" {
		t.Fatalf("expected default transport type stdio, got %q", server.Transport.Type)
	}
	if server.StartupTimeoutSeconds != DefaultMCPStartupTimeoutSeconds {
		t.Fatalf("expected default startup timeout %d, got %d", DefaultMCPStartupTimeoutSeconds, server.StartupTimeoutSeconds)
	}
	if server.CallTimeoutSeconds != DefaultMCPCallTimeoutSeconds {
		t.Fatalf("expected default call timeout %d, got %d", DefaultMCPCallTimeoutSeconds, server.CallTimeoutSeconds)
	}
	if server.MaxConcurrency != DefaultMCPMaxConcurrency {
		t.Fatalf("expected default max concurrency %d, got %d", DefaultMCPMaxConcurrency, server.MaxConcurrency)
	}
	if len(server.ProtocolVersions) != 2 || server.ProtocolVersions[0] != "2025-06-18" || server.ProtocolVersions[1] != "2024-11-05" {
		t.Fatalf("expected normalized protocol versions [2025-06-18 2024-11-05], got %#v", server.ProtocolVersions)
	}
}

func TestLoadAppliesExtensionsDefaultsAndNormalization(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"extensions": map[string]any{
			"sources":                       []string{" skills ", "mcp", "SKILLS"},
			"failure_threshold":             0,
			"recovery_cooldown_sec":         0,
			"health_check_interval_sec":     0,
			"max_concurrency_per_extension": 0,
			"conflict_policy":               " REJECT ",
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Extensions.Sources) != 2 || cfg.Extensions.Sources[0] != "skills" || cfg.Extensions.Sources[1] != "mcp" {
		t.Fatalf("expected normalized extensions.sources [skills mcp], got %#v", cfg.Extensions.Sources)
	}
	if !cfg.Extensions.AutoLoadValue() {
		t.Fatal("expected extensions.auto_load default true")
	}
	if cfg.Extensions.FailureThreshold != DefaultExtensionsFailureThreshold {
		t.Fatalf("expected default failure_threshold=%d, got %d", DefaultExtensionsFailureThreshold, cfg.Extensions.FailureThreshold)
	}
	if cfg.Extensions.RecoveryCooldownSec != DefaultExtensionsRecoveryCooldownSec {
		t.Fatalf("expected default recovery_cooldown_sec=%d, got %d", DefaultExtensionsRecoveryCooldownSec, cfg.Extensions.RecoveryCooldownSec)
	}
	if cfg.Extensions.HealthCheckIntervalSec != DefaultExtensionsHealthCheckInterval {
		t.Fatalf("expected default health_check_interval_sec=%d, got %d", DefaultExtensionsHealthCheckInterval, cfg.Extensions.HealthCheckIntervalSec)
	}
	if cfg.Extensions.MaxConcurrencyPerExtension != DefaultExtensionsMaxConcurrency {
		t.Fatalf("expected default max_concurrency_per_extension=%d, got %d", DefaultExtensionsMaxConcurrency, cfg.Extensions.MaxConcurrencyPerExtension)
	}
	if cfg.Extensions.ConflictPolicy != DefaultExtensionsConflictPolicy {
		t.Fatalf("expected normalized conflict_policy=%q, got %q", DefaultExtensionsConflictPolicy, cfg.Extensions.ConflictPolicy)
	}
}

func TestLoadRejectsInvalidExtensionsConflictPolicy(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"extensions": map[string]any{
			"conflict_policy": "unsupported",
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected invalid extensions.conflict_policy error")
	}
	if !strings.Contains(err.Error(), "extensions.conflict_policy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsDuplicateNormalizedMCPServerID(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "Server A",
					"transport": map[string]any{
						"command": "node",
					},
				},
				{
					"id": "server/a",
					"transport": map[string]any{
						"command": "node",
					},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected duplicate normalized mcp server id error")
	}
	if !strings.Contains(err.Error(), "duplicate server id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsMCPEnabledStdioServerWithoutCommand(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "missing-command",
					"transport": map[string]any{
						"type": "stdio",
					},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected missing command validation error")
	}
	if !strings.Contains(err.Error(), "requires transport.command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNormalizesMCPToolOverrides(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "server1",
					"transport": map[string]any{
						"command": "node",
					},
					"tool_overrides": []map[string]any{
						{
							"tool_name":         "  fetch_data ",
							"safety_class":      "SENSITIVE",
							"allowed_modes":     []string{" build ", "plan", "BUILD"},
							"default_timeout_s": 12,
							"max_timeout_s":     30,
							"max_result_chars":  2048,
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MCP.Servers) != 1 || len(cfg.MCP.Servers[0].ToolOverrides) != 1 {
		t.Fatalf("expected one tool override, got %#v", cfg.MCP.Servers)
	}
	override := cfg.MCP.Servers[0].ToolOverrides[0]
	if override.ToolName != "fetch_data" {
		t.Fatalf("expected trimmed tool_name, got %q", override.ToolName)
	}
	if override.SafetyClass != "sensitive" {
		t.Fatalf("expected lower-cased safety_class sensitive, got %q", override.SafetyClass)
	}
	if len(override.AllowedModes) != 2 || override.AllowedModes[0] != "build" || override.AllowedModes[1] != "plan" {
		t.Fatalf("expected normalized allowed_modes [build plan], got %#v", override.AllowedModes)
	}
}

func TestLoadRejectsInvalidMCPToolOverrideSafetyClass(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(projectConfigPath(workspace), map[string]any{
		"provider": minimalProviderConfigDoc("gpt-5.4-mini", "test-key"),
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "server1",
					"transport": map[string]any{
						"command": "node",
					},
					"tool_overrides": []map[string]any{
						{
							"tool_name":    "fetch_data",
							"safety_class": "ultra-dangerous",
						},
					},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected invalid safety class validation error")
	}
	if !strings.Contains(err.Error(), "unsupported safety_class") {
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
