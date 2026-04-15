package agent

import (
	"fmt"
	"strings"
	"time"

	"bytemind/internal/session"
	"bytemind/internal/skills"
)

func (r *Runner) ListSkills() ([]skills.Skill, []skills.Diagnostic) {
	if r.skillManager == nil {
		return nil, nil
	}
	return r.skillManager.List()
}

func (r *Runner) AuthorSkill(name, brief string) (skills.AuthorResult, error) {
	if r.skillManager == nil {
		return skills.AuthorResult{}, fmt.Errorf("skill manager is unavailable")
	}
	brief = r.normalizeSkillAuthorBrief(brief)
	return r.skillManager.Author(name, skills.ScopeProject, brief)
}

func (r *Runner) ClearSkill(name string) (skills.ClearResult, error) {
	if r.skillManager == nil {
		return skills.ClearResult{}, fmt.Errorf("skill manager is unavailable")
	}
	return r.skillManager.Clear(name)
}

func (r *Runner) ActivateSkill(sess *session.Session, name string, args map[string]string) (skills.Skill, error) {
	if sess == nil {
		return skills.Skill{}, fmt.Errorf("session is required")
	}
	if r.skillManager == nil {
		return skills.Skill{}, fmt.Errorf("skill manager is unavailable")
	}
	skill, ok := r.skillManager.Find(name)
	if !ok {
		return skills.Skill{}, fmt.Errorf("skill not found: %s", strings.TrimSpace(name))
	}

	normalizedArgs := normalizeSkillArgs(args)
	for _, arg := range skill.Args {
		if _, exists := normalizedArgs[arg.Name]; !exists && strings.TrimSpace(arg.Default) != "" {
			normalizedArgs[arg.Name] = strings.TrimSpace(arg.Default)
		}
		if arg.Required && strings.TrimSpace(normalizedArgs[arg.Name]) == "" {
			return skills.Skill{}, fmt.Errorf("missing required skill arg: %s", arg.Name)
		}
	}
	if len(normalizedArgs) == 0 {
		normalizedArgs = nil
	}

	sess.ActiveSkill = &session.ActiveSkill{
		Name:        skill.Name,
		Args:        normalizedArgs,
		ActivatedAt: time.Now().UTC(),
	}
	if r.store != nil {
		if err := r.store.Save(sess); err != nil {
			return skills.Skill{}, err
		}
	}
	return skill, nil
}

func (r *Runner) ClearActiveSkill(sess *session.Session) error {
	if sess == nil {
		return fmt.Errorf("session is required")
	}
	sess.ActiveSkill = nil
	if r.store != nil {
		return r.store.Save(sess)
	}
	return nil
}

func (r *Runner) GetActiveSkill(sess *session.Session) (skills.Skill, bool) {
	if sess == nil || sess.ActiveSkill == nil || r.skillManager == nil {
		return skills.Skill{}, false
	}
	return r.skillManager.Find(sess.ActiveSkill.Name)
}
