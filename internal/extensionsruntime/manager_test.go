package extensionsruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configpkg "bytemind/internal/config"
	extensionspkg "bytemind/internal/extensions"
	"bytemind/internal/llm"
	toolspkg "bytemind/internal/tools"
)

func TestManagerListIncludesConfiguredMCPServer(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id":         "local",
					"auto_start": false,
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	items, err := manager.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one mcp extension, got %d", len(items))
	}
	if items[0].ID != "mcp.local" {
		t.Fatalf("unexpected extension id: %q", items[0].ID)
	}
	if items[0].Status != extensionspkg.ExtensionStatusReady {
		t.Fatalf("expected ready status, got %q", items[0].Status)
	}

	resolved, err := manager.ResolveAllTools(context.Background())
	if err != nil {
		t.Fatalf("ResolveAllTools failed: %v", err)
	}
	if len(resolved) != 0 {
		t.Fatalf("expected no tools when auto_start=false, got %#v", resolved)
	}
}

func TestManagerAutoStartDoesNotEagerDiscoverOnInit(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "lazy-start",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "bytemind-command-that-does-not-exist-for-startup-check",
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	items, err := manager.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one mcp extension, got %d", len(items))
	}
	if items[0].ID != "mcp.lazy-start" {
		t.Fatalf("unexpected extension id: %q", items[0].ID)
	}
	if items[0].Status != extensionspkg.ExtensionStatusReady {
		t.Fatalf("expected ready status before first discovery, got %q", items[0].Status)
	}
}

func TestManagerUnloadAndLoadMCPServerToggleVisibility(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id":         "local",
					"auto_start": false,
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	if err := manager.Unload(context.Background(), "mcp.local"); err != nil {
		t.Fatalf("Unload failed: %v", err)
	}
	items, err := manager.List(context.Background())
	if err != nil {
		t.Fatalf("List after unload failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected unloaded mcp server to be hidden, got %#v", items)
	}
	if _, err := manager.Load(context.Background(), "mcp:local"); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	items, err = manager.List(context.Background())
	if err != nil {
		t.Fatalf("List after load failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "mcp.local" {
		t.Fatalf("expected mcp.local after load, got %#v", items)
	}
}

func TestManagerGetReloadAndTestPaths(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id":         "local",
					"auto_start": false,
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))

	item, err := manager.Get(context.Background(), "mcp.local")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if item.ID != "mcp.local" {
		t.Fatalf("expected mcp.local, got %q", item.ID)
	}
	if item.Status != extensionspkg.ExtensionStatusReady {
		t.Fatalf("expected ready status, got %q", item.Status)
	}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if _, err := manager.Get(context.Background(), "mcp.missing"); err == nil {
		t.Fatal("expected missing mcp extension lookup to fail")
	}
	if _, err := manager.Test(context.Background(), "skill.demo"); err == nil {
		t.Fatal("expected non-mcp test target to fail")
	}
	if _, err := manager.Test(context.Background(), "mcp.missing"); err == nil {
		t.Fatal("expected missing mcp test target to fail")
	}
}

func TestManagerReloadUsesBaseReloaderWhenAvailable(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": false,
		},
	})

	baseReloadErr := errors.New("base reload failed")
	base := &fakeBaseRuntimeManagerWithReloader{
		base: fakeBaseRuntimeManager{
			listErr: errors.New("list should not be called"),
		},
		reloadErr: baseReloadErr,
	}
	manager := NewManager(workspace, "", base, loadRuntimeConfig(t, workspace))

	err := manager.Reload(context.Background())
	if !errors.Is(err, baseReloadErr) {
		t.Fatalf("expected reload to return base reload error, got %v", err)
	}
	if base.reloadCalls != 1 {
		t.Fatalf("expected base reload to be called once, got %d", base.reloadCalls)
	}
	if base.base.listCalls != 0 {
		t.Fatalf("expected base list to be skipped when reloader exists, got %d", base.base.listCalls)
	}
}

