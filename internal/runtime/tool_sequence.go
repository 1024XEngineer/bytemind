package runtime

import (
	"encoding/json"
	"strings"

	"bytemind/internal/llm"
)

const DefaultRepeatedToolSequenceThreshold = 3

type ToolSequenceTracker struct {
	lastSignature string
	repeatCount   int
	threshold     int
}

type ToolSequenceObservation struct {
	Signature        string
	RepeatCount      int
	ReachedThreshold bool
	UniqueToolNames  []string
}

func NewToolSequenceTracker(threshold int) *ToolSequenceTracker {
	if threshold <= 0 {
		threshold = DefaultRepeatedToolSequenceThreshold
	}
	return &ToolSequenceTracker{threshold: threshold}
}

func (t *ToolSequenceTracker) Observe(calls []llm.ToolCall) ToolSequenceObservation {
	signature := SignatureToolCalls(calls)
	if signature == t.lastSignature {
		t.repeatCount++
	} else {
		t.lastSignature = signature
		t.repeatCount = 1
	}
	return ToolSequenceObservation{
		Signature:        signature,
		RepeatCount:      t.repeatCount,
		ReachedThreshold: t.repeatCount >= t.threshold,
		UniqueToolNames:  UniqueToolCallNames(calls),
	}
}

func SignatureToolCalls(calls []llm.ToolCall) string {
	parts := make([]string, 0, len(calls))
	for _, call := range calls {
		parts = append(parts, call.Function.Name+":"+NormalizeToolArguments(call.Function.Arguments))
	}
	return strings.Join(parts, "|")
}

func NormalizeToolArguments(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	data, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return string(data)
}

func UniqueToolCallNames(calls []llm.ToolCall) []string {
	seen := make(map[string]struct{}, len(calls))
	result := make([]string, 0, len(calls))
	for _, call := range calls {
		if _, ok := seen[call.Function.Name]; ok {
			continue
		}
		seen[call.Function.Name] = struct{}{}
		result = append(result, call.Function.Name)
	}
	return result
}
