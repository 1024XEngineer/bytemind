package agent

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/skills"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

type runtimeBridgeTool struct {
	name string
}

func (t runtimeBridgeTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        t.name,
			Description: "runtime bridge tool",
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

func (runtimeBridgeTool) Run(context.Context, json.RawMessage, *tools.ExecutionContext) (string, error) {
	return `{"ok":true}`, nil
}

func TestPrepareRunPromptRegistersActiveSkillBridgeTools(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	userDir := filepath.Join(workspace, "user")
	projectDir := filepath.Join(workspace, "project")
	reviewDir := filepath.Join(builtinDir, "review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reviewDir, "SKILL.md"), []byte(`---
name: review
allowed-tools: "open_doc"
---
# /review
`), 0o644); err != nil {
		t.Fatal(err)
	}

	skillManager := skills.NewManagerWithDirs(workspace, builtinDir, userDir, projectDir)
	registry := tools.DefaultRegistry()
	if err := registry.Register(runtimeBridgeTool{name: "open_doc"}, tools.RegisterOptions{
		Source: tools.RegistrationSourceBuiltin,
	}); err != nil {
		t.Fatalf("register open_doc failed: %v", err)
	}
	runner := NewRunner(Options{
		Workspace:    workspace,
		Registry:     registry,
		SkillManager: skillManager,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
	})
	sess := session.New(workspace)
	sess.ActiveSkill = &session.ActiveSkill{Name: "review"}

	setup, err := runner.prepareRunPrompt(sess, RunPromptInput{
		UserMessage: llm.NewUserTextMessage("bridge setup"),
		DisplayText: "bridge setup",
	}, "build")
	if err != nil {
		t.Fatalf("prepareRunPrompt failed: %v", err)
	}

	expectedStable := "skill:skill_review:open_doc"
	if !slices.Contains(setup.AllowedToolNames, expectedStable) {
		t.Fatalf("expected stable bridge key in allowlist, got %#v", setup.AllowedToolNames)
	}
	if slices.Contains(setup.AllowedToolNames, "open_doc") {
		t.Fatalf("did not expect legacy tool name in allowlist after bridge, got %#v", setup.AllowedToolNames)
	}
	metas := registry.FindByExtensionID("skill.review")
	if len(metas) == 0 {
		t.Fatal("expected extension bridge metadata to be registered")
	}
}

func TestPrepareRunPromptSwitchingActiveSkillReplacesBridgeTools(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	userDir := filepath.Join(workspace, "user")
	projectDir := filepath.Join(workspace, "project")
	for _, name := range []string{"review", "plan"} {
		dir := filepath.Join(builtinDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		allowed := "open_doc"
		if name == "plan" {
			allowed = "plan_doc"
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: `+name+`
allowed-tools: "`+allowed+`"
---
# /`+name+`
`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	skillManager := skills.NewManagerWithDirs(workspace, builtinDir, userDir, projectDir)
	registry := tools.DefaultRegistry()
	if err := registry.Register(runtimeBridgeTool{name: "open_doc"}, tools.RegisterOptions{
		Source: tools.RegistrationSourceBuiltin,
	}); err != nil {
		t.Fatalf("register open_doc failed: %v", err)
	}
	if err := registry.Register(runtimeBridgeTool{name: "plan_doc"}, tools.RegisterOptions{
		Source: tools.RegistrationSourceBuiltin,
	}); err != nil {
		t.Fatalf("register plan_doc failed: %v", err)
	}
	runner := NewRunner(Options{
		Workspace:    workspace,
		Registry:     registry,
		SkillManager: skillManager,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
	})
	sess := session.New(workspace)
	sess.ActiveSkill = &session.ActiveSkill{Name: "review"}
	if _, err := runner.prepareRunPrompt(sess, RunPromptInput{
		UserMessage: llm.NewUserTextMessage("review"),
		DisplayText: "review",
	}, "build"); err != nil {
		t.Fatalf("prepareRunPrompt review failed: %v", err)
	}

	sess.ActiveSkill = &session.ActiveSkill{Name: "plan"}
	setup, err := runner.prepareRunPrompt(sess, RunPromptInput{
		UserMessage: llm.NewUserTextMessage("plan"),
		DisplayText: "plan",
	}, "build")
	if err != nil {
		t.Fatalf("prepareRunPrompt plan failed: %v", err)
	}

	if len(registry.FindByExtensionID("skill.review")) != 0 {
		t.Fatalf("expected stale review bridge metadata to be removed")
	}
	if len(registry.FindByExtensionID("skill.plan")) == 0 {
		t.Fatalf("expected active plan bridge metadata")
	}
	if !slices.Contains(setup.AllowedToolNames, "skill:skill_plan:plan_doc") {
		t.Fatalf("expected plan stable key in allowlist, got %#v", setup.AllowedToolNames)
	}
}

func TestPrepareRunPromptSessionIsolationKeepsOtherSessionBridges(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	userDir := filepath.Join(workspace, "user")
	projectDir := filepath.Join(workspace, "project")
	for _, name := range []string{"review", "plan"} {
		dir := filepath.Join(builtinDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		allowed := "open_doc"
		if name == "plan" {
			allowed = "plan_doc"
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: `+name+`
allowed-tools: "`+allowed+`"
---
# /`+name+`
`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	skillManager := skills.NewManagerWithDirs(workspace, builtinDir, userDir, projectDir)
	registry := tools.DefaultRegistry()
	if err := registry.Register(runtimeBridgeTool{name: "open_doc"}, tools.RegisterOptions{
		Source: tools.RegistrationSourceBuiltin,
	}); err != nil {
		t.Fatalf("register open_doc failed: %v", err)
	}
	if err := registry.Register(runtimeBridgeTool{name: "plan_doc"}, tools.RegisterOptions{
		Source: tools.RegistrationSourceBuiltin,
	}); err != nil {
		t.Fatalf("register plan_doc failed: %v", err)
	}
	runner := NewRunner(Options{
		Workspace:    workspace,
		Registry:     registry,
		SkillManager: skillManager,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
	})

	sessReview := session.New(workspace)
	sessReview.ActiveSkill = &session.ActiveSkill{Name: "review"}
	if _, err := runner.prepareRunPrompt(sessReview, RunPromptInput{
		UserMessage: llm.NewUserTextMessage("review"),
		DisplayText: "review",
	}, "build"); err != nil {
		t.Fatalf("prepareRunPrompt review failed: %v", err)
	}

	sessPlan := session.New(workspace)
	sessPlan.ActiveSkill = &session.ActiveSkill{Name: "plan"}
	if _, err := runner.prepareRunPrompt(sessPlan, RunPromptInput{
		UserMessage: llm.NewUserTextMessage("plan"),
		DisplayText: "plan",
	}, "build"); err != nil {
		t.Fatalf("prepareRunPrompt plan failed: %v", err)
	}

	if len(registry.FindByExtensionID("skill.review")) == 0 {
		t.Fatalf("expected review bridge metadata to remain registered for review session")
	}
	if len(registry.FindByExtensionID("skill.plan")) == 0 {
		t.Fatalf("expected plan bridge metadata to be registered")
	}
}

func TestClearSessionSkillBridgesHonorsSharedBridgeReferences(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	userDir := filepath.Join(workspace, "user")
	projectDir := filepath.Join(workspace, "project")
	reviewDir := filepath.Join(builtinDir, "review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reviewDir, "SKILL.md"), []byte(`---
name: review
allowed-tools: "open_doc"
---
# /review
`), 0o644); err != nil {
		t.Fatal(err)
	}

	skillManager := skills.NewManagerWithDirs(workspace, builtinDir, userDir, projectDir)
	registry := tools.DefaultRegistry()
	if err := registry.Register(runtimeBridgeTool{name: "open_doc"}, tools.RegisterOptions{
		Source: tools.RegistrationSourceBuiltin,
	}); err != nil {
		t.Fatalf("register open_doc failed: %v", err)
	}
	runner := NewRunner(Options{
		Workspace:    workspace,
		Registry:     registry,
		SkillManager: skillManager,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
	})

	sessA := session.New(workspace)
	sessA.ActiveSkill = &session.ActiveSkill{Name: "review"}
	sessB := session.New(workspace)
	sessB.ActiveSkill = &session.ActiveSkill{Name: "review"}

	if _, err := runner.prepareRunPrompt(sessA, RunPromptInput{
		UserMessage: llm.NewUserTextMessage("a"),
		DisplayText: "a",
	}, "build"); err != nil {
		t.Fatalf("prepareRunPrompt session A failed: %v", err)
	}
	if _, err := runner.prepareRunPrompt(sessB, RunPromptInput{
		UserMessage: llm.NewUserTextMessage("b"),
		DisplayText: "b",
	}, "build"); err != nil {
		t.Fatalf("prepareRunPrompt session B failed: %v", err)
	}

	stable := "skill:skill_review:open_doc"
	if _, ok := registry.Get(stable); !ok {
		t.Fatalf("expected shared bridge tool %q", stable)
	}

	runner.clearSessionSkillBridges(sessA)
	if _, ok := registry.Get(stable); !ok {
		t.Fatalf("expected bridge tool %q to stay while another session still references it", stable)
	}

	runner.clearSessionSkillBridges(sessB)
	if _, ok := registry.Get(stable); ok {
		t.Fatalf("expected bridge tool %q to be removed after last session is cleared", stable)
	}
}

func TestPrepareRunPromptConcurrentSessionsIsolation(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "builtin")
	userDir := filepath.Join(workspace, "user")
	projectDir := filepath.Join(workspace, "project")
	for _, name := range []string{"review", "plan"} {
		dir := filepath.Join(builtinDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: `+name+`
allowed-tools: "open_doc"
---
# /`+name+`
`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	skillManager := skills.NewManagerWithDirs(workspace, builtinDir, userDir, projectDir)
	registry := tools.DefaultRegistry()
	if err := registry.Register(runtimeBridgeTool{name: "open_doc"}, tools.RegisterOptions{
		Source: tools.RegistrationSourceBuiltin,
	}); err != nil {
		t.Fatalf("register open_doc failed: %v", err)
	}
	runner := NewRunner(Options{
		Workspace:    workspace,
		Registry:     registry,
		SkillManager: skillManager,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
	})

	sessReview := session.New(workspace)
	sessReview.ActiveSkill = &session.ActiveSkill{Name: "review"}
	sessPlan := session.New(workspace)
	sessPlan.ActiveSkill = &session.ActiveSkill{Name: "plan"}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := runner.prepareRunPrompt(sessReview, RunPromptInput{
			UserMessage: llm.NewUserTextMessage("review"),
			DisplayText: "review",
		}, "build")
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, err := runner.prepareRunPrompt(sessPlan, RunPromptInput{
			UserMessage: llm.NewUserTextMessage("plan"),
			DisplayText: "plan",
		}, "build")
		errs <- err
	}()
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("prepareRunPrompt failed: %v", err)
		}
	}

	if len(registry.FindByExtensionID("skill.review")) == 0 {
		t.Fatalf("expected review bridge metadata after concurrent sync")
	}
	if len(registry.FindByExtensionID("skill.plan")) == 0 {
		t.Fatalf("expected plan bridge metadata after concurrent sync")
	}
	if _, ok := registry.Get("skill:skill_review:open_doc"); !ok {
		t.Fatalf("expected review bridge tool to be registered")
	}
	if _, ok := registry.Get("skill:skill_plan:open_doc"); !ok {
		t.Fatalf("expected plan bridge tool to be registered")
	}
}
