package extensions

import (
	"context"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

type Manager interface {
	Load(ctx context.Context, source string) (ExtensionInfo, error)
	Unload(ctx context.Context, extensionID string) error
	Get(ctx context.Context, extensionID string) (ExtensionInfo, error)
	List(ctx context.Context) ([]ExtensionInfo, error)
}

// Reloader is an optional manager capability for force-refreshing extension
// runtime state (for example after config mutations).
type Reloader interface {
	Reload(ctx context.Context) error
}

// ToolResolver is an optional manager capability for resolving extension tools
// into bridgeable tool definitions for registry sync.
type ToolResolver interface {
	ResolveAllTools(ctx context.Context) ([]ExtensionTool, error)
}

// HealthTester is an optional manager capability used by /mcp and CLI health
// checks to force a server probe and return the latest snapshot.
type HealthTester interface {
	Test(ctx context.Context, extensionID string) (HealthSnapshot, error)
}

// Invalidator is an optional manager capability for marking extension runtime
// caches dirty so the next sync can refresh eagerly.
type Invalidator interface {
	Invalidate(extensionID string)
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

func (NopManager) Reload(_ context.Context) error {
	return nil
}

func (NopManager) ResolveAllTools(_ context.Context) ([]ExtensionTool, error) {
	return nil, nil
}

func (NopManager) Test(_ context.Context, _ string) (HealthSnapshot, error) {
	return HealthSnapshot{}, wrapError(ErrCodeNotFound, "extension not found", nil)
}

func (NopManager) Invalidate(_ string) {}
