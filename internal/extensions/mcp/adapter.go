package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	extensionspkg "bytemind/internal/extensions"
	"bytemind/internal/llm"
	toolspkg "bytemind/internal/tools"
)

type Option func(*adapterOptions)

type adapterOptions struct {
	client     Client
	now        func() time.Time
	refreshTTL time.Duration
}

func WithClient(client Client) Option {
	return func(opts *adapterOptions) {
		if opts == nil {
			return
		}
		opts.client = client
	}
}

func WithRefreshTTL(ttl time.Duration) Option {
	return func(opts *adapterOptions) {
		if opts == nil {
			return
		}
		opts.refreshTTL = ttl
	}
}

const defaultRefreshTTL = 30 * time.Second

type Adapter struct {
	mu        sync.RWMutex
	refreshMu sync.Mutex

	cfg        ServerConfig
	client     Client
	now        func() time.Time
	refreshTTL time.Duration

	info        extensionspkg.ExtensionInfo
	tools       []ToolDescriptor
	lastRefresh time.Time
	dirty       bool
}

func FromMCPServer(cfg ServerConfig, opts ...Option) (extensionspkg.Extension, error) {
	options := adapterOptions{
		now:        time.Now,
		refreshTTL: defaultRefreshTTL,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&options)
	}
	cfg = normalizeServerConfig(cfg)
	if options.client == nil {
		options.client = NewStdioClient()
	}
	if options.now == nil {
		options.now = time.Now
	}
	if options.refreshTTL <= 0 {
		options.refreshTTL = defaultRefreshTTL
	}
	if err := validateServerConfig(cfg, true); err != nil {
		return nil, toExtensionError(err, extensionspkg.ErrCodeInvalidSource, "invalid mcp server config")
	}
	if options.client == nil {
		return nil, newExtensionError(extensionspkg.ErrCodeInvalidSource, "mcp client is required", nil)
	}

	adapter := &Adapter{
		cfg:    cfg,
		client: options.client,
		now:    options.now,
		info:   baseExtensionInfo(cfg, options.now()),
		tools:  nil,

		refreshTTL:  options.refreshTTL,
		lastRefresh: time.Time{},
		dirty:       true,
	}

	startupCtx, cancel := withTimeoutIfMissing(context.Background(), cfg.StartupTimeout)
	defer cancel()
	_ = adapter.maybeRefresh(startupCtx, true)
	return adapter, nil
}

func (a *Adapter) Info() extensionspkg.ExtensionInfo {
	if a == nil {
		return extensionspkg.ExtensionInfo{}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.info
}

func (a *Adapter) ResolveTools(ctx context.Context) ([]extensionspkg.ExtensionTool, error) {
	if a == nil {
		return nil, newExtensionError(extensionspkg.ErrCodeInvalidExtension, "mcp adapter is nil", nil)
	}
	if err := a.maybeRefresh(ctx, false); err != nil && contextError(err) != nil {
		return nil, err
	}

	a.mu.RLock()
	descriptors := cloneToolDescriptors(a.tools)
	extensionID := a.info.ID
	client := a.client
	cfg := a.cfg
	a.mu.RUnlock()

	tools := make([]extensionspkg.ExtensionTool, 0, len(descriptors))
	for _, descriptor := range descriptors {
		name := strings.TrimSpace(descriptor.Name)
		if name == "" {
			continue
		}
		tools = append(tools, extensionspkg.ExtensionTool{
			Source:      extensionspkg.ExtensionMCP,
			ExtensionID: extensionID,
			Tool: mcpTool{
				server:     cfg,
				client:     client,
				descriptor: descriptor,
			},
		})
	}
	return tools, nil
}

func (a *Adapter) Health(ctx context.Context) (extensionspkg.HealthSnapshot, error) {
	if a == nil {
		return extensionspkg.HealthSnapshot{}, newExtensionError(extensionspkg.ErrCodeInvalidExtension, "mcp adapter is nil", nil)
	}
	err := a.maybeRefresh(ctx, false)
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.info.Health, err
}

func (a *Adapter) Reload(ctx context.Context) error {
	return a.maybeRefresh(ctx, true)
}

func (a *Adapter) Invalidate() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dirty = true
}

func (a *Adapter) maybeRefresh(ctx context.Context, force bool) error {
	if a == nil {
		return newExtensionError(extensionspkg.ErrCodeInvalidExtension, "mcp adapter is nil", nil)
	}
	if !force {
		a.mu.RLock()
		should := a.shouldRefreshLocked(a.now().UTC())
		a.mu.RUnlock()
		if !should {
			return nil
		}
	}

	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()

	if !force {
		a.mu.RLock()
		should := a.shouldRefreshLocked(a.now().UTC())
		a.mu.RUnlock()
		if !should {
			return nil
		}
	}
	return a.refresh(ctx)
}

func (a *Adapter) shouldRefreshLocked(now time.Time) bool {
	if a == nil {
		return false
	}
	if a.dirty || a.lastRefresh.IsZero() {
		return true
	}
	if a.refreshTTL <= 0 {
		return true
	}
	return now.Sub(a.lastRefresh) >= a.refreshTTL
}

