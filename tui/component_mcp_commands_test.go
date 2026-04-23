package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"bytemind/internal/mcpctl"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type stubMCPService struct {
	listStatuses []mcpctl.ServerStatus
	lastShowID   string
	addReq       mcpctl.AddRequest
	addCalls     int
	testCalls    int
	testID       string
	enableCalls  int
	enableID     string
	enableValue  bool
	reloadCalls  int
	addStatus    mcpctl.ServerStatus
	testStatus   mcpctl.ServerStatus
	enableStatus mcpctl.ServerStatus
	addErr       error
	testErr      error
	enableErr    error
	reloadErr    error
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

func (s *stubMCPService) Add(_ context.Context, req mcpctl.AddRequest) (mcpctl.ServerStatus, error) {
	s.addCalls++
	s.addReq = cloneAddRequest(req)
	if s.addErr != nil {
		return mcpctl.ServerStatus{}, s.addErr
	}
	if strings.TrimSpace(s.addStatus.ID) == "" {
		s.addStatus.ID = strings.TrimSpace(req.ID)
	}
	if strings.TrimSpace(string(s.addStatus.Status)) == "" {
		s.addStatus.Status = "active"
	}
	return s.addStatus, nil
}

func (s *stubMCPService) Remove(context.Context, string) error {
	return nil
}

func (s *stubMCPService) Enable(_ context.Context, serverID string, enabled bool) (mcpctl.ServerStatus, error) {
	s.enableCalls++
	s.enableID = strings.TrimSpace(serverID)
	s.enableValue = enabled
	if s.enableErr != nil {
		return mcpctl.ServerStatus{}, s.enableErr
	}
	if strings.TrimSpace(s.enableStatus.ID) == "" {
		s.enableStatus.ID = s.enableID
	}
	if strings.TrimSpace(string(s.enableStatus.Status)) == "" {
		s.enableStatus.Status = "active"
	}
	return s.enableStatus, nil
}

func (s *stubMCPService) Test(_ context.Context, serverID string) (mcpctl.ServerStatus, error) {
	s.testCalls++
	s.testID = strings.TrimSpace(serverID)
	if s.testErr != nil {
		return mcpctl.ServerStatus{}, s.testErr
	}
	if strings.TrimSpace(s.testStatus.ID) == "" {
		s.testStatus.ID = s.testID
	}
	if strings.TrimSpace(string(s.testStatus.Status)) == "" {
		s.testStatus.Status = "active"
	}
	return s.testStatus, nil
}

func (s *stubMCPService) Reload(context.Context) error {
	s.reloadCalls++
	return s.reloadErr
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

func TestRunMCPCommandHelp(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	if err := m.runMCPCommand("/mcp help", []string{"/mcp", "help"}); err != nil {
		t.Fatalf("runMCPCommand help failed: %v", err)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected command exchange in chat, got %#v", m.chatItems)
	}
	got := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(got, "usage: /mcp <list|show <id>|setup <id>|help>") {
		t.Fatalf("expected help output to include mcp usage, got %q", got)
	}
}

func TestRunMCPCommandUsageDoesNotMentionMCPAddAlias(t *testing.T) {
	service := &stubMCPService{}
	m := model{mcpService: service}
	err := m.runMCPCommand("/mcp", []string{"/mcp"})
	if err == nil {
		t.Fatal("expected missing subcommand usage error")
	}
	if strings.Contains(err.Error(), "/mcp-add") {
		t.Fatalf("did not expect usage to mention /mcp-add, got %v", err)
	}
	if !strings.Contains(err.Error(), "/mcp <list|show <id>|setup <id>|help>") {
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

func TestHandleSlashCommandMCPListAsync(t *testing.T) {
	service := &stubMCPService{
		listStatuses: []mcpctl.ServerStatus{
			{ID: "local", Enabled: true, Status: "active", Tools: 1, Message: "ok"},
		},
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

func TestHandleSlashCommandMCPSetupGithubAppliesDirectly(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	if err := m.handleSlashCommand("/mcp setup github"); err != nil {
		t.Fatalf("expected /mcp setup github to run directly, got %v", err)
	}
	if m.mcpSetup != nil {
		t.Fatalf("expected slash setup not to open wizard, got %#v", m.mcpSetup)
	}
	if service.addCalls != 1 || service.testCalls != 1 || service.enableCalls != 1 || service.reloadCalls != 1 {
		t.Fatalf("expected add/test/enable/reload to run once, got add=%d test=%d enable=%d reload=%d", service.addCalls, service.testCalls, service.enableCalls, service.reloadCalls)
	}
	if strings.TrimSpace(service.addReq.ID) != "github" {
		t.Fatalf("expected github id, got %#v", service.addReq.ID)
	}
	if strings.TrimSpace(service.addReq.Command) != "npx" {
		t.Fatalf("expected github preset command npx, got %#v", service.addReq.Command)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected setup output in chat, got %#v", m.chatItems)
	}
	last := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(last, "MCP setup completed for `github`.") {
		t.Fatalf("expected setup completion in output, got %q", last)
	}
}

func TestHandleSlashCommandMCPSetupAnyIDRequiresCommand(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	err := m.handleSlashCommand("/mcp setup docs")
	if err == nil {
		t.Fatal("expected setup without --cmd for non-preset id to fail")
	}
	if !strings.Contains(err.Error(), "usage: /mcp setup <id>") {
		t.Fatalf("expected setup usage error, got %v", err)
	}
}

func TestHandleSlashCommandMCPSetupWithCommandArgsEnv(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	err := m.handleSlashCommand("/mcp setup docs --cmd npx --args -y,@modelcontextprotocol/server-filesystem --env API_KEY=abc")
	if err != nil {
		t.Fatalf("expected one-line slash setup to succeed, got %v", err)
	}
	if m.mcpSetup != nil {
		t.Fatalf("expected slash setup not to open wizard, got %#v", m.mcpSetup)
	}
	if service.addCalls != 1 || service.testCalls != 1 || service.enableCalls != 1 || service.reloadCalls != 1 {
		t.Fatalf("expected add/test/enable/reload to run once, got add=%d test=%d enable=%d reload=%d", service.addCalls, service.testCalls, service.enableCalls, service.reloadCalls)
	}
	if strings.TrimSpace(service.addReq.ID) != "docs" {
		t.Fatalf("expected docs id, got %#v", service.addReq.ID)
	}
	if strings.TrimSpace(service.addReq.Command) != "npx" {
		t.Fatalf("expected command npx, got %#v", service.addReq.Command)
	}
	if len(service.addReq.Args) != 2 || service.addReq.Args[1] != "@modelcontextprotocol/server-filesystem" {
		t.Fatalf("expected parsed args, got %#v", service.addReq.Args)
	}
	if service.addReq.Env["API_KEY"] != "abc" {
		t.Fatalf("expected parsed env, got %#v", service.addReq.Env)
	}
}

func TestNaturalLanguageMCPSetupStartsWizard(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	m.setInputValue("please configure github mcp")

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(model)

	if updated.mcpSetup == nil {
		t.Fatal("expected natural language to start mcp setup")
	}
	if updated.mcpSetup.step != mcpSetupStepGithubToken {
		t.Fatalf("expected github setup token step, got %q", updated.mcpSetup.step)
	}
	if strings.TrimSpace(updated.input.Value()) != "" {
		t.Fatalf("expected input to reset after setup trigger, got %q", updated.input.Value())
	}
	last := updated.chatItems[len(updated.chatItems)-1].Body
	if !strings.Contains(last, "Preset auto-detected: github") {
		t.Fatalf("expected setup intro in chat, got %q", last)
	}
}

func TestNaturalLanguageMCPSetupMissingIDShowsHint(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	m.setInputValue("please configure mcp")

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(model)

	if updated.mcpSetup != nil {
		t.Fatalf("expected setup not to start without id, got %#v", updated.mcpSetup)
	}
	if service.addCalls != 0 || service.testCalls != 0 || service.enableCalls != 0 || service.reloadCalls != 0 {
		t.Fatalf("expected no runtime calls, got add=%d test=%d enable=%d reload=%d", service.addCalls, service.testCalls, service.enableCalls, service.reloadCalls)
	}
	if len(updated.chatItems) < 2 {
		t.Fatalf("expected guidance exchange in chat, got %#v", updated.chatItems)
	}
	last := updated.chatItems[len(updated.chatItems)-1].Body
	if !strings.Contains(last, "server id is missing") {
		t.Fatalf("expected missing id guidance, got %q", last)
	}
	if !strings.Contains(updated.statusNote, "requires server id") {
		t.Fatalf("expected status note about missing id, got %q", updated.statusNote)
	}
}

func TestMCPSetupAllowsTypingAfterWizardStarts(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	m.setInputValue("please configure github mcp")
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	updated := next.(model)
	if strings.TrimSpace(updated.input.Value()) != "g" {
		t.Fatalf("expected input to accept typing during setup, got %q", updated.input.Value())
	}
}

func TestNaturalLanguageMCPSetupGithubFlowAppliesConfigSynchronously(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	m.setInputValue("configure github mcp")
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.mcpSetup == nil || m.mcpSetup.step != mcpSetupStepGithubToken {
		t.Fatalf("expected setup to move to github token step, got %#v", m.mcpSetup)
	}

	m = submitMCPSetupEnter(t, m, "ghp_test_token")
	if m.mcpSetup == nil || m.mcpSetup.step != mcpSetupStepConfirm {
		t.Fatalf("expected setup to move to confirm step, got %#v", m.mcpSetup)
	}

	m = submitMCPSetupEnter(t, m, "yes")
	if m.mcpSetup != nil {
		t.Fatalf("expected setup to complete and clear state, got %#v", m.mcpSetup)
	}
	if service.addCalls != 1 || service.testCalls != 1 || service.enableCalls != 1 || service.reloadCalls != 1 {
		t.Fatalf("expected add/test/enable/reload to run exactly once, got add=%d test=%d enable=%d reload=%d", service.addCalls, service.testCalls, service.enableCalls, service.reloadCalls)
	}
	if strings.TrimSpace(service.addReq.ID) != "github" {
		t.Fatalf("expected default github id, got %#v", service.addReq.ID)
	}
	if strings.TrimSpace(service.addReq.Command) != "npx" {
		t.Fatalf("expected github preset command npx, got %#v", service.addReq.Command)
	}
	if len(service.addReq.Args) != 2 || service.addReq.Args[1] != "@modelcontextprotocol/server-github" {
		t.Fatalf("expected github preset args, got %#v", service.addReq.Args)
	}
	if service.addReq.Env[mcpSetupGithubTokenEnv] != "ghp_test_token" {
		t.Fatalf("expected github token env to be set, got %#v", service.addReq.Env)
	}
	if !service.enableValue || service.enableID != "github" {
		t.Fatalf("expected enable call for github=true, got id=%q enabled=%v", service.enableID, service.enableValue)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected final setup output in chat, got %#v", m.chatItems)
	}
	last := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(last, "MCP setup completed for `github`.") {
		t.Fatalf("expected completion output, got %q", last)
	}
}

func TestNaturalLanguageMCPSetupAnyIDUsesGenericWizard(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	m.setInputValue("configure docs mcp")
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.mcpSetup == nil || m.mcpSetup.step != mcpSetupStepCommand {
		t.Fatalf("expected generic setup command step, got %#v", m.mcpSetup)
	}
	last := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(last, "for `docs`") || !strings.Contains(last, "Step 1/4") {
		t.Fatalf("expected docs setup intro and step, got %q", last)
	}

	m = submitMCPSetupEnter(t, m, "npx")
	m = submitMCPSetupEnter(t, m, "-y,@modelcontextprotocol/server-filesystem")
	m = submitMCPSetupEnter(t, m, "API_KEY=abc")
	if m.mcpSetup == nil || m.mcpSetup.step != mcpSetupStepConfirm {
		t.Fatalf("expected generic setup to reach confirm step, got %#v", m.mcpSetup)
	}

	m = submitMCPSetupEnter(t, m, "yes")
	if service.addCalls != 1 {
		t.Fatalf("expected one add call, got %d", service.addCalls)
	}
	if strings.TrimSpace(service.addReq.ID) != "docs" {
		t.Fatalf("expected setup id docs, got %q", service.addReq.ID)
	}
	if strings.TrimSpace(service.addReq.Command) != "npx" {
		t.Fatalf("expected command npx, got %q", service.addReq.Command)
	}
	if len(service.addReq.Args) != 2 || service.addReq.Args[1] != "@modelcontextprotocol/server-filesystem" {
		t.Fatalf("expected parsed args, got %#v", service.addReq.Args)
	}
	if service.addReq.Env["API_KEY"] != "abc" {
		t.Fatalf("expected parsed env, got %#v", service.addReq.Env)
	}
}

func TestMCPSetupRequiresID(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	err := m.handleSlashCommand("/mcp setup")
	if err == nil {
		t.Fatal("expected setup without id to fail")
	}
	if !strings.Contains(err.Error(), "usage: /mcp setup <id>") {
		t.Fatalf("expected setup usage error, got %v", err)
	}
}

func TestMCPSetupAsyncShowsApplyingStatus(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	m.async = make(chan tea.Msg, 2)

	if err := m.handleSlashCommand("/mcp setup docs --cmd npx --args -y,@modelcontextprotocol/server-filesystem"); err != nil {
		t.Fatalf("one-line setup failed: %v", err)
	}

	if !m.mcpCommandPending {
		t.Fatal("expected setup apply to run asynchronously")
	}
	if !strings.Contains(m.statusNote, "Applying MCP setup") {
		t.Fatalf("expected applying status note, got %q", m.statusNote)
	}
	if len(m.chatItems) == 0 || m.chatItems[len(m.chatItems)-1].Status != "pending" {
		t.Fatalf("expected pending applying chat entry, got %#v", m.chatItems)
	}

	var msg tea.Msg
	select {
	case msg = <-m.async:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async mcp setup result")
	}

	next, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatal("expected update to continue async wait command")
	}
	updated := next.(model)
	if updated.mcpCommandPending {
		t.Fatal("expected pending flag to clear after async result")
	}
	last := updated.chatItems[len(updated.chatItems)-1]
	if last.Status == "pending" {
		t.Fatalf("expected pending applying entry to be finalized, got %#v", last)
	}
}

func TestMCPSetupCanBeCanceled(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	m.setInputValue("configure github mcp")
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	m = submitMCPSetupEnter(t, m, "cancel")
	if m.mcpSetup != nil {
		t.Fatalf("expected setup to be canceled, got %#v", m.mcpSetup)
	}
	if service.addCalls != 0 || service.testCalls != 0 || service.enableCalls != 0 || service.reloadCalls != 0 {
		t.Fatalf("expected no mcp mutations on cancel, got add=%d test=%d enable=%d reload=%d", service.addCalls, service.testCalls, service.enableCalls, service.reloadCalls)
	}
	if !strings.Contains(m.statusNote, "canceled") {
		t.Fatalf("expected canceled status note, got %q", m.statusNote)
	}
}

func TestMCPSetupBlocksOtherSlashCommandsUntilCanceled(t *testing.T) {
	service := &stubMCPService{}
	m := newMCPSetupTestModel(service)
	m.setInputValue("configure github mcp")
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	m = submitMCPSetupEnter(t, m, "/mcp list")
	if m.mcpSetup == nil {
		t.Fatal("expected setup to remain active after invalid slash input")
	}
	if !strings.Contains(strings.ToLower(m.statusNote), "mcp setup in progress") {
		t.Fatalf("expected in-progress warning, got %q", m.statusNote)
	}
}

func newMCPSetupTestModel(service *stubMCPService) model {
	input := textarea.New()
	input.Focus()
	return model{
		mcpService: service,
		input:      input,
		screen:     screenChat,
	}
}

func submitMCPSetupEnter(t *testing.T, m model, value string) model {
	t.Helper()
	m.setInputValue(value)
	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	return got.(model)
}

func cloneAddRequest(input mcpctl.AddRequest) mcpctl.AddRequest {
	output := input
	output.Args = append([]string(nil), input.Args...)
	output.ProtocolVersions = append([]string(nil), input.ProtocolVersions...)
	if input.Env != nil {
		env := make(map[string]string, len(input.Env))
		for key, value := range input.Env {
			env[key] = value
		}
		output.Env = env
	}
	if input.AutoStart != nil {
		value := *input.AutoStart
		output.AutoStart = &value
	}
	return output
}
