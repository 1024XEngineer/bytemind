package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"bytemind/internal/mcpctl"

	tea "github.com/charmbracelet/bubbletea"
)

type stubMCPService struct {
	listStatuses []mcpctl.ServerStatus
	lastEnableID string
	lastEnabled  bool
	lastAdd      mcpctl.AddRequest
	lastShowID   string
}

func (s *stubMCPService) List(context.Context) ([]mcpctl.ServerStatus, error) {
	out := make([]mcpctl.ServerStatus, len(s.listStatuses))
	copy(out, s.listStatuses)
	return out, nil
}

func (s *stubMCPService) Add(_ context.Context, req mcpctl.AddRequest) (mcpctl.ServerStatus, error) {
	s.lastAdd = req
	return mcpctl.ServerStatus{ID: strings.TrimSpace(req.ID), Enabled: true, Status: "ready"}, nil
}

func (s *stubMCPService) Show(_ context.Context, serverID string) (mcpctl.ServerDetail, error) {
	s.lastShowID = strings.TrimSpace(serverID)
	return mcpctl.ServerDetail{
		Status: mcpctl.ServerStatus{
			ID:        s.lastShowID,
			Name:      "demo",
			Enabled:   true,
			AutoStart: true,
			Status:    "active",
			Tools:     2,
			Message:   "ok",
		},
		TransportType:    "stdio",
		Command:          "npx",
		Args:             []string{"-y", "server"},
		EnvKeys:          []string{"TOKEN"},
		StartupTimeoutS:  30,
		CallTimeoutS:     60,
		MaxConcurrency:   2,
		ProtocolVersions: []string{"2025-03-26"},
	}, nil
}

func (s *stubMCPService) Remove(context.Context, string) error {
	return nil
}

func (s *stubMCPService) Enable(_ context.Context, serverID string, enabled bool) (mcpctl.ServerStatus, error) {
	s.lastEnableID = serverID
	s.lastEnabled = enabled
	return mcpctl.ServerStatus{ID: strings.TrimSpace(serverID), Enabled: enabled, Status: "ready"}, nil
}

func (s *stubMCPService) Test(context.Context, string) (mcpctl.ServerStatus, error) {
	return mcpctl.ServerStatus{ID: "local", Enabled: true, Status: "active", Message: "ok"}, nil
}

func (s *stubMCPService) Reload(context.Context) error {
	return nil
}

func TestRunMCPCommandList(t *testing.T) {
	service := &stubMCPService{
		listStatuses: []mcpctl.ServerStatus{
			{ID: "local", Enabled: true, Status: "active", Tools: 3, Message: "ok"},
		},
	}
	m := model{mcpService: service}
	if err := m.runMCPCommand("/mcp list", []string{"/mcp", "list"}); err != nil {
		t.Fatalf("runMCPCommand list failed: %v", err)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected command exchange in chat, got %#v", m.chatItems)
	}
	got := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(got, "local") || !strings.Contains(got, "active") {
		t.Fatalf("expected status output to include server and status, got %q", got)
	}
}

func TestRunMCPCommandEnable(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	if err := m.runMCPCommand("/mcp enable local", []string{"/mcp", "enable", "local"}); err != nil {
		t.Fatalf("runMCPCommand enable failed: %v", err)
	}
	if service.lastEnableID != "local" || !service.lastEnabled {
		t.Fatalf("expected enable call for local=true, got id=%q enabled=%v", service.lastEnableID, service.lastEnabled)
	}
}

func TestRunMCPCommandShow(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	if err := m.runMCPCommand("/mcp show local", []string{"/mcp", "show", "local"}); err != nil {
		t.Fatalf("runMCPCommand show failed: %v", err)
	}
	if service.lastShowID != "local" {
		t.Fatalf("expected show call for local, got %q", service.lastShowID)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected command exchange in chat, got %#v", m.chatItems)
	}
	got := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(got, "id: local") || !strings.Contains(got, "command: npx") {
		t.Fatalf("expected show output to include server detail, got %q", got)
	}
}

