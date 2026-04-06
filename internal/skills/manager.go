package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var validSkillName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

type Manager struct {
	mu sync.RWMutex

	workspace  string
	builtinDir string
	userDir    string
	projectDir string

	catalog Catalog
	lookup  map[string]string
}

func NewManager(workspace string) *Manager {
	home, _ := os.UserHomeDir()
	return NewManagerWithDirs(
		workspace,
		filepath.Join(workspace, "internal", "skills"),
		filepath.Join(home, ".bytemind", "skills"),
		filepath.Join(workspace, ".bytemind", "skills"),
	)
}

func NewManagerWithDirs(workspace, builtinDir, userDir, projectDir string) *Manager {
	return &Manager{
		workspace:  workspace,
		builtinDir: builtinDir,
		userDir:    userDir,
		projectDir: projectDir,
		lookup:     map[string]string{},
	}
}

func (m *Manager) Reload() Catalog {
	m.mu.Lock()
	defer m.mu.Unlock()

	scopes := []struct {
		scope Scope
		dir   string
	}{
		{scope: ScopeBuiltin, dir: m.builtinDir},
		{scope: ScopeUser, dir: m.userDir},
		{scope: ScopeProject, dir: m.projectDir},
	}

	loaded := map[string]Skill{}
	diags := make([]Diagnostic, 0, 8)
	overrides := make([]Override, 0, 4)

	for _, item := range scopes {
		skills, skillDiags := loadSkillsFromScope(item.scope, item.dir)
		diags = append(diags, skillDiags...)
		for _, skill := range skills {
			prev, exists := loaded[skill.Name]
			if exists {
				overrides = append(overrides, Override{
					Name:       skill.Name,
					Winner:     skill.Scope,
					Loser:      prev.Scope,
					WinnerPath: skill.SourceDir,
					LoserPath:  prev.SourceDir,
				})
			}
			loaded[skill.Name] = skill
		}
	}

	names := make([]string, 0, len(loaded))
	for name := range loaded {
		names = append(names, name)
	}
	sort.Strings(names)

	skills := make([]Skill, 0, len(names))
	lookup := make(map[string]string, len(names)*4)
	for _, name := range names {
		skill := loaded[name]
		skills = append(skills, skill)
		for _, alias := range skill.Aliases {
			normalized := normalizeAlias(alias)
			if normalized == "" {
				continue
			}
			if _, exists := lookup[normalized]; !exists {
				lookup[normalized] = skill.Name
			}
		}
		normalizedName := normalizeAlias(skill.Name)
		if normalizedName != "" {
			lookup[normalizedName] = skill.Name
		}
	}

	m.lookup = lookup
	m.catalog = Catalog{
		Skills:      skills,
		Diagnostics: diags,
		Overrides:   overrides,
		LoadedAt:    time.Now().UTC(),
	}
	return cloneCatalog(m.catalog)
}

func (m *Manager) List() ([]Skill, []Diagnostic) {
	catalog := m.Reload()
	return catalog.Skills, catalog.Diagnostics
}

func (m *Manager) Find(name string) (Skill, bool) {
	m.mu.RLock()
	lookup := cloneLookup(m.lookup)
	m.mu.RUnlock()
	if len(lookup) == 0 {
		m.Reload()
		m.mu.RLock()
		lookup = cloneLookup(m.lookup)
		m.mu.RUnlock()
	}

	normalized := normalizeAlias(name)
	if normalized == "" {
		return Skill{}, false
	}
	canonical, ok := lookup[normalized]
	if !ok {
		return Skill{}, false
	}

	catalog := m.Reload()
	for _, skill := range catalog.Skills {
		if skill.Name == canonical {
			return skill, true
		}
	}
	return Skill{}, false
}

func (m *Manager) Workspace() string {
	return m.workspace
}

type skillManifest struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	DescriptionZH string `json:"description_zh"`
	DescriptionCN string `json:"description_cn"`
	Scope         string `json:"scope"`
	Entry         Entry  `json:"entry"`
	Prompts       []struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	} `json:"prompts"`
	Resources []struct {
		ID       string `json:"id"`
		URI      string `json:"uri"`
		Optional bool   `json:"optional"`
	} `json:"resources"`
	Tools struct {
		Policy string   `json:"policy"`
		Items  []string `json:"items"`
	} `json:"tools"`
	Args []struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Required    bool   `json:"required"`
		Description string `json:"description"`
		Default     string `json:"default"`
	} `json:"args"`
}

