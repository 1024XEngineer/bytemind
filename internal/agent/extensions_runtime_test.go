package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	extensionspkg "bytemind/internal/extensions"
	"bytemind/internal/llm"
	toolspkg "bytemind/internal/tools"
)

type runtimeSyncStubManager struct {
	items           []extensionspkg.ExtensionTool
	resolveCount    int
	invalidateCount int
	resolveStarted  chan struct{}
	resolveRelease  chan struct{}
	resolveOnce     sync.Once
}

func (m *runtimeSyncStubManager) Load(context.Context, string) (extensionspkg.ExtensionInfo, error) {
	return extensionspkg.ExtensionInfo{}, nil
}

func (m *runtimeSyncStubManager) Unload(context.Context, string) error {
	return nil
}

func (m *runtimeSyncStubManager) Get(context.Context, string) (extensionspkg.ExtensionInfo, error) {
	return extensionspkg.ExtensionInfo{}, nil
}

func (m *runtimeSyncStubManager) List(context.Context) ([]extensionspkg.ExtensionInfo, error) {
	return nil, nil
}

func (m *runtimeSyncStubManager) ResolveAllTools(context.Context) ([]extensionspkg.ExtensionTool, error) {
	m.resolveCount++
	if m.resolveStarted != nil {
		m.resolveOnce.Do(func() {
			close(m.resolveStarted)
		})
	}
	if m.resolveRelease != nil {
		<-m.resolveRelease
	}
	out := make([]extensionspkg.ExtensionTool, len(m.items))
	copy(out, m.items)
	return out, nil
}

func (m *runtimeSyncStubManager) Invalidate(string) {
	m.invalidateCount++
}

type runtimeSyncTool struct {
	name string
}

func (t runtimeSyncTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:       t.name,
			Parameters: map[string]any{"type": "object"},
		},
	}
}

func (runtimeSyncTool) Run(context.Context, json.RawMessage, *toolspkg.ExecutionContext) (string, error) {
	return "ok", nil
}

func TestSyncExtensionToolsRegistersAndRemovesMCPTools(t *testing.T) {
	registry := toolspkg.DefaultRegistry()
	manager := &runtimeSyncStubManager{
		items: []extensionspkg.ExtensionTool{
			{
				Source:      extensionspkg.ExtensionMCP,
				ExtensionID: "mcp.docs",
				Tool:        runtimeSyncTool{name: "fetch_doc"},
			},
		},
	}
	runner := &Runner{
		registry:           registry,
		extensions:         manager,
		extensionSyncTTL:   time.Minute,
		extensionSyncDirty: true,
		extensionToolKeys:  map[string]map[string]struct{}{},
	}

	if err := runner.syncExtensionTools(context.Background(), true); err != nil {
		t.Fatalf("syncExtensionTools register failed: %v", err)
	}
	if manager.resolveCount != 1 {
		t.Fatalf("expected one resolve call, got %d", manager.resolveCount)
	}
	if _, ok := registry.Get("mcp:docs:fetch_doc"); !ok {
		t.Fatalf("expected stable mcp tool key to be registered")
	}

	manager.items = nil
	if err := runner.syncExtensionTools(context.Background(), true); err != nil {
		t.Fatalf("syncExtensionTools cleanup failed: %v", err)
	}
	if _, ok := registry.Get("mcp:docs:fetch_doc"); ok {
		t.Fatalf("expected stable mcp tool key to be removed after sync")
	}
}

func TestSyncExtensionToolsHonorsTTLAndInvalidate(t *testing.T) {
	registry := toolspkg.DefaultRegistry()
	manager := &runtimeSyncStubManager{
		items: []extensionspkg.ExtensionTool{
			{
				Source:      extensionspkg.ExtensionMCP,
				ExtensionID: "mcp.docs",
				Tool:        runtimeSyncTool{name: "fetch_doc"},
			},
		},
	}
	runner := &Runner{
		registry:           registry,
		extensions:         manager,
		extensionSyncTTL:   time.Hour,
		extensionSyncDirty: true,
		extensionToolKeys:  map[string]map[string]struct{}{},
	}

	if err := runner.syncExtensionTools(context.Background(), false); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}
	if err := runner.syncExtensionTools(context.Background(), false); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if manager.resolveCount != 1 {
		t.Fatalf("expected ttl to skip duplicate sync, resolveCount=%d", manager.resolveCount)
	}

	runner.invalidateExtensionTools("mcp.docs")
	if manager.invalidateCount != 1 {
		t.Fatalf("expected extension invalidator to be called once, got %d", manager.invalidateCount)
	}
	if err := runner.syncExtensionTools(context.Background(), false); err != nil {
		t.Fatalf("sync after invalidate failed: %v", err)
	}
	if manager.resolveCount != 2 {
		t.Fatalf("expected invalidate to force sync, resolveCount=%d", manager.resolveCount)
	}
}

func TestSyncExtensionToolsDoesNotHoldLockDuringResolve(t *testing.T) {
	registry := toolspkg.DefaultRegistry()
	resolveStarted := make(chan struct{})
	resolveRelease := make(chan struct{})
	manager := &runtimeSyncStubManager{
		resolveStarted: resolveStarted,
		resolveRelease: resolveRelease,
	}
	runner := &Runner{
		registry:           registry,
		extensions:         manager,
		extensionSyncTTL:   time.Minute,
		extensionSyncDirty: true,
		extensionToolKeys:  map[string]map[string]struct{}{},
	}

	syncErr := make(chan error, 1)
	go func() {
		syncErr <- runner.syncExtensionTools(context.Background(), true)
	}()

	select {
	case <-resolveStarted:
	case <-time.After(time.Second):
		t.Fatal("sync did not reach resolve stage")
	}

	invalidateDone := make(chan struct{})
	go func() {
		runner.invalidateExtensionTools("mcp.docs")
		close(invalidateDone)
	}()

	select {
	case <-invalidateDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("invalidate blocked while resolve was in progress")
	}

	close(resolveRelease)
	if err := <-syncErr; err != nil {
		t.Fatalf("sync failed: %v", err)
	}
}