func TestManagerReloadFallsBackToBaseListWhenNoBaseReloader(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": false,
		},
	})

	baseListErr := errors.New("base list failed")
	base := &fakeBaseRuntimeManager{
		listErr: baseListErr,
	}
	manager := NewManager(workspace, "", base, loadRuntimeConfig(t, workspace))

	err := manager.Reload(context.Background())
	if !errors.Is(err, baseListErr) {
		t.Fatalf("expected reload to return base list error fallback, got %v", err)
	}
	if base.listCalls != 1 {
		t.Fatalf("expected base list to be called once without reloader, got %d", base.listCalls)
	}
}

func TestManagerResolveAllToolsAndInvalidate(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "alpha",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
				{
					"id": "beta",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	alpha := manager.entries["mcp.alpha"]
	beta := manager.entries["mcp.beta"]
	if alpha == nil || beta == nil {
		t.Fatalf("expected alpha and beta entries, got alpha=%v beta=%v", alpha != nil, beta != nil)
	}

	alphaExt := &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.alpha",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusActive,
		},
		tools: []extensionspkg.ExtensionTool{
			{
				Source:      extensionspkg.ExtensionMCP,
				ExtensionID: "mcp.alpha",
				Tool:        fakeRuntimeTool{name: "alpha_tool"},
			},
			{
				Source:      extensionspkg.ExtensionSkill,
				ExtensionID: "skill.alpha",
				Tool:        fakeRuntimeTool{name: "skill_tool"},
			},
		},
		health: extensionspkg.HealthSnapshot{Status: extensionspkg.ExtensionStatusActive},
	}
	betaExt := &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.beta",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusDegraded,
		},
		resolveErr: errors.New("beta resolve failed"),
		health:     extensionspkg.HealthSnapshot{Status: extensionspkg.ExtensionStatusDegraded},
	}
	alpha.extension = alphaExt
	beta.extension = betaExt

	tools, err := manager.ResolveAllTools(context.Background())
	if err == nil || !strings.Contains(err.Error(), "beta resolve failed") {
		t.Fatalf("expected beta resolve error, got %v", err)
	}
	if len(tools) != 1 || tools[0].ExtensionID != "mcp.alpha" {
		t.Fatalf("expected only alpha mcp tool, got %#v", tools)
	}

	manager.Invalidate("mcp.alpha")
	if alphaExt.invalidateCalls != 1 {
		t.Fatalf("expected alpha invalidate once, got %d", alphaExt.invalidateCalls)
	}
	manager.Invalidate("")
	if alphaExt.invalidateCalls != 2 {
		t.Fatalf("expected alpha invalidate twice, got %d", alphaExt.invalidateCalls)
	}
	if betaExt.invalidateCalls != 1 {
		t.Fatalf("expected beta invalidate once, got %d", betaExt.invalidateCalls)
	}
}

func TestManagerResolveAllToolsContextErrorAndTestUsesExtensionHealth(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "local",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	entry := manager.entries["mcp.local"]
	if entry == nil {
		t.Fatal("expected local entry")
	}
	localExt := &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.local",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusActive,
		},
		resolveErr: context.Canceled,
		health: extensionspkg.HealthSnapshot{
			Status:       extensionspkg.ExtensionStatusActive,
			Message:      "ok",
			CheckedAtUTC: "2026-04-21T00:00:00Z",
		},
	}
	entry.extension = localExt

	if _, err := manager.ResolveAllTools(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from ResolveAllTools, got %v", err)
	}

	health, err := manager.Test(context.Background(), "mcp.local")
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if localExt.reloadCalls == 0 {
		t.Fatal("expected extension reload to be called before health")
	}
	if health.Status != extensionspkg.ExtensionStatusActive {
		t.Fatalf("expected active health status, got %q", health.Status)
	}
}

