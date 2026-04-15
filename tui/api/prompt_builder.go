package api

import (
	"bytemind/internal/agent"
	"bytemind/internal/llm"
)

type PromptBuildRequest struct {
	RawInput        string
	MentionBindings map[string]llm.AssetID
	Pasted          any
}

type PromptBuildResult struct {
	Prompt      agent.RunPromptInput
	DisplayText string
}
