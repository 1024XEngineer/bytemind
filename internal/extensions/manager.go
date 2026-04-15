package extensions

import (
	"context"

	corepkg "bytemind/internal/core"
)

type ExtensionKind string

const (
	ExtensionMCP   ExtensionKind = "mcp"
	ExtensionSkill ExtensionKind = "skill"
)

type ExtensionInfo struct {
	ID          string
	Name        string
	Kind        ExtensionKind
	Version     string
	Description string
}

type Manager interface {
	Load(ctx context.Context, source string) (ExtensionInfo, error)
	Unload(ctx context.Context, extensionID string) error
	List(ctx context.Context) ([]ExtensionInfo, error)
}

type ToolUseContext struct {
	SessionID corepkg.SessionID
	TaskID    corepkg.TaskID
	TraceID   corepkg.TraceID
	Workspace string
	Metadata  map[string]string
}

// NopManager keeps extension layer explicit while integration is incremental.
type NopManager struct{}

func (NopManager) Load(_ context.Context, _ string) (ExtensionInfo, error) {
	return ExtensionInfo{}, nil
}

func (NopManager) Unload(_ context.Context, _ string) error {
	return nil
}

func (NopManager) List(_ context.Context) ([]ExtensionInfo, error) {
	return nil, nil
}
