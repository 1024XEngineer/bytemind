package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/mcpctl"

	tea "github.com/charmbracelet/bubbletea"
)

type stubMCPService struct {
	listStatuses []mcpctl.ServerStatus
	lastShowID   string
}

func (s *stubMCPService) List(context.Context) ([]mcpctl.ServerStatus, error) {
	out := make([]mcpctl.ServerStatus, len(s.listStatuses))
	copy(out, s.listStatuses)
	return out, nil
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

func (s *stubMCPService) Add(context.Context, mcpctl.AddRequest) (mcpctl.ServerStatus, error) {
	return mcpctl.ServerStatus{}, nil
}

func (s *stubMCPService) Remove(context.Context, string) error {
	return nil
}

func (s *stubMCPService) Enable(context.Context, string, bool) (mcpctl.ServerStatus, error) {
	return mcpctl.ServerStatus{}, nil
}

func (s *stubMCPService) Test(context.Context, string) (mcpctl.ServerStatus, error) {
	return mcpctl.ServerStatus{}, nil
}

func (s *stubMCPService) Reload(context.Context) error {
	return nil
}

func TestRunMCPCommandList(t *testing.T) {
	service := &stubMCPService{
		listStatuses: []mcpctl.ServerStatus{{ID: "local", Enabled: true, Status: "active", Tools: 3, Message: "ok"}},
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

func TestRunMCPCommandHelpMentionsProjectConfigFile(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	if err := m.runMCPCommand("/mcp help", []string{"/mcp", "help"}); err != nil {
		t.Fatalf("runMCPCommand help failed: %v", err)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected command exchange in chat, got %#v", m.chatItems)
	}
	got := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(got, "usage: /mcp <list|show <id>|help>") {
		t.Fatalf("expected help output to include mcp usage, got %q", got)
	}
	if !strings.Contains(got, ".bytemind/mcp.json") {
		t.Fatalf("expected help output to mention project MCP config file, got %q", got)
	}
}

func TestRunMCPCommandUsageDoesNotMentionRemovedCommands(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	err := m.runMCPCommand("/mcp", []string{"/mcp"})
	if err == nil {
		t.Fatal("expected missing subcommand usage error")
	}
	if strings.Contains(err.Error(), "/mcp-add") {
		t.Fatalf("did not expect usage to mention /mcp-add, got %v", err)
	}
	if strings.Contains(err.Error(), "setup") {
		t.Fatalf("did not expect usage to mention setup, got %v", err)
	}
	if !strings.Contains(err.Error(), "/mcp <list|show <id>|help>") {
		t.Fatalf("expected narrowed mcp usage, got %v", err)
	}
}

func TestHandleSlashCommandMCPAddAliasRejected(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	err := m.handleSlashCommand("/mcp-add local --cmd npx")
	if err == nil {
		t.Fatal("expected /mcp-add to be rejected")
	}
	if !strings.Contains(err.Error(), "unknown command: /mcp-add") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleSlashCommandMCPSetupRejected(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	err := m.handleSlashCommand("/mcp setup github")
	if err == nil {
		t.Fatal("expected /mcp setup to be rejected")
	}
	if strings.Contains(err.Error(), "setup") {
		t.Fatalf("did not expect setup to be listed in usage, got %v", err)
	}
	if !strings.Contains(err.Error(), "/mcp <list|show <id>|help>") {
		t.Fatalf("expected mcp usage error, got %v", err)
	}
}

func TestHandleSlashCommandMCPListAsync(t *testing.T) {
	service := &stubMCPService{
		listStatuses: []mcpctl.ServerStatus{{ID: "local", Enabled: true, Status: "active", Tools: 1, Message: "ok"}},
	}
	m := model{
		mcpService: service,
		async:      make(chan tea.Msg, 2),
	}
	err := m.handleSlashCommand("/mcp list")
	if err != nil {
		t.Fatalf("expected async /mcp list to succeed, got %v", err)
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
	if len(updated.chatItems) < 2 {
		t.Fatalf("expected chat exchange after async result, got %#v", updated.chatItems)
	}
}
