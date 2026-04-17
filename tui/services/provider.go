package services

import (
	"bytemind/internal/session"
	"bytemind/tui/api"
	tuiruntime "bytemind/tui/runtime"
)

type Provider struct {
	api         tuiruntime.UIAPI
	sess        *session.Session
	skills      *SkillManager
	inputPolicy *InputPolicy
	prompt      *PromptBuilder
}

func NewProvider(api tuiruntime.UIAPI, sess *session.Session) *Provider {
	return &Provider{
		api:         api,
		sess:        sess,
		skills:      NewSkillManager(api, sess),
		inputPolicy: NewInputPolicy(),
		prompt:      NewPromptBuilder(api, sess),
	}
}

func (p *Provider) BindSession(sess *session.Session) {
	if p == nil {
		return
	}
	p.sess = sess
	if p.skills != nil {
		p.skills.BindSession(sess)
	}
	if p.prompt != nil {
		p.prompt.BindSession(sess)
	}
}

func (p *Provider) Skills() api.SkillsManager {
	if p == nil {
		return nil
	}
	return p.skills
}

func (p *Provider) InputPolicy() api.InputPolicy {
	if p == nil {
		return nil
	}
	return p.inputPolicy
}

func (p *Provider) PromptBuilder() api.PromptBuilder {
	if p == nil {
		return nil
	}
	return p.prompt
}
