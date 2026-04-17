package api

import (
	"bytemind/internal/session"
	tuiruntime "bytemind/tui/runtime"
)

type PromptBuilder interface {
	Build(req PromptBuildRequest, pasted tuiruntime.PastedState) Result[PromptBuildResult]
}

type Provider interface {
	BindSession(sess *session.Session)
	Skills() SkillsManager
	InputPolicy() InputPolicy
	PromptBuilder() PromptBuilder
}
