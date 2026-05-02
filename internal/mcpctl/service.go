package mcpctl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
	extensionsruntime "github.com/1024XEngineer/bytemind/internal/extensionsruntime"
)

type ServerStatus struct {
	ID          string
	Name        string
	Enabled     bool
	AutoStart   bool
	Status      extensionspkg.ExtensionStatus
	Tools       int
	Message     string
	LastError   extensionspkg.ErrorCode
	CheckedAt   string
	ExtensionID string
}

type ServerDetail struct {
	Status           ServerStatus
	TransportType    string
	Command          string
	Args             []string
	CWD              string
	EnvKeys          []string
	StartupTimeoutS  int
	CallTimeoutS     int
	MaxConcurrency   int
	ProtocolVersions []string
}

type AddRequest struct {
	ID               string
	Name             string
	Command          string
	Args             []string
	Env              map[string]string
	CWD              string
	AutoStart        *bool
	StartupTimeoutS  int
	CallTimeoutS     int
	MaxConcurrency   int
	ProtocolVersion  string
	ProtocolVersions []string
}

type Service struct {
	workspace     string
	configPath    string
	mcpConfigPath string
	manager       extensionspkg.Manager
}

func NewService(workspace, configPath string, manager extensionspkg.Manager) *Service {
	return NewServiceWithMCPConfigPath(workspace, configPath, "", manager)
}

func NewServiceWithMCPConfigPath(workspace, configPath, mcpConfigPath string, manager extensionspkg.Manager) *Service {
	return &Service{
		workspace:     strings.TrimSpace(workspace),
		configPath:    strings.TrimSpace(configPath),
		mcpConfigPath: strings.TrimSpace(mcpConfigPath),
		manager:       manager,
	}
}

func (s *Service) List(ctx context.Context) ([]ServerStatus, error) {
	cfg, err := configpkg.LoadWithMCPConfigPath(s.workspace, s.configPath, s.mcpConfigPath)
	if err != nil {
		return nil, err
	}
	manager := s.managerForConfig(cfg)
	items, listErr := manager.List(ctx)

	mcpByID := map[string]extensionspkg.ExtensionInfo{}
	for _, item := range items {
		if item.Kind != extensionspkg.ExtensionMCP {
			continue
		}
		mcpByID[item.ID] = item
	}

	statuses := make([]ServerStatus, 0, len(cfg.MCP.Servers))
	for _, server := range cfg.MCP.Servers {
		serverID := strings.TrimSpace(server.ID)
		extensionID := "mcp." + serverID
		status := ServerStatus{
			ID:          serverID,
			Name:        firstNonEmpty(strings.TrimSpace(server.Name), serverID),
			Enabled:     cfg.MCP.Enabled && server.EnabledValue(),
			AutoStart:   server.AutoStartValue(),
			Status:      extensionspkg.ExtensionStatusStopped,
			Message:     "mcp disabled",
			ExtensionID: extensionID,
		}

		if !cfg.MCP.Enabled {
			status.Status = extensionspkg.ExtensionStatusStopped
			status.Message = "mcp disabled"
		} else if !server.EnabledValue() {
			status.Status = extensionspkg.ExtensionStatusStopped
			status.Message = "server disabled"
		} else if item, ok := mcpByID[extensionID]; ok {
			status.Status = item.Status
			status.Tools = item.Capabilities.Tools
			status.Message = strings.TrimSpace(item.Health.Message)
			status.LastError = item.Health.LastError
			status.CheckedAt = strings.TrimSpace(item.Health.CheckedAtUTC)
		} else if !server.AutoStartValue() {
			status.Status = extensionspkg.ExtensionStatusReady
			status.Message = "configured (auto_start disabled)"
		} else {
			status.Status = extensionspkg.ExtensionStatusFailed
			status.Message = "server not loaded"
		}
		statuses = append(statuses, status)
	}

	sort.Slice(statuses, func(i, j int) bool { return statuses[i].ID < statuses[j].ID })
	return statuses, listErr
}

