package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	defaultAuthorVersion     = "0.1.0"
	defaultAuthorDescription = "Describe what this skill should do and refine the workflow below."
)

func (m *Manager) Author(name string, scope Scope, brief string) (AuthorResult, error) {
	name = strings.TrimSpace(strings.TrimPrefix(name, "/"))
	if name == "" {
		return AuthorResult{}, fmt.Errorf("skill name is required")
	}
	if !validSkillName.MatchString(name) {
		return AuthorResult{}, fmt.Errorf("invalid skill name: %s", name)
	}

	scope = normalizeAuthorScope(scope)
	root := m.scopeDir(scope)
	if strings.TrimSpace(root) == "" {
		return AuthorResult{}, fmt.Errorf("skill scope path is unavailable: %s", scope)
	}

	brief = compactAuthorText(brief)

	skillDir := filepath.Join(root, name)
	manifestPath := filepath.Join(skillDir, "skill.json")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	result := AuthorResult{
		Name:         name,
		Scope:        scope,
		Dir:          skillDir,
		ManifestPath: manifestPath,
		SkillPath:    skillPath,
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return result, err
	}

	manifestExists := fileExists(manifestPath)
	skillExists := fileExists(skillPath)
	result.Created = !manifestExists && !skillExists

	manifest := skillManifest{}
	if manifestExists {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return result, err
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return result, fmt.Errorf("invalid skill.json: %w", err)
		}
	}
	mergeManifestDefaults(&manifest, name, brief)

	manifestContent, err := marshalManifest(manifest)
	if err != nil {
		return result, err
	}
	manifestUpdated, err := writeIfChanged(manifestPath, manifestContent)
	if err != nil {
		return result, err
	}

	frontmatter := map[string]string{}
	body := ""
	if skillExists {
		data, err := os.ReadFile(skillPath)
		if err != nil {
			return result, err
		}
		frontmatter, body = parseFrontmatterMarkdown(string(data))
	}

	skillContent := renderSkillMarkdown(name, manifest.Description, brief, frontmatter, body)
	skillUpdated, err := writeIfChanged(skillPath, []byte(skillContent))
	if err != nil {
		return result, err
	}

	result.Updated = manifestUpdated || skillUpdated
	m.Reload()
	return result, nil
}

func (m *Manager) scopeDir(scope Scope) string {
	switch scope {
	case ScopeBuiltin:
		return m.builtinDir
	case ScopeUser:
		return m.userDir
	default:
		return m.projectDir
	}
}

func normalizeAuthorScope(scope Scope) Scope {
	switch scope {
	case ScopeBuiltin, ScopeUser, ScopeProject:
		return scope
	default:
		return ScopeProject
	}
}

func mergeManifestDefaults(manifest *skillManifest, name, brief string) {
	if manifest == nil {
		return
	}

	if strings.TrimSpace(manifest.Name) == "" {
		manifest.Name = name
	}
	if strings.TrimSpace(manifest.Version) == "" {
		manifest.Version = defaultAuthorVersion
	}
	if strings.TrimSpace(manifest.Title) == "" {
		manifest.Title = humanizeSkillName(name)
	}
	if strings.TrimSpace(manifest.Description) == "" {
		manifest.Description = defaultAuthorDescription
	}
	if brief != "" {
		manifest.Description = brief
	}

	manifest.Entry.Slash = strings.TrimSpace(manifest.Entry.Slash)
	if manifest.Entry.Slash == "" {
		manifest.Entry.Slash = "/" + name
	} else if !strings.HasPrefix(manifest.Entry.Slash, "/") {
		manifest.Entry.Slash = "/" + manifest.Entry.Slash
	}

	policy := strings.ToLower(strings.TrimSpace(manifest.Tools.Policy))
	switch policy {
	case "", string(ToolPolicyInherit), string(ToolPolicyAllowlist), string(ToolPolicyDenylist):
	default:
		policy = string(ToolPolicyInherit)
	}
	if policy == "" {
		policy = string(ToolPolicyInherit)
	}
	manifest.Tools.Policy = policy
	manifest.Tools.Items = uniqueStrings(manifest.Tools.Items)
	if policy == string(ToolPolicyInherit) {
		manifest.Tools.Items = nil
	}
}

func marshalManifest(manifest skillManifest) ([]byte, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func renderSkillMarkdown(name, description, brief string, frontmatter map[string]string, body string) string {
	description = compactAuthorText(description)
	if description == "" {
		description = defaultAuthorDescription
	}
	whenToUse := compactAuthorText(frontmatter["when_to_use"])
	if whenToUse == "" {
		whenToUse = compactAuthorText(frontmatter["when-to-use"])
	}
	if brief != "" {
		whenToUse = brief
	}
	if whenToUse == "" {
		whenToUse = "Use this skill when the task repeatedly follows the same workflow."
	}

	body = strings.TrimSpace(body)
	if body == "" {
		body = defaultSkillBody(name)
	}

	lines := []string{
		"---",
		fmt.Sprintf("name: %q", name),
		"description: |",
	}
	lines = append(lines, indentBlock(description)...)
	lines = append(lines,
		"when_to_use: |",
	)
	lines = append(lines, indentBlock(whenToUse)...)
	lines = append(lines,
		"---",
		"",
		body,
		"",
	)
	return strings.Join(lines, "\n")
}

func defaultSkillBody(name string) string {
	return strings.Join([]string{
		"# " + name,
		"",
		"## Goal",
		"",
		"- Clarify what this skill should achieve.",
		"",
		"## Workflow",
		"",
		"1. Confirm scope and expected output.",
		"2. Gather the minimum context needed for execution.",
		"3. Apply focused changes and validate results.",
		"4. Report outcomes, risks, and next steps.",
		"",
		"## Output Contract",
		"",
		"- Key findings or changes",
		"- Verification performed",
		"- Residual risks or follow-up",
	}, "\n")
}

func indentBlock(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, "  ")
			continue
		}
		out = append(out, "  "+trimmed)
	}
	if len(out) == 0 {
		return []string{"  "}
	}
	return out
}

func compactAuthorText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if len(runes) > 220 {
		return strings.TrimSpace(string(runes[:217])) + "..."
	}
	return text
}

func writeIfChanged(path string, content []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func humanizeSkillName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ':' || unicode.IsSpace(r)
	})
	if len(parts) == 0 {
		return name
	}
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		runes := []rune(strings.ToLower(parts[i]))
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}