func TestRunMCPCommandAddRequiresCommand(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	err := m.runMCPCommand("/mcp add local", []string{"/mcp", "add", "local"})
	if err == nil {
		t.Fatal("expected missing command error")
	}
	if !strings.Contains(err.Error(), "usage: /mcp-add") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMCPCommandUsageMentionsMCPAddAlias(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	err := m.runMCPCommand("/mcp", []string{"/mcp"})
	if err == nil {
		t.Fatal("expected missing subcommand usage error")
	}
	if !strings.Contains(err.Error(), "/mcp-add") {
		t.Fatalf("expected usage to mention /mcp-add, got %v", err)
	}
}

func TestHandleSlashCommandMCPAddAlias(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	err := m.handleSlashCommand("/mcp-add local --cmd npx --args -y,server --env API_KEY=token --auto-start false")
	if err != nil {
		t.Fatalf("expected /mcp-add alias to succeed, got %v", err)
	}
	if service.lastAdd.ID != "local" {
		t.Fatalf("expected add id local, got %#v", service.lastAdd)
	}
	if service.lastAdd.Command != "npx" {
		t.Fatalf("expected add command npx, got %#v", service.lastAdd)
	}
	if len(service.lastAdd.Args) != 2 || service.lastAdd.Args[0] != "-y" || service.lastAdd.Args[1] != "server" {
		t.Fatalf("unexpected add args: %#v", service.lastAdd.Args)
	}
	if service.lastAdd.Env["API_KEY"] != "token" {
		t.Fatalf("unexpected add env map: %#v", service.lastAdd.Env)
	}
	if service.lastAdd.AutoStart == nil || *service.lastAdd.AutoStart {
		t.Fatalf("expected auto_start=false, got %#v", service.lastAdd.AutoStart)
	}
}

func TestParseMCPAddFieldsSupportsExtendedOptions(t *testing.T) {
	fields := []string{
		"/mcp",
		"add",
		"local",
		"--cmd", "npx",
		"--startup-timeout-s", "45",
		"--call-timeout-s", "90",
		"--max-concurrency", "2",
		"--protocol-version", "2025-03-26",
		"--protocol-versions", "2025-03-26,2024-11-05",
	}
	request, err := parseMCPAddFields(fields)
	if err != nil {
		t.Fatalf("parseMCPAddFields failed: %v", err)
	}
	if request.StartupTimeoutS != 45 {
		t.Fatalf("expected startup timeout 45, got %d", request.StartupTimeoutS)
	}
	if request.CallTimeoutS != 90 {
		t.Fatalf("expected call timeout 90, got %d", request.CallTimeoutS)
	}
	if request.MaxConcurrency != 2 {
		t.Fatalf("expected max concurrency 2, got %d", request.MaxConcurrency)
	}
	if request.ProtocolVersion != "2025-03-26" {
		t.Fatalf("expected protocol version 2025-03-26, got %q", request.ProtocolVersion)
	}
	if len(request.ProtocolVersions) != 2 || request.ProtocolVersions[0] != "2025-03-26" || request.ProtocolVersions[1] != "2024-11-05" {
		t.Fatalf("unexpected protocol versions: %#v", request.ProtocolVersions)
	}
}

func TestHandleSlashCommandMCPAddAliasAsync(t *testing.T) {
	service := &stubMCPService{}
	m := model{
		mcpService: service,
		async:      make(chan tea.Msg, 2),
	}
	err := m.handleSlashCommand("/mcp-add local --cmd npx --startup-timeout-s 33")
	if err != nil {
		t.Fatalf("expected async /mcp-add alias to succeed, got %v", err)
	}
	if !m.mcpCommandPending {
		t.Fatal("expected mcp command pending flag to be set")
	}
	if m.statusNote != "MCP command running..." {
		t.Fatalf("expected pending status note, got %q", m.statusNote)
	}

	var msg tea.Msg
	select {
	case msg = <-m.async:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async mcp result")
	}

	next, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatal("expected update to keep waiting for async events")
	}
	updated := next.(model)
	if updated.mcpCommandPending {
		t.Fatal("expected pending flag to be cleared")
	}
	if service.lastAdd.ID != "local" || service.lastAdd.Command != "npx" {
		t.Fatalf("expected async add to execute with local/npx, got %#v", service.lastAdd)
	}
	if service.lastAdd.StartupTimeoutS != 33 {
		t.Fatalf("expected async add startup timeout 33, got %#v", service.lastAdd)
	}
	if len(updated.chatItems) < 2 {
		t.Fatalf("expected chat exchange after async result, got %#v", updated.chatItems)
	}
}
