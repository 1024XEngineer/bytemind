package extensions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerLoadDiscoversExtensionFromSource(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, ".bytemind", "skills")
	if err := os.MkdirAll(filepath.Join(project, "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "review", "skill.json"), []byte(`{"name":"review","description":"Review code changes","prompts":[{"id":"p1","path":"prompt.md"}],"tools":{"items":["read_file","search_text"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "review", "SKILL.md"), []byte("# /review"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(root)
	item, err := mgr.Load(context.Background(), filepath.Join(root, ".bytemind", "skills", "review"))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if item.ID != "skill.review" {
		t.Fatalf("unexpected id: %q", item.ID)
	}
	if item.Status != ExtensionStatusReady {
		t.Fatalf("unexpected status: %q", item.Status)
	}
	if item.Capabilities.Prompts != 1 || item.Capabilities.Tools != 2 {
		t.Fatalf("unexpected capabilities: %#v", item.Capabilities)
	}
}

func TestManagerLoadRemainsVisibleAfterReload(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "review")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "skill.json"), []byte(`{"name":"review","description":"remote"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "SKILL.md"), []byte("# /review"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(root)
	loaded, err := mgr.Load(context.Background(), external)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	got, err := mgr.Get(context.Background(), loaded.ID)
	if err != nil {
		t.Fatalf("Get failed after Load: %v", err)
	}
	if got.Source.Ref != external {
		t.Fatalf("expected loaded source ref %q, got %q", external, got.Source.Ref)
	}
	items, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed after Load: %v", err)
	}
	if len(items) != 1 || items[0].ID != loaded.ID {
		t.Fatalf("expected loaded extension to remain visible, got %#v", items)
	}
}

func TestManagerLoadMarksUnknownRootAsRemote(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "review")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "skill.json"), []byte(`{"name":"review"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "SKILL.md"), []byte("# /review"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(root)
	item, err := mgr.Load(context.Background(), external)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if item.Source.Scope != ExtensionScopeRemote {
		t.Fatalf("expected remote scope, got %q", item.Source.Scope)
	}
}

func TestManagerUnloadIgnoresUnrelatedBrokenManifest(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, ".bytemind", "skills")
	good := filepath.Join(project, "review")
	bad := filepath.Join(project, "broken")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "skill.json"), []byte(`{"name":"review"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "SKILL.md"), []byte("# /review"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(root)
	if err := mgr.Unload(context.Background(), "skill.review"); err != nil {
		t.Fatalf("Unload should ignore unrelated broken manifest: %v", err)
	}
	items, err := mgr.List(context.Background())
	if err == nil {
		t.Fatal("expected observable discovery error")
	}
	if len(items) != 0 {
		t.Fatalf("expected unloaded extension to stay hidden, got %#v", items)
	}
}

func TestManagerUnloadCanDisableBrokenExtension(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, ".bytemind", "skills")
	bad := filepath.Join(project, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(root)
	if err := mgr.Unload(context.Background(), "skill.bad"); err != nil {
		t.Fatalf("expected broken extension to be disable-able, got %v", err)
	}
	items, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("expected discovery error to clear after unload, got %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no visible extensions, got %#v", items)
	}
}

func TestManagerListDiscoversAcrossScopesWithPriority(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	project := filepath.Join(root, "project")
	for _, dir := range []string{
		filepath.Join(builtin, "review"),
		filepath.Join(user, "review"),
		filepath.Join(project, "review"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(builtin, "review", "skill.json"), []byte(`{"name":"review","description":"builtin"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "review", "skill.json"), []byte(`{"name":"review","description":"user"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "review", "skill.json"), []byte(`{"name":"review","description":"project"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "review", "SKILL.md"), []byte("# /review"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManagerWithDirs(root, builtin, user, project)
	items, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(items))
	}
	if items[0].Description != "project" {
		t.Fatalf("expected project scope to win, got %q", items[0].Description)
	}
	if items[0].Source.Scope != ExtensionScopeProject {
		t.Fatalf("unexpected scope: %q", items[0].Source.Scope)
	}
}