func loadSkillsFromScope(scope Scope, root string) ([]Skill, []Diagnostic) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Diagnostic{{
			Scope:   scope,
			Path:    root,
			Level:   "warn",
			Message: err.Error(),
		}}
	}

	skills := make([]Skill, 0, len(entries))
	diags := make([]Diagnostic, 0, 4)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(root, entry.Name())
		skill, ok, skillDiags := loadSkillFromDir(scope, skillDir, entry.Name())
		diags = append(diags, skillDiags...)
		if ok {
			skills = append(skills, skill)
		}
	}
	return skills, diags
}

func loadSkillFromDir(scope Scope, skillDir, dirName string) (Skill, bool, []Diagnostic) {
	manifestPath := filepath.Join(skillDir, "skill.json")
	skillPath := filepath.Join(skillDir, "SKILL.md")

	hasManifest := fileExists(manifestPath)
	hasSkill := fileExists(skillPath)
	if !hasManifest && !hasSkill {
		return Skill{}, false, nil
	}

	var manifest skillManifest
	diags := make([]Diagnostic, 0, 2)
	if hasManifest {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return Skill{}, false, []Diagnostic{{
				Scope:   scope,
				Path:    manifestPath,
				Skill:   dirName,
				Level:   "error",
				Message: fmt.Sprintf("failed to read skill.json: %v", err),
			}}
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return Skill{}, false, []Diagnostic{{
				Scope:   scope,
				Path:    manifestPath,
				Skill:   dirName,
				Level:   "error",
				Message: fmt.Sprintf("invalid skill.json: %v", err),
			}}
		}
	}

	frontmatter := map[string]string{}
	body := ""
	if hasSkill {
		data, err := os.ReadFile(skillPath)
		if err != nil {
			diags = append(diags, Diagnostic{
				Scope:   scope,
				Path:    skillPath,
				Skill:   dirName,
				Level:   "error",
				Message: fmt.Sprintf("failed to read SKILL.md: %v", err),
			})
			hasSkill = false
		} else {
			frontmatter, body = parseFrontmatterMarkdown(string(data))
		}
	}

	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = strings.TrimSpace(frontmatter["name"])
	}
	if name == "" {
		name = dirName
	}
	if !validSkillName.MatchString(name) {
		diags = append(diags, Diagnostic{
			Scope:   scope,
			Path:    skillDir,
			Skill:   name,
			Level:   "error",
			Message: "invalid skill name",
		})
		return Skill{}, false, diags
	}

	description := strings.TrimSpace(manifest.Description)
	if description == "" {
		description = strings.TrimSpace(frontmatter["description"])
	}
	if description == "" {
		description = extractDescription(body)
	}
	if description == "" {
		description = "No description provided."
	}
	descriptionZH := strings.TrimSpace(manifest.DescriptionZH)
	if descriptionZH == "" {
		descriptionZH = strings.TrimSpace(manifest.DescriptionCN)
	}
	if descriptionZH == "" {
		descriptionZH = strings.TrimSpace(frontmatter["description_zh"])
	}
	if descriptionZH == "" {
		descriptionZH = strings.TrimSpace(frontmatter["description-cn"])
	}

	title := strings.TrimSpace(manifest.Title)
	if title == "" {
		title = name
	}

	whenToUse := strings.TrimSpace(frontmatter["when_to_use"])
	if whenToUse == "" {
		whenToUse = strings.TrimSpace(frontmatter["when-to-use"])
	}

	entry := manifest.Entry
	if strings.TrimSpace(entry.Slash) == "" {
		entry.Slash = "/" + name
	} else if !strings.HasPrefix(entry.Slash, "/") {
		entry.Slash = "/" + strings.TrimSpace(entry.Slash)
	}

	toolPolicy, policyDiag := buildToolPolicy(manifest.Tools.Policy, manifest.Tools.Items, frontmatter)
	if policyDiag != nil {
		policyDiag.Scope = scope
		policyDiag.Path = skillDir
		policyDiag.Skill = name
		diags = append(diags, *policyDiag)
	}

	prompts := make([]PromptRef, 0, len(manifest.Prompts))
	for _, prompt := range manifest.Prompts {
		prompts = append(prompts, PromptRef{
			ID:   strings.TrimSpace(prompt.ID),
			Path: strings.TrimSpace(prompt.Path),
		})
	}
	resources := make([]ResourceRef, 0, len(manifest.Resources))
	for _, resource := range manifest.Resources {
		resources = append(resources, ResourceRef{
			ID:       strings.TrimSpace(resource.ID),
			URI:      strings.TrimSpace(resource.URI),
			Optional: resource.Optional,
		})
	}
	args := make([]Arg, 0, len(manifest.Args))
	for _, arg := range manifest.Args {
		if strings.TrimSpace(arg.Name) == "" {
			continue
		}
		args = append(args, Arg{
			Name:        strings.TrimSpace(arg.Name),
			Type:        strings.TrimSpace(arg.Type),
			Required:    arg.Required,
			Description: strings.TrimSpace(arg.Description),
			Default:     strings.TrimSpace(arg.Default),
		})
	}

	aliases := uniqueStrings([]string{
		name,
		dirName,
		entry.Slash,
		strings.TrimPrefix(entry.Slash, "/"),
	})

	skill := Skill{
		Name:          name,
		Version:       strings.TrimSpace(manifest.Version),
		Title:         title,
		Description:   description,
		DescriptionZH: descriptionZH,
		WhenToUse:     whenToUse,
		Scope:         scope,
		SourceDir:     skillDir,
		Instruction:   strings.TrimSpace(body),
		Entry:         entry,
		Prompts:       prompts,
		Resources:     resources,
		ToolPolicy:    toolPolicy,
		Args:          args,
		Aliases:       aliases,
		DiscoveredAt:  time.Now().UTC(),
	}
	if !hasSkill {
		skill.Instruction = ""
	}
	return skill, true, diags
}

