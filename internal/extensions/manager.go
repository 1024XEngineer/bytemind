package extensions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	skillspkg "bytemind/internal/skills"
)

type extensionManager struct {
	mu sync.RWMutex

	workspace  string
	builtinDir string
	userDir    string
	projectDir string

	skills  *skillspkg.Manager
	adapter *skillAdapter

	state        *stateStore
	manual       map[string]struct{}
	disabled     map[string]struct{}
	discoverErrs map[string]error
}

func NewManager(workspace string) Manager {
	userDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		userDir = filepath.Join(home, ".bytemind", "skills")
	}
	return NewManagerWithDirs(
		workspace,
		filepath.Join(workspace, "internal", "skills"),
		userDir,
		filepath.Join(workspace, ".bytemind", "skills"),
	)
}

func NewManagerWithDirs(workspace, builtinDir, userDir, projectDir string) Manager {
	return &extensionManager{
		workspace:    workspace,
		builtinDir:   builtinDir,
		userDir:      userDir,
		projectDir:   projectDir,
		skills:       skillspkg.NewManagerWithDirs(workspace, builtinDir, userDir, projectDir),
		adapter:      newSkillAdapter(),
		state:        newStateStore(),
		manual:       map[string]struct{}{},
		disabled:     map[string]struct{}{},
		discoverErrs: map[string]error{},
	}
}

func (m *extensionManager) Load(_ context.Context, source string) (ExtensionInfo, error) {
	loaded, err := m.discoverOne(source)
	if err != nil {
		return ExtensionInfo{}, err
	}
	_ = m.reload()
	if current, ok := m.state.get(loaded.ID); ok {
		m.mu.RLock()
		_, manual := m.manual[loaded.ID]
		m.mu.RUnlock()
		if manual {
			return ExtensionInfo{}, wrapError(ErrCodeAlreadyLoaded, "extension already loaded", nil)
		}
		if sameExtensionSource(current.Source.Ref, loaded.Source.Ref) {
			return current, nil
		}
	}
	if err := m.state.withLock(loaded.ID, func() error {
		if err := m.state.beginLoad(loaded.ID); err != nil {
			return err
		}
		loaded.Status = ExtensionStatusLoaded
		loaded.Health.Status = ExtensionStatusLoaded
		loaded.Health.Message = "extension loaded"
		loaded.Health.LastError = ""
		loaded.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
		loadEvent := ExtensionEvent{
			Type:        "load",
			ExtensionID: loaded.ID,
			Kind:        loaded.Kind,
			Status:      loaded.Status,
			Reason:      "extension loaded",
			OccurredAt:  loaded.Health.CheckedAtUTC,
			Message:     "extension loaded",
		}
		active, activateEvent, err := activateTransition(loaded)
		if err != nil {
			m.state.cancelLoad(loaded.ID)
			return err
		}
		m.state.finishLoad(loaded.ID, active, loadEvent, activateEvent)
		m.mu.Lock()
		m.manual[loaded.ID] = struct{}{}
		delete(m.disabled, loaded.ID)
		delete(m.discoverErrs, loaded.ID)
		m.mu.Unlock()
		loaded = active
		return nil
	}); err != nil {
		return ExtensionInfo{}, err
	}
	return loaded, nil
}

func (m *extensionManager) Unload(_ context.Context, extensionID string) error {
	id := strings.TrimSpace(extensionID)
	if id == "" {
		return wrapError(ErrCodeInvalidExtension, "extension id is required", nil)
	}
	reloadErr := m.reload()
	return m.state.withLock(id, func() error {
		item, ok := m.state.get(id)
		if !ok {
			m.mu.Lock()
			defer m.mu.Unlock()
			if _, ok := m.discoverErrs[id]; ok {
				m.disabled[id] = struct{}{}
				delete(m.discoverErrs, id)
				return nil
			}
			return wrapError(ErrCodeNotFound, "extension not found", nil)
		}
		_, event, err := stopTransition(item, "extension unloaded")
		if err != nil {
			return err
		}
		m.state.delete(id, event)
		m.mu.Lock()
		delete(m.manual, id)
		m.disabled[id] = struct{}{}
		delete(m.discoverErrs, id)
		m.mu.Unlock()
		_ = reloadErr
		return nil
	})
}

