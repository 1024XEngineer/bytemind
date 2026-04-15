package tui

import (
	"strings"

	"bytemind/internal/session"
	tuiapi "bytemind/internal/tui/api"
	tuiruntime "bytemind/internal/tui/runtime"
	tuiservices "bytemind/internal/tui/services"
)

func (m *model) refreshSkillCatalog() error {
	if m == nil {
		return nil
	}
	api := m.runtimeAPI()
	if api == nil {
		m.skillCatalog = tuiapi.SkillsState{}
		return nil
	}
	manager := m.skillsManager()
	result := manager.GetState()
	if !result.Success {
		return nil
	}
	m.skillCatalog = result.Data
	return nil
}

func (m *model) runtimeAPI() tuiruntime.UIAPI {
	if m == nil {
		return nil
	}
	if m.runtime != nil {
		return m.runtime
	}
	m.runtime = tuiruntime.NewService(tuiruntime.Dependencies{
		Runner:     m.runner,
		Store:      m.store,
		ImageStore: m.imageStore,
		Workspace:  m.workspace,
	})
	return m.runtime
}

func (m *model) skillsManager() tuiapi.SkillsManager {
	if m == nil {
		return nil
	}
	if m.services != nil {
		m.services.BindSession(m.sess)
		return m.services.Skills()
	}
	if m.skills != nil {
		if binder, ok := m.skills.(interface{ BindSession(*session.Session) }); ok {
			binder.BindSession(m.sess)
		}
		return m.skills
	}
	m.skills = tuiservices.NewSkillManager(m.runtimeAPI(), m.sess)
	return m.skills
}

func (m *model) promptBuildService() tuiapi.PromptBuilder {
	if m == nil {
		return nil
	}
	if m.services != nil {
		m.services.BindSession(m.sess)
		return m.services.PromptBuilder()
	}
	if m.promptBuilder != nil {
		return m.promptBuilder
	}
	m.promptBuilder = tuiservices.NewPromptBuilder(m.runtimeAPI(), m.sess)
	return m.promptBuilder
}

func (m model) activeSkillSnapshot() *tuiapi.ActiveSkill {
	if m.skillCatalog.Active == nil {
		if m.sess != nil && m.sess.ActiveSkill != nil {
			return &tuiapi.ActiveSkill{
				Name: strings.TrimSpace(m.sess.ActiveSkill.Name),
				Args: cloneSkillArgs(m.sess.ActiveSkill.Args),
			}
		}
		return nil
	}
	active := *m.skillCatalog.Active
	active.Args = cloneSkillArgs(m.skillCatalog.Active.Args)
	return &active
}

func cloneSkillArgs(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		cloned[key] = value
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}