func TestManagerTestReturnsCircuitOpenErrorWhenIsolated(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"extensions": map[string]any{
			"failure_threshold":     1,
			"recovery_cooldown_sec": 30,
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "local",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	if manager.health == nil {
		t.Fatal("expected health manager")
	}
	manager.health.RecordFailure("mcp.local")

	_, err := manager.Test(context.Background(), "mcp.local")
	if err == nil {
		t.Fatal("expected circuit-open error")
	}
	if !strings.Contains(err.Error(), "circuit open") {
		t.Fatalf("expected circuit-open error, got %v", err)
	}
}

func TestManagerTestReloadContextErrorReturnsImmediately(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "local",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	entry := manager.entries["mcp.local"]
	if entry == nil {
		t.Fatal("expected local entry")
	}
	localExt := &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.local",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusActive,
		},
		reloadErr: context.Canceled,
		health: extensionspkg.HealthSnapshot{
			Status: extensionspkg.ExtensionStatusActive,
		},
	}
	entry.extension = localExt

	_, err := manager.Test(context.Background(), "mcp.local")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if localExt.healthCalls != 0 {
		t.Fatalf("expected health not to run after reload cancellation, got %d calls", localExt.healthCalls)
	}
}

func TestManagerTestReturnsContextErrorFromHealth(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "local",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	entry := manager.entries["mcp.local"]
	if entry == nil {
		t.Fatal("expected local entry")
	}
	localExt := &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.local",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusActive,
		},
		health: extensionspkg.HealthSnapshot{
			Status:  extensionspkg.ExtensionStatusFailed,
			Message: "deadline",
		},
		healthErr: context.DeadlineExceeded,
	}
	entry.extension = localExt

	_, err := manager.Test(context.Background(), "mcp.local")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded from extension health, got %v", err)
	}
}

func TestManagerLoadCircuitOpenWithNilExtensionDoesNotPanic(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"extensions": map[string]any{
			"failure_threshold":     1,
			"recovery_cooldown_sec": 30,
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id":         "local",
					"auto_start": false,
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	entry := manager.entries["mcp.local"]
	if entry == nil {
		t.Fatal("expected local entry")
	}
	entry.lastErr = errors.New("bootstrap failed")
	entry.info = failedMCPInfo(entry.server, entry.lastErr, time.Now().UTC())

	manager.health.RecordFailure("mcp.local")

	_, err := manager.Load(context.Background(), "mcp:local")
	if err == nil {
		t.Fatal("expected circuit-open error")
	}
	if !strings.Contains(err.Error(), "circuit open") {
		t.Fatalf("expected circuit-open error, got %v", err)
	}
}

func TestManagerRefreshUpdatesHealthPolicyFromConfig(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"extensions": map[string]any{
			"failure_threshold":     3,
			"recovery_cooldown_sec": 30,
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id":         "local",
					"auto_start": false,
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	if manager.health == nil {
		t.Fatal("expected health manager")
	}
	now := time.Date(2026, 4, 24, 9, 0, 0, 0, time.UTC)
	manager.health.SetClockForTesting(func() time.Time {
		return now
	})
	first := manager.health.RecordFailure("mcp.local")
	if first.CircuitState != extensionspkg.CircuitClosed {
		t.Fatalf("expected first failure to stay closed under initial policy, got %#v", first)
	}

	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"extensions": map[string]any{
			"failure_threshold":     1,
			"recovery_cooldown_sec": 5,
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id":         "local",
					"auto_start": false,
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	second := manager.health.RecordFailure("mcp.local")
	if second.CircuitState != extensionspkg.CircuitOpen {
		t.Fatalf("expected updated threshold to open circuit, got %#v", second)
	}
	nextRetryAt, err := time.Parse(time.RFC3339, second.NextRetryAtUTC)
	if err != nil {
		t.Fatalf("expected valid retry timestamp, got %q (%v)", second.NextRetryAtUTC, err)
	}
	expected := now.Add(5 * time.Second)
	if !nextRetryAt.Equal(expected) {
		t.Fatalf("expected updated cooldown retry %s, got %s", expected.Format(time.RFC3339), nextRetryAt.Format(time.RFC3339))
	}
}

func TestManagerLoadDoesNotFailFromOtherServerReloadError(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "alpha",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
				{
					"id": "beta",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	alpha := manager.entries["mcp.alpha"]
	beta := manager.entries["mcp.beta"]
	if alpha == nil || beta == nil {
		t.Fatalf("expected alpha and beta entries, got alpha=%v beta=%v", alpha != nil, beta != nil)
	}

	alpha.extension = &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.alpha",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusReady,
			Health: extensionspkg.HealthSnapshot{
				Status: extensionspkg.ExtensionStatusReady,
			},
		},
	}
	beta.extension = &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.beta",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusReady,
			Health: extensionspkg.HealthSnapshot{
				Status: extensionspkg.ExtensionStatusReady,
			},
		},
		reloadErr: errors.New("beta reload failed"),
	}

	loaded, err := manager.Load(context.Background(), "mcp:alpha")
	if err != nil {
		t.Fatalf("expected alpha load to succeed despite beta reload failure, got %v", err)
	}
	if loaded.ID != "mcp.alpha" {
		t.Fatalf("expected loaded extension id mcp.alpha, got %q", loaded.ID)
	}

	alphaExt, _ := alpha.extension.(*fakeMCPRuntimeExtension)
	betaExt, _ := beta.extension.(*fakeMCPRuntimeExtension)
	if alphaExt == nil || betaExt == nil {
		t.Fatalf("expected fake extensions, got alpha=%T beta=%T", alpha.extension, beta.extension)
	}
	if alphaExt.reloadCalls != 1 {
		t.Fatalf("expected alpha reload once during load, got %d", alphaExt.reloadCalls)
	}
	if betaExt.reloadCalls != 0 {
		t.Fatalf("expected beta reload to be skipped during alpha load, got %d", betaExt.reloadCalls)
	}
}

