package tui

import (
	"context"
	"io"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/mcpctl"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/skills"
)

type EventType string

const (
	EventRunStarted        EventType = "run_started"
	EventAssistantDelta    EventType = "assistant_delta"
	EventAssistantMessage  EventType = "assistant_message"
	EventToolCallStarted   EventType = "tool_call_started"
	EventToolCallCompleted EventType = "tool_call_completed"
	EventPlanUpdated       EventType = "plan_updated"
	EventUsageUpdated      EventType = "usage_updated"
	EventRunFinished       EventType = "run_finished"
)

type Event struct {
	Type          EventType
	SessionID     string
	UserInput     string
	Content       string
	ToolName      string
	ToolArguments string
	ToolResult    string
	Error         string
	Plan          planpkg.State
	Usage         llm.Usage
}

type ApprovalRequest struct {
	ToolName string
	Command  string
	Reason   string
}

type ApprovalDisposition string

const (
	ApprovalApproveOnce            ApprovalDisposition = "approve_once"
	ApprovalApproveSameToolSession ApprovalDisposition = "approve_same_tool_session"
	ApprovalApproveAllSession      ApprovalDisposition = "approve_all_session"
	ApprovalDeny                   ApprovalDisposition = "deny"
)

type ApprovalDecision struct {
	Disposition ApprovalDisposition
}

func (d ApprovalDecision) Approved() bool {
	switch d.Disposition {
	case ApprovalApproveOnce, ApprovalApproveSameToolSession, ApprovalApproveAllSession:
		return true
	default:
		return false
	}
}

func NormalizeApprovalDecision(d ApprovalDecision) ApprovalDecision {
	switch d.Disposition {
	case ApprovalApproveOnce, ApprovalApproveSameToolSession, ApprovalApproveAllSession, ApprovalDeny:
		return d
	default:
		return ApprovalDecision{Disposition: ApprovalDeny}
	}
}

type ApprovalHandler func(ApprovalRequest) (ApprovalDecision, error)

type Observer func(Event)

type RunPromptInput struct {
	UserMessage llm.Message
	Assets      map[llm.AssetID]llm.ImageAsset
	DisplayText string
}

type Runner interface {
	RunPromptWithInput(ctx context.Context, sess *session.Session, input RunPromptInput, mode string, out io.Writer) (string, error)
	SetObserver(observer Observer)
	SetApprovalHandler(handler ApprovalHandler)
	UpdateProvider(providerCfg config.ProviderConfig, client llm.Client)
	ListSkills() ([]skills.Skill, []skills.Diagnostic)
	GetActiveSkill(sess *session.Session) (skills.Skill, bool)
	ActivateSkill(sess *session.Session, name string, args map[string]string) (skills.Skill, error)
	ClearActiveSkill(sess *session.Session) error
	ClearSkill(name string) (skills.ClearResult, error)
}

type SessionStore interface {
	Save(session *session.Session) error
	Load(id string) (*session.Session, error)
	List(limit int) ([]session.Summary, []string, error)
	DeleteInWorkspace(workspace, id string) error
	CleanupZeroMessageSessions(workspace, activeSessionID string) (session.CleanupResult, error)
}

type MCPService interface {
	List(ctx context.Context) ([]mcpctl.ServerStatus, error)
	Show(ctx context.Context, serverID string) (mcpctl.ServerDetail, error)
	Add(ctx context.Context, req mcpctl.AddRequest) (mcpctl.ServerStatus, error)
	Remove(ctx context.Context, serverID string) error
	Enable(ctx context.Context, serverID string, enabled bool) (mcpctl.ServerStatus, error)
	Test(ctx context.Context, serverID string) (mcpctl.ServerStatus, error)
	Reload(ctx context.Context) error
}