func TestManagerListLocalizesManifestErrors(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	good := filepath.Join(project, "good")
	bad := filepath.Join(project, "bad")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "skill.json"), []byte(`{"name":"good","description":"ok"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "SKILL.md"), []byte("# /good"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManagerWithDirs(root, filepath.Join(root, "builtin"), filepath.Join(root, "user"), project)
	items, err := mgr.List(context.Background())
	if err == nil {
		t.Fatal("expected observable discovery error")
	}
	if len(items) != 1 || items[0].ID != "skill.good" {
		t.Fatalf("expected healthy extension to remain visible, got %#v", items)
	}
	var extErr *ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeLoadFailed {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
}

func TestManagerGetReturnsHealthyExtensionWithObservableDiscoveryError(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	good := filepath.Join(project, "good")
	bad := filepath.Join(project, "bad")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "skill.json"), []byte(`{"name":"good"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "SKILL.md"), []byte("# /good"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManagerWithDirs(root, filepath.Join(root, "builtin"), filepath.Join(root, "user"), project)
	item, err := mgr.Get(context.Background(), "skill.good")
	if item.ID != "skill.good" {
		t.Fatalf("expected healthy extension, got %#v", item)
	}
	if err == nil {
		t.Fatal("expected observable discovery error")
	}
}

func TestManagerGetMissingReturnsNotFoundDespiteOtherDiscoveryErrors(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	bad := filepath.Join(project, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManagerWithDirs(root, filepath.Join(root, "builtin"), filepath.Join(root, "user"), project)
	item, err := mgr.Get(context.Background(), "skill.missing")
	if item != (ExtensionInfo{}) {
		t.Fatal("expected zero extension info")
	}
	var extErr *ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeNotFound {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
}

func TestManagerGetReturnsNotFound(t *testing.T) {
	mgr := NewManager(t.TempDir())
	item, err := mgr.Get(context.Background(), "skill.review")
	if item != (ExtensionInfo{}) {
		t.Fatal("expected zero extension info")
	}
	if err == nil {
		t.Fatal("expected not found error")
	}
	var extErr *ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeNotFound {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
}

func TestManagerUnloadMissingReturnsNotFoundDespiteOtherDiscoveryErrors(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	bad := filepath.Join(project, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManagerWithDirs(root, filepath.Join(root, "builtin"), filepath.Join(root, "user"), project)
	err := mgr.Unload(context.Background(), "skill.missing")
	var extErr *ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeNotFound {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
}

func TestManagerUnloadPersistsAcrossReload(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, ".bytemind", "skills")
	if err := os.MkdirAll(filepath.Join(project, "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "review", "skill.json"), []byte(`{"name":"review"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "review", "SKILL.md"), []byte("# /review"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(root)
	if _, err := mgr.Load(context.Background(), filepath.Join(project, "review")); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Unload(context.Background(), "skill.review"); err != nil {
		t.Fatalf("Unload failed: %v", err)
	}
	items, err := mgr.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected unloaded extension to stay hidden, got %#v", items)
	}
	if _, err := mgr.Get(context.Background(), "skill.review"); err == nil {
		t.Fatal("expected not found after unload")
	}
}

func TestManagerUnloadReturnsNotFoundForUnknownExtension(t *testing.T) {
	mgr := NewManager(t.TempDir())
	if err := mgr.Unload(context.Background(), "skill.missing"); err == nil {
		t.Fatal("expected not found error")
	} else {
		var extErr *ExtensionError
		if !errors.As(err, &extErr) {
			t.Fatalf("expected ExtensionError, got %T", err)
		}
		if extErr.Code != ErrCodeNotFound {
			t.Fatalf("unexpected code: %s", extErr.Code)
		}
	}
}

func TestManagerRejectsInvalidSourceAndManifest(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(root)
	if _, err := mgr.Load(context.Background(), ""); err == nil {
		t.Fatal("expected invalid source error")
	}
	bad := filepath.Join(root, ".bytemind", "skills", "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "skill.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Load(context.Background(), bad); err == nil {
		t.Fatal("expected invalid manifest error")
	} else {
		var extErr *ExtensionError
		if !errors.As(err, &extErr) {
			t.Fatalf("expected ExtensionError, got %T", err)
		}
		if extErr.Code != ErrCodeInvalidManifest {
			t.Fatalf("unexpected code: %s", extErr.Code)
		}
	}
}

func TestScopeForPathReturnsFoundFlag(t *testing.T) {
	mgr := &extensionManager{
		builtinDir: filepath.Join("repo", "internal", "skills"),
		userDir:    filepath.Join("home", ".bytemind", "skills"),
		projectDir: filepath.Join("repo", ".bytemind", "skills"),
	}
	if scope, ok := scopeForPath(filepath.Join("repo", ".bytemind", "skills", "review"), mgr); !ok || scope != ExtensionScopeProject {
		t.Fatalf("expected project scope, got %q %v", scope, ok)
	}
	if scope, ok := scopeForPath(filepath.Join("outside", "review"), mgr); ok || scope != "" {
		t.Fatalf("expected unknown scope, got %q %v", scope, ok)
	}
}

func TestNopManagerLoad(t *testing.T) {
	mgr := NopManager{}
	item, err := mgr.Load(nil, ".bytemind/skills/review")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !item.IsZero() {
		t.Fatal("expected zero extension info")
	}
}

func TestNopManagerUnload(t *testing.T) {
	mgr := NopManager{}
	if err := mgr.Unload(nil, "skill.review"); err != nil {
		t.Fatalf("Unload failed: %v", err)
	}
}

func TestNopManagerGet(t *testing.T) {
	mgr := NopManager{}
	item, err := mgr.Get(nil, "skill.review")
	if item != (ExtensionInfo{}) {
		t.Fatal("expected zero extension info")
	}
	if err == nil {
		t.Fatal("expected not found error")
	}
	var extErr *ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeNotFound {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
}

func TestNopManagerList(t *testing.T) {
	mgr := NopManager{}
	items, err := mgr.List(nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no extensions, got %d", len(items))
	}
}

func TestExtensionInfoValid(t *testing.T) {
	valid := ExtensionInfo{
		ID:   "skill.review",
		Name: "review",
		Kind: ExtensionSkill,
		Source: ExtensionSource{
			Scope: ExtensionScopeProject,
			Ref:   ".bytemind/skills/review",
		},
		Status:       ExtensionStatusReady,
		Capabilities: CapabilitySet{Prompts: 1, Tools: 2},
	}
	if !valid.Valid() {
		t.Fatal("expected extension info to be valid")
	}

	cases := []ExtensionInfo{
		{Name: "review", Kind: ExtensionSkill},
		{ID: "skill.review", Kind: ExtensionSkill},
		{ID: "skill.review", Name: "review"},
		{ID: "skill.review", Name: "review", Kind: ExtensionKind("unknown")},
		{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Ref: ".bytemind/skills/review"}},
		{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScopeProject}},
		{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScope("bad"), Ref: ".bytemind/skills/review"}},
	}
	for _, tc := range cases {
		if tc.Valid() {
			t.Fatalf("expected invalid extension info: %+v", tc)
		}
	}
}

func TestExtensionInfoIsZero(t *testing.T) {
	if !((ExtensionInfo{}).IsZero()) {
		t.Fatal("expected zero extension info")
	}

	cases := []ExtensionInfo{
		{ID: "skill.review"},
		{Version: "1.0.0"},
		{Title: "Review"},
		{Description: "desc"},
		{Source: ExtensionSource{Scope: ExtensionScopeProject}},
		{Source: ExtensionSource{Ref: ".bytemind/skills/review"}},
		{Capabilities: CapabilitySet{Tools: 1}},
		{Manifest: Manifest{Name: "review"}},
		{Manifest: Manifest{Kind: ExtensionSkill}},
		{Manifest: Manifest{Source: ExtensionSource{Ref: "manifest.json"}}},
		{Health: HealthSnapshot{Status: ExtensionStatusReady}},
		{Health: HealthSnapshot{Message: "ok"}},
		{Health: HealthSnapshot{LastError: ErrCodeLoadFailed}},
		{Health: HealthSnapshot{CheckedAtUTC: "2026-04-17T00:00:00Z"}},
		{Status: ExtensionStatusReady},
	}
	for _, tc := range cases {
		if tc.IsZero() {
			t.Fatalf("expected non-zero extension info: %+v", tc)
		}
	}
}

func TestExtensionErrorWrap(t *testing.T) {
	err := wrapError(ErrCodeLoadFailed, "load extension", nil)
	extErr, ok := err.(*ExtensionError)
	if !ok {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeLoadFailed {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
	if extErr.Message == "" {
		t.Fatal("expected message")
	}
	if extErr.Unwrap() != nil {
		t.Fatal("expected nil unwrap")
	}
	if extErr.CodeString() != string(ErrCodeLoadFailed) {
		t.Fatalf("unexpected code string: %q", extErr.CodeString())
	}
}

func TestExtensionErrorWithCause(t *testing.T) {
	cause := errors.New("boom")
	err := wrapError(ErrCodeUnloadFailed, "unload extension", cause)
	extErr, ok := err.(*ExtensionError)
	if !ok {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if !errors.Is(extErr, cause) {
		t.Fatal("expected wrapped cause")
	}
	if extErr.Error() == "" {
		t.Fatal("expected error string")
	}
}

func TestNilExtensionErrorBehaviors(t *testing.T) {
	var err *ExtensionError
	if err.Error() != "" {
		t.Fatalf("expected empty error string, got %q", err.Error())
	}
	if err.Unwrap() != nil {
		t.Fatal("expected nil unwrap")
	}
	if err.CodeString() != "" {
		t.Fatalf("expected empty code string, got %q", err.CodeString())
	}
}
