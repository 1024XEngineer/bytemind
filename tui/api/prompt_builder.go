package api

import (
	"github.com/1024XEngineer/bytemind/internal/agent"
	"github.com/1024XEngineer/bytemind/internal/llm"
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
