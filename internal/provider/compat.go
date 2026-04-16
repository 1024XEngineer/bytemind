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

type RoutedClient struct {
	router        Router
	allowFallback bool
}

func NewRoutedClient(router Router) llm.Client {
	return NewRoutedClientWithPolicy(router, false)
}

func NewRoutedClientWithPolicy(router Router, allowFallback bool) llm.Client {
	if router == nil {
		return nil
	}
	return &RoutedClient{router: router, allowFallback: allowFallback}
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

func (c *RoutedClient) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	return c.execute(ctx, req, false, nil)
}

func (c *RoutedClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	return c.execute(ctx, req, true, onDelta)
}

func (c *RoutedClient) execute(ctx context.Context, req llm.ChatRequest, stream bool, onDelta func(string)) (llm.Message, error) {
	if c == nil || c.router == nil {
		return llm.Message{}, unavailableRouteError("no provider candidates available")
	}
	result, err := c.router.Route(ctx, ModelID(strings.TrimSpace(req.Model)), RouteContext{AllowFallback: c.allowFallback})
	if err != nil {
		return llm.Message{}, err
	}
	targets := make([]RouteTarget, 0, 1+len(result.Fallbacks))
	targets = append(targets, result.Primary)
	targets = append(targets, result.Fallbacks...)
	var lastErr error
	for _, target := range targets {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return llm.Message{}, ctxErr
		}
		if target.Client == nil {
			continue
		}
		callReq := Request{ChatRequest: req}
		callReq.Model = string(target.ModelID)
		msg, deltas, err := executeTarget(ctx, target, callReq)
		if err == nil {
			if stream && onDelta != nil {
				for _, delta := range deltas {
					onDelta(delta)
				}
			}
			return msg, nil
		}
		lastErr = err
		var providerErr *Error
		if errors.As(err, &providerErr) {
			if !providerErr.Retryable {
				return llm.Message{}, providerErr
			}
			continue
		}
		mapped := mapCompatError(target.ProviderID, err)
		if !mapped.Retryable {
			return llm.Message{}, mapped
		}
		lastErr = mapped
	}
	if lastErr != nil {
		var providerErr *Error
		if errors.As(lastErr, &providerErr) {
			return llm.Message{}, providerErr
		}
		return llm.Message{}, mapCompatError("", lastErr)
	}
	return llm.Message{}, unavailableRouteError("no provider candidates available")
}

func executeTarget(ctx context.Context, target RouteTarget, req Request) (llm.Message, []string, error) {
	streamCh, err := target.Client.Stream(ctx, req)
	if err != nil {
		return llm.Message{}, nil, err
	}
	var result llm.Message
	var deltas []string
	hasTerminal := false
	hasDelta := false
	for event := range streamCh {
		switch event.Type {
		case EventDelta:
			if event.Delta != "" {
				result.Content += event.Delta
				deltas = append(deltas, event.Delta)
				hasDelta = true
			}
		case EventToolCall:
			if event.ToolCall != nil {
				result.ToolCalls = append(result.ToolCalls, *event.ToolCall)
			}
		case EventUsage:
			if event.Usage != nil {
				result.Usage = &llm.Usage{InputTokens: int(event.Usage.InputTokens), OutputTokens: int(event.Usage.OutputTokens), TotalTokens: int(event.Usage.TotalTokens)}
			}
		case EventResult:
			hasTerminal = true
			if event.Result != nil {
				merged := *event.Result
				if strings.TrimSpace(merged.Content) == "" && result.Content != "" {
					merged.Content = result.Content
				}
				if len(merged.ToolCalls) == 0 && len(result.ToolCalls) > 0 {
					merged.ToolCalls = append([]llm.ToolCall(nil), result.ToolCalls...)
				}
				if merged.Usage == nil && result.Usage != nil {
					usage := *result.Usage
					merged.Usage = &usage
				}
				merged.Normalize()
				return merged, deltas, nil
			}
			result.Normalize()
			return result, deltas, nil
		case EventError:
			hasTerminal = true
			if event.Error != nil {
				return llm.Message{}, nil, event.Error
			}
			return llm.Message{}, nil, unavailableRouteError("provider stream emitted error event without error payload")
		}
	}
	if !hasTerminal {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return llm.Message{}, nil, ctxErr
		}
		if hasDelta || len(result.ToolCalls) > 0 || result.Usage != nil {
			return llm.Message{}, nil, unavailableRouteError("provider stream terminated without terminal event")
		}
		return llm.Message{}, nil, unavailableRouteError("provider stream terminated unexpectedly")
	}
	result.Normalize()
	return result, deltas, nil
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
