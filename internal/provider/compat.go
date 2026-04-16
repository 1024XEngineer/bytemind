package provider

import (
	"context"
	"errors"
	"strings"

	"bytemind/internal/llm"
)

type clientAdapter struct {
	providerID   ProviderID
	defaultModel ModelID
	client       llm.Client
}

func WrapClient(providerID ProviderID, defaultModel ModelID, client llm.Client) Client {
	if client == nil {
		return nil
	}
	id := ProviderID(strings.TrimSpace(string(providerID)))
	if id == "" {
		id = ProviderID("unknown")
	}
	return &clientAdapter{
		providerID:   id,
		defaultModel: ModelID(strings.TrimSpace(string(defaultModel))),
		client:       client,
	}
}

func (a *clientAdapter) ProviderID() ProviderID {
	return a.providerID
}

func (a *clientAdapter) ListModels(context.Context) ([]ModelInfo, error) {
	if a.defaultModel == "" {
		return nil, nil
	}
	return []ModelInfo{{
		ProviderID: a.providerID,
		ModelID:    a.defaultModel,
	}}, nil
}

func (a *clientAdapter) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	stream := make(chan Event, 4)
	go func() {
		defer close(stream)
		modelID := ModelID(strings.TrimSpace(req.Model))
		if modelID == "" {
			modelID = a.defaultModel
		}
		if !emit(ctx, stream, Event{
			Type:       EventStart,
			TraceID:    req.TraceID,
			ProviderID: a.providerID,
			ModelID:    modelID,
		}) {
			return
		}
		message, err := a.client.StreamMessage(ctx, req.ChatRequest, func(delta string) {
			if strings.TrimSpace(delta) == "" {
				return
			}
			_ = emit(ctx, stream, Event{
				Type:       EventDelta,
				TraceID:    req.TraceID,
				ProviderID: a.providerID,
				ModelID:    modelID,
				Delta:      delta,
			})
		})
		if err != nil {
			_ = emit(ctx, stream, Event{
				Type:       EventError,
				TraceID:    req.TraceID,
				ProviderID: a.providerID,
				ModelID:    modelID,
				Error:      mapCompatError(a.providerID, err),
			})
			return
		}
		if message.Usage != nil {
			if !emit(ctx, stream, Event{
				Type:       EventUsage,
				TraceID:    req.TraceID,
				ProviderID: a.providerID,
				ModelID:    modelID,
				Usage: &Usage{
					InputTokens:  int64(message.Usage.InputTokens),
					OutputTokens: int64(message.Usage.OutputTokens),
					TotalTokens:  int64(message.Usage.TotalTokens),
				},
			}) {
				return
			}
		}
		message.Normalize()
		for _, toolCall := range message.ToolCalls {
			call := toolCall
			if !emit(ctx, stream, Event{
				Type:       EventToolCall,
				TraceID:    req.TraceID,
				ProviderID: a.providerID,
				ModelID:    modelID,
				ToolCall:   &call,
			}) {
				return
			}
		}
		_ = emit(ctx, stream, Event{
			Type:       EventResult,
			TraceID:    req.TraceID,
			ProviderID: a.providerID,
			ModelID:    modelID,
			Result:     &message,
		})
	}()
	return stream, nil
}

func emit(ctx context.Context, ch chan<- Event, evt Event) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- evt:
		return true
	}
}

func mapCompatError(providerID ProviderID, err error) *Error {
	mapped := &Error{
		Code:      ErrCodeUnavailable,
		Provider:  providerID,
		Message:   "provider request failed",
		Retryable: false,
		Err:       err,
	}
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return mapped
	}
	mapped.Retryable = providerErr.Retryable
	switch providerErr.Code {
	case llm.ErrorCodeRateLimited:
		mapped.Code = ErrCodeRateLimited
		mapped.Message = "provider rate limited"
	case llm.ErrorCodeContextTooLong:
		mapped.Code = ErrCodeBadRequest
		mapped.Message = "request exceeds provider context limit"
	default:
		mapped.Message = "provider unavailable"
	}
	return mapped
}
