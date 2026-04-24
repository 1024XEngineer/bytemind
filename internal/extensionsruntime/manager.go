package extensionsruntime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	configpkg "bytemind/internal/config"
	extensionspkg "bytemind/internal/extensions"
	mcppkg "bytemind/internal/extensions/mcp"
)

type Manager struct {
	mu sync.RWMutex

	workspace  string
	configPath string
	base       extensionspkg.Manager

	disabledMCP map[string]struct{}
	entries     map[string]*mcpEntry
}

type mcpEntry struct {
	server      configpkg.MCPServerConfig
	clientCfg   mcppkg.ServerConfig
	extension   extensionspkg.Extension
	info        extensionspkg.ExtensionInfo
	lastRefresh time.Time
	lastErr     error
}

func NewManager(workspace, configPath string, base extensionspkg.Manager, cfg configpkg.Config) *Manager {
	if base == nil {
		base = extensionspkg.NewManager(workspace)
	}
	manager := &Manager{
		workspace:   strings.TrimSpace(workspace),
		configPath:  strings.TrimSpace(configPath),
		base:        base,
		disabledMCP: map[string]struct{}{},
		entries:     map[string]*mcpEntry{},
	}
	manager.applyConfig(cfg.MCP)
	return manager
}

func (m *Manager) Load(ctx context.Context, source string) (extensionspkg.ExtensionInfo, error) {
	extensionID, _, isMCP := normalizeMCPInput(source)
	if !isMCP {
		return m.base.Load(ctx, source)
	}

	m.mu.Lock()
	delete(m.disabledMCP, extensionID)
	m.mu.Unlock()
	if err := m.refresh(ctx, false); err != nil {
		return extensionspkg.ExtensionInfo{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[extensionID]
	if !ok || entry == nil {
		return extensionspkg.ExtensionInfo{}, &extensionspkg.ExtensionError{
			Code:    extensionspkg.ErrCodeNotFound,
			Message: fmt.Sprintf("mcp extension %q not found", extensionID),
		}
	}

	if reloader, ok := entry.extension.(interface{ Reload(context.Context) error }); ok {
		if err := reloader.Reload(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return extensionspkg.ExtensionInfo{}, err
			}
			now := time.Now().UTC()
			entry.info = normalizeMCPInfo(entry.extension.Info(), entry.server, now)
			entry.lastRefresh = now
			entry.lastErr = err
			m.entries[extensionID] = entry
			return cloneInfo(entry.info), err
		}
		now := time.Now().UTC()
		entry.info = normalizeMCPInfo(entry.extension.Info(), entry.server, now)
		entry.lastRefresh = now
		entry.lastErr = nil
		m.entries[extensionID] = entry
	}
	return cloneInfo(entry.info), nil
}

func (m *Manager) Unload(ctx context.Context, extensionID string) error {
	normalizedID, _, isMCP := normalizeMCPInput(extensionID)
	if !isMCP {
		return m.base.Unload(ctx, extensionID)
	}

	m.mu.Lock()
	m.disabledMCP[normalizedID] = struct{}{}
	delete(m.entries, normalizedID)
	m.mu.Unlock()
	_ = ctx
	return nil
}

func (m *Manager) Get(ctx context.Context, extensionID string) (extensionspkg.ExtensionInfo, error) {
	normalizedID, _, isMCP := normalizeMCPInput(extensionID)
	if !isMCP {
		return m.base.Get(ctx, extensionID)
	}

	if err := m.refresh(ctx, false); err != nil {
		m.mu.RLock()
		entry, ok := m.entries[normalizedID]
		m.mu.RUnlock()
		if ok {
			return cloneInfo(entry.info), err
		}
		return extensionspkg.ExtensionInfo{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.entries[normalizedID]
	if !ok {
		return extensionspkg.ExtensionInfo{}, &extensionspkg.ExtensionError{
			Code:    extensionspkg.ErrCodeNotFound,
			Message: fmt.Sprintf("mcp extension %q not found", normalizedID),
		}
	}
	return cloneInfo(entry.info), nil
}

func (m *Manager) List(ctx context.Context) ([]extensionspkg.ExtensionInfo, error) {
	skillItems, skillErr := m.base.List(ctx)
	mcpErr := m.refresh(ctx, false)

	m.mu.RLock()
	mcpItems := make([]extensionspkg.ExtensionInfo, 0, len(m.entries))
	for _, entry := range m.entries {
		mcpItems = append(mcpItems, cloneInfo(entry.info))
	}
	m.mu.RUnlock()

	items := make([]extensionspkg.ExtensionInfo, 0, len(skillItems)+len(mcpItems))
	items = append(items, skillItems...)
	items = append(items, mcpItems...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	return items, mergeErrors(skillErr, mcpErr)
}

func (m *Manager) Reload(ctx context.Context) error {
	var baseErr error
	if reloader, ok := m.base.(extensionspkg.Reloader); ok {
		baseErr = reloader.Reload(ctx)
	} else {
		_, baseErr = m.base.List(ctx)
	}
	mcpErr := m.refresh(ctx, true)
	return mergeErrors(baseErr, mcpErr)
}

func (m *Manager) ResolveAllTools(ctx context.Context) ([]extensionspkg.ExtensionTool, error) {
	if err := m.refresh(ctx, false); err != nil {
		// Keep degraded MCP non-fatal; only return context cancellation errors.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
	}

	m.mu.RLock()
	entryIDs := make([]string, 0, len(m.entries))
	for extensionID := range m.entries {
		entryIDs = append(entryIDs, extensionID)
	}
	sort.Strings(entryIDs)
	entries := make([]*mcpEntry, 0, len(entryIDs))
	for _, extensionID := range entryIDs {
		entries = append(entries, m.entries[extensionID])
	}
	m.mu.RUnlock()

	var firstErr error
	tools := make([]extensionspkg.ExtensionTool, 0, 16)
	for _, entry := range entries {
		if entry == nil || entry.extension == nil {
			continue
		}
		resolved, err := entry.extension.ResolveTools(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, item := range resolved {
			if item.Source != extensionspkg.ExtensionMCP {
				continue
			}
			tools = append(tools, item)
		}
	}
	return tools, firstErr
}

func (m *Manager) Test(ctx context.Context, extensionID string) (extensionspkg.HealthSnapshot, error) {
	normalizedID, serverID, isMCP := normalizeMCPInput(extensionID)
	if !isMCP {
		return extensionspkg.HealthSnapshot{}, &extensionspkg.ExtensionError{
			Code:    extensionspkg.ErrCodeInvalidExtension,
			Message: "test only supports mcp extensions",
		}
	}
	if err := m.refresh(ctx, false); err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return extensionspkg.HealthSnapshot{}, err
	}

	m.mu.RLock()
	entry := m.entries[normalizedID]
	m.mu.RUnlock()
	if entry == nil {
		return extensionspkg.HealthSnapshot{}, &extensionspkg.ExtensionError{
			Code:    extensionspkg.ErrCodeNotFound,
			Message: fmt.Sprintf("mcp extension %q not found", normalizedID),
		}
	}

	if entry.extension != nil {
		if reloader, ok := entry.extension.(interface{ Reload(context.Context) error }); ok {
			_ = reloader.Reload(ctx)
		}
		return entry.extension.Health(ctx)
	}

	if entry.clientCfg.ID == "" {
		return entry.info.Health, entry.lastErr
	}
	ext, err := mcppkg.FromMCPServer(entry.clientCfg, mcppkg.WithRefreshTTL(time.Second))
	if err != nil {
		return extensionspkg.HealthSnapshot{
			Status:       extensionspkg.ExtensionStatusFailed,
			Message:      err.Error(),
			LastError:    extensionspkg.ErrCodeLoadFailed,
			CheckedAtUTC: time.Now().UTC().Format(time.RFC3339),
		}, err
	}
	health, err := ext.Health(ctx)
	if err != nil {
		return health, err
	}

	_ = serverID
	return health, nil
}

func (m *Manager) Invalidate(extensionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(extensionID) == "" {
		for _, entry := range m.entries {
			invalidateEntry(entry)
		}
		return
	}

	normalizedID, _, isMCP := normalizeMCPInput(extensionID)
	if !isMCP {
		return
	}
	invalidateEntry(m.entries[normalizedID])
}

func invalidateEntry(entry *mcpEntry) {
	if entry == nil || entry.extension == nil {
		return
	}
	if invalidator, ok := entry.extension.(interface{ Invalidate() }); ok {
		invalidator.Invalidate()
	}
}

func (m *Manager) refresh(ctx context.Context, force bool) error {
	cfg, err := configpkg.Load(m.workspace, m.configPath)
	if err != nil {
		return err
	}
	m.applyConfig(cfg.MCP)

	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	now := time.Now().UTC()
	for extensionID, entry := range m.entries {
		if entry == nil || entry.extension == nil {
			continue
		}
		if force {
			if reloader, ok := entry.extension.(interface{ Reload(context.Context) error }); ok {
				if err := reloader.Reload(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && firstErr == nil {
					firstErr = err
				}
			}
		}
		info := entry.extension.Info()
		entry.info = normalizeMCPInfo(info, entry.server, now)
		entry.lastRefresh = now
		m.entries[extensionID] = entry
	}
	return firstErr
}

func (m *Manager) applyConfig(cfg configpkg.MCPConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	if !cfg.Enabled {
		m.entries = map[string]*mcpEntry{}
		return
	}

	refreshTTL := time.Duration(cfg.SyncTTLSeconds) * time.Second
	desired := map[string]configpkg.MCPServerConfig{}
	for _, server := range cfg.Servers {
		if !server.EnabledValue() {
			continue
		}
		extensionID := mcpExtensionID(server.ID)
		if _, disabled := m.disabledMCP[extensionID]; disabled {
			continue
		}
		desired[extensionID] = server
	}

	nextEntries := make(map[string]*mcpEntry, len(desired))
	for extensionID, server := range desired {
		clientCfg := toMCPServerConfig(server)
		existing := m.entries[extensionID]
		if !server.AutoStartValue() {
			nextEntries[extensionID] = &mcpEntry{
				server:      server,
				clientCfg:   clientCfg,
				extension:   nil,
				info:        readyMCPInfo(server, now),
				lastRefresh: now,
			}
			continue
		}

		if existing != nil && existing.extension != nil && reflect.DeepEqual(existing.clientCfg, clientCfg) {
			nextEntries[extensionID] = existing
			continue
		}

		ext, err := mcppkg.FromMCPServer(
			clientCfg,
			mcppkg.WithRefreshTTL(refreshTTL),
			mcppkg.WithEagerDiscover(false),
		)
		if err != nil {
			nextEntries[extensionID] = &mcpEntry{
				server:      server,
				clientCfg:   clientCfg,
				extension:   nil,
				info:        failedMCPInfo(server, err, now),
				lastRefresh: now,
				lastErr:     err,
			}
			continue
		}

		info := normalizeMCPInfo(ext.Info(), server, now)
		nextEntries[extensionID] = &mcpEntry{
			server:      server,
			clientCfg:   clientCfg,
			extension:   ext,
			info:        info,
			lastRefresh: now,
		}
	}
	m.entries = nextEntries
}

func toMCPServerConfig(server configpkg.MCPServerConfig) mcppkg.ServerConfig {
	env := make(map[string]string, len(server.Transport.Env))
	for key, value := range server.Transport.Env {
		env[key] = value
	}
	overrides := make(map[string]mcppkg.ToolOverride, len(server.ToolOverrides))
	for _, override := range server.ToolOverrides {
		toolName := strings.ToLower(strings.TrimSpace(override.ToolName))
		if toolName == "" {
			continue
		}
		overrides[toolName] = mcppkg.ToolOverride{
			SafetyClass:     strings.ToLower(strings.TrimSpace(override.SafetyClass)),
			ReadOnly:        override.ReadOnly,
			Destructive:     override.Destructive,
			AllowedModes:    append([]string(nil), override.AllowedModes...),
			DefaultTimeoutS: override.DefaultTimeoutS,
			MaxTimeoutS:     override.MaxTimeoutS,
			MaxResultChars:  override.MaxResultChars,
		}
	}

	return mcppkg.ServerConfig{
		ID:               strings.TrimSpace(server.ID),
		Name:             strings.TrimSpace(server.Name),
		Version:          "1.0.0",
		ProtocolVersion:  strings.TrimSpace(server.ProtocolVersion),
		ProtocolVersions: append([]string(nil), server.ProtocolVersions...),
		Command:          strings.TrimSpace(server.Transport.Command),
		Args:             append([]string(nil), server.Transport.Args...),
		Env:              env,
		CWD:              strings.TrimSpace(server.Transport.CWD),
		StartupTimeout:   time.Duration(server.StartupTimeoutSeconds) * time.Second,
		CallTimeout:      time.Duration(server.CallTimeoutSeconds) * time.Second,
		MaxConcurrency:   server.MaxConcurrency,
		ToolOverrides:    overrides,
	}
}

func normalizeMCPInfo(info extensionspkg.ExtensionInfo, server configpkg.MCPServerConfig, now time.Time) extensionspkg.ExtensionInfo {
	normalized := cloneInfo(info)
	normalized.ID = mcpExtensionID(server.ID)
	normalized.Kind = extensionspkg.ExtensionMCP
	normalized.Name = firstNonEmpty(strings.TrimSpace(server.Name), strings.TrimSpace(server.ID))
	normalized.Title = firstNonEmpty(normalized.Title, normalized.Name)
	normalized.Source = extensionspkg.ExtensionSource{
		Scope: extensionspkg.ExtensionScopeRemote,
		Ref:   "mcp:" + strings.TrimSpace(server.ID),
	}
	normalized.Manifest.Source = normalized.Source
	normalized.Manifest.Kind = extensionspkg.ExtensionMCP
	normalized.Manifest.Name = firstNonEmpty(normalized.Manifest.Name, normalized.Name)
	normalized.Manifest.Title = firstNonEmpty(normalized.Manifest.Title, normalized.Title)
	normalized.Manifest.Version = firstNonEmpty(normalized.Manifest.Version, normalized.Version)
	normalized.Health.CheckedAtUTC = now.Format(time.RFC3339)
	switch normalized.Status {
	case extensionspkg.ExtensionStatusLoaded:
		normalized.Status = extensionspkg.ExtensionStatusReady
		normalized.Health.Status = extensionspkg.ExtensionStatusReady
		if strings.TrimSpace(normalized.Health.Message) == "" {
			normalized.Health.Message = "mcp server ready"
		}
	case extensionspkg.ExtensionStatusUnknown:
		normalized.Status = extensionspkg.ExtensionStatusReady
		normalized.Health.Status = extensionspkg.ExtensionStatusReady
		normalized.Health.Message = "mcp server ready"
	}
	return normalized
}

func readyMCPInfo(server configpkg.MCPServerConfig, now time.Time) extensionspkg.ExtensionInfo {
	name := firstNonEmpty(strings.TrimSpace(server.Name), strings.TrimSpace(server.ID))
	return extensionspkg.ExtensionInfo{
		ID:          mcpExtensionID(server.ID),
		Name:        name,
		Kind:        extensionspkg.ExtensionMCP,
		Version:     "1.0.0",
		Title:       name,
		Description: fmt.Sprintf("MCP server %s", name),
		Source: extensionspkg.ExtensionSource{
			Scope: extensionspkg.ExtensionScopeRemote,
			Ref:   "mcp:" + strings.TrimSpace(server.ID),
		},
		Status: extensionspkg.ExtensionStatusReady,
		Manifest: extensionspkg.Manifest{
			Name:        name,
			Title:       name,
			Version:     "1.0.0",
			Description: fmt.Sprintf("MCP server %s", name),
			Kind:        extensionspkg.ExtensionMCP,
			Source: extensionspkg.ExtensionSource{
				Scope: extensionspkg.ExtensionScopeRemote,
				Ref:   "mcp:" + strings.TrimSpace(server.ID),
			},
		},
		Health: extensionspkg.HealthSnapshot{
			Status:       extensionspkg.ExtensionStatusReady,
			Message:      "mcp server configured (auto_start disabled)",
			CheckedAtUTC: now.Format(time.RFC3339),
		},
	}
}

func failedMCPInfo(server configpkg.MCPServerConfig, err error, now time.Time) extensionspkg.ExtensionInfo {
	info := readyMCPInfo(server, now)
	info.Status = extensionspkg.ExtensionStatusFailed
	info.Health.Status = extensionspkg.ExtensionStatusFailed
	info.Health.LastError = extensionspkg.ErrCodeLoadFailed
	info.Health.Message = strings.TrimSpace(err.Error())
	return info
}

func mcpExtensionID(serverID string) string {
	return "mcp." + strings.TrimSpace(serverID)
}

func normalizeMCPInput(value string) (extensionID string, serverID string, isMCP bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}

	switch {
	case strings.HasPrefix(value, "mcp:"):
		serverID = strings.TrimSpace(strings.TrimPrefix(value, "mcp:"))
		isMCP = true
	case strings.HasPrefix(value, "mcp."):
		serverID = strings.TrimSpace(strings.TrimPrefix(value, "mcp."))
		isMCP = true
	default:
		serverID = strings.TrimSpace(value)
	}

	serverID = normalizeServerID(serverID)
	if serverID == "" {
		return "", "", false
	}
	extensionID = mcpExtensionID(serverID)
	return extensionID, serverID, isMCP
}

func normalizeServerID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	raw = replacer.Replace(raw)
	raw = strings.Trim(raw, "-_")
	return raw
}

func cloneInfo(info extensionspkg.ExtensionInfo) extensionspkg.ExtensionInfo {
	return extensionspkg.ExtensionInfo{
		ID:           info.ID,
		Name:         info.Name,
		Kind:         info.Kind,
		Version:      info.Version,
		Title:        info.Title,
		Description:  info.Description,
		Source:       info.Source,
		Status:       info.Status,
		Capabilities: info.Capabilities,
		Manifest:     info.Manifest,
		Health:       info.Health,
	}
}

func mergeErrors(left, right error) error {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return fmt.Errorf("%v; %w", left, right)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