func TestManagerIsolationAndRecoveryCircuitBreaker(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeRuntimeConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"extensions": map[string]any{
			"failure_threshold":     2,
			"recovery_cooldown_sec": 10,
		},
		"mcp": map[string]any{
			"enabled": true,
			"servers": []map[string]any{
				{
					"id": "alpha",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
				{
					"id": "beta",
					"transport": map[string]any{
						"type":    "stdio",
						"command": "cmd",
						"args":    []string{"/c", "echo", "ok"},
					},
				},
			},
		},
	})

	manager := NewManager(workspace, "", extensionspkg.NopManager{}, loadRuntimeConfig(t, workspace))
	if manager.health == nil {
		t.Fatal("expected runtime manager health manager")
	}
	now := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	manager.health.SetClockForTesting(func() time.Time {
		return now
	})

	alpha := manager.entries["mcp.alpha"]
	beta := manager.entries["mcp.beta"]
	if alpha == nil || beta == nil {
		t.Fatalf("expected alpha and beta entries, got alpha=%v beta=%v", alpha != nil, beta != nil)
	}

	alphaExt := &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.alpha",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusActive,
		},
		resolveErr: errors.New("alpha resolve failed"),
		health:     extensionspkg.HealthSnapshot{Status: extensionspkg.ExtensionStatusDegraded},
	}
	betaExt := &fakeMCPRuntimeExtension{
		info: extensionspkg.ExtensionInfo{
			ID:     "mcp.beta",
			Kind:   extensionspkg.ExtensionMCP,
			Status: extensionspkg.ExtensionStatusActive,
		},
		tools: []extensionspkg.ExtensionTool{
			{
				Source:      extensionspkg.ExtensionMCP,
				ExtensionID: "mcp.beta",
				Tool:        fakeRuntimeTool{name: "beta_tool"},
			},
		},
		health: extensionspkg.HealthSnapshot{Status: extensionspkg.ExtensionStatusActive},
	}
	alpha.extension = alphaExt
	beta.extension = betaExt

	if _, err := manager.ResolveAllTools(context.Background()); err == nil || !strings.Contains(err.Error(), "alpha resolve failed") {
		t.Fatalf("expected first alpha resolve error, got %v", err)
	}
	if _, err := manager.ResolveAllTools(context.Background()); err == nil || !strings.Contains(err.Error(), "alpha resolve failed") {
		t.Fatalf("expected second alpha resolve error to trip circuit, got %v", err)
	}
	if alphaExt.resolveCalls != 2 {
		t.Fatalf("expected two alpha resolve attempts before open circuit, got %d", alphaExt.resolveCalls)
	}

	tools, err := manager.ResolveAllTools(context.Background())
	if err != nil {
		t.Fatalf("expected open-circuit resolve to keep healthy tools, got %v", err)
	}
	if len(tools) != 1 || tools[0].ExtensionID != "mcp.beta" {
		t.Fatalf("expected only beta tool while alpha circuit is open, got %#v", tools)
	}
	if alphaExt.resolveCalls != 2 {
		t.Fatalf("expected alpha resolve to be skipped while circuit open, got %d calls", alphaExt.resolveCalls)
	}

	items, err := manager.List(context.Background())
	if err != nil {
		t.Fatalf("list after circuit open failed: %v", err)
	}
	var alphaInfo extensionspkg.ExtensionInfo
	for _, item := range items {
		if item.ID == "mcp.alpha" {
			alphaInfo = item
			break
		}
	}
	if alphaInfo.ID == "" {
		t.Fatalf("expected mcp.alpha in list, got %#v", items)
	}
	if alphaInfo.Status != extensionspkg.ExtensionStatusDegraded || !strings.Contains(alphaInfo.Health.Message, "circuit open") {
		t.Fatalf("expected alpha degraded with circuit-open message, got %#v", alphaInfo)
	}

	now = now.Add(11 * time.Second)
	alphaExt.resolveErr = nil
	alphaExt.tools = []extensionspkg.ExtensionTool{
		{
			Source:      extensionspkg.ExtensionMCP,
			ExtensionID: "mcp.alpha",
			Tool:        fakeRuntimeTool{name: "alpha_tool"},
		},
	}

	tools, err = manager.ResolveAllTools(context.Background())
	if err != nil {
		t.Fatalf("expected half-open recovery resolve to succeed, got %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected both alpha/beta tools after recovery, got %#v", tools)
	}
	if alphaExt.resolveCalls != 3 {
		t.Fatalf("expected one half-open probe call after cooldown, got %d", alphaExt.resolveCalls)
	}

	items, err = manager.List(context.Background())
	if err != nil {
		t.Fatalf("list after recovery failed: %v", err)
	}
	for _, item := range items {
		if item.ID != "mcp.alpha" {
			continue
		}
		if item.Status != extensionspkg.ExtensionStatusActive {
			t.Fatalf("expected alpha active after recovery, got %#v", item)
		}
		if strings.Contains(item.Health.Message, "circuit open") {
			t.Fatalf("expected circuit-open marker cleared after recovery, got %#v", item)
		}
	}
}

