package tui

import (
	"context"
	"io"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/skills"
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
	Command string
	Reason  string
}

type ApprovalHandler func(ApprovalRequest) (bool, error)

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
}
