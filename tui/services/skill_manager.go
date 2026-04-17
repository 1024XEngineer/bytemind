package services

import (
	"fmt"
	"strings"

	"bytemind/internal/session"
	"bytemind/tui/api"
	tuiruntime "bytemind/tui/runtime"
)

type SkillManager struct {
	api  tuiruntime.UIAPI
	sess *session.Session
}

func NewSkillManager(api tuiruntime.UIAPI, sess *session.Session) *SkillManager {
	return &SkillManager{
		api:  api,
		sess: sess,
	}
}

func (m *SkillManager) BindSession(sess *session.Session) {
	m.sess = sess
}

func (m *SkillManager) GetState() api.Result[api.SkillsState] {
	if m == nil || m.api == nil {
		return api.Unavailable[api.SkillsState]("skills service")
	}
	catalog, err := m.api.LoadSkillCatalog(m.sess)
	if err != nil {
		return failResult[api.SkillsState]("skills service", err)
	}
	state := api.SkillsState{
		Items:       make([]api.SkillItem, 0, len(catalog.Items)),
		Diagnostics: make([]api.SkillDiagnostic, 0, len(catalog.Diagnostics)),
	}
	if catalog.Active != nil {
		state.Active = &api.ActiveSkill{
			Name:  catalog.Active.Name,
			Scope: catalog.Active.Scope,
			Args:  cloneArgs(catalog.Active.Args),
		}
	}
	for _, item := range catalog.Items {
		state.Items = append(state.Items, api.SkillItem{
			Name:        item.Name,
			Scope:       item.Scope,
			Description: item.Description,
			Slash:       item.Slash,
			Aliases:     append([]string(nil), item.Aliases...),
			ToolPolicy:  item.ToolPolicy,
		})
	}
	for _, diag := range catalog.Diagnostics {
		state.Diagnostics = append(state.Diagnostics, api.SkillDiagnostic{
			Level:   diag.Level,
			Skill:   diag.Skill,
			Path:    diag.Path,
			Message: diag.Message,
		})
	}
	return api.Ok(state)
}

func (m *SkillManager) Activate(name string, args map[string]string) api.Result[api.SkillActivation] {
	if m == nil || m.api == nil {
		return api.Unavailable[api.SkillActivation]("skills service")
	}
	activation, err := m.api.ActivateSkill(m.sess, name, args)
	if err != nil {
		return failResult[api.SkillActivation]("skills service", err)
	}
	return api.Ok(api.SkillActivation{
		Name:       activation.Name,
		Scope:      activation.Scope,
		EntrySlash: activation.EntrySlash,
		ToolPolicy: activation.ToolPolicy,
		Args:       cloneArgs(activation.Args),
	})
}

func (m *SkillManager) Clear() api.Result[string] {
	if m == nil || m.api == nil {
		return api.Unavailable[string]("skills service")
	}
	result, err := m.api.ClearActiveSkill(m.sess)
	if err != nil {
		return failResult[string]("skills service", err)
	}
	if result.HadActive {
		return api.Ok(fmt.Sprintf("Cleared active skill `%s` from this session.", result.PreviousName))
	}
	return api.Ok("No active skill in this session; state remains empty.")
}

func (m *SkillManager) Delete(name string) api.Result[api.SkillDeleteResult] {
	if m == nil || m.api == nil {
		return api.Unavailable[api.SkillDeleteResult]("skills service")
	}
	result, err := m.api.DeleteSkill(m.sess, name)
	if err != nil {
		return failResult[api.SkillDeleteResult]("skills service", err)
	}
	return api.Ok(api.SkillDeleteResult{
		Name:          strings.TrimSpace(result.Name),
		Dir:           strings.TrimSpace(result.Dir),
		ClearedActive: result.ClearedActive,
	})
}

func cloneArgs(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
