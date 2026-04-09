package tokenusage

import (
	"context"
	"time"
)

// LLMProvider 示例接口，演示如何集成TokenUsageManager。
type LLMProvider interface {
	Complete(ctx context.Context, content string) (*Response, error)
	ModelName() string
}

type Message struct {
	Content string
}

type Response struct {
	Content string
}

// Agent 示例：将token记录接入业务调用链。
type Agent struct {
	tokenManager *TokenUsageManager
	llm          LLMProvider
}

func (a *Agent) ProcessMessage(ctx context.Context, msg *Message) (*Response, error) {
	if a == nil || a.llm == nil {
		return nil, wrapError(ErrCodeInvalidInput, "agent is not initialized", nil)
	}
	if msg == nil {
		return nil, wrapError(ErrCodeInvalidInput, "message is nil", nil)
	}

	sessionID := a.getSessionID(msg)
	startTime := time.Now()
	inputTokens := ApproximateTokens(msg.Content)

	response, err := a.llm.Complete(ctx, msg.Content)
	latency := time.Since(startTime)

	var outputTokens int64
	if response != nil {
		outputTokens = ApproximateTokens(response.Content)
	}

	if a.tokenManager != nil {
		_ = a.tokenManager.RecordTokenUsage(ctx, &TokenRecordRequest{
			SessionID:    sessionID,
			ModelName:    a.llm.ModelName(),
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			RequestID:    a.generateRequestID(),
			Latency:      latency,
			Success:      err == nil,
		})
	}
	return response, err
}

func (a *Agent) getSessionID(_ *Message) string {
	return "default-session"
}

func (a *Agent) generateRequestID() string {
	return time.Now().UTC().Format("20060102150405.000000000")
}