func buildToolPolicy(policy string, items []string, frontmatter map[string]string) (ToolPolicy, *Diagnostic) {
	policy = strings.TrimSpace(strings.ToLower(policy))
	if policy == "" {
		if allowed := parseToolList(frontmatter["allowed-tools"]); len(allowed) > 0 {
			return ToolPolicy{
				Policy: ToolPolicyAllowlist,
				Items:  allowed,
			}, nil
		}
		return ToolPolicy{Policy: ToolPolicyInherit}, nil
	}

	switch ToolPolicyMode(policy) {
	case ToolPolicyInherit, ToolPolicyAllowlist, ToolPolicyDenylist:
		return ToolPolicy{
			Policy: ToolPolicyMode(policy),
			Items:  uniqueStrings(items),
		}, nil
	default:
		return ToolPolicy{Policy: ToolPolicyInherit}, &Diagnostic{
			Level:   "warn",
			Message: "invalid tool policy, fallback to inherit",
		}
	}
}

func parseToolList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	parts := strings.Split(raw, ",")
	list := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		item = trimOuterQuotes(item)
		if item != "" {
			list = append(list, item)
		}
	}
	return uniqueStrings(list)
}

func extractDescription(body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	candidate := make([]string, 0, 3)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(candidate) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		candidate = append(candidate, trimmed)
		if len(candidate) >= 2 {
			break
		}
	}
	if len(candidate) == 0 {
		return ""
	}
	desc := strings.TrimSpace(strings.Join(candidate, " "))
	runes := []rune(desc)
	if len(runes) > 220 {
		return strings.TrimSpace(string(runes[:217])) + "..."
	}
	return desc
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeAlias(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "/")
	return raw
}

func cloneLookup(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneCatalog(in Catalog) Catalog {
	skills := make([]Skill, len(in.Skills))
	copy(skills, in.Skills)
	diags := make([]Diagnostic, len(in.Diagnostics))
	copy(diags, in.Diagnostics)
	overrides := make([]Override, len(in.Overrides))
	copy(overrides, in.Overrides)
	return Catalog{
		Skills:      skills,
		Diagnostics: diags,
		Overrides:   overrides,
		LoadedAt:    in.LoadedAt,
	}
}
