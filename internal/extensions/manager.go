package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type extensionManager struct {
	mu sync.RWMutex

	workspace  string
	builtinDir string
	userDir    string
	projectDir string

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
	if err := m.reload(); err != nil {
		if current, ok := m.state.get(loaded.ID); ok {
			return current, wrapError(ErrCodeAlreadyLoaded, "extension already loaded", nil)
		}
	}
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
		if _, ok := m.discoverErrs[id]; ok {
			return nil
		}
		if reloadErr != nil {
			return nil
		}
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
	type scopeDir struct {
		scope ExtensionScope
		dir   string
	}
	loaded := map[string]ExtensionInfo{}
	discoverErrs := map[string]error{}
	for _, item := range []scopeDir{{ExtensionScopeBuiltin, m.builtinDir}, {ExtensionScopeUser, m.userDir}, {ExtensionScopeProject, m.projectDir}} {
		entries, errs, err := discoverScope(item.scope, item.dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			loaded[entry.ID] = entry
		}
		for id, discoverErr := range errs {
			discoverErrs[id] = discoverErr
		}
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
			item.Status = current.Status
			item.Health = current.Health
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
	loaded := cloneExtensionInfo(info)
	loaded.Status = ExtensionStatusLoaded
	loaded.Health.Status = ExtensionStatusLoaded
	loaded.Health.Message = "extension loaded"
	loaded.Health.LastError = ""
	loaded.Health.CheckedAtUTC = time.Now().UTC().Format(time.RFC3339)
	active, _, err := activateTransition(loaded)
	if err != nil {
		return loaded
	}
	return active
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
	item, ok, err := discoverExtension(scope, resolved, filepath.Base(resolved))
	if err != nil {
		return ExtensionInfo{}, err
	}
	if !ok {
		return ExtensionInfo{}, wrapError(ErrCodeInvalidSource, "extension source does not contain a supported extension", nil)
	}
	return item, nil
}

func discoverScope(scope ExtensionScope, root string) ([]ExtensionInfo, map[string]error, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		id := extensionIDForDir(filepath.Base(root))
		if id == "" {
			id = root
		}
		return nil, map[string]error{id: wrapError(ErrCodeLoadFailed, fmt.Sprintf("discover extensions from %s", root), err)}, nil
	}
	items := make([]ExtensionInfo, 0, len(entries))
	discoverErrs := make(map[string]error)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		item, ok, err := discoverExtension(scope, dir, entry.Name())
		if err != nil {
			discoverErrs[extensionIDForDir(entry.Name())] = err
			continue
		}
		if ok {
			items = append(items, item)
		}
	}
	return items, discoverErrs, nil
}

func discoverExtension(scope ExtensionScope, dir, dirName string) (ExtensionInfo, bool, error) {
	manifestPath := filepath.Join(dir, "skill.json")
	skillPath := filepath.Join(dir, "SKILL.md")
	if !fileExists(manifestPath) && !fileExists(skillPath) {
		return ExtensionInfo{}, false, nil
	}
	manifest, err := readManifest(manifestPath, dirName)
	if err != nil {
		return ExtensionInfo{}, false, err
	}
	info := buildExtensionInfo(scope, dir, dirName, manifest, fileExists(skillPath))
	if !info.Valid() {
		return ExtensionInfo{}, false, wrapError(ErrCodeInvalidExtension, "extension info is invalid", nil)
	}
	return info, true, nil
}

func readManifest(path, dirName string) (Manifest, error) {
	manifest := Manifest{}
	if !fileExists(path) {
		manifest.Name = dirName
		manifest.Title = dirName
		manifest.Kind = ExtensionSkill
		manifest.Source = ExtensionSource{Ref: path}
		return manifest, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, wrapError(ErrCodeInvalidManifest, "failed to read manifest", err)
	}
	var raw struct {
		Name        string     `json:"name"`
		Version     string     `json:"version"`
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Prompts     []struct{} `json:"prompts"`
		Resources   []struct{} `json:"resources"`
		Tools       struct {
			Items []string `json:"items"`
		} `json:"tools"`
		Args []struct{} `json:"args"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, wrapError(ErrCodeInvalidManifest, "invalid manifest", err)
	}
	manifest.Name = strings.TrimSpace(raw.Name)
	if manifest.Name == "" {
		manifest.Name = dirName
	}
	manifest.Version = strings.TrimSpace(raw.Version)
	manifest.Title = strings.TrimSpace(raw.Title)
	if manifest.Title == "" {
		manifest.Title = manifest.Name
	}
	manifest.Description = strings.TrimSpace(raw.Description)
	manifest.Kind = ExtensionSkill
	manifest.Source = ExtensionSource{Ref: path}
	manifest.Capabilities = CapabilitySet{
		Prompts:   len(raw.Prompts),
		Resources: len(raw.Resources),
		Tools:     len(raw.Tools.Items),
		Commands:  len(raw.Args),
	}
	return manifest, nil
}

func buildExtensionInfo(scope ExtensionScope, dir, dirName string, manifest Manifest, hasSkill bool) ExtensionInfo {
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = dirName
	}
	ref := dir
	status := ExtensionStatusLoaded
	message := "extension loaded"
	if !hasSkill {
		status = ExtensionStatusDegraded
		message = "manifest discovered without SKILL.md"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return ExtensionInfo{
		ID:           "skill." + name,
		Name:         name,
		Kind:         ExtensionSkill,
		Version:      strings.TrimSpace(manifest.Version),
		Title:        strings.TrimSpace(manifest.Title),
		Description:  strings.TrimSpace(manifest.Description),
		Source:       ExtensionSource{Scope: scope, Ref: ref},
		Status:       status,
		Capabilities: manifest.Capabilities,
		Manifest: Manifest{
			Name:         name,
			Version:      strings.TrimSpace(manifest.Version),
			Title:        strings.TrimSpace(manifest.Title),
			Description:  strings.TrimSpace(manifest.Description),
			Kind:         ExtensionSkill,
			Source:       ExtensionSource{Scope: scope, Ref: filepath.Join(dir, "skill.json")},
			Capabilities: manifest.Capabilities,
		},
		Health: HealthSnapshot{
			Status:       status,
			Message:      message,
			CheckedAtUTC: now,
		},
	}
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