func (m *extensionManager) Get(_ context.Context, extensionID string) (ExtensionInfo, error) {
	id := strings.TrimSpace(extensionID)
	if id == "" {
		return ExtensionInfo{}, wrapError(ErrCodeInvalidExtension, "extension id is required", nil)
	}
	if err := m.reload(); err != nil {
		m.mu.RLock()
		defer m.mu.RUnlock()
		if item, ok := m.state.get(id); ok {
			return item, err
		}
		if discoverErr, ok := m.discoverErrs[id]; ok {
			return ExtensionInfo{}, discoverErr
		}
		return ExtensionInfo{}, wrapError(ErrCodeNotFound, "extension not found", nil)
	}
	item, ok := m.state.get(id)
	if !ok {
		return ExtensionInfo{}, wrapError(ErrCodeNotFound, "extension not found", nil)
	}
	return item, nil
}

func (m *extensionManager) List(_ context.Context) ([]ExtensionInfo, error) {
	err := m.reload()
	items := m.state.list()
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items, err
}

func (m *extensionManager) reload() error {
	loaded := map[string]ExtensionInfo{}
	discoverErrs := map[string]error{}
	catalog := m.skills.Reload()
	for _, entry := range m.adapter.Sync(catalog) {
		if strings.TrimSpace(entry.Source.Ref) != "" {
			entry.Status = extensionStatusForPath(entry.Source.Ref)
			entry.Health.Status = entry.Status
			if entry.Status == ExtensionStatusDegraded {
				entry.Health.Message = "manifest discovered without SKILL.md"
			} else {
				entry.Health.Message = "extension loaded"
			}
			entry.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
			entry.Manifest.Source.Ref = filepath.Join(entry.Source.Ref, "skill.json")
		}
		loaded[entry.ID] = entry
	}
	for _, diag := range catalog.Diagnostics {
		id := extensionIDForDir(diag.Skill)
		if id == "" {
			id = extensionIDForDir(filepath.Base(diag.Path))
		}
		if id == "" {
			id = strings.TrimSpace(diag.Path)
		}
		if id == "" {
			continue
		}
		discoverErrs[id] = wrapError(ErrCodeLoadFailed, diag.Message, nil)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.disabled {
		delete(loaded, id)
		delete(discoverErrs, id)
	}
	for id := range m.manual {
		if _, ok := loaded[id]; ok {
			continue
		}
		if item, ok := m.state.get(id); ok {
			loaded[id] = item
		}
	}
	for id, item := range loaded {
		current, ok := m.state.get(id)
		if ok {
			item = mergeDiscoveredState(current, item)
		} else {
			item = prepareLoadedInfo(item)
		}
		m.state.set(item)
	}
	for _, item := range m.state.list() {
		if _, ok := loaded[item.ID]; !ok {
			m.state.delete(item.ID)
		}
	}
	m.discoverErrs = discoverErrs
	return discoveryError(discoverErrs)
}

func prepareLoadedInfo(info ExtensionInfo) ExtensionInfo {
	prepared := cloneExtensionInfo(info)
	prepared.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
	if prepared.Status == ExtensionStatusDegraded {
		prepared.Health.Status = ExtensionStatusDegraded
		if strings.TrimSpace(prepared.Health.Message) == "" {
			prepared.Health.Message = "extension degraded"
		}
		return prepared
	}
	prepared.Status = ExtensionStatusLoaded
	prepared.Health.Status = ExtensionStatusLoaded
	prepared.Health.Message = "extension loaded"
	prepared.Health.LastError = ""
	active, _, err := activateTransition(prepared)
	if err != nil {
		return prepared
	}
	return active
}

func mergeDiscoveredState(current, discovered ExtensionInfo) ExtensionInfo {
	merged := cloneExtensionInfo(discovered)
	merged.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
	if discovered.Status == ExtensionStatusDegraded {
		merged.Status = ExtensionStatusDegraded
		merged.Health = discovered.Health
		merged.Health.Status = ExtensionStatusDegraded
		merged.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
		return merged
	}
	if current.Status == ExtensionStatusDegraded {
		recovered, _, err := recoverTransition(ExtensionInfo{
			ID:          current.ID,
			Name:        current.Name,
			Kind:        current.Kind,
			Version:     current.Version,
			Title:       current.Title,
			Description: current.Description,
			Source:      current.Source,
			Status:      current.Status,
			Manifest:    current.Manifest,
			Health:      current.Health,
		}, "extension recovered")
		if err == nil {
			merged.Status = recovered.Status
			merged.Health = recovered.Health
			merged.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
			return merged
		}
	}
	merged.Status = current.Status
	merged.Health = current.Health
	merged.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
	return merged
}

func (m *extensionManager) discoverOne(source string) (ExtensionInfo, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return ExtensionInfo{}, wrapError(ErrCodeInvalidSource, "extension source is required", nil)
	}
	resolved := source
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.workspace, resolved)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return ExtensionInfo{}, wrapError(ErrCodeInvalidSource, "extension source not found", err)
	}
	if !info.IsDir() {
		return ExtensionInfo{}, wrapError(ErrCodeInvalidSource, "extension source must be a directory", nil)
	}
	scope, ok := scopeForPath(resolved, m)
	if !ok {
		scope = ExtensionScopeRemote
	}
	matched, ok, diags := skillspkg.LoadFromDir(skillsScopeForExtension(scope), resolved)
	if !ok {
		if len(diags) > 0 {
			return ExtensionInfo{}, wrapError(ErrCodeInvalidManifest, diags[0].Message, nil)
		}
		return ExtensionInfo{}, wrapError(ErrCodeInvalidSource, "extension source does not contain a supported extension", nil)
	}
	item := m.adapter.FromSkill(matched)
	item.Source.Scope = scope
	item.Source.Ref = resolved
	item.Manifest.Source.Scope = scope
	item.Manifest.Source.Ref = filepath.Join(resolved, "skill.json")
	if !item.Valid() {
		return ExtensionInfo{}, wrapError(ErrCodeInvalidExtension, "extension info is invalid", nil)
	}
	return item, nil
}

