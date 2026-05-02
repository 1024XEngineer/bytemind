package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
	toolspkg "github.com/1024XEngineer/bytemind/internal/tools"
)

func TestWithClientOptionHandlesNilOptions(t *testing.T) {
	opt := WithClient(&stubClient{})
	opt(nil)
}

func TestFromMCPServerInvalidConfigAndDefaultClientPath(t *testing.T) {
	_, err := FromMCPServer(ServerConfig{
		Command: "echo",
	}, WithClient(&stubClient{}))
	if err == nil {
		t.Fatal("expected invalid config error when id is missing")
	}
	var extErr *extensionspkg.ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != extensionspkg.ErrCodeInvalidSource {
		t.Fatalf("expected invalid source code, got %q", extErr.Code)
	}

	_, err = FromMCPServer(ServerConfig{
		ID:   "auto-client",
		Name: "Auto Client",
	})
	if err == nil {
		t.Fatal("expected invalid config when command is missing in stdio mode")
	}
	var extErr2 *extensionspkg.ExtensionError
	if !errors.As(err, &extErr2) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr2.Code != extensionspkg.ErrCodeInvalidSource {
		t.Fatalf("expected invalid source code, got %q", extErr2.Code)
	}
}

func TestFromMCPServerHandlesNilOptionAndNilNowOverride(t *testing.T) {
	ext, err := FromMCPServer(ServerConfig{
		ID:      "github",
		Name:    "GitHub",
		Command: "stub",
	}, nil, WithClient(&stubClient{
		discoverSnapshot: ServerSnapshot{
			ID:      "github",
			Name:    "GitHub",
			Version: "1.0.0",
			Tools: []ToolDescriptor{
				{Name: "list_prs"},
			},
		},
	}), func(opts *adapterOptions) {
		opts.now = nil
	})
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	info := ext.Info()
	if info.ID != "mcp.github" {
		t.Fatalf("unexpected extension id: %q", info.ID)
	}
	if info.Health.CheckedAtUTC == "" {
		t.Fatal("expected health timestamp to be set")
	}
}

func TestAdapterNilReceiverBranches(t *testing.T) {
	var adapter *Adapter
	if info := adapter.Info(); !info.IsZero() {
		t.Fatalf("expected zero info for nil receiver, got %#v", info)
	}

	if _, err := adapter.ResolveTools(context.Background()); err == nil {
		t.Fatal("expected resolve error for nil receiver")
	}
	if _, err := adapter.Health(context.Background()); err == nil {
		t.Fatal("expected health error for nil receiver")
	}
	if err := adapter.refresh(context.Background()); err == nil {
		t.Fatal("expected refresh error for nil receiver")
	}
}

