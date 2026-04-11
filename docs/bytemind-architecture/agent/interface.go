package agent

import (
	"context"
	"encoding/json"
	"time"

	"bytemind/internal/core"
)

type ErrorCode string

const (
	ErrCodePromptTooLong    ErrorCode = "prompt_too_long"
	ErrCodeModelUnavailable ErrorCode = "model_unavailable"
	ErrCodeModelStream      ErrorCode = "model_stream_error"
	ErrCodeToolExecute      ErrorCode = "tool_execute_error"
	ErrCodePermissionDenied ErrorCode = "permission_denied"
	ErrCodePersistFailed    ErrorCode = "persist_failed"
)

type Message struct {
	Role       core.Role
	Content    string
	Name       string // message source name, e.g. tool or sub-agent name
	ToolCallID string
	CreatedAt  time.Time
}

type TurnRequest struct {
	SessionID       core.SessionID
	Input           string
	MaxInputTokens  int
	MaxOutputTokens int
	Metadata        map[string]string
}

type TurnEventType string

const (
	TurnEventStart    TurnEventType = "start"
	TurnEventDelta    TurnEventType = "delta"
	TurnEventToolUse  TurnEventType = "tool_use"
	TurnEventToolOut  TurnEventType = "tool_result"
	TurnEventComplete TurnEventType = "complete"
	TurnEventError    TurnEventType = "error"
)

type TurnEvent struct {
	Type      TurnEventType
	TurnID    string
	SessionID core.SessionID
	TaskID    core.TaskID
	Payload   json.RawMessage
	ErrorCode string
	Timestamp time.Time
}

type ToolCall struct {
	CallID string
	Name   string
	Args   json.RawMessage
}

type PermissionDecision struct {
	Decision   core.Decision
	ReasonCode string
}

type Engine interface {
	HandleTurn(ctx context.Context, req TurnRequest) (<-chan TurnEvent, error)
}

type SessionSnapshot struct {
	SessionID core.SessionID
	Mode      string
	Messages  []Message
	Metadata  map[string]string
}

type SessionGateway interface {
	Snapshot(ctx context.Context, sessionID core.SessionID) (SessionSnapshot, error)
	AppendTurn(ctx context.Context, sessionID core.SessionID, event TurnEvent) error
}

type ContextBuildInput struct {
	Request TurnRequest
	Session SessionSnapshot
}

type ModelRequest struct {
	Messages        []Message
	ToolsSchemaJSON json.RawMessage
	MaxOutputTokens int
}

type ContextGateway interface {
	Build(ctx context.Context, in ContextBuildInput) (ModelRequest, error)
}

type ModelEvent struct {
	Type    string
	Payload json.RawMessage
}

type ModelGateway interface {
	Stream(ctx context.Context, req ModelRequest) (<-chan ModelEvent, error)
}

type PolicyGateway interface {
	EvaluateToolUse(ctx context.Context, sessionID core.SessionID, call ToolCall) (PermissionDecision, error)
}

type ToolResultEvent struct {
	Type    string
	Payload json.RawMessage
}

type ToolGateway interface {
	Execute(ctx context.Context, call ToolCall, sessionID core.SessionID) (<-chan ToolResultEvent, error)
}

type RuntimeGateway interface {
	SpawnSubAgent(ctx context.Context, req SubAgentRequest) (SubAgentHandle, error)
	WaitSubAgent(ctx context.Context, handle SubAgentHandle) (SubAgentResult, error)
}

type SubAgentRequest struct {
	ParentSessionID core.SessionID
	Mode            string
	Prompt          string
	Background      bool
}

type SubAgentHandle struct {
	SubTaskID core.TaskID
}

type SubAgentResult struct {
	SubTaskID core.TaskID
	Output    string
	ErrorCode string
}

