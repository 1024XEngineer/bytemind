package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
	toolspkg "github.com/1024XEngineer/bytemind/internal/tools"
)

func TestFromMCPServerBuildsActiveExtensionAndMapsTools(t *testing.T) {
	client := &stubClient{
		discoverSnapshot: ServerSnapshot{
			ID:      "github",
			Name:    "GitHub MCP",
			Version: "1.0.0",
			Tools: []ToolDescriptor{
				{
					Name:        "list_prs",
					Description: "List PRs",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"repo": map[string]any{"type": "string"},
						},
					},
				},
				{
					Name:        "",
					Description: "invalid",
				},
			},
		},
		callOutput: "ok",
	}

	ext, err := FromMCPServer(ServerConfig{
		ID:      "github",
		Name:    "GitHub MCP",
		Command: "stub",
	}, WithClient(client))
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}

	info := ext.Info()
	if info.ID != "mcp.github" {
		t.Fatalf("unexpected extension id: %q", info.ID)
	}
	if info.Kind != extensionspkg.ExtensionMCP {
		t.Fatalf("unexpected extension kind: %q", info.Kind)
	}
	if info.Status != extensionspkg.ExtensionStatusActive {
		t.Fatalf("expected active status, got %q", info.Status)
	}
	if info.Capabilities.Tools != 1 {
		t.Fatalf("expected 1 valid tool, got %d", info.Capabilities.Tools)
	}

	tools, err := ext.ResolveTools(context.Background())
	if err != nil {
		t.Fatalf("ResolveTools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 bridged tool, got %d", len(tools))
	}
	if tools[0].Source != extensionspkg.ExtensionMCP {
		t.Fatalf("unexpected tool source: %q", tools[0].Source)
	}
	if tools[0].ExtensionID != "mcp.github" {
		t.Fatalf("unexpected extension id: %q", tools[0].ExtensionID)
	}

	definition := tools[0].Tool.Definition()
	if definition.Function.Name != "list_prs" {
		t.Fatalf("unexpected tool definition name: %q", definition.Function.Name)
	}

	output, err := tools[0].Tool.Run(context.Background(), json.RawMessage(`{"repo":"openai/openai"}`), nil)
	if err != nil {
		t.Fatalf("tool run failed: %v", err)
	}
	if output != "ok" {
		t.Fatalf("unexpected tool output: %q", output)
	}
	if client.callCount != 1 {
		t.Fatalf("expected one mcp call, got %d", client.callCount)
	}
}

func TestFromMCPServerHandshakeFailureMarksDegraded(t *testing.T) {
	client := &stubClient{
		discoverErr: &ClientError{
			Code:    ClientErrorHandshakeFailed,
			Message: "handshake failed",
		},
	}

	ext, err := FromMCPServer(ServerConfig{
		ID:      "broken",
		Name:    "Broken Server",
		Command: "stub",
	}, WithClient(client))
	if err != nil {
		t.Fatalf("FromMCPServer should not fail hard on handshake issue, got %v", err)
	}

	info := ext.Info()
	if info.Status != extensionspkg.ExtensionStatusDegraded {
		t.Fatalf("expected degraded status, got %q", info.Status)
	}
	if info.Health.Status != extensionspkg.ExtensionStatusDegraded {
		t.Fatalf("expected degraded health status, got %q", info.Health.Status)
	}
	if info.Health.LastError != extensionspkg.ErrCodeLoadFailed {
		t.Fatalf("expected mapped load_failed code, got %q", info.Health.LastError)
	}

	tools, err := ext.ResolveTools(context.Background())
	if err != nil {
		t.Fatalf("ResolveTools should not fail hard when degraded, got %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("expected no tools from degraded extension, got %d", len(tools))
	}

	health, healthErr := ext.Health(context.Background())
	adapter, ok := ext.(*Adapter)
	if !ok {
		t.Fatalf("expected *Adapter type, got %T", ext)
	}
	adapter.Invalidate()
	health, healthErr = ext.Health(context.Background())
	if healthErr == nil {
		t.Fatal("expected health check to surface handshake error after invalidate")
	}
	if health.Status != extensionspkg.ExtensionStatusDegraded {
		t.Fatalf("expected degraded health snapshot, got %q", health.Status)
	}
}

func TestAdapterRefreshFailureFallsBackToStaleSnapshot(t *testing.T) {
	client := &stubClient{
		discoverSnapshot: ServerSnapshot{
			ID:      "docs",
			Name:    "Docs MCP",
			Version: "1.0.0",
			Tools: []ToolDescriptor{
				{
					Name:        "search_docs",
					Description: "Search docs",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		},
	}
	ext, err := FromMCPServer(ServerConfig{
		ID:      "docs",
		Name:    "Docs MCP",
		Command: "stub",
	}, WithClient(client))
	if err != nil {
		t.Fatalf("FromMCPServer failed: %v", err)
	}
	initialTools, err := ext.ResolveTools(context.Background())
	if err != nil {
		t.Fatalf("initial ResolveTools failed: %v", err)
	}
	if len(initialTools) != 1 {
		t.Fatalf("expected one initial tool, got %d", len(initialTools))
	}

	client.discoverErr = &ClientError{
		Code:    ClientErrorListToolsFailed,
		Message: "tools/list failed",
	}
	adapter := ext.(*Adapter)
	adapter.Invalidate()

	staleTools, err := ext.ResolveTools(context.Background())
	if err != nil {
		t.Fatalf("ResolveTools should fallback to stale snapshot on non-context refresh error, got %v", err)
	}
	if len(staleTools) != 1 {
		t.Fatalf("expected stale snapshot to retain one tool, got %d", len(staleTools))
	}
	info := ext.Info()
	if info.Status != extensionspkg.ExtensionStatusDegraded {
		t.Fatalf("expected degraded status after refresh failure, got %q", info.Status)
	}
	if info.Health.LastError != extensionspkg.ErrCodeLoadFailed {
		t.Fatalf("expected load_failed error code, got %q", info.Health.LastError)
	}
	if got := info.Health.Message; got == "" || !strings.Contains(got, "stale_snapshot_fallback") {
		t.Fatalf("expected stale snapshot reason code in health message, got %q", got)
	}
}

type stubClient struct {
	discoverSnapshot ServerSnapshot
	discoverErr      error
	callOutput       string
	callErr          error
	discoverCount    int
	callCount        int
}

func (s *stubClient) Discover(context.Context, ServerConfig) (ServerSnapshot, error) {
	s.discoverCount++
	if s.discoverErr != nil {
		return ServerSnapshot{}, s.discoverErr
	}
	return s.discoverSnapshot, nil
}

func (s *stubClient) CallTool(_ context.Context, _ ServerConfig, _ string, _ json.RawMessage) (string, error) {
	s.callCount++
	if s.callErr != nil {
		return "", s.callErr
	}
	return s.callOutput, nil
}

func TestMapClientErrorToToolExecError(t *testing.T) {
	err := mapClientErrorToToolExecError(&ClientError{Code: ClientErrorInvalidArgs, Message: "bad args"})
	if err == nil {
		t.Fatal("expected mapped error")
	}
	execErr, ok := toolspkg.AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != toolspkg.ToolErrorInvalidArgs {
		t.Fatalf("expected invalid_args, got %q", execErr.Code)
	}
}