func TestManagerHelperFunctionsAndTransformers(t *testing.T) {
	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	server := configpkg.MCPServerConfig{
		ID:   "demo",
		Name: "Demo",
		Transport: configpkg.MCPTransportConfig{
			Type:    "stdio",
			Command: "cmd",
			Args:    []string{"/c", "echo", "ok"},
		},
		ProtocolVersion:  "2026-04-01",
		ProtocolVersions: []string{"2025-03-26"},
		MaxConcurrency:   9,
		ToolOverrides: []configpkg.MCPToolOverrideConfig{
			{
				ToolName:        "list_docs",
				SafetyClass:     "sensitive",
				AllowedModes:    []string{"build"},
				DefaultTimeoutS: 10,
				MaxTimeoutS:     20,
				MaxResultChars:  5000,
			},
		},
	}
	mapped := toMCPServerConfig(server)
	if mapped.MaxConcurrency != 9 {
		t.Fatalf("expected mapped max concurrency 9, got %d", mapped.MaxConcurrency)
	}
	if mapped.ToolOverrides["list_docs"].MaxResultChars != 5000 {
		t.Fatalf("expected mapped override max_result_chars=5000, got %#v", mapped.ToolOverrides["list_docs"])
	}

	loaded := normalizeMCPInfo(extensionspkg.ExtensionInfo{
		ID:     "mcp.demo",
		Kind:   extensionspkg.ExtensionMCP,
		Name:   "Demo",
		Title:  "Demo",
		Status: extensionspkg.ExtensionStatusLoaded,
		Health: extensionspkg.HealthSnapshot{
			Status: extensionspkg.ExtensionStatusLoaded,
		},
	}, server, now)
	if loaded.Status != extensionspkg.ExtensionStatusReady {
		t.Fatalf("expected loaded->ready normalization, got %q", loaded.Status)
	}
	unknown := normalizeMCPInfo(extensionspkg.ExtensionInfo{
		ID:     "mcp.demo",
		Kind:   extensionspkg.ExtensionMCP,
		Name:   "Demo",
		Status: extensionspkg.ExtensionStatusUnknown,
	}, server, now)
	if unknown.Status != extensionspkg.ExtensionStatusReady {
		t.Fatalf("expected unknown->ready normalization, got %q", unknown.Status)
	}
	failed := failedMCPInfo(server, errors.New("boom"), now)
	if failed.Status != extensionspkg.ExtensionStatusFailed {
		t.Fatalf("expected failed status, got %q", failed.Status)
	}
	if failed.Health.LastError != extensionspkg.ErrCodeLoadFailed {
		t.Fatalf("expected load_failed error code, got %q", failed.Health.LastError)
	}

	if id, serverID, ok := normalizeMCPInput("mcp:Demo.Main"); !ok || id != "mcp.demo-main" || serverID != "demo-main" {
		t.Fatalf("unexpected normalizeMCPInput result: id=%q serverID=%q ok=%t", id, serverID, ok)
	}
	if id, serverID, ok := normalizeMCPInput("skill.demo"); ok || id != "mcp.skill-demo" || serverID != "skill-demo" {
		t.Fatalf("expected normalized fallback for non-mcp input, got id=%q serverID=%q ok=%t", id, serverID, ok)
	}
	if got := normalizeServerID(" A/B:C "); got != "a-b-c" {
		t.Fatalf("expected normalized server id a-b-c, got %q", got)
	}
	if got := mergeErrors(errors.New("left"), errors.New("right")); got == nil || !strings.Contains(got.Error(), "left") || !strings.Contains(got.Error(), "right") {
		t.Fatalf("expected merged error to include both sides, got %v", got)
	}
	if mergeErrors(nil, nil) != nil {
		t.Fatal("expected nil merge for nil errors")
	}
	if firstNonEmpty("", " ", "ok") != "ok" {
		t.Fatal("expected first non-empty value to be ok")
	}

	baseInfo := extensionspkg.ExtensionInfo{
		ID:     "mcp.demo",
		Status: extensionspkg.ExtensionStatusReady,
		Health: extensionspkg.HealthSnapshot{Status: extensionspkg.ExtensionStatusReady},
	}
	openNoRetry := applyIsolationSnapshot(baseInfo, extensionspkg.IsolationSnapshot{
		CircuitState: extensionspkg.CircuitOpen,
	}, nil, now)
	if !strings.Contains(openNoRetry.Health.Message, "mcp circuit open") {
		t.Fatalf("expected open message without retry, got %#v", openNoRetry)
	}
	halfOpen := applyIsolationSnapshot(baseInfo, extensionspkg.IsolationSnapshot{
		CircuitState: extensionspkg.CircuitHalfOpen,
	}, nil, now)
	if !strings.Contains(halfOpen.Health.Message, "half-open") {
		t.Fatalf("expected half-open message, got %#v", halfOpen)
	}
	degraded := applyIsolationSnapshot(extensionspkg.ExtensionInfo{
		Status: extensionspkg.ExtensionStatusReady,
		Health: extensionspkg.HealthSnapshot{Status: extensionspkg.ExtensionStatusDegraded},
	}, extensionspkg.IsolationSnapshot{}, nil, now)
	if degraded.Status != extensionspkg.ExtensionStatusDegraded {
		t.Fatalf("expected degraded status to persist, got %#v", degraded)
	}

	if got := infoForEntry(nil, now); got.ID != "" {
		t.Fatalf("expected zero info for nil entry, got %#v", got)
	}
	withoutID := infoForEntry(&mcpEntry{server: server}, now)
	if withoutID.ID != "mcp.demo" {
		t.Fatalf("expected ready info fallback for empty entry info, got %#v", withoutID)
	}

	cfg := configpkg.ExtensionsConfig{FailureThreshold: 0, RecoveryCooldownSec: 0}
	healthMgr := newRuntimeHealthManager(cfg)
	if healthMgr == nil || !healthMgr.AllowProbe("mcp.any") {
		t.Fatal("expected runtime health manager with default fallback policy")
	}
}