func discoveryError(discoverErrs map[string]error) error {
	if len(discoverErrs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(discoverErrs))
	for id := range discoverErrs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		keys = append(keys, id)
	}
	if len(keys) == 0 {
		return wrapError(ErrCodeLoadFailed, "extension discovery encountered errors", nil)
	}
	sort.Strings(keys)
	first := keys[0]
	if err := discoverErrs[first]; err != nil {
		return wrapError(ErrCodeLoadFailed, fmt.Sprintf("extension discovery encountered errors (first failure: %s)", first), err)
	}
	return wrapError(ErrCodeLoadFailed, fmt.Sprintf("extension discovery encountered errors (first failure: %s)", first), nil)
}

func scopeForPath(path string, m *extensionManager) (ExtensionScope, bool) {
	clean := filepath.Clean(path)
	for _, item := range []struct {
		scope ExtensionScope
		dir   string
	}{
		{scope: ExtensionScopeProject, dir: m.projectDir},
		{scope: ExtensionScopeUser, dir: m.userDir},
		{scope: ExtensionScopeBuiltin, dir: m.builtinDir},
	} {
		if item.dir == "" {
			continue
		}
		root := filepath.Clean(item.dir)
		if clean == root || strings.HasPrefix(clean, root+string(os.PathSeparator)) {
			return item.scope, true
		}
	}
	return "", false
}

func skillsScopeForExtension(scope ExtensionScope) skillspkg.Scope {
	switch scope {
	case ExtensionScopeBuiltin:
		return skillspkg.ScopeBuiltin
	case ExtensionScopeUser:
		return skillspkg.ScopeUser
	default:
		return skillspkg.ScopeProject
	}
}

func extensionIDForDir(dirName string) string {
	name := strings.TrimSpace(dirName)
	if name == "" {
		return ""
	}
	return "skill." + name
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func sameExtensionSource(left, right string) bool {
	return filepath.Clean(strings.TrimSpace(left)) == filepath.Clean(strings.TrimSpace(right))
}