func (s *Service) Add(ctx context.Context, req AddRequest) (ServerStatus, error) {
	req = normalizeAddRequest(req)
	if req.ID == "" {
		return ServerStatus{}, fmt.Errorf("server id is required")
	}
	if req.Command == "" {
		return ServerStatus{}, fmt.Errorf("server command is required")
	}

	cfg, _, err := configpkg.MutateMCPConfig(s.workspace, s.mcpConfigPath, func(mcp *configpkg.MCPConfig) error {
		mcp.Enabled = true
		for _, existing := range mcp.Servers {
			if strings.EqualFold(strings.TrimSpace(existing.ID), req.ID) {
				return fmt.Errorf("mcp server %q already exists", req.ID)
			}
		}
		autoStart := req.AutoStart
		if autoStart == nil {
			value := true
			autoStart = &value
		}
		enabled := true
		mcp.Servers = append(mcp.Servers, configpkg.MCPServerConfig{
			ID:                    req.ID,
			Name:                  firstNonEmpty(req.Name, req.ID),
			Enabled:               &enabled,
			AutoStart:             autoStart,
			StartupTimeoutSeconds: req.StartupTimeoutS,
			CallTimeoutSeconds:    req.CallTimeoutS,
			MaxConcurrency:        req.MaxConcurrency,
			ProtocolVersion:       req.ProtocolVersion,
			ProtocolVersions:      append([]string(nil), req.ProtocolVersions...),
			Transport: configpkg.MCPTransportConfig{
				Type:    "stdio",
				Command: req.Command,
				Args:    append([]string(nil), req.Args...),
				Env:     cloneStringMap(req.Env),
				CWD:     req.CWD,
			},
		})
		return nil
	})
	if err != nil {
		return ServerStatus{}, err
	}
	_ = cfg
	reloadErr := s.reloadRuntime(ctx)
	status, statusErr := s.getStatus(ctx, req.ID)
	if reloadErr != nil {
		wrapped := fmt.Errorf("runtime reload failed after config persisted: %w", reloadErr)
		if statusErr == nil {
			return status, wrapped
		}
		return ServerStatus{}, errors.Join(wrapped, statusErr)
	}
	if statusErr != nil {
		return ServerStatus{}, statusErr
	}
	return status, nil
}

func (s *Service) Remove(ctx context.Context, serverID string) error {
	serverID = normalizeServerID(serverID)
	if serverID == "" {
		return fmt.Errorf("server id is required")
	}
	_, _, err := configpkg.MutateMCPConfig(s.workspace, s.mcpConfigPath, func(mcp *configpkg.MCPConfig) error {
		next := make([]configpkg.MCPServerConfig, 0, len(mcp.Servers))
		found := false
		for _, server := range mcp.Servers {
			if strings.EqualFold(strings.TrimSpace(server.ID), serverID) {
				found = true
				continue
			}
			next = append(next, server)
		}
		if !found {
			return fmt.Errorf("mcp server %q not found", serverID)
		}
		mcp.Servers = next
		return nil
	})
	if err != nil {
		return err
	}
	return s.reloadRuntime(ctx)
}

func (s *Service) Enable(ctx context.Context, serverID string, enabled bool) (ServerStatus, error) {
	serverID = normalizeServerID(serverID)
	if serverID == "" {
		return ServerStatus{}, fmt.Errorf("server id is required")
	}
	_, _, err := configpkg.MutateMCPConfig(s.workspace, s.mcpConfigPath, func(mcp *configpkg.MCPConfig) error {
		if enabled {
			mcp.Enabled = true
		}
		for index := range mcp.Servers {
			if !strings.EqualFold(strings.TrimSpace(mcp.Servers[index].ID), serverID) {
				continue
			}
			value := enabled
			mcp.Servers[index].Enabled = &value
			return nil
		}
		return fmt.Errorf("mcp server %q not found", serverID)
	})
	if err != nil {
		return ServerStatus{}, err
	}
	reloadErr := s.reloadRuntime(ctx)
	status, statusErr := s.getStatus(ctx, serverID)
	if reloadErr != nil {
		wrapped := fmt.Errorf("runtime reload failed after config persisted: %w", reloadErr)
		if statusErr == nil {
			return status, wrapped
		}
		return ServerStatus{}, errors.Join(wrapped, statusErr)
	}
	if statusErr != nil {
		return ServerStatus{}, statusErr
	}
	return status, nil
}

func (s *Service) Reload(ctx context.Context) error {
	return s.reloadRuntime(ctx)
}

