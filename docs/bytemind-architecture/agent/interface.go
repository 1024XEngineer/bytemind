package agent

import "context"

// Service defines the agent interaction contract.
type Service interface {
	HandleUserMessage(ctx context.Context, sessionID string, input string) (<-chan Event, error)
}

// Event is the stream unit emitted by agent workflows.
type Event struct {
	Type    string
	Payload []byte
}