func (a *Adapter) refresh(ctx context.Context) error {
	if a == nil {
		return newExtensionError(extensionspkg.ErrCodeInvalidExtension, "mcp adapter is nil", nil)
	}
	now := a.now().UTC()
	snapshot, err := a.client.Discover(ctx, a.cfg)
	if err != nil {
		code := mapClientErrorToExtensionCode(err)
		a.markDegraded(err, code, now)
		return toExtensionError(err, code, "mcp discovery failed")
	}

	validTools, skipped := filterValidToolDescriptors(snapshot.Tools)
	a.mu.Lock()
	defer a.mu.Unlock()

	if strings.TrimSpace(snapshot.Name) != "" {
		a.info.Name = strings.TrimSpace(snapshot.Name)
		a.info.Title = strings.TrimSpace(snapshot.Name)
		a.info.Manifest.Name = strings.TrimSpace(snapshot.Name)
		a.info.Manifest.Title = strings.TrimSpace(snapshot.Name)
	}
	if strings.TrimSpace(snapshot.Version) != "" {
		a.info.Version = strings.TrimSpace(snapshot.Version)
		a.info.Manifest.Version = strings.TrimSpace(snapshot.Version)
	}
	a.tools = validTools
	a.info.Status = extensionspkg.ExtensionStatusActive
	a.info.Capabilities.Tools = len(validTools)
	a.info.Manifest.Capabilities.Tools = len(validTools)
	a.info.Health.Status = extensionspkg.ExtensionStatusActive
	a.info.Health.LastError = ""
	a.info.Health.CheckedAtUTC = now.Format(time.RFC3339)
	a.lastRefresh = now
	a.dirty = false
	if skipped > 0 {
		a.info.Health.Message = fmt.Sprintf("mcp server active; skipped %d invalid tool declarations", skipped)
	} else {
		a.info.Health.Message = "mcp server active"
	}
	return nil
}

func (a *Adapter) markDegraded(err error, code extensionspkg.ErrorCode, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.info.Status = extensionspkg.ExtensionStatusDegraded
	a.info.Health.Status = extensionspkg.ExtensionStatusDegraded
	a.info.Health.LastError = code
	a.info.Health.Message = strings.TrimSpace(err.Error())
	a.info.Health.CheckedAtUTC = now.Format(time.RFC3339)
	a.lastRefresh = now
	a.dirty = false
}

func baseExtensionInfo(cfg ServerConfig, now time.Time) extensionspkg.ExtensionInfo {
	extensionID := "mcp." + cfg.ID
	manifestRef := "mcp:" + cfg.ID
	return extensionspkg.ExtensionInfo{
		ID:          extensionID,
		Name:        cfg.Name,
		Kind:        extensionspkg.ExtensionMCP,
		Version:     cfg.Version,
		Title:       cfg.Name,
		Description: fmt.Sprintf("MCP server %s", cfg.Name),
		Source: extensionspkg.ExtensionSource{
			Scope: extensionspkg.ExtensionScopeRemote,
			Ref:   manifestRef,
		},
		Status: extensionspkg.ExtensionStatusLoaded,
		Manifest: extensionspkg.Manifest{
			Name:        cfg.Name,
			Version:     cfg.Version,
			Title:       cfg.Name,
			Description: fmt.Sprintf("MCP server %s", cfg.Name),
			Kind:        extensionspkg.ExtensionMCP,
			Source: extensionspkg.ExtensionSource{
				Scope: extensionspkg.ExtensionScopeRemote,
				Ref:   manifestRef,
			},
		},
		Health: extensionspkg.HealthSnapshot{
			Status:       extensionspkg.ExtensionStatusLoaded,
			Message:      "mcp server loaded",
			CheckedAtUTC: now.UTC().Format(time.RFC3339),
		},
	}
}

type mcpTool struct {
	server     ServerConfig
	client     Client
	descriptor ToolDescriptor
}

func (t mcpTool) Definition() llm.ToolDefinition {
	name := strings.TrimSpace(t.descriptor.Name)
	if name == "" {
		name = "mcp_tool"
	}
	description := strings.TrimSpace(t.descriptor.Description)
	if description == "" {
		description = fmt.Sprintf("MCP tool %s", name)
	}
	parameters := normalizedSchema(t.descriptor.InputSchema)
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

func (t mcpTool) Spec() toolspkg.ToolSpec {
	base := toolspkg.DefaultToolSpec(t.Definition())
	return toolspkg.MergeToolSpec(base, toolspkg.ToolSpec{
		Name:        strings.TrimSpace(t.descriptor.Name),
		SafetyClass: toolspkg.SafetyClassSensitive,
	})
}

func (t mcpTool) Run(ctx context.Context, raw json.RawMessage, _ *toolspkg.ExecutionContext) (string, error) {
	if t.client == nil {
		return "", toolspkg.NewToolExecError(toolspkg.ToolErrorInternal, "mcp client is unavailable", true, nil)
	}
	callCtx := ctx
	cancel := func() {}
	if _, has := ctx.Deadline(); !has && t.server.CallTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, t.server.CallTimeout)
	}
	defer cancel()

	output, err := t.client.CallTool(callCtx, t.server, t.descriptor.Name, raw)
	if err != nil {
		return "", mapClientErrorToToolExecError(err)
	}
	return output, nil
}
