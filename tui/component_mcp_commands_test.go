package tui

import (
	"context"
	"strings"
	"testing"

	"bytemind/internal/mcpctl"
)

type stubMCPService struct {
	listStatuses []mcpctl.ServerStatus
	lastEnableID string
	lastEnabled  bool
	lastAdd      mcpctl.AddRequest
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
