package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) Clear(name string) (ClearResult, error) {
	name = strings.TrimSpace(strings.TrimPrefix(name, "/"))
	if name == "" {
		return ClearResult{}, fmt.Errorf("skill name is required")
	}
	if !validSkillName.MatchString(name) {
		return ClearResult{}, fmt.Errorf("invalid skill name: %s", name)
	}

	result := ClearResult{
		Name:  name,
		Scope: ScopeProject,
	}

	// Prefer canonical resolution so aliases map to the actual project skill.
	if skill, ok := m.Find(name); ok {
		if skill.Scope != ScopeProject {
			return result, fmt.Errorf("skill `%s` exists in %s scope and cannot be deleted by /skill delete", skill.Name, skill.Scope)
		}
		result.Name = skill.Name
		result.Dir = strings.TrimSpace(skill.SourceDir)
		if result.Dir == "" {
			result.Dir = filepath.Join(m.projectDir, skill.Name)
		}
	} else {
		result.Dir = filepath.Join(m.projectDir, name)
	}

	dir := strings.TrimSpace(result.Dir)
	if dir == "" {
		return result, fmt.Errorf("project skill directory is unavailable")
	}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			result.NotFound = true
			return result, fmt.Errorf("project skill `%s` not found", result.Name)
		}
		return result, err
	}
	if !info.IsDir() {
		return result, fmt.Errorf("skill path is not a directory: %s", dir)
	}

	if err := os.RemoveAll(dir); err != nil {
		return result, err
	}
	result.Removed = true
	m.Reload()
	return result, nil
}
