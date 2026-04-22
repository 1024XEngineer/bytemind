package extensions

import (
	"context"

	corepkg "bytemind/internal/core"
)

type Manager interface {
	Load(ctx context.Context, source string) (ExtensionInfo, error)
	Unload(ctx context.Context, extensionID string) error
	Get(ctx context.Context, extensionID string) (ExtensionInfo, error)
	List(ctx context.Context) ([]ExtensionInfo, error)
}

// Extension models a loadable extension source (for example skill or mcp).
// Implementations should keep failures local to the extension instance and
// expose health degradation through Health rather than process-level errors.
type Extension interface {
	Info() ExtensionInfo
	ResolveTools(ctx context.Context) ([]ExtensionTool, error)
	Health(ctx context.Context) (HealthSnapshot, error)
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

func (NopManager) Get(_ context.Context, extensionID string) (ExtensionInfo, error) {
	return ExtensionInfo{}, wrapError(ErrCodeNotFound, "extension not found", nil)
}

func (NopManager) List(_ context.Context) ([]ExtensionInfo, error) {
	return nil, nil
}
