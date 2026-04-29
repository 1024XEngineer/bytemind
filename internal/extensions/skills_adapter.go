package extensions

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	skillspkg "github.com/1024XEngineer/bytemind/internal/skills"
)

type skillAdapter struct {
	mu    sync.RWMutex
	cache map[string]cachedSkillExtension
}

type cachedSkillExtension struct {
	item      ExtensionInfo
	ref       string
	updatedAt time.Time
}

func newSkillAdapter() *skillAdapter {
	return &skillAdapter{cache: map[string]cachedSkillExtension{}}
}

func (a *skillAdapter) Sync(catalog skillspkg.Catalog) []ExtensionInfo {
	if a == nil {
		return nil
	}

	a.mu.RLock()
	current := make(map[string]cachedSkillExtension, len(a.cache))
	for id, entry := range a.cache {
		current[id] = entry
	}
	a.mu.RUnlock()

	nextCache := make(map[string]cachedSkillExtension, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		item := a.FromSkill(skill)
		entry, ok := current[item.ID]
		if ok && entry.ref == item.Source.Ref && entry.updatedAt.Equal(skill.DiscoveredAt) {
			nextCache[item.ID] = entry
			continue
		}
		nextCache[item.ID] = cachedSkillExtension{
			item:      item,
			ref:       item.Source.Ref,
			updatedAt: skill.DiscoveredAt,
		}
	}

	items := make([]ExtensionInfo, 0, len(nextCache))
	for _, entry := range nextCache {
		items = append(items, cloneExtensionInfo(entry.item))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	a.mu.Lock()
	a.cache = nextCache
	a.mu.Unlock()

	return items
}

func (a *skillAdapter) FromSkill(skill skillspkg.Skill) ExtensionInfo {
	normalized := NormalizeLegacySkill(skill)
	status := extensionStatusForPath(normalized.SourceDir)
	message := "extension loaded"
	if status == ExtensionStatusDegraded {
		message = "manifest discovered without SKILL.md"
	}
	checkedAt := normalized.DiscoveredAt.UTC().Format(time.RFC3339)
	if normalized.DiscoveredAt.IsZero() {
		checkedAt = time.Now().UTC().Format(time.RFC3339)
	}
	capabilities := CapabilitySet{
		Prompts:   len(normalized.Prompts),
		Resources: len(normalized.Resources),
		Tools:     len(normalized.ToolPolicy.Items),
		Commands:  len(normalized.Args),
	}
	manifestRef := ""
	if normalized.SourceDir != "" {
		manifestRef = filepath.Join(normalized.SourceDir, "skill.json")
	}
	return ExtensionInfo{
		ID:           SkillExtensionID(normalized.Name),
		Name:         normalized.Name,
		Kind:         ExtensionSkill,
		Version:      strings.TrimSpace(normalized.Version),
		Title:        strings.TrimSpace(normalized.Title),
		Description:  strings.TrimSpace(normalized.Description),
		Source:       ExtensionSource{Scope: extensionScope(normalized.Scope), Ref: strings.TrimSpace(normalized.SourceDir)},
		Status:       status,
		Capabilities: capabilities,
		Manifest: Manifest{
			Name:         normalized.Name,
			Version:      strings.TrimSpace(normalized.Version),
			Title:        strings.TrimSpace(normalized.Title),
			Description:  strings.TrimSpace(normalized.Description),
			Kind:         ExtensionSkill,
			Source:       ExtensionSource{Scope: extensionScope(normalized.Scope), Ref: manifestRef},
			Capabilities: capabilities,
		},
		Health: HealthSnapshot{
			Status:       status,
			Message:      message,
			CheckedAtUTC: checkedAt,
		},
	}
}

func NormalizeLegacySkill(skill skillspkg.Skill) skillspkg.Skill {
	normalized := skill
	normalized.Name = strings.TrimSpace(normalized.Name)
	if normalized.Name == "" {
		normalized.Name = strings.TrimSpace(normalized.Title)
	}
	if normalized.Title == "" {
		normalized.Title = normalized.Name
	}
	if normalized.DiscoveredAt.IsZero() {
		normalized.DiscoveredAt = time.Now().UTC()
	}
	return normalized
}

func extensionScope(scope skillspkg.Scope) ExtensionScope {
	switch scope {
	case skillspkg.ScopeBuiltin:
		return ExtensionScopeBuiltin
	case skillspkg.ScopeUser:
		return ExtensionScopeUser
	case skillspkg.ScopeProject:
		return ExtensionScopeProject
	default:
		return ExtensionScopeRemote
	}
}

func extensionStatusForPath(dir string) ExtensionStatus {
	if strings.TrimSpace(dir) == "" {
		return ExtensionStatusDegraded
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		return ExtensionStatusDegraded
	}
	return ExtensionStatusLoaded
}
