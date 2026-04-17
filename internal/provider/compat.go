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
	health        HealthChecker
	allowFallback bool
}

func NewRoutedClient(router Router) llm.Client {
	return NewRoutedClientWithPolicy(router, nil, false)
}

func NewRoutedClientWithPolicy(router Router, health HealthChecker, allowFallback bool) llm.Client {
	if router == nil {
		return nil
	}
	return &RoutedClient{router: router, health: health, allowFallback: allowFallback}
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
	hasStreamedDelta := false
	forwardDelta := onDelta
	if stream && onDelta != nil {
		forwardDelta = func(delta string) {
			hasStreamedDelta = true
			onDelta(delta)
		}
	}
	for _, target := range targets {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return llm.Message{}, ctxErr
		}
		if target.Client == nil {
			continue
		}
		callReq := Request{ChatRequest: req}
		callReq.Model = string(target.ModelID)
		msg, err := executeTarget(ctx, target, callReq, stream, forwardDelta)
		if err == nil {
			if c.health != nil {
				c.health.RecordSuccess(ctx, target.ProviderID)
			}
			return msg, nil
		}
		if c.health != nil {
			c.health.RecordFailure(ctx, target.ProviderID, err)
		}
		if errors.Is(err, context.Canceled) {
			return llm.Message{}, err
		}
		mapped := mapCompatError(target.ProviderID, err)
		if mapped == nil {
			return llm.Message{}, err
		}
		lastErr = mapped
		if !mapped.Retryable || hasStreamedDelta {
			return llm.Message{}, mapped
		}
	}
	if lastErr != nil {
		return llm.Message{}, lastErr
	}
	return llm.Message{}, unavailableRouteError("no provider candidates available")
}

func executeTarget(ctx context.Context, target RouteTarget, req Request, stream bool, onDelta func(string)) (llm.Message, error) {
	streamCh, err := target.Client.Stream(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	var result llm.Message
	hasTerminal := false
	hasDelta := false
	for event := range streamCh {
		switch event.Type {
		case EventDelta:
			if event.Delta != "" {
				result.Content += event.Delta
				hasDelta = true
				if stream && onDelta != nil {
					onDelta(event.Delta)
				}
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
				return merged, nil
			}
			result.Normalize()
			return result, nil
		case EventError:
			hasTerminal = true
			if event.Error != nil {
				mapped := mapCompatError(target.ProviderID, event.Error)
				if mapped == nil && errors.Is(event.Error, context.Canceled) {
					return llm.Message{}, context.Canceled
				}
				if mapped == nil {
					return llm.Message{}, unavailableRouteError("provider stream emitted invalid error payload")
				}
				return llm.Message{}, mapped
			}
			return llm.Message{}, unavailableRouteError("provider stream emitted error event without error payload")
		}
	}
	if !hasTerminal {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return llm.Message{}, ctxErr
		}
		if hasDelta || len(result.ToolCalls) > 0 || result.Usage != nil {
			return llm.Message{}, unavailableRouteError("provider stream terminated without terminal event")
		}
		return llm.Message{}, unavailableRouteError("provider stream terminated unexpectedly")
	}
	result.Normalize()
	return result, nil
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
		normalizer := newStreamNormalizer(req.TraceID, a.providerID, modelID)
		if !normalizeEvent(ctx, normalizer, stream, Event{
			Type:       EventStart,
			TraceID:    req.TraceID,
			ProviderID: a.providerID,
			ModelID:    modelID,
		}) {
			return
		}

		deltas := make(chan string, 16)
		deltaDone := make(chan struct{})
		deltaInputClosed := make(chan struct{})
		go func() {
			defer close(deltaDone)
			for {
				select {
				case <-ctx.Done():
					return
				case <-deltaInputClosed:
					for {
						select {
						case delta := <-deltas:
							if strings.TrimSpace(delta) == "" {
								continue
							}
							if !normalizeEvent(ctx, normalizer, stream, Event{
								Type:       EventDelta,
								TraceID:    req.TraceID,
								ProviderID: a.providerID,
								ModelID:    modelID,
								Delta:      delta,
							}) {
								return
							}
						default:
							return
						}
					}
				case delta := <-deltas:
					if strings.TrimSpace(delta) == "" {
						continue
					}
					if !normalizeEvent(ctx, normalizer, stream, Event{
						Type:       EventDelta,
						TraceID:    req.TraceID,
						ProviderID: a.providerID,
						ModelID:    modelID,
						Delta:      delta,
					}) {
						return
					}
				}
			}
		}()

		message, err := a.client.StreamMessage(ctx, req.ChatRequest, func(delta string) {
			select {
			case <-ctx.Done():
				return
			case <-deltaInputClosed:
				return
			case deltas <- delta:
			}
		})
		close(deltaInputClosed)
		<-deltaDone
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			mapped := mapCompatError(a.providerID, err)
			if mapped == nil {
				return
			}
			_ = normalizeEvent(ctx, normalizer, stream, Event{
				Type:       EventError,
				TraceID:    req.TraceID,
				ProviderID: a.providerID,
				ModelID:    modelID,
				Error:      mapped,
			})
			return
		}
		if message.Usage != nil {
			if !normalizeEvent(ctx, normalizer, stream, Event{
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
			if !normalizeEvent(ctx, normalizer, stream, Event{
				Type:       EventToolCall,
				TraceID:    req.TraceID,
				ProviderID: a.providerID,
				ModelID:    modelID,
				ToolCall:   &call,
			}) {
				return
			}
		}
		_ = normalizeEvent(ctx, normalizer, stream, Event{
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
	return mapError(providerID, err)
}