func TestResolveToolsReturnsContextError(t *testing.T) {
	client := &stubClient{
		discoverErr: context.DeadlineExceeded,
	}
	ext, err := FromMCPServer(ServerConfig{
		ID:      "timeout",
		Name:    "Timeout",
		Command: "stub",
	}, WithClient(client))
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	adapter, ok := ext.(*Adapter)
	if !ok {
		t.Fatalf("expected *Adapter type, got %T", ext)
	}
	adapter.Invalidate()
	_, err = ext.ResolveTools(context.Background())
	if err == nil {
		t.Fatal("expected context error from ResolveTools")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestAdapterRefreshActiveMessageWithoutSkippedTools(t *testing.T) {
	client := &stubClient{
		discoverSnapshot: ServerSnapshot{
			ID:      "server",
			Name:    "Server",
			Version: "2.0.0",
			Tools: []ToolDescriptor{
				{Name: "echo"},
			},
		},
	}
	ext, err := FromMCPServer(ServerConfig{
		ID:      "server",
		Name:    "Server",
		Command: "stub",
	}, WithClient(client), func(opts *adapterOptions) {
		opts.now = func() time.Time {
			return time.Date(2026, 4, 21, 1, 2, 3, 0, time.UTC)
		}
	})
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	info := ext.Info()
	if info.Health.Message != "mcp server active" {
		t.Fatalf("expected active message without skipped tools, got %q", info.Health.Message)
	}
}

func TestAdapterResolveToolsUsesTTLCacheAndInvalidate(t *testing.T) {
	client := &stubClient{
		discoverSnapshot: ServerSnapshot{
			ID:      "cache",
			Name:    "Cache",
			Version: "1.0.0",
			Tools: []ToolDescriptor{
				{Name: "echo"},
			},
		},
	}
	ext, err := FromMCPServer(ServerConfig{
		ID:      "cache",
		Name:    "Cache",
		Command: "stub",
	}, WithClient(client), WithRefreshTTL(time.Hour))
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	adapter, ok := ext.(*Adapter)
	if !ok {
		t.Fatalf("expected *Adapter type, got %T", ext)
	}
	initialDiscoverCount := client.discoverCount

	_, err = ext.ResolveTools(context.Background())
	if err != nil {
		t.Fatalf("first ResolveTools failed: %v", err)
	}
	_, err = ext.ResolveTools(context.Background())
	if err != nil {
		t.Fatalf("second ResolveTools failed: %v", err)
	}
	if client.discoverCount != initialDiscoverCount {
		t.Fatalf("expected cached resolve without extra discover, got %d -> %d", initialDiscoverCount, client.discoverCount)
	}

	adapter.Invalidate()
	_, err = ext.ResolveTools(context.Background())
	if err != nil {
		t.Fatalf("ResolveTools after invalidate failed: %v", err)
	}
	if client.discoverCount != initialDiscoverCount+1 {
		t.Fatalf("expected one extra discover after invalidate, got %d -> %d", initialDiscoverCount, client.discoverCount)
	}

	if err := adapter.Reload(context.Background()); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if client.discoverCount != initialDiscoverCount+2 {
		t.Fatalf("expected force reload to discover again, got %d -> %d", initialDiscoverCount, client.discoverCount)
	}
}

func TestFromMCPServerWithEagerDiscoverDisabledDefersInitialDiscover(t *testing.T) {
	client := &stubClient{
		discoverSnapshot: ServerSnapshot{
			ID:      "lazy",
			Name:    "Lazy",
			Version: "1.0.0",
			Tools: []ToolDescriptor{
				{Name: "echo"},
			},
		},
	}
	ext, err := FromMCPServer(
		ServerConfig{
			ID:      "lazy",
			Name:    "Lazy",
			Command: "stub",
		},
		WithClient(client),
		WithEagerDiscover(false),
	)
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	if client.discoverCount != 0 {
		t.Fatalf("expected no discover during construction, got %d", client.discoverCount)
	}

	_, err = ext.ResolveTools(context.Background())
	if err != nil {
		t.Fatalf("ResolveTools failed: %v", err)
	}
	if client.discoverCount != 1 {
		t.Fatalf("expected discover to happen on first tool resolve, got %d", client.discoverCount)
	}
}

func TestMCPToolDefinitionSpecAndRunBranches(t *testing.T) {
	tool := mcpTool{
		descriptor: ToolDescriptor{},
	}
	def := tool.Definition()
	if def.Function.Name != "mcp_tool" {
		t.Fatalf("expected fallback name, got %q", def.Function.Name)
	}
	if def.Function.Description != "MCP tool mcp_tool" {
		t.Fatalf("expected fallback description, got %q", def.Function.Description)
	}
	if def.Function.Parameters["type"] != "object" {
		t.Fatalf("expected normalized object schema, got %#v", def.Function.Parameters)
	}

	spec := tool.Spec()
	if spec.SafetyClass != toolspkg.SafetyClassSensitive {
		t.Fatalf("expected sensitive safety class, got %q", spec.SafetyClass)
	}
	if spec.Name != "mcp_tool" {
		t.Fatalf("expected fallback spec name, got %q", spec.Name)
	}

	namedTool := mcpTool{
		descriptor: ToolDescriptor{Name: "echo", Description: "echo"},
		client:     &stubClient{callOutput: "ok"},
	}
	spec = namedTool.Spec()
	if spec.Name != "echo" {
		t.Fatalf("expected spec name to keep descriptor name, got %q", spec.Name)
	}

	tool = mcpTool{}
	_, err := tool.Run(context.Background(), json.RawMessage(`{}`), nil)
	if err == nil {
		t.Fatal("expected internal error when client is nil")
	}
	execErr, ok := toolspkg.AsToolExecError(err)
	if !ok || execErr.Code != toolspkg.ToolErrorInternal {
		t.Fatalf("expected internal tool error, got %v", err)
	}

	tool = mcpTool{
		client: &stubClient{
			callErr: &ClientError{Code: ClientErrorPermission, Message: "denied"},
		},
	}
	_, err = tool.Run(context.Background(), json.RawMessage(`{}`), nil)
	execErr, ok = toolspkg.AsToolExecError(err)
	if !ok || execErr.Code != toolspkg.ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tool = mcpTool{
		client:     &stubClient{callOutput: "ok"},
		descriptor: ToolDescriptor{Name: "echo"},
	}
	output, err := tool.Run(ctx, json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("expected success with deadline context, got %v", err)
	}
	if output != "ok" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestMCPToolSpecAppliesOverrideFields(t *testing.T) {
	readonly := true
	destructive := true
	tool := mcpTool{
		descriptor: ToolDescriptor{
			Name:        "echo",
			Description: "echo",
		},
		override: ToolOverride{
			SafetyClass:     "destructive",
			ReadOnly:        &readonly,
			Destructive:     &destructive,
			AllowedModes:    []string{"build", " plan ", "invalid"},
			DefaultTimeoutS: 7,
			MaxTimeoutS:     11,
			MaxResultChars:  4096,
		},
	}
	spec := tool.Spec()
	if spec.SafetyClass != toolspkg.SafetyClassDestructive {
		t.Fatalf("expected destructive safety class, got %q", spec.SafetyClass)
	}
	if !spec.ReadOnly {
		t.Fatal("expected readonly override to be applied")
	}
	if spec.Destructive {
		t.Fatal("expected readonly normalization to clear destructive flag")
	}
	if spec.DefaultTimeoutS != 7 || spec.MaxTimeoutS != 11 || spec.MaxResultChars != 4096 {
		t.Fatalf("unexpected timeout/result overrides: %#v", spec)
	}
	if len(spec.AllowedModes) != 2 {
		t.Fatalf("expected 2 normalized allowed modes, got %#v", spec.AllowedModes)
	}
}

func TestMCPToolSpecSkipsInvalidAllowedModes(t *testing.T) {
	tool := mcpTool{
		descriptor: ToolDescriptor{
			Name: "echo",
		},
		override: ToolOverride{
			AllowedModes: []string{"invalid", ""},
		},
	}
	spec := tool.Spec()
	if len(spec.AllowedModes) != 1 || spec.AllowedModes[0] != "build" {
		t.Fatalf("expected fallback default allowed mode, got %#v", spec.AllowedModes)
	}
}

func TestOverrideForTool(t *testing.T) {
	if got := overrideForTool(nil, "echo"); got.ReadOnly != nil || got.Destructive != nil || got.SafetyClass != "" || len(got.AllowedModes) != 0 {
		t.Fatalf("expected empty override for nil map, got %#v", got)
	}

	readonly := true
	overrides := map[string]ToolOverride{
		"echo": {ReadOnly: &readonly},
	}
	got := overrideForTool(overrides, " Echo ")
	if got.ReadOnly == nil || !*got.ReadOnly {
		t.Fatalf("expected matched override for normalized key, got %#v", got)
	}
	if got := overrideForTool(overrides, "missing"); got.ReadOnly != nil || got.Destructive != nil || got.SafetyClass != "" || len(got.AllowedModes) != 0 {
		t.Fatalf("expected empty override for missing key, got %#v", got)
	}
}
