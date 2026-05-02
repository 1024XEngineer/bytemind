package api

import (
	"github.com/1024XEngineer/bytemind/internal/session"
	tuiruntime "github.com/1024XEngineer/bytemind/tui/runtime"
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
