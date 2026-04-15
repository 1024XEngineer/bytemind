package agent

import (
	"context"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

// SessionStore defines the persistence contract consumed by Runner.
type SessionStore interface {
	Save(session *session.Session) error
}

// ToolRegistry defines the tool definition query contract consumed by Runner.
type ToolRegistry interface {
	DefinitionsForMode(mode planpkg.AgentMode) []llm.ToolDefinition
	DefinitionsForModeWithFilters(mode planpkg.AgentMode, allowlist, denylist []string) []llm.ToolDefinition
}

// ToolExecutor defines the tool execution contract consumed by Runner.
type ToolExecutor interface {
	ExecuteForMode(ctx context.Context, mode planpkg.AgentMode, name, rawArgs string, execCtx *tools.ExecutionContext) (string, error)
}
