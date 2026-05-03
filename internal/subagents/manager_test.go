package subagents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerReloadAppliesScopeOverridesAndLookup(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	userDir := filepath.Join(workspace, "user")
	projectDir := filepath.Join(workspace, "project")

	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: builtin review
aliases: [/review]
---
builtin review body
`)
	writeSubAgentFile(t, filepath.Join(userDir, "review.md"), `---
name: review
description: user review
aliases: [review-user]
---
user review body
`)
	writeSubAgentFile(t, filepath.Join(projectDir, "explorer.md"), `---
name: explorer
description: project explorer
aliases: [/scan]
---
project explorer body
`)

	manager := NewManagerWithDirs(workspace, builtinDir, userDir, projectDir)
	catalog := manager.Reload()
	if len(catalog.Agents) != 2 {
		t.Fatalf("expected 2 effective subagents, got %d", len(catalog.Agents))
	}

	// Hardcoded builtins generate overrides when directory/user/project scopes provide same-name definitions.
	foundUserOverride := false
	for _, o := range catalog.Overrides {
		if o.Name == "review" && o.Winner == ScopeUser && o.Loser == ScopeBuiltin {
			foundUserOverride = true
		}
	}
	if !foundUserOverride {
		t.Fatalf("expected user→builtin override for review, got %#v", catalog.Overrides)
	}

	review, ok := manager.Find("review")
	if !ok {
		t.Fatal("expected effective review subagent to resolve")
	}
	if review.Scope != ScopeUser || review.Description != "user review" {
		t.Fatalf("unexpected effective review subagent: %+v", review)
	}
	explorer, ok := manager.Find("/scan")
	if !ok {
		t.Fatal("expected alias /scan to resolve")
	}
	if explorer.Name != "explorer" || explorer.Scope != ScopeProject {
		t.Fatalf("unexpected explorer subagent: %+v", explorer)
	}
}

func TestManagerFindBuiltinIgnoresOverrides(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	userDir := filepath.Join(workspace, "user")
	projectDir := filepath.Join(workspace, "project")

	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: builtin review
aliases: [/review]
---
builtin review body
`)
	writeSubAgentFile(t, filepath.Join(projectDir, "review.md"), `---
name: review
description: project review
aliases: [/review]
---
project review body
`)

	manager := NewManagerWithDirs(workspace, builtinDir, userDir, projectDir)
	manager.Reload()

	agent, ok := manager.FindBuiltin("/review")
	if !ok {
		t.Fatal("expected builtin /review to resolve")
	}
	if agent.Scope != ScopeBuiltin || agent.Description != "builtin review" {
		t.Fatalf("expected builtin review definition, got %+v", agent)
	}
}

