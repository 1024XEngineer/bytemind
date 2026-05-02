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
	ToolArguments string
	ToolResult    string
	Error         string
	Plan          planpkg.State
	Usage         llm.Usage
}

type Observer interface {
	HandleEvent(Event)
}

type ObserverFunc func(Event)

func (f ObserverFunc) HandleEvent(event Event) {
	f(event)
}
