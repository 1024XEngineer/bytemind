package tui

import (
	"math"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

type realtimeTokenEstimator struct {
	encoder *tiktoken.Tiktoken
}

func newRealtimeTokenEstimator(model string) *realtimeTokenEstimator {
	name := strings.TrimSpace(model)
	if name == "" {
		name = "gpt-4"
	}
	enc, err := tiktoken.EncodingForModel(name)
	if err != nil {
		enc, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return nil
		}
	}
	return &realtimeTokenEstimator{encoder: enc}
}

func estimateDeltaTokens(estimator *realtimeTokenEstimator, text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if estimator != nil && estimator.encoder != nil {
		tokens := estimator.encoder.Encode(text, nil, nil)
		return max(0, len(tokens))
	}
	// Fallback if tokenizer init failed: rough 4 chars ~= 1 token.
	return max(1, int(math.Ceil(float64(len([]rune(text)))/4.0)))
}
