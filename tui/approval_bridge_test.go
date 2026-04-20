package tui

import (
	"context"
	"io"
	"testing"
	"time"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	"bytemind/internal/skills"

	tea "github.com/charmbracelet/bubbletea"
)

type approvalBridgeRunnerStub struct {
	handler ApprovalHandler
}

func (r *approvalBridgeRunnerStub) RunPromptWithInput(_ context.Context, _ *session.Session, _ RunPromptInput, _ string, _ io.Writer) (string, error) {
	return "", nil
}

func (r *approvalBridgeRunnerStub) SetObserver(_ Observer) {}

func (r *approvalBridgeRunnerStub) SetApprovalHandler(handler ApprovalHandler) {
	r.handler = handler
}

func (r *approvalBridgeRunnerStub) UpdateProvider(_ config.ProviderConfig, _ llm.Client) {}

func (r *approvalBridgeRunnerStub) ListSkills() ([]skills.Skill, []skills.Diagnostic) {
	return nil, nil
}

func (r *approvalBridgeRunnerStub) GetActiveSkill(_ *session.Session) (skills.Skill, bool) {
	return skills.Skill{}, false
}

func (r *approvalBridgeRunnerStub) ActivateSkill(_ *session.Session, _ string, _ map[string]string) (skills.Skill, error) {
	return skills.Skill{}, nil
}

func (r *approvalBridgeRunnerStub) ClearActiveSkill(_ *session.Session) error {
	return nil
}

func (r *approvalBridgeRunnerStub) ClearSkill(_ string) (skills.ClearResult, error) {
	return skills.ClearResult{}, nil
}

func TestInstallApprovalBridgeRoutesRunnerApprovalsToAsyncChannel(t *testing.T) {
	runner := &approvalBridgeRunnerStub{}
	m := &model{
		runner: runner,
		async:  make(chan tea.Msg, 1),
	}
	m.installApprovalBridge()
	if runner.handler == nil {
		t.Fatal("expected approval handler to be installed on runner")
	}

	done := make(chan struct{})
	var approved bool
	var callErr error
	go func() {
		approved, callErr = runner.handler(ApprovalRequest{
			Command: "go test ./...",
			Reason:  "outside lease scope",
		})
		close(done)
	}()

	select {
	case msg := <-m.async:
		req, ok := msg.(approvalRequestMsg)
		if !ok {
			t.Fatalf("expected approval request msg, got %T", msg)
		}
		if req.Request.Command != "go test ./..." {
			t.Fatalf("unexpected approval command: %#v", req.Request)
		}
		req.Reply <- approvalDecision{Approved: true}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval request message")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval handler completion")
	}
	if callErr != nil {
		t.Fatalf("unexpected approval handler error: %v", callErr)
	}
	if !approved {
		t.Fatal("expected approval decision to propagate back to runner handler")
	}
}