func TestManagerReloadReportsInvalidNameDiagnostic(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	writeSubAgentFile(t, filepath.Join(builtinDir, "bad.md"), `---
name: bad name!
description: invalid
---
body
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	catalog := manager.Reload()
	// Hardcoded builtins (explorer, review) are always present; the invalid file should not add to them.
	if len(catalog.Agents) != 2 {
		t.Fatalf("expected 2 hardcoded subagent definitions, got %#v", catalog.Agents)
	}
	if len(catalog.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for invalid name")
	}
	if !strings.Contains(catalog.Diagnostics[0].Message, "invalid subagent name") {
		t.Fatalf("unexpected diagnostic: %#v", catalog.Diagnostics[0])
	}
}

func TestLoadAgentFromFileParsesListFieldsAndWarnsOnInvalidMaxTurns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "explorer.md")
	writeSubAgentFile(t, path, `---
name: explorer
description: read-only explorer
aliases:
  - /scan
  - exp
tools:
  - read_file
  - search_text
disallowed_tools:
  - run_shell
max_turns: nope
---
Inspect files and summarize findings.
`)

	agent, ok, diags := loadAgentFromFile(ScopeProject, path)
	if !ok {
		t.Fatal("expected file to load")
	}
	if agent.Name != "explorer" || agent.Entry != "/explorer" {
		t.Fatalf("unexpected agent metadata: %+v", agent)
	}
	if len(agent.Aliases) < 2 || !containsString(agent.Aliases, "/scan") {
		t.Fatalf("expected aliases to include /scan, got %#v", agent.Aliases)
	}
	if !containsString(agent.Tools, "read_file") || !containsString(agent.DisallowedTools, "run_shell") {
		t.Fatalf("expected tools/disallowed_tools parsed, got tools=%#v disallowed=%#v", agent.Tools, agent.DisallowedTools)
	}
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "invalid max_turns") {
		t.Fatalf("expected invalid max_turns warning, got %#v", diags)
	}
}

func TestNewManagerDisablesUserScopeWhenHomeResolutionFails(t *testing.T) {
	original := resolveSubAgentHomeDir
	resolveSubAgentHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}
	t.Cleanup(func() {
		resolveSubAgentHomeDir = original
	})

	workspace := t.TempDir()
	manager := NewManager(workspace)
	if strings.TrimSpace(manager.userDir) != "" {
		t.Fatalf("expected empty user dir when home resolution fails, got %q", manager.userDir)
	}

	catalog := manager.Reload()
	if len(catalog.Diagnostics) == 0 {
		t.Fatalf("expected bootstrap diagnostics, got %#v", catalog.Diagnostics)
	}
	if !strings.Contains(catalog.Diagnostics[0].Message, "user subagent scope disabled") {
		t.Fatalf("unexpected first diagnostic: %#v", catalog.Diagnostics[0])
	}
}

func TestManagerFindUsesLoadedSnapshotWithoutReload(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: builtin review
aliases: [/review]
---
builtin review body
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	manager.Reload()
	before := manager.Snapshot().LoadedAt

	if err := os.Remove(filepath.Join(builtinDir, "review.md")); err != nil {
		t.Fatalf("remove review.md: %v", err)
	}

	agent, ok := manager.Find("/review")
	if !ok {
		t.Fatalf("expected /review to resolve from loaded snapshot")
	}
	if agent.Name != "review" || agent.Scope != ScopeBuiltin {
		t.Fatalf("unexpected agent payload: %+v", agent)
	}

	after := manager.Snapshot().LoadedAt
	if !after.Equal(before) {
		t.Fatalf("expected Find hit not to trigger reload; before=%s after=%s", before.Format(time.RFC3339Nano), after.Format(time.RFC3339Nano))
	}
}

func TestHardcodedBuiltinAgentsResolveWithoutFiles(t *testing.T) {
	workspace := t.TempDir()
	manager := NewManagerWithDirs(
		workspace,
		filepath.Join(workspace, "nonexistent_builtin"),
		filepath.Join(workspace, "nonexistent_user"),
		filepath.Join(workspace, "nonexistent_project"),
	)
	manager.Reload()

	for _, name := range []string{"/explorer", "explorer", "/review", "review"} {
		agent, ok := manager.FindBuiltin(name)
		if !ok {
			t.Fatalf("expected FindBuiltin(%q) to resolve hardcoded builtin", name)
		}
		if agent.Scope != ScopeBuiltin {
			t.Fatalf("expected builtin scope for %q, got %s", name, agent.Scope)
		}
	}
}

func TestManagerFindMissDoesNotReloadWhenSnapshotLoaded(t *testing.T) {
	workspace := t.TempDir()
	manager := NewManagerWithDirs(
		workspace,
		filepath.Join(workspace, "builtin"),
		filepath.Join(workspace, "user"),
		filepath.Join(workspace, "project"),
	)
	manager.Reload()
	before := manager.Snapshot().LoadedAt

	if _, ok := manager.Find("missing-agent"); ok {
		t.Fatal("expected missing-agent lookup miss")
	}

	after := manager.Snapshot().LoadedAt
	if !after.Equal(before) {
		t.Fatalf("expected Find miss not to trigger reload; before=%s after=%s", before.Format(time.RFC3339Nano), after.Format(time.RFC3339Nano))
	}
}

func writeSubAgentFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
