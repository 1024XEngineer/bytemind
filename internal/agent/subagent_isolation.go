package agent

import (
	"io"
	"sync"

	"github.com/1024XEngineer/bytemind/internal/tools"
)

// nonInteractiveApproval returns an ApprovalHandler that auto-approves all requests.
// This is safe for subagents because their toolset is already narrowed by Gateway.Preflight()
// to only include tools the parent has authorized.
func nonInteractiveApproval() tools.ApprovalHandler {
	return func(req tools.ApprovalRequest) (bool, error) {
		return true, nil
	}
}

// threadSafeObserver wraps an Observer to be safe for concurrent use from multiple goroutines.
// Subagents run in separate goroutines and need thread-safe event dispatch.
type threadSafeObserver struct {
	inner Observer
	mu    sync.Mutex
}

func (o *threadSafeObserver) HandleEvent(event Event) {
	if o.inner == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.inner.HandleEvent(event)
}

// noOpObserver discards all events. Used for child runners so their internal
// LLM events (tool calls, streaming deltas, run lifecycle) do not leak into
// the parent's UI event channel.
type noOpObserver struct{}

func (o *noOpObserver) HandleEvent(Event) {}

// subAgentStdout returns a writer suitable for a subagent's stdout.
// Currently discards all output. In the future this can be upgraded to a buffer
// that captures output for the parent to display.
func subAgentStdout() io.Writer {
	return io.Discard
}