func (s *Service) Show(ctx context.Context, serverID string) (ServerDetail, error) {
	serverID = normalizeServerID(serverID)
	if serverID == "" {
		return ServerDetail{}, fmt.Errorf("server id is required")
	}
	cfg, err := configpkg.LoadWithMCPConfigPath(s.workspace, s.configPath, s.mcpConfigPath)
	if err != nil {
		return ServerDetail{}, err
	}
	var server *configpkg.MCPServerConfig
	for index := range cfg.MCP.Servers {
		if strings.EqualFold(strings.TrimSpace(cfg.MCP.Servers[index].ID), serverID) {
			server = &cfg.MCP.Servers[index]
			break
		}
	}
	if server == nil {
		return ServerDetail{}, fmt.Errorf("mcp server %q not found", serverID)
	}

	status, err := s.getStatus(ctx, serverID)
	if err != nil {
		return ServerDetail{}, err
	}
	envKeys := make([]string, 0, len(server.Transport.Env))
	for key := range server.Transport.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)

	protocolVersions := append([]string(nil), server.ProtocolVersions...)
	if len(protocolVersions) == 0 && strings.TrimSpace(server.ProtocolVersion) != "" {
		protocolVersions = append(protocolVersions, strings.TrimSpace(server.ProtocolVersion))
	}

	return ServerDetail{
		Status:           status,
		TransportType:    strings.TrimSpace(server.Transport.Type),
		Command:          strings.TrimSpace(server.Transport.Command),
		Args:             append([]string(nil), server.Transport.Args...),
		CWD:              strings.TrimSpace(server.Transport.CWD),
		EnvKeys:          envKeys,
		StartupTimeoutS:  server.StartupTimeoutSeconds,
		CallTimeoutS:     server.CallTimeoutSeconds,
		MaxConcurrency:   server.MaxConcurrency,
		ProtocolVersions: protocolVersions,
	}, nil
}

func (s *Service) Test(ctx context.Context, serverID string) (ServerStatus, error) {
	serverID = normalizeServerID(serverID)
	if serverID == "" {
		return ServerStatus{}, fmt.Errorf("server id is required")
	}
	cfg, err := configpkg.LoadWithMCPConfigPath(s.workspace, s.configPath, s.mcpConfigPath)
	if err != nil {
		return ServerStatus{}, err
	}
	manager := s.managerForConfig(cfg)

	tester, ok := manager.(extensionspkg.HealthTester)
	if !ok {
		return ServerStatus{}, fmt.Errorf("extensions manager does not support test")
	}
	health, testErr := tester.Test(ctx, "mcp."+serverID)
	status, getErr := s.getStatus(ctx, serverID)
	if getErr != nil {
		return ServerStatus{}, getErr
	}
	status.Status = health.Status
	status.Message = strings.TrimSpace(health.Message)
	status.LastError = health.LastError
	status.CheckedAt = health.CheckedAtUTC
	return status, testErr
}

func (s *Service) getStatus(ctx context.Context, serverID string) (ServerStatus, error) {
	serverID = normalizeServerID(serverID)
	if serverID == "" {
		return ServerStatus{}, fmt.Errorf("server id is required")
	}
	items, err := s.List(ctx)
	for _, item := range items {
		if strings.EqualFold(item.ID, serverID) {
			return item, nil
		}
	}
	if err != nil {
		return ServerStatus{}, err
	}
	return ServerStatus{}, fmt.Errorf("mcp server %q not found", serverID)
}

func (s *Service) managerForConfig(cfg configpkg.Config) extensionspkg.Manager {
	if s.manager != nil {
		return s.manager
	}
	return extensionsruntime.NewManagerWithMCPConfigPath(s.workspace, s.configPath, s.mcpConfigPath, extensionspkg.NewManager(s.workspace), cfg)
}

func (s *Service) reloadRuntime(ctx context.Context) error {
	if s.manager == nil {
		cfg, err := configpkg.LoadWithMCPConfigPath(s.workspace, s.configPath, s.mcpConfigPath)
		if err != nil {
			return err
		}
		manager := s.managerForConfig(cfg)
		if reloader, ok := manager.(extensionspkg.Reloader); ok {
			return reloader.Reload(ctx)
		}
		_, err = manager.List(ctx)
		return err
	}
	if invalidator, ok := s.manager.(extensionspkg.Invalidator); ok {
		invalidator.Invalidate("")
	}
	if reloader, ok := s.manager.(extensionspkg.Reloader); ok {
		return reloader.Reload(ctx)
	}
	_, err := s.manager.List(ctx)
	return err
}

func normalizeAddRequest(req AddRequest) AddRequest {
	req.ID = normalizeServerID(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.Command = strings.TrimSpace(req.Command)
	req.CWD = strings.TrimSpace(req.CWD)
	if req.StartupTimeoutS <= 0 {
		req.StartupTimeoutS = configpkg.DefaultMCPStartupTimeoutSeconds
	}
	if req.CallTimeoutS <= 0 {
		req.CallTimeoutS = configpkg.DefaultMCPCallTimeoutSeconds
	}
	if req.MaxConcurrency <= 0 {
		req.MaxConcurrency = configpkg.DefaultMCPMaxConcurrency
	}
	return req
}

func normalizeServerID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	raw = strings.TrimPrefix(raw, "mcp.")
	raw = strings.TrimPrefix(raw, "mcp:")
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	raw = replacer.Replace(raw)
	raw = strings.Trim(raw, "-_")
	return raw
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func boolPtr(value bool) *bool {
	return &value
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
