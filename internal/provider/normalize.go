package provider

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
)

var eventCounter uint64

type streamNormalizer struct {
	traceID    string
	providerID ProviderID
	modelID    ModelID
	emitted    bool
	terminated bool
}

func newStreamNormalizer(traceID string, providerID ProviderID, modelID ModelID) *streamNormalizer {
	return &streamNormalizer{
		traceID:    strings.TrimSpace(traceID),
		providerID: normalizeRouteProviderID(providerID),
		modelID:    normalizeRouteModelID(modelID),
	}
}

func normalizeEvent(ctx context.Context, n *streamNormalizer, ch chan<- Event, evt Event) bool {
	if n == nil {
		n = newStreamNormalizer(evt.TraceID, evt.ProviderID, evt.ModelID)
	}
	normalized, err := n.normalize(evt)
	if err != nil {
		if n.terminated {
			return false
		}
		normalized = n.errorEvent(err)
	}
	return emit(ctx, ch, normalized)
}

func (n *streamNormalizer) normalize(evt Event) (Event, error) {
	if n == nil {
		return Event{}, fmt.Errorf("provider stream normalizer unavailable")
	}
	if n.terminated {
		return Event{}, fmt.Errorf("provider stream emitted event after terminal event")
	}
	if !n.emitted {
		if evt.Type != EventStart {
			return Event{}, fmt.Errorf("provider stream missing start event")
		}
		n.emitted = true
	} else if evt.Type == EventStart {
		return Event{}, fmt.Errorf("provider stream emitted duplicate start event")
	}
	evt = n.enrich(evt)
	switch evt.Type {
	case EventStart:
	case EventDelta:
	case EventToolCall:
	case EventUsage:
	case EventResult:
		n.terminated = true
	case EventError:
		if evt.Error == nil {
			return Event{}, fmt.Errorf("provider stream emitted error event without error payload")
		}
		n.terminated = true
	default:
		return Event{}, fmt.Errorf("provider stream emitted unknown event type %q", evt.Type)
	}
	return evt, nil
}

func (n *streamNormalizer) enrich(evt Event) Event {
	evt.Type = EventType(strings.TrimSpace(string(evt.Type)))
	if strings.TrimSpace(evt.ID) == "" {
		evt.ID = nextEventID()
	}
	if strings.TrimSpace(evt.TraceID) == "" {
		evt.TraceID = n.traceID
	}
	if evt.ProviderID == "" {
		evt.ProviderID = n.providerID
	}
	evt.ProviderID = normalizeRouteProviderID(evt.ProviderID)
	if evt.ModelID == "" {
		evt.ModelID = n.modelID
	}
	evt.ModelID = normalizeRouteModelID(evt.ModelID)
	if evt.Error != nil {
		if evt.Error.Provider == "" {
			evt.Error.Provider = evt.ProviderID
		}
		evt.Error.Retryable = isRetryableCode(evt.Error.Code)

	}
	return evt
}

func (n *streamNormalizer) errorEvent(err error) Event {
	n.terminated = true
	mapped := mapError(n.providerID, err)
	if mapped == nil {
		mapped = unavailableRouteError(err.Error())
	}
	return n.enrich(Event{Type: EventError, Error: mapped})
}

func nextEventID() string {
	return fmt.Sprintf("evt-%d", atomic.AddUint64(&eventCounter, 1))
}