func loadRuntimeConfig(t *testing.T, workspace string) configpkg.Config {
	t.Helper()
	cfg, err := configpkg.Load(workspace, "")
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	return cfg
}

func writeRuntimeConfig(t *testing.T, workspace string, doc map[string]any) {
	t.Helper()
	path := filepath.Join(workspace, ".bytemind", "config.json")
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal config failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config dir failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
}

type fakeRuntimeTool struct {
	name string
}

func (f fakeRuntimeTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name: f.name,
			Parameters: map[string]any{
				"type": "object",
			},
		},
	}
}

func (f fakeRuntimeTool) Run(context.Context, json.RawMessage, *toolspkg.ExecutionContext) (string, error) {
	return "ok", nil
}

type fakeMCPRuntimeExtension struct {
	info            extensionspkg.ExtensionInfo
	tools           []extensionspkg.ExtensionTool
	resolveErr      error
	resolveCalls    int
	health          extensionspkg.HealthSnapshot
	healthErr       error
	healthCalls     int
	reloadErr       error
	reloadCalls     int
	invalidateCalls int
}

func (f *fakeMCPRuntimeExtension) Info() extensionspkg.ExtensionInfo {
	return f.info
}

func (f *fakeMCPRuntimeExtension) ResolveTools(context.Context) ([]extensionspkg.ExtensionTool, error) {
	f.resolveCalls++
	return append([]extensionspkg.ExtensionTool(nil), f.tools...), f.resolveErr
}

