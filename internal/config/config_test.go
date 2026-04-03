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
	t.Setenv("BYTEMIND_PROVIDER_TYPE", "anthropic")
	t.Setenv("BYTEMIND_STREAM", "false")

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
	if cfg.Stream {
		t.Fatalf("expected stream override to disable streaming")
	}
	if cfg.MaxIterations != 32 {
		t.Fatalf("expected default max iterations 32, got %d", cfg.MaxIterations)
	}
	if cfg.Provider.ResolveAPIKey() != "secret" {
		t.Fatalf("expected api key from env")
	}
	wantSessionDir := filepath.Join(home, "sessions")
	if cfg.SessionDir != wantSessionDir {
		t.Fatalf("unexpected session dir: want %q, got %q", wantSessionDir, cfg.SessionDir)
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
		"max_iterations":  40,
		"stream":          true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := writeConfig(filepath.Join(workspace, "config.json"), map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "project-model",
			"api_key":  "project-key",
		},
		"approval_policy": "never",
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
	if cfg.MaxIterations != 16 {
		t.Fatalf("expected project max iterations precedence, got %d", cfg.MaxIterations)
	}
	if cfg.Stream {
		t.Fatalf("expected project stream value false")
	}
}

func TestLoadNormalizesRelativeSessionDir(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := writeConfig(filepath.Join(workspace, "config.json"), map[string]any{
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
	want := filepath.Join(workspace, "tmp", "sessions")
	if cfg.SessionDir != want {
		t.Fatalf("expected normalized session dir %q, got %q", want, cfg.SessionDir)
	}
}

func TestLoadRejectsUnsupportedProviderType(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(workspace, "config.json"), []byte(`{
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
	if err := os.WriteFile(filepath.Join(workspace, "config.json"), []byte(`{
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

func TestLoadRejectsMalformedConfigJSON(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	configPath := filepath.Join(workspace, "config.json")
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
	if cfg.SessionDir != filepath.Join(home, "sessions") {
		t.Fatalf("expected default session_dir %q, got %q", filepath.Join(home, "sessions"), cfg.SessionDir)
	}
	if strings.TrimSpace(cfg.Provider.Model) == "" {
		t.Fatalf("expected default provider model to be present")
	}
}

func writeConfig(path string, cfg map[string]any) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
