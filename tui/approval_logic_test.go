package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestApprovalDecisionHelpers(t *testing.T) {
	for _, decision := range []ApprovalDecision{
		{Disposition: ApprovalApproveOnce},
		{Disposition: ApprovalApproveSameToolSession},
		{Disposition: ApprovalApproveAllSession},
	} {
		if !decision.Approved() {
			t.Fatalf("expected %+v to count as approved", decision)
		}
	}

	denied := ApprovalDecision{Disposition: ApprovalDeny}
	if denied.Approved() {
		t.Fatalf("expected %+v not to count as approved", denied)
	}

	normalized := NormalizeApprovalDecision(ApprovalDecision{Disposition: ApprovalDisposition("mystery")})
	if normalized.Disposition != ApprovalDeny {
		t.Fatalf("expected unknown approval disposition to normalize to deny, got %+v", normalized)
	}
}

func TestApprovalToolKeyAndOptionCatalog(t *testing.T) {
	if got := approvalToolKey("  Run_Shell  "); got != "run_shell" {
		t.Fatalf("expected normalized tool key, got %q", got)
	}

	options := model{}.approvalOptions()
	if len(options) != 3 {
		t.Fatalf("expected 3 approval options, got %d", len(options))
	}
	if options[0].Decision != ApprovalApproveOnce || options[1].Decision != ApprovalApproveSameToolSession || options[2].Decision != ApprovalApproveAllSession {
		t.Fatalf("unexpected approval options ordering: %+v", options)
	}
}

func TestApprovalStateHelpers(t *testing.T) {
	m := &model{
		sessionApprovalAll:   true,
		sessionApprovedTools: map[string]struct{}{"run_shell": {}},
	}

	if decision, ok := m.cachedApprovalDecision(ApprovalRequest{ToolName: "anything"}); !ok || decision.Disposition != ApprovalApproveAllSession {
		t.Fatalf("expected global session approval cache, got ok=%v decision=%+v", ok, decision)
	}

	m.resetSessionApprovalState()
	if m.sessionApprovalAll {
		t.Fatal("expected reset to clear global session approval")
	}
	if len(m.sessionApprovedTools) != 0 {
		t.Fatalf("expected reset to clear approved tools, got %#v", m.sessionApprovedTools)
	}

	m.rememberApprovalDecision(ApprovalRequest{ToolName: "Run_Shell"}, ApprovalDecision{Disposition: ApprovalApproveSameToolSession})
	if _, ok := m.sessionApprovedTools["run_shell"]; !ok {
		t.Fatalf("expected same-tool approval to be remembered, got %#v", m.sessionApprovedTools)
	}

	if decision, ok := m.cachedApprovalDecision(ApprovalRequest{ToolName: "run_shell"}); !ok || decision.Disposition != ApprovalApproveSameToolSession {
		t.Fatalf("expected same-tool cached approval, got ok=%v decision=%+v", ok, decision)
	}

	if decision, ok := m.cachedApprovalDecision(ApprovalRequest{}); ok || decision.Disposition != "" {
		t.Fatalf("expected empty tool name not to hit cache, got ok=%v decision=%+v", ok, decision)
	}
}

func TestApprovalKeyNavigationAndApproveAllSession(t *testing.T) {
	reply := make(chan approvalDecision, 1)
	m := model{
		approval: &approvalPrompt{
			ToolName: "run_shell",
			Command:  "go test ./tui",
			Reason:   "run focused tests",
			Reply:    reply,
		},
		sessionApprovedTools: make(map[string]struct{}),
		phase:                "approval",
		async:                make(chan tea.Msg, 1),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	updated := got.(model)
	if updated.approval.Cursor != 1 {
		t.Fatalf("expected down to move approval cursor to 1, got %d", updated.approval.Cursor)
	}

	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	updated = got.(model)
	if updated.approval.Cursor != 2 {
		t.Fatalf("expected second down to move approval cursor to 2, got %d", updated.approval.Cursor)
	}

	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	updated = got.(model)
	if updated.approval.Cursor != 1 {
		t.Fatalf("expected up to move approval cursor back to 1, got %d", updated.approval.Cursor)
	}

	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated = got.(model)
	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated = got.(model)
	if updated.approval.Cursor != 2 {
		t.Fatalf("expected j navigation to reach final option, got %d", updated.approval.Cursor)
	}

	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated = got.(model)
	if updated.approval != nil {
		t.Fatal("expected approval prompt to close after confirm")
	}
	if !updated.sessionApprovalAll {
		t.Fatal("expected approve-all-session to persist on the model")
	}
	if updated.statusNote != "Session approvals disabled for this TUI session." {
		t.Fatalf("unexpected status note after approve-all-session: %q", updated.statusNote)
	}

	select {
	case decision := <-reply:
		if decision.Decision.Disposition != ApprovalApproveAllSession {
			t.Fatalf("expected approve-all-session decision, got %+v", decision)
		}
	default:
		t.Fatal("expected approval decision to be sent")
	}

	reply2 := make(chan approvalDecision, 1)
	got, cmd := updated.Update(approvalRequestMsg{
		Request: ApprovalRequest{ToolName: "different_tool", Command: "echo hi", Reason: "cached globally"},
		Reply:   reply2,
	})
	updated = got.(model)
	if cmd == nil {
		t.Fatal("expected async wait command after cached global approval")
	}
	if updated.approval != nil {
		t.Fatalf("expected cached global approval to skip prompt, got %+v", updated.approval)
	}

	select {
	case decision := <-reply2:
		if decision.Decision.Disposition != ApprovalApproveAllSession {
			t.Fatalf("expected cached global approval decision, got %+v", decision)
		}
	default:
		t.Fatal("expected cached global approval reply")
	}
}
