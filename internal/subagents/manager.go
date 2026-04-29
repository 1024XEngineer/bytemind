package subagents

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	configpkg "bytemind/internal/config"
)

var validAgentName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

type Manager struct {
	mu sync.RWMutex

	workspace  string
	builtinDir string
	userDir    string
	projectDir string

	catalog       Catalog
	lookup        map[string]string
	builtinLookup map[string]string
	builtinByName map[string]Agent
}

func NewManager(workspace string) *Manager {
	home, _ := configpkg.ResolveHomeDir()
	return NewManagerWithDirs(
		workspace,
		filepath.Join(workspace, "internal", "subagents"),
		filepath.Join(home, "subagents"),
		filepath.Join(workspace, ".bytemind", "subagents"),
	)
}

func NewManagerWithDirs(workspace, builtinDir, userDir, projectDir string) *Manager {
	return &Manager{
		workspace:     workspace,
		builtinDir:    builtinDir,
		userDir:       userDir,
		projectDir:    projectDir,
		lookup:        map[string]string{},
		builtinLookup: map[string]string{},
		builtinByName: map[string]Agent{},
	}
}

func (m *Manager) Workspace() string {
	return m.workspace
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

	loaded := map[string]Agent{}
	builtinByName := map[string]Agent{}
	diags := make([]Diagnostic, 0, 8)
	overrides := make([]Override, 0, 4)

	for _, item := range scopes {
		agents, agentDiags := loadAgentsFromScope(item.scope, item.dir)
		diags = append(diags, agentDiags...)
		for _, agent := range agents {
			if agent.Scope == ScopeBuiltin {
				builtinByName[agent.Name] = agent
			}
			prev, exists := loaded[agent.Name]
			if exists {
				overrides = append(overrides, Override{
					Name:       agent.Name,
					Winner:     agent.Scope,
					Loser:      prev.Scope,
					WinnerPath: agent.SourcePath,
					LoserPath:  prev.SourcePath,
				})
			}
			loaded[agent.Name] = agent
		}
	}

	names := make([]string, 0, len(loaded))
	for name := range loaded {
		names = append(names, name)
	}
	sort.Strings(names)

	agents := make([]Agent, 0, len(names))
	lookup := make(map[string]string, len(names)*4)
	for _, name := range names {
		agent := loaded[name]
		agents = append(agents, agent)
		for _, alias := range agent.Aliases {
			normalized := normalizeAlias(alias)
			if normalized == "" {
				continue
			}
			if _, exists := lookup[normalized]; !exists {
				lookup[normalized] = agent.Name
			}
		}
		normalizedName := normalizeAlias(agent.Name)
		if normalizedName != "" {
			lookup[normalizedName] = agent.Name
		}
	}

	builtinLookup := make(map[string]string, len(builtinByName)*4)
	builtinNames := make([]string, 0, len(builtinByName))
	for name := range builtinByName {
		builtinNames = append(builtinNames, name)
	}
	sort.Strings(builtinNames)
	for _, name := range builtinNames {
		agent := builtinByName[name]
		for _, alias := range agent.Aliases {
			normalized := normalizeAlias(alias)
			if normalized == "" {
				continue
			}
			if _, exists := builtinLookup[normalized]; !exists {
				builtinLookup[normalized] = agent.Name
			}
		}
		normalizedName := normalizeAlias(agent.Name)
		if normalizedName != "" {
			builtinLookup[normalizedName] = agent.Name
		}
	}

	m.lookup = lookup
	m.builtinLookup = builtinLookup
	m.builtinByName = builtinByName
	m.catalog = Catalog{
		Agents:      agents,
		Diagnostics: diags,
		Overrides:   overrides,
		LoadedAt:    time.Now().UTC(),
	}
	return cloneCatalog(m.catalog)
}

func (m *Manager) Snapshot() Catalog {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneCatalog(m.catalog)
}

func (m *Manager) List() ([]Agent, []Diagnostic) {
	catalog := m.Reload()
	return catalog.Agents, catalog.Diagnostics
}

func (m *Manager) Find(name string) (Agent, bool) {
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
		return Agent{}, false
	}
	canonical, ok := lookup[normalized]
	if !ok {
		return Agent{}, false
	}

	catalog := m.Reload()
	for _, agent := range catalog.Agents {
		if agent.Name == canonical {
			return agent, true
		}
	}
	return Agent{}, false
}

