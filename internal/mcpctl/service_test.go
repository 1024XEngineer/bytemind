package mcpctl

import (
	"context"
	"errors"
	"strings"
	"testing"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
)

func TestServiceAddEnablesMCPAndListsReadyServer(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())

	service := NewService(workspace, "", nil)
	autoStart := false
	status, err := service.Add(context.Background(), AddRequest{
		ID:        " Docs ",
		Command:   "cmd",
		Args:      []string{"/c", "echo", "ok"},
		AutoStart: &autoStart,
	})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if status.ID != "docs" {
		t.Fatalf("expected normalized id docs, got %q", status.ID)
	}
	if !status.Enabled {
		t.Fatal("expected added server to be enabled")
	}
	if status.Status != extensionspkg.ExtensionStatusReady {
		t.Fatalf("expected ready status for auto_start=false, got %q", status.Status)
	}

	cfg, err := configpkg.Load(workspace, "")
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}
	if !cfg.MCP.Enabled {
		t.Fatal("expected mcp.enabled=true after add")
	}
	if len(cfg.MCP.Servers) != 1 {
		t.Fatalf("expected one server in config, got %d", len(cfg.MCP.Servers))
	}
	if cfg.MCP.Servers[0].ID != "docs" {
		t.Fatalf("expected normalized server id docs in config, got %q", cfg.MCP.Servers[0].ID)
	}

	items, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one list item, got %d", len(items))
	}
	if items[0].ID != "docs" || !items[0].Enabled {
		t.Fatalf("unexpected list item: %#v", items[0])
	}
}

func TestServiceEnableSetsGlobalFlagAndTogglesServer(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	seedMCPConfig(t, workspace, false, []configpkg.MCPServerConfig{
		{
			ID:        "demo",
			Enabled:   boolPtr(false),
			AutoStart: boolPtr(false),
			Transport: configpkg.MCPTransportConfig{
				Type:    "stdio",
				Command: "cmd",
				Args:    []string{"/c", "echo", "ok"},
			},
		},
	})

	service := NewService(workspace, "", nil)
	status, err := service.Enable(context.Background(), "demo", true)
	if err != nil {
		t.Fatalf("Enable(true) failed: %v", err)
	}
	if !status.Enabled {
		t.Fatal("expected enabled status after Enable(true)")
	}
	if status.Status != extensionspkg.ExtensionStatusReady {
		t.Fatalf("expected ready status after enabling auto_start=false server, got %q", status.Status)
	}

	cfg, err := configpkg.Load(workspace, "")
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}
	if !cfg.MCP.Enabled {
		t.Fatal("expected mcp.enabled=true after Enable(true)")
	}
	if len(cfg.MCP.Servers) != 1 || !cfg.MCP.Servers[0].EnabledValue() {
		t.Fatalf("expected server to be enabled in config, got %#v", cfg.MCP.Servers)
	}

	disabledStatus, err := service.Enable(context.Background(), "demo", false)
	if err != nil {
		t.Fatalf("Enable(false) failed: %v", err)
	}
	if disabledStatus.Enabled {
		t.Fatal("expected disabled status after Enable(false)")
	}
}

