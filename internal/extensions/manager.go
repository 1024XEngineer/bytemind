package extensions

import (
	"context"
	"strings"

	corepkg "bytemind/internal/core"
)

type ExtensionKind string

const (
	ExtensionMCP   ExtensionKind = "mcp"
	ExtensionSkill ExtensionKind = "skill"
)

type ExtensionScope string

const (
	ExtensionScopeBuiltin ExtensionScope = "builtin"
	ExtensionScopeUser    ExtensionScope = "user"
	ExtensionScopeProject ExtensionScope = "project"
	ExtensionScopeRemote  ExtensionScope = "remote"
)

type ExtensionSource struct {
	Scope ExtensionScope
	Ref   string
}

type CapabilitySet struct {
	Prompts   int
	Resources int
	Tools     int
	Commands  int
}

type ExtensionInfo struct {
	ID           string
	Name         string
	Kind         ExtensionKind
	Version      string
	Title        string
	Description  string
	Source       ExtensionSource
	Capabilities CapabilitySet
}

func (info ExtensionInfo) Valid() bool {
	if strings.TrimSpace(info.ID) == "" {
		return false
	}
	if strings.TrimSpace(info.Name) == "" {
		return false
	}
	switch info.Kind {
	case ExtensionMCP, ExtensionSkill:
		return true
	default:
		return false
	}
}

func (info ExtensionInfo) IsZero() bool {
	return strings.TrimSpace(info.ID) == "" && strings.TrimSpace(info.Name) == "" && strings.TrimSpace(string(info.Kind)) == ""
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
