package tokenusage

import "github.com/1024XEngineer/bytemind/internal/llm"

// ResolveTurnUsage returns provider usage when present, otherwise falls back to
// approximation from request/reply payloads.
func ResolveTurnUsage(request llm.ChatRequest, reply *llm.Message) llm.Usage {
	if reply != nil && reply.Usage != nil {
		usage := *reply.Usage
		input := max(0, usage.InputTokens)
		output := max(0, usage.OutputTokens)
		context := max(0, usage.ContextTokens)
		total := usage.TotalTokens
		if total <= 0 {
			total = input + output + context
		}
		return llm.Usage{
			InputTokens:   input,
			OutputTokens:  output,
			ContextTokens: context,
			TotalTokens:   max(0, total),
		}
	}

	input := int(ApproximateRequestTokens(request.Messages))
	output := 0
	if reply != nil {
		output += int(ApproximateTokens(reply.Content))
		for _, call := range reply.ToolCalls {
			output += int(ApproximateTokens(call.Function.Name))
			output += int(ApproximateTokens(call.Function.Arguments))
		}
	}
	total := input + output
	return llm.Usage{
		InputTokens:   max(0, input),
		OutputTokens:  max(0, output),
		ContextTokens: 0,
		TotalTokens:   max(0, total),
	}
}