func TestServiceRemoveHandlesMissingAndExistingServer(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	seedMCPConfig(t, workspace, true, []configpkg.MCPServerConfig{
		{
			ID:        "demo",
			Enabled:   boolPtr(true),
			AutoStart: boolPtr(false),
			Transport: configpkg.MCPTransportConfig{
				Type:    "stdio",
				Command: "cmd",
				Args:    []string{"/c", "echo", "ok"},
			},
		},
	})

	service := NewService(workspace, "", nil)
	if err := service.Remove(context.Background(), "missing"); err == nil {
		t.Fatal("expected remove missing server to fail")
	}
	if err := service.Remove(context.Background(), "demo"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	cfg, err := configpkg.Load(workspace, "")
	if err != nil {
		t.Fatalf("Load config failed: %v", err)
	}
	if len(cfg.MCP.Servers) != 0 {
		t.Fatalf("expected no servers after remove, got %#v", cfg.MCP.Servers)
	}
}

func TestServiceReloadAndTestUseInjectedManager(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	seedMCPConfig(t, workspace, true, []configpkg.MCPServerConfig{
		{
			ID:        "demo",
			Enabled:   boolPtr(true),
			AutoStart: boolPtr(false),
			Transport: configpkg.MCPTransportConfig{
				Type:    "stdio",
				Command: "cmd",
				Args:    []string{"/c", "echo", "ok"},
			},
		},
	})

	manager := &fakeManager{
		listItems: []extensionspkg.ExtensionInfo{
			{
				ID:     "mcp.demo",
				Name:   "demo",
				Kind:   extensionspkg.ExtensionMCP,
				Status: extensionspkg.ExtensionStatusActive,
				Capabilities: extensionspkg.CapabilitySet{
					Tools: 2,
				},
				Health: extensionspkg.HealthSnapshot{
					Status:       extensionspkg.ExtensionStatusActive,
					Message:      "active",
					LastError:    "",
					CheckedAtUTC: "2026-04-21T00:00:00Z",
				},
			},
		},
		testHealth: extensionspkg.HealthSnapshot{
			Status:       extensionspkg.ExtensionStatusDegraded,
			Message:      "test degraded",
			LastError:    extensionspkg.ErrCodeLoadFailed,
			CheckedAtUTC: "2026-04-21T00:00:01Z",
		},
	}
	service := NewService(workspace, "", manager)

	if err := service.Reload(context.Background()); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if manager.reloadCalls != 1 {
		t.Fatalf("expected one reload call, got %d", manager.reloadCalls)
	}
	if len(manager.invalidateArgs) != 1 || manager.invalidateArgs[0] != "" {
		t.Fatalf("expected invalidate(\"\") to be called once, got %#v", manager.invalidateArgs)
	}

	status, err := service.Test(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if manager.testArg != "mcp.demo" {
		t.Fatalf("expected Test to query mcp.demo, got %q", manager.testArg)
	}
	if status.Status != extensionspkg.ExtensionStatusDegraded {
		t.Fatalf("expected degraded status from health test, got %q", status.Status)
	}
	if status.LastError != extensionspkg.ErrCodeLoadFailed {
		t.Fatalf("expected load_failed last error, got %q", status.LastError)
	}
}

func TestServiceReloadRuntimeFallsBackToListWhenManagerLacksReloader(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	listErr := errors.New("list refresh failed")
	manager := &fakeManagerNoReloader{
		listErr: listErr,
	}
	service := NewService(workspace, "", manager)

	err := service.reloadRuntime(context.Background())
	if !errors.Is(err, listErr) {
		t.Fatalf("expected reloadRuntime to return list fallback error, got %v", err)
	}
	if manager.listCalls != 1 {
		t.Fatalf("expected one list fallback call, got %d", manager.listCalls)
	}
	if len(manager.invalidateArgs) != 1 || manager.invalidateArgs[0] != "" {
		t.Fatalf("expected invalidate(\"\") before fallback list, got %#v", manager.invalidateArgs)
	}
}

func TestServiceAddReturnsErrorWhenReloadFallbackListFails(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	listErr := errors.New("list refresh failed")
	manager := &fakeManagerNoReloader{
		listErr: listErr,
	}
	service := NewService(workspace, "", manager)

	status, err := service.Add(context.Background(), AddRequest{
		ID:      "docs",
		Command: "cmd",
		Args:    []string{"/c", "echo", "ok"},
	})
	if err == nil {
		t.Fatal("expected add to return reload fallback list error")
	}
	if !errors.Is(err, listErr) {
		t.Fatalf("expected add error to wrap fallback list error, got %v", err)
	}
	if !strings.Contains(err.Error(), "runtime reload failed after config persisted") {
		t.Fatalf("expected add error message to include runtime reload context, got %v", err)
	}
	if status.ID != "docs" {
		t.Fatalf("expected status for added server even on reload fallback error, got %#v", status)
	}
}

func TestServiceListReturnsStatusesAlongsideManagerListError(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	seedMCPConfig(t, workspace, true, []configpkg.MCPServerConfig{
		{
			ID:        "demo",
			Enabled:   boolPtr(true),
			AutoStart: boolPtr(false),
			Transport: configpkg.MCPTransportConfig{
				Type:    "stdio",
				Command: "cmd",
				Args:    []string{"/c", "echo", "ok"},
			},
		},
	})

	manager := &fakeManager{
		listErr: errors.New("list degraded"),
	}
	service := NewService(workspace, "", manager)
	items, err := service.List(context.Background())
	if err == nil || err.Error() != "list degraded" {
		t.Fatalf("expected propagated list error, got %v", err)
	}
	if len(items) != 1 || items[0].ID != "demo" {
		t.Fatalf("expected list to still include configured server status, got %#v", items)
	}
}

func TestServiceGetStatusReturnsItemWhenListIsDegraded(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	seedMCPConfig(t, workspace, true, []configpkg.MCPServerConfig{
		{
			ID:        "demo",
			Enabled:   boolPtr(true),
			AutoStart: boolPtr(true),
			Transport: configpkg.MCPTransportConfig{
				Type:    "stdio",
				Command: "cmd",
				Args:    []string{"/c", "echo", "ok"},
			},
		},
	})

	manager := &fakeManager{
		listItems: []extensionspkg.ExtensionInfo{
			{
				ID:     "mcp.demo",
				Kind:   extensionspkg.ExtensionMCP,
				Status: extensionspkg.ExtensionStatusActive,
			},
		},
		listErr: errors.New("degraded list"),
	}
	service := NewService(workspace, "", manager)
	status, err := service.getStatus(context.Background(), "demo")
	if err != nil {
		t.Fatalf("expected getStatus success with item+err degraded list, got %v", err)
	}
	if status.ID != "demo" || status.Status != extensionspkg.ExtensionStatusActive {
		t.Fatalf("unexpected status payload: %#v", status)
	}
}

func TestServiceAddReturnsErrorWhenReloadFails(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	reloadErr := errors.New("reload failed")
	service := NewService(workspace, "", &fakeManager{reloadErr: reloadErr})

	status, err := service.Add(context.Background(), AddRequest{
		ID:      "docs",
		Command: "cmd",
		Args:    []string{"/c", "echo", "ok"},
	})
	if err == nil {
		t.Fatal("expected add to return reload error")
	}
	if !errors.Is(err, reloadErr) {
		t.Fatalf("expected add error to wrap reload error, got %v", err)
	}
	if !strings.Contains(err.Error(), "runtime reload failed after config persisted") {
		t.Fatalf("expected add error message to include runtime reload context, got %v", err)
	}
	if status.ID != "docs" {
		t.Fatalf("expected status for added server even on reload error, got %#v", status)
	}
}

func TestServiceEnableReturnsErrorWhenReloadFails(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	seedMCPConfig(t, workspace, true, []configpkg.MCPServerConfig{
		{
			ID:        "demo",
			Enabled:   boolPtr(false),
			AutoStart: boolPtr(false),
			Transport: configpkg.MCPTransportConfig{
				Type:    "stdio",
				Command: "cmd",
				Args:    []string{"/c", "echo", "ok"},
			},
		},
	})
	reloadErr := errors.New("reload failed")
	service := NewService(workspace, "", &fakeManager{reloadErr: reloadErr})

	status, err := service.Enable(context.Background(), "demo", true)
	if err == nil {
		t.Fatal("expected enable to return reload error")
	}
	if !errors.Is(err, reloadErr) {
		t.Fatalf("expected enable error to wrap reload error, got %v", err)
	}
	if status.ID != "demo" {
		t.Fatalf("expected status for target server even on reload error, got %#v", status)
	}
}

func TestServiceShowReturnsConfigAndRuntimeStatus(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	seedMCPConfig(t, workspace, true, []configpkg.MCPServerConfig{
		{
			ID:        "demo",
			Name:      "Demo MCP",
			Enabled:   boolPtr(true),
			AutoStart: boolPtr(false),
			Transport: configpkg.MCPTransportConfig{
				Type:    "stdio",
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-demo"},
				Env: map[string]string{
					"DEMO_TOKEN": "secret",
					"ANOTHER":    "value",
				},
				CWD: "C:/tmp/demo",
			},
			StartupTimeoutSeconds: 22,
			CallTimeoutSeconds:    66,
			MaxConcurrency:        3,
			ProtocolVersions:      []string{"2025-03-26", "2024-11-05"},
		},
	})

	service := NewService(workspace, "", nil)
	detail, err := service.Show(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Show failed: %v", err)
	}
	if detail.Status.ID != "demo" {
		t.Fatalf("expected demo id, got %#v", detail.Status)
	}
	if detail.TransportType != "stdio" {
		t.Fatalf("expected stdio transport, got %q", detail.TransportType)
	}
	if detail.Command != "npx" {
		t.Fatalf("expected command npx, got %q", detail.Command)
	}
	if len(detail.Args) != 2 {
		t.Fatalf("expected args to be preserved, got %#v", detail.Args)
	}
	if len(detail.EnvKeys) != 2 || detail.EnvKeys[0] != "ANOTHER" || detail.EnvKeys[1] != "DEMO_TOKEN" {
		t.Fatalf("expected sorted env keys, got %#v", detail.EnvKeys)
	}
	if detail.StartupTimeoutS != 22 || detail.CallTimeoutS != 66 || detail.MaxConcurrency != 3 {
		t.Fatalf("unexpected timeout/concurrency details: %#v", detail)
	}
	if len(detail.ProtocolVersions) != 2 || detail.ProtocolVersions[0] != "2025-03-26" {
		t.Fatalf("unexpected protocol versions: %#v", detail.ProtocolVersions)
	}
}

func TestNormalizeAddRequestAndHelpers(t *testing.T) {
	normalized := normalizeAddRequest(AddRequest{
		ID:      " My/Server ",
		Command: " cmd ",
	})
	if normalized.ID != "my-server" {
		t.Fatalf("expected normalized id my-server, got %q", normalized.ID)
	}
	if normalized.Command != "cmd" {
		t.Fatalf("expected trimmed command, got %q", normalized.Command)
	}
	if normalized.StartupTimeoutS != configpkg.DefaultMCPStartupTimeoutSeconds {
		t.Fatalf("expected default startup timeout, got %d", normalized.StartupTimeoutS)
	}
	if normalized.CallTimeoutS != configpkg.DefaultMCPCallTimeoutSeconds {
		t.Fatalf("expected default call timeout, got %d", normalized.CallTimeoutS)
	}
	if normalized.MaxConcurrency != configpkg.DefaultMCPMaxConcurrency {
		t.Fatalf("expected default max concurrency, got %d", normalized.MaxConcurrency)
	}

	if id := normalizeServerID(" mcp:Docs.Main "); id != "docs-main" {
		t.Fatalf("expected normalized server id docs-main, got %q", id)
	}
	cloned := cloneStringMap(map[string]string{"A": "1"})
	cloned["A"] = "2"
	if got := cloneStringMap(nil); got != nil {
		t.Fatalf("expected nil clone for nil map, got %#v", got)
	}
	if cloned["A"] != "2" {
		t.Fatal("expected mutable clone map")
	}
	if first := firstNonEmpty("", " ", "x"); first != "x" {
		t.Fatalf("expected first non-empty value x, got %q", first)
	}
	if !*boolPtr(true) {
		t.Fatal("expected boolPtr(true) to dereference to true")
	}
	if nowRFC3339() == "" {
		t.Fatal("expected non-empty RFC3339 timestamp")
	}
}

func seedMCPConfig(t *testing.T, workspace string, enabled bool, servers []configpkg.MCPServerConfig) {
	t.Helper()
	_, _, err := configpkg.MutateMCPConfig(workspace, "", func(mcp *configpkg.MCPConfig) error {
		mcp.Enabled = enabled
		mcp.Servers = append([]configpkg.MCPServerConfig(nil), servers...)
		return nil
	})
	if err != nil {
		t.Fatalf("seed mcp config failed: %v", err)
	}
}

type fakeManager struct {
	listItems      []extensionspkg.ExtensionInfo
	listErr        error
	reloadErr      error
	reloadCalls    int
	invalidateArgs []string
	testHealth     extensionspkg.HealthSnapshot
	testErr        error
	testArg        string
}

func (f *fakeManager) Load(context.Context, string) (extensionspkg.ExtensionInfo, error) {
	return extensionspkg.ExtensionInfo{}, nil
}

func (f *fakeManager) Unload(context.Context, string) error {
	return nil
}

func (f *fakeManager) Get(context.Context, string) (extensionspkg.ExtensionInfo, error) {
	return extensionspkg.ExtensionInfo{}, nil
}

func (f *fakeManager) List(context.Context) ([]extensionspkg.ExtensionInfo, error) {
	items := append([]extensionspkg.ExtensionInfo(nil), f.listItems...)
	return items, f.listErr
}

func (f *fakeManager) Reload(context.Context) error {
	f.reloadCalls++
	return f.reloadErr
}

func (f *fakeManager) Test(_ context.Context, extensionID string) (extensionspkg.HealthSnapshot, error) {
	f.testArg = extensionID
	return f.testHealth, f.testErr
}

func (f *fakeManager) Invalidate(extensionID string) {
	f.invalidateArgs = append(f.invalidateArgs, extensionID)
}

type fakeManagerNoReloader struct {
	listItems      []extensionspkg.ExtensionInfo
	listErr        error
	listCalls      int
	invalidateArgs []string
}

func (f *fakeManagerNoReloader) Load(context.Context, string) (extensionspkg.ExtensionInfo, error) {
	return extensionspkg.ExtensionInfo{}, nil
}

func (f *fakeManagerNoReloader) Unload(context.Context, string) error {
	return nil
}

func (f *fakeManagerNoReloader) Get(context.Context, string) (extensionspkg.ExtensionInfo, error) {
	return extensionspkg.ExtensionInfo{}, nil
}

func (f *fakeManagerNoReloader) List(context.Context) ([]extensionspkg.ExtensionInfo, error) {
	f.listCalls++
	items := append([]extensionspkg.ExtensionInfo(nil), f.listItems...)
	return items, f.listErr
}

func (f *fakeManagerNoReloader) Invalidate(extensionID string) {
	f.invalidateArgs = append(f.invalidateArgs, extensionID)
}
