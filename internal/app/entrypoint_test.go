package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapEntrypointResolvesWorkspaceOverride(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "config.json")
	data, err := json.Marshal(map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BYTEMIND_HOME", filepath.Join(workspace, ".bytemind-home"))

	runtime, err := BootstrapEntrypoint(EntrypointRequest{
		WorkspaceOverride: workspace,
		ConfigPath:        configPath,
		RequireAPIKey:     true,
		Stdin:             strings.NewReader(""),
		Stdout:            ioDiscard{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Runner == nil || runtime.Store == nil || runtime.Session == nil {
		t.Fatalf("expected populated runtime, got %#v", runtime)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func TestBootstrapCreatesSessionInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	writeEntrypointTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})

	runtimeBundle, err := BootstrapEntrypoint(EntrypointRequest{
		RequireAPIKey: true,
		Stdin:         strings.NewReader(""),
		Stdout:        &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtimeBundle.Runner == nil || runtimeBundle.Store == nil || runtimeBundle.Session == nil {
		t.Fatal("expected bootstrap to return runner, store, and session")
	}
	sess := runtimeBundle.Session
	if sess.Workspace != workspace {
		t.Fatalf("expected workspace %q, got %q", workspace, sess.Workspace)
	}
	if strings.TrimSpace(sess.ID) == "" {
		t.Fatal("expected session id to be created")
	}
}

func TestBootstrapEntrypointRejectsBroadWorkspaceWithoutOverride(t *testing.T) {
	workspace := t.TempDir()
	for i := 0; i < DefaultBroadWorkspaceEntryThreshold; i++ {
		path := filepath.Join(workspace, fmt.Sprintf("entry-%03d.tmp", i))
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("seed broad workspace entries: %v", err)
		}
	}
	t.Chdir(workspace)

	_, err := BootstrapEntrypoint(EntrypointRequest{
		RequireAPIKey: false,
		Stdin:         strings.NewReader(""),
		Stdout:        &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected broad workspace error")
	}
	if !strings.Contains(err.Error(), "too broad for default workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapFailsWhenHomeLayoutCannotBeCreated(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	writeEntrypointTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})

	blockParent := t.TempDir()
	blockFile := filepath.Join(blockParent, "not-a-dir")
	if err := os.WriteFile(blockFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BYTEMIND_HOME", filepath.Join(blockFile, "child"))

	_, err := BootstrapEntrypoint(EntrypointRequest{
		RequireAPIKey: true,
		Stdin:         strings.NewReader(""),
		Stdout:        &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected bootstrap to fail when home layout cannot be created")
	}
}

func TestBootstrapRejectsMissingAPIKey(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("BYTEMIND_API_KEY", "")
	writeEntrypointTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
		},
		"stream": false,
	})

	_, err := BootstrapEntrypoint(EntrypointRequest{
		RequireAPIKey: true,
		Stdin:         strings.NewReader(""),
		Stdout:        &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected missing API key error")
	}
	if !strings.Contains(err.Error(), "missing API key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapForTUIAllowsMissingAPIKey(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("BYTEMIND_API_KEY", "")
	writeEntrypointTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
		},
		"stream": false,
	})

	runtimeBundle, err := BootstrapEntrypoint(EntrypointRequest{
		RequireAPIKey: false,
		Stdin:         strings.NewReader(""),
		Stdout:        &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("expected bootstrapForTUI to continue without api key, got %v", err)
	}
	if runtimeBundle.Runner == nil || runtimeBundle.Store == nil || runtimeBundle.Session == nil {
		t.Fatal("expected bootstrapForTUI to return runner, store, and session")
	}
}

func TestBootstrapRejectsExplicitMissingConfigFile(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("BYTEMIND_HOME", filepath.Join(workspace, ".bytemind-home"))

	_, err := BootstrapEntrypoint(EntrypointRequest{
		ConfigPath:    filepath.Join(workspace, "missing-config.json"),
		RequireAPIKey: true,
		Stdin:         strings.NewReader(""),
		Stdout:        &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected missing config file error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestBootstrapRejectsExplicitMalformedConfigFile(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("BYTEMIND_HOME", filepath.Join(workspace, ".bytemind-home"))
	badConfigPath := filepath.Join(workspace, "bad-config.json")
	if err := os.WriteFile(badConfigPath, []byte(`{"provider":`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := BootstrapEntrypoint(EntrypointRequest{
		ConfigPath:    badConfigPath,
		RequireAPIKey: true,
		Stdin:         strings.NewReader(""),
		Stdout:        &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected malformed config error")
	}
	if !strings.Contains(err.Error(), "unexpected end of JSON input") && !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapRejectsUnavailableSystemSandboxBackend(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	writeEntrypointTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream":              false,
		"sandbox_enabled":     true,
		"system_sandbox_mode": "required",
	})

	original := validateSystemSandboxRuntime
	validateSystemSandboxRuntime = func(enabled bool, mode string) error {
		if !enabled {
			t.Fatalf("expected sandbox enabled in validation")
		}
		if mode != "required" {
			t.Fatalf("expected required mode in validation, got %q", mode)
		}
		return errors.New("sandbox backend unavailable in test")
	}
	t.Cleanup(func() {
		validateSystemSandboxRuntime = original
	})

	_, err := BootstrapEntrypoint(EntrypointRequest{
		RequireAPIKey: true,
		Stdin:         strings.NewReader(""),
		Stdout:        &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected bootstrap to fail when system sandbox backend is unavailable")
	}
	if !strings.Contains(err.Error(), "sandbox backend unavailable in test") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeEntrypointTestConfig(t *testing.T, workspace string, cfg map[string]any) {
	t.Helper()
	t.Setenv("BYTEMIND_HOME", filepath.Join(workspace, ".bytemind-home"))
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	projectConfigDir := filepath.Join(workspace, ".bytemind")
	if err := os.MkdirAll(projectConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectConfigDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
