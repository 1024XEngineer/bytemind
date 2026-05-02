package runtime

import (
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/session"
)

type ActiveSkill struct {
	Name  string
	Scope string
	Args  map[string]string
}

type SkillDescriptor struct {
	Name        string
	Scope       string
	Description string
	Slash       string
	Aliases     []string
	ToolPolicy  string
}

type SkillDiagnostic struct {
	Level   string
	Skill   string
	Path    string
	Message string
}

type SkillCatalog struct {
	Active      *ActiveSkill
	Items       []SkillDescriptor
	Diagnostics []SkillDiagnostic
}

type SkillActivation struct {
	Name       string
	Scope      string
	EntrySlash string
	ToolPolicy string
	Args       map[string]string
}

type SkillClearResult struct {
	PreviousName string
	HadActive    bool
}

type SkillDeleteResult struct {
	Name          string
	Dir           string
	ClearedActive bool
}

func (s *Service) LoadSkillCatalog(sess *session.Session) (SkillCatalog, error) {
	if s == nil || s.runner == nil {
		return SkillCatalog{}, nil
	}

	skillsList, diagnostics := s.runner.ListSkills()
	catalog := SkillCatalog{
		Items:       make([]SkillDescriptor, 0, len(skillsList)),
		Diagnostics: make([]SkillDiagnostic, 0, len(diagnostics)),
	}

	seen := make(map[string]struct{}, len(skillsList))
	for _, skill := range skillsList {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		aliases := make([]string, 0, len(skill.Aliases))
		for _, alias := range skill.Aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				aliases = append(aliases, alias)
			}
		}
		sort.Strings(aliases)

		catalog.Items = append(catalog.Items, SkillDescriptor{
			Name:        name,
			Scope:       strings.TrimSpace(string(skill.Scope)),
			Description: strings.TrimSpace(skill.Description),
			Slash:       strings.TrimSpace(skill.Entry.Slash),
			Aliases:     aliases,
			ToolPolicy:  strings.TrimSpace(string(skill.ToolPolicy.Policy)),
		})
	}
	sort.Slice(catalog.Items, func(i, j int) bool {
		return catalog.Items[i].Name < catalog.Items[j].Name
	})

	for _, diag := range diagnostics {
		catalog.Diagnostics = append(catalog.Diagnostics, SkillDiagnostic{
			Level:   strings.TrimSpace(diag.Level),
			Skill:   strings.TrimSpace(diag.Skill),
			Path:    strings.TrimSpace(diag.Path),
			Message: strings.TrimSpace(diag.Message),
		})
	}

	if active, hasActive := s.runner.GetActiveSkill(sess); hasActive {
		catalog.Active = &ActiveSkill{
			Name:  strings.TrimSpace(active.Name),
			Scope: strings.TrimSpace(string(active.Scope)),
		}
	}
	if sess != nil && sess.ActiveSkill != nil {
		if catalog.Active == nil {
			catalog.Active = &ActiveSkill{Name: strings.TrimSpace(sess.ActiveSkill.Name)}
		}
		catalog.Active.Args = cloneStringMap(sess.ActiveSkill.Args)
	}
	return catalog, nil
}

func (s *Service) ActivateSkill(sess *session.Session, name string, args map[string]string) (SkillActivation, error) {
	if err := s.requireRunner(); err != nil {
		return SkillActivation{}, err
	}
	skill, err := s.runner.ActivateSkill(sess, name, args)
	if err != nil {
		return SkillActivation{}, err
	}
	return SkillActivation{
		Name:       strings.TrimSpace(skill.Name),
		Scope:      strings.TrimSpace(string(skill.Scope)),
		EntrySlash: strings.TrimSpace(skill.Entry.Slash),
		ToolPolicy: strings.TrimSpace(string(skill.ToolPolicy.Policy)),
		Args:       cloneStringMap(args),
	}, nil
}

func (s *Service) ClearActiveSkill(sess *session.Session) (SkillClearResult, error) {
	if err := s.requireRunner(); err != nil {
		return SkillClearResult{}, err
	}

	result := SkillClearResult{}
	if sess != nil && sess.ActiveSkill != nil {
		result.PreviousName = strings.TrimSpace(sess.ActiveSkill.Name)
		result.HadActive = result.PreviousName != ""
	}
	if err := s.runner.ClearActiveSkill(sess); err != nil {
		return SkillClearResult{}, err
	}
	return result, nil
}

func (s *Service) DeleteSkill(sess *session.Session, name string) (SkillDeleteResult, error) {
	if err := s.requireRunner(); err != nil {
		return SkillDeleteResult{}, err
	}

	result, err := s.runner.ClearSkill(name)
	if err != nil {
		return SkillDeleteResult{}, err
	}

	deleted := SkillDeleteResult{
		Name: strings.TrimSpace(result.Name),
		Dir:  strings.TrimSpace(result.Dir),
	}
	if sess != nil && sess.ActiveSkill != nil && strings.EqualFold(strings.TrimSpace(sess.ActiveSkill.Name), deleted.Name) {
		if err := s.runner.ClearActiveSkill(sess); err != nil {
			return SkillDeleteResult{}, err
		}
		deleted.ClearedActive = true
	}
	return deleted, nil
}