func (m *Manager) FindBuiltin(name string) (Agent, bool) {
	m.mu.RLock()
	lookup := cloneLookup(m.builtinLookup)
	builtins := cloneAgentMap(m.builtinByName)
	m.mu.RUnlock()
	if len(lookup) == 0 || len(builtins) == 0 {
		m.Reload()
		m.mu.RLock()
		lookup = cloneLookup(m.builtinLookup)
		builtins = cloneAgentMap(m.builtinByName)
		m.mu.RUnlock()
	}

	normalized := normalizeAlias(name)
	if normalized == "" {
		return Agent{}, false
	}
	canonical, ok := lookup[normalized]
	if !ok {
		return Agent{}, false
	}
	agent, exists := builtins[canonical]
	if !exists {
		return Agent{}, false
	}
	return agent, true
}

func loadAgentsFromScope(scope Scope, root string) ([]Agent, []Diagnostic) {
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

	agents := make([]Agent, 0, len(entries))
	diags := make([]Diagnostic, 0, 4)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		agent, ok, agentDiags := loadAgentFromFile(scope, path)
		diags = append(diags, agentDiags...)
		if ok {
			agents = append(agents, agent)
		}
	}
	return agents, diags
}

func loadAgentFromFile(scope Scope, path string) (Agent, bool, []Diagnostic) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Agent{}, false, []Diagnostic{{
			Scope:   scope,
			Path:    path,
			Level:   "error",
			Message: fmt.Sprintf("failed to read subagent file: %v", err),
		}}
	}

	frontmatter, body := parseFrontmatterMarkdown(string(data))
	fileName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name := strings.TrimSpace(frontmatter["name"])
	if name == "" {
		name = fileName
	}
	if !validAgentName.MatchString(name) {
		return Agent{}, false, []Diagnostic{{
			Scope:   scope,
			Path:    path,
			Agent:   name,
			Level:   "error",
			Message: "invalid subagent name",
		}}
	}

	description := strings.TrimSpace(frontmatter["description"])
	if description == "" {
		description = extractDescription(body)
	}
	if description == "" {
		description = "No description provided."
	}

	entry := strings.TrimSpace(frontmatter["slash"])
	if entry == "" {
		entry = strings.TrimSpace(frontmatter["entry"])
	}
	if entry == "" {
		entry = "/" + name
	}
	if !strings.HasPrefix(entry, "/") {
		entry = "/" + entry
	}

	maxTurns := 0
	diags := make([]Diagnostic, 0, 1)
	if raw := strings.TrimSpace(frontmatter["max_turns"]); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 0 {
			diags = append(diags, Diagnostic{
				Scope:   scope,
				Path:    path,
				Agent:   name,
				Level:   "warn",
				Message: "invalid max_turns, fallback to default",
			})
		} else {
			maxTurns = parsed
		}
	}

	aliases := uniqueStrings(append(
		parseList(frontmatter["aliases"]),
		name,
		fileName,
		entry,
		strings.TrimPrefix(entry, "/"),
	))

	agent := Agent{
		Name:            name,
		Description:     description,
		Scope:           scope,
		SourcePath:      path,
		Entry:           entry,
		Instruction:     strings.TrimSpace(body),
		Tools:           parseList(frontmatter["tools"]),
		DisallowedTools: parseList(preferField(frontmatter, "disallowed_tools", "disallowed-tools")),
		Model:           strings.TrimSpace(frontmatter["model"]),
		Mode:            strings.TrimSpace(frontmatter["mode"]),
		MaxTurns:        maxTurns,
		Timeout:         strings.TrimSpace(frontmatter["timeout"]),
		Output:          strings.TrimSpace(frontmatter["output"]),
		Isolation:       strings.TrimSpace(frontmatter["isolation"]),
		Aliases:         aliases,
		DiscoveredAt:    time.Now().UTC(),
	}
	return agent, true, diags
}

func parseList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	items := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		token = strings.TrimPrefix(token, "-")
		token = strings.TrimPrefix(token, "*")
		token = strings.TrimSpace(token)
		token = trimOuterQuotes(token)
		if token == "" {
			continue
		}
		items = append(items, token)
	}
	return uniqueStrings(items)
}

func preferField(fields map[string]string, primary string, fallback string) string {
	if value := strings.TrimSpace(fields[primary]); value != "" {
		return value
	}
	return strings.TrimSpace(fields[fallback])
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

func normalizeAlias(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "/")
	return raw
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

func cloneLookup(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAgentMap(in map[string]Agent) map[string]Agent {
	out := make(map[string]Agent, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneCatalog(in Catalog) Catalog {
	agents := make([]Agent, len(in.Agents))
	copy(agents, in.Agents)
	diags := make([]Diagnostic, len(in.Diagnostics))
	copy(diags, in.Diagnostics)
	overrides := make([]Override, len(in.Overrides))
	copy(overrides, in.Overrides)
	return Catalog{
		Agents:      agents,
		Diagnostics: diags,
		Overrides:   overrides,
		LoadedAt:    in.LoadedAt,
	}
}
