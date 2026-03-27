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
	if filepath.Dir(cfg.SessionDir) != filepath.Join(workspace, ".bytemind") {
		t.Fatalf("unexpected session dir: %s", cfg.SessionDir)
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

func TestLoadUsesWorkspaceRootConfigFile(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	data, err := json.Marshal(map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"approval_policy": "never",
		"max_iterations":  16,
		"stream":          false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.ResolveAPIKey() != "test-key" {
		t.Fatalf("expected api key from workspace root config, got %q", cfg.Provider.ResolveAPIKey())
	}
	if cfg.Provider.Model != "gpt-5.4-mini" {
		t.Fatalf("expected model from workspace root config, got %q", cfg.Provider.Model)
	}
	if cfg.ApprovalPolicy != "never" {
		t.Fatalf("expected approval policy never, got %q", cfg.ApprovalPolicy)
	}
	if cfg.MaxIterations != 16 {
		t.Fatalf("expected max iterations 16, got %d", cfg.MaxIterations)
	}
	if cfg.Stream {
		t.Fatalf("expected stream false from workspace root config")
	}
}

func TestLoadRejectsUnsupportedProviderType(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
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

func TestLoadNormalizesRelativeSessionDir(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  },
  "session_dir": "tmp/sessions"
}`), 0o644); err != nil {
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

func TestLoadRejectsInvalidApprovalPolicy(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	if err := os.WriteFile(configPath, []byte(`{
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
