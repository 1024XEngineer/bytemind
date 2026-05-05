package agent

import (
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
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
	SessionID     corepkg.SessionID
	UserInput     string
	Content       string
	ToolName      string
	ToolCallID    string
	ToolArguments string
	ToolResult    string
	Error         string
	Plan          planpkg.State
	Usage         llm.Usage
	AgentID       string // non-empty when emitted by a subagent
}

type Observer interface {
	HandleEvent(Event)
}

type ObserverFunc func(Event)

func (f ObserverFunc) HandleEvent(event Event) {
	f(event)
}

// SubAgentObserver wraps an Observer to tag all events with the given agentID,
// so the TUI can distinguish subagent events from main agent events.
func SubAgentObserver(inner Observer, agentID string) Observer {
	if inner == nil {
		return ObserverFunc(func(Event) {})
	}
	return ObserverFunc(func(event Event) {
		event.AgentID = agentID
		inner.HandleEvent(event)
	})
}
