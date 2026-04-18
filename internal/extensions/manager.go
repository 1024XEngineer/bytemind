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

	catalog      map[string]ExtensionInfo
	manual       map[string]ExtensionInfo
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
		catalog:      map[string]ExtensionInfo{},
		manual:       map[string]ExtensionInfo{},
		disabled:     map[string]struct{}{},
		discoverErrs: map[string]error{},
	}
}

func (m *extensionManager) Load(_ context.Context, source string) (ExtensionInfo, error) {
	loaded, err := m.discoverOne(source)
	if err != nil {
		return ExtensionInfo{}, err
	}
	m.mu.Lock()
	m.catalog[loaded.ID] = loaded
	m.manual[loaded.ID] = loaded
	delete(m.disabled, loaded.ID)
	m.mu.Unlock()
	return loaded, nil
}

func (m *extensionManager) Unload(_ context.Context, extensionID string) error {
	id := strings.TrimSpace(extensionID)
	if id == "" {
		return wrapError(ErrCodeInvalidExtension, "extension id is required", nil)
	}
	if err := m.reload(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.catalog[id]; !ok {
		if _, ok := m.manual[id]; !ok {
			if _, ok := m.discoverErrs[id]; !ok {
				return wrapError(ErrCodeNotFound, "extension not found", nil)
			}
		}
	}
	delete(m.catalog, id)
	delete(m.manual, id)
	m.disabled[id] = struct{}{}
	delete(m.discoverErrs, id)
	return nil
}

func (m *extensionManager) Get(_ context.Context, extensionID string) (ExtensionInfo, error) {
	id := strings.TrimSpace(extensionID)
	if id == "" {
		return ExtensionInfo{}, wrapError(ErrCodeInvalidExtension, "extension id is required", nil)
	}
	if err := m.reload(); err != nil {
		return ExtensionInfo{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if discoverErr := m.discoveryErrorLocked(); discoverErr != nil {
		if item, ok := m.catalog[id]; ok {
			if _, disabled := m.disabled[id]; disabled {
				return ExtensionInfo{}, wrapError(ErrCodeNotFound, "extension not found", nil)
			}
			return item, discoverErr
		}
		if err, ok := m.discoverErrs[id]; ok {
			return ExtensionInfo{}, err
		}
		return ExtensionInfo{}, wrapError(ErrCodeNotFound, "extension not found", nil)
	}
	if _, disabled := m.disabled[id]; disabled {
		return ExtensionInfo{}, wrapError(ErrCodeNotFound, "extension not found", nil)
	}
	item, ok := m.catalog[id]
	if !ok {
		return ExtensionInfo{}, wrapError(ErrCodeNotFound, "extension not found", nil)
	}
	return item, nil
}

func (m *extensionManager) List(_ context.Context) ([]ExtensionInfo, error) {
	if err := m.reload(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]ExtensionInfo, 0, len(m.catalog))
	for id, item := range m.catalog {
		if _, disabled := m.disabled[id]; disabled {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	if discoverErr := m.discoveryErrorLocked(); discoverErr != nil {
		return items, discoverErr
	}
	return items, nil
}

func (m *extensionManager) reload() error {
	loaded := map[string]ExtensionInfo{}
	discoverErrs := map[string]error{}
	for _, item := range []struct {
		scope ExtensionScope
		dir   string
	}{
		{scope: ExtensionScopeBuiltin, dir: m.builtinDir},
		{scope: ExtensionScopeUser, dir: m.userDir},
		{scope: ExtensionScopeProject, dir: m.projectDir},
	} {
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
	for id, entry := range m.manual {
		loaded[id] = entry
	}
	for id := range m.disabled {
		delete(loaded, id)
		delete(discoverErrs, id)
	}
	m.catalog = loaded
	m.discoverErrs = discoverErrs
	m.mu.Unlock()
	return nil
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
	status := ExtensionStatusReady
	message := "extension discovered"
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

func (m *extensionManager) discoveryErrorLocked() error {
	return discoveryError(m.discoverErrs)
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
	if err != nil {
		return false
	}
	return !info.IsDir()
}