func (f *fakeMCPRuntimeExtension) Health(context.Context) (extensionspkg.HealthSnapshot, error) {
	f.healthCalls++
	return f.health, f.healthErr
}

func (f *fakeMCPRuntimeExtension) Reload(context.Context) error {
	f.reloadCalls++
	return f.reloadErr
}

func (f *fakeMCPRuntimeExtension) Invalidate() {
	f.invalidateCalls++
}

type fakeBaseRuntimeManager struct {
	listErr   error
	listCalls int
}

func (f *fakeBaseRuntimeManager) Load(context.Context, string) (extensionspkg.ExtensionInfo, error) {
	return extensionspkg.ExtensionInfo{}, nil
}

func (f *fakeBaseRuntimeManager) Unload(context.Context, string) error {
	return nil
}

func (f *fakeBaseRuntimeManager) Get(context.Context, string) (extensionspkg.ExtensionInfo, error) {
	return extensionspkg.ExtensionInfo{}, nil
}

func (f *fakeBaseRuntimeManager) List(context.Context) ([]extensionspkg.ExtensionInfo, error) {
	f.listCalls++
	return nil, f.listErr
}

type fakeBaseRuntimeManagerWithReloader struct {
	base        fakeBaseRuntimeManager
	reloadErr   error
	reloadCalls int
}

func (f *fakeBaseRuntimeManagerWithReloader) Load(ctx context.Context, source string) (extensionspkg.ExtensionInfo, error) {
	return f.base.Load(ctx, source)
}

func (f *fakeBaseRuntimeManagerWithReloader) Unload(ctx context.Context, extensionID string) error {
	return f.base.Unload(ctx, extensionID)
}

func (f *fakeBaseRuntimeManagerWithReloader) Get(ctx context.Context, extensionID string) (extensionspkg.ExtensionInfo, error) {
	return f.base.Get(ctx, extensionID)
}

func (f *fakeBaseRuntimeManagerWithReloader) List(ctx context.Context) ([]extensionspkg.ExtensionInfo, error) {
	return f.base.List(ctx)
}

func (f *fakeBaseRuntimeManagerWithReloader) Reload(context.Context) error {
	f.reloadCalls++
	return f.reloadErr
}
