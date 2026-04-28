package app

import (
	"os"
	"strings"
	"testing"
)

func TestLoadRuntimeConfigAppliesOverrides(t *testing.T) {
	workspace := t.TempDir()
	writeCfg := `{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  },
  "stream": false,
  "max_iterations": 16
}`
	if err := osWriteFile(workspace+"/config.json", writeCfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:                 workspace,
		ConfigPath:                workspace + "/config.json",
		ModelOverride:             "gpt-5.4",
		StreamOverride:            "true",
		SandboxEnabledOverride:    "true",
		SystemSandboxModeOverride: "required",
		ApprovalModeOverride:      "away",
		AwayPolicyOverride:        "fail_fast",
		MaxIterationsOverride:     9,
	})
	if err != nil {
		t.Fatalf("LoadRuntimeConfig failed: %v", err)
	}
	if cfg.Provider.Model != "gpt-5.4" {
		t.Fatalf("unexpected model: %q", cfg.Provider.Model)
	}
	if !cfg.Stream {
		t.Fatal("expected stream=true")
	}
	if cfg.MaxIterations != 9 {
		t.Fatalf("unexpected max iterations: %d", cfg.MaxIterations)
	}
	if cfg.ApprovalMode != "away" {
		t.Fatalf("unexpected approval mode: %q", cfg.ApprovalMode)
	}
	if cfg.AwayPolicy != "fail_fast" {
		t.Fatalf("unexpected away policy: %q", cfg.AwayPolicy)
	}
	if !cfg.SandboxEnabled {
		t.Fatalf("expected sandbox-enabled override to apply")
	}
	if cfg.SystemSandboxMode != "required" {
		t.Fatalf("expected system sandbox mode override to apply, got %q", cfg.SystemSandboxMode)
	}
}

func TestLoadRuntimeConfigRejectsInvalidStream(t *testing.T) {
	workspace := t.TempDir()
	writeCfg := `{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  }
}`
	if err := osWriteFile(workspace+"/config.json", writeCfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:      workspace,
		ConfigPath:     workspace + "/config.json",
		StreamOverride: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid stream")
	}
	if !strings.Contains(err.Error(), "invalid -stream value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRuntimeConfigRejectsInvalidApprovalMode(t *testing.T) {
	workspace := t.TempDir()
	writeCfg := `{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  }
}`
	if err := osWriteFile(workspace+"/config.json", writeCfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:            workspace,
		ConfigPath:           workspace + "/config.json",
		ApprovalModeOverride: "batch",
	})
	if err == nil {
		t.Fatal("expected error for invalid approval mode")
	}
	if !strings.Contains(err.Error(), "invalid -approval-mode value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRuntimeConfigRejectsInvalidAwayPolicy(t *testing.T) {
	workspace := t.TempDir()
	writeCfg := `{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  }
}`
	if err := osWriteFile(workspace+"/config.json", writeCfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:          workspace,
		ConfigPath:         workspace + "/config.json",
		AwayPolicyOverride: "queue",
	})
	if err == nil {
		t.Fatal("expected error for invalid away policy")
	}
	if !strings.Contains(err.Error(), "invalid -away-policy value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRuntimeConfigRejectsInvalidSandboxEnabled(t *testing.T) {
	workspace := t.TempDir()
	writeCfg := `{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  }
}`
	if err := osWriteFile(workspace+"/config.json", writeCfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:              workspace,
		ConfigPath:             workspace + "/config.json",
		SandboxEnabledOverride: "maybe",
	})
	if err == nil {
		t.Fatal("expected error for invalid sandbox-enabled")
	}
	if !strings.Contains(err.Error(), "invalid -sandbox-enabled value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRuntimeConfigRejectsInvalidSystemSandboxMode(t *testing.T) {
	workspace := t.TempDir()
	writeCfg := `{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  }
}`
	if err := osWriteFile(workspace+"/config.json", writeCfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:                 workspace,
		ConfigPath:                workspace + "/config.json",
		SystemSandboxModeOverride: "strictest",
	})
	if err == nil {
		t.Fatal("expected error for invalid system-sandbox-mode")
	}
	if !strings.Contains(err.Error(), "invalid -system-sandbox-mode value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRuntimeConfigRejectsSystemSandboxModeWithoutSandboxEnabled(t *testing.T) {
	workspace := t.TempDir()
	writeCfg := `{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  },
  "sandbox_enabled": false,
  "system_sandbox_mode": "off"
}`
	if err := osWriteFile(workspace+"/config.json", writeCfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:                 workspace,
		ConfigPath:                workspace + "/config.json",
		SystemSandboxModeOverride: "required",
	})
	if err == nil {
		t.Fatal("expected error when system sandbox mode requires sandbox-enabled")
	}
	if !strings.Contains(err.Error(), "system_sandbox_mode requires sandbox_enabled=true") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRuntimeConfigRejectsNegativeMaxIterationsOverride(t *testing.T) {
	workspace := t.TempDir()
	writeCfg := `{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "test-key"
  }
}`
	if err := osWriteFile(workspace+"/config.json", writeCfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:             workspace,
		ConfigPath:            workspace + "/config.json",
		MaxIterationsOverride: -1,
	})
	if err == nil {
		t.Fatal("expected error for negative max-iterations")
	}
	if !strings.Contains(err.Error(), "-max-iterations must be greater than 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
