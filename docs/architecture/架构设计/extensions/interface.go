//go:build ignore

package extensions

import (
	"context"
	"encoding/json"
	"time"

	"bytemind/internal/core"
)

type ExtensionKind string

const (
	ExtensionMCP   ExtensionKind = "mcp"
	ExtensionSkill ExtensionKind = "skill"
)

type ExtensionStatus string

const (
	StatusLoaded   ExtensionStatus = "loaded"
	StatusActive   ExtensionStatus = "active"
	StatusDegraded ExtensionStatus = "degraded"
	StatusStopped  ExtensionStatus = "stopped"
)

type ErrorCode string

const (
	ErrCodeInvalidManifest ErrorCode = "invalid_manifest"
	ErrCodeIncompatible    ErrorCode = "incompatible_version"
	ErrCodeLoadFailed      ErrorCode = "load_failed"
	ErrCodeActivateFailed  ErrorCode = "activate_failed"
	ErrCodeToolBridge      ErrorCode = "tool_bridge_failed"
)

type Manifest struct {
	ID          string
	Name        string
	Kind        ExtensionKind
	Version     string
	Description string
	Entry       string
	UpdatedAt   time.Time
}

type Capability struct {
	Name        string
	Description string
	SideEffects []string
}

type ExtensionInfo struct {
	Manifest     Manifest
	Status       ExtensionStatus
	Capabilities []Capability
}

type ActivateOptions struct {
	WorkspaceRoot string
	ConfigPath    string
	Env           map[string]string
}

type Extension interface {
	Info() ExtensionInfo
	Activate(ctx context.Context, opts ActivateOptions) error
	Deactivate(ctx context.Context) error
	Health(ctx context.Context) (ExtensionStatus, error)
}

type ToolUseContext struct {
	SessionID core.SessionID
	TaskID    core.TaskID
	TraceID   core.TraceID
	Workspace string
	Metadata  map[string]string
}

type ToolEvent struct {
	Type      string
	CallID    string
	Meta      core.EventMeta
	Payload   json.RawMessage
	ErrorCode string
}

type ExtensionTool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage, tctx ToolUseContext) (<-chan ToolEvent, error)
}

type ToolProvider interface {
	Tools(ctx context.Context) ([]ExtensionTool, error)
}

type Manager interface {
	Load(ctx context.Context, source string) (ExtensionInfo, error)
	Unload(ctx context.Context, extensionID string) error
	List(ctx context.Context) ([]ExtensionInfo, error)
	Get(ctx context.Context, extensionID string) (ExtensionInfo, bool, error)
}

type Resolver interface {
	ResolveTools(ctx context.Context, extensionID string) ([]ExtensionTool, error)
}
