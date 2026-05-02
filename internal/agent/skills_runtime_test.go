package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
	skillspkg "github.com/1024XEngineer/bytemind/internal/skills"
	toolspkg "github.com/1024XEngineer/bytemind/internal/tools"
)

type skillRuntimeTestTool struct {
	name string
}

func (t skillRuntimeTestTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        t.name,
			Description: "skills runtime test tool",
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

func (skillRuntimeTestTool) Run(context.Context, json.RawMessage, *toolspkg.ExecutionContext) (string, error) {
	return "ok", nil
}

func TestResolveSkillToolSetsMapsActiveSkillPolicyToStableKeys(t *testing.T) {
	registry := &toolspkg.Registry{}
	if err := registry.Register(skillRuntimeTestTool{name: "skill:skill_review:open_doc"}, toolspkg.RegisterOptions{
		Source:       toolspkg.RegistrationSourceExtension,
		ExtensionID:  "skill.review",
		OriginalName: "Open_Doc",
	}); err != nil {
		t.Fatalf("register extension tool failed: %v", err)
	}

	active := &activeSkillRuntime{
		Skill: skillspkg.Skill{
			Name: "review",
			ToolPolicy: skillspkg.ToolPolicy{
				Policy: skillspkg.ToolPolicyAllowlist,
				Items:  []string{"open_doc", "read_file"},
			},
		},
	}
	allow, deny, err := resolveSkillToolSets(active, registry)
	if err != nil {
		t.Fatalf("resolveSkillToolSets failed: %v", err)
	}
	if deny != nil {
		t.Fatalf("expected nil deny set, got %#v", deny)
	}
	if _, ok := allow["skill:skill_review:open_doc"]; !ok {
		t.Fatalf("expected stable tool key in allow set, got %#v", allow)
	}
	if _, ok := allow["open_doc"]; ok {
		t.Fatalf("did not expect original tool name in allow set, got %#v", allow)
	}
	if _, ok := allow["read_file"]; !ok {
		t.Fatalf("expected builtin tool name to remain in allow set, got %#v", allow)
	}
}

func TestResolveSkillToolSetsMapsWhenSkillNameAlreadyHasPrefix(t *testing.T) {
	registry := &toolspkg.Registry{}
	if err := registry.Register(skillRuntimeTestTool{name: "skill:skill_skill_review:open_doc"}, toolspkg.RegisterOptions{
		Source:       toolspkg.RegistrationSourceExtension,
		ExtensionID:  "skill.skill.review",
		OriginalName: "open_doc",
	}); err != nil {
		t.Fatalf("register extension tool failed: %v", err)
	}

	active := &activeSkillRuntime{
		Skill: skillspkg.Skill{
			Name: "skill.review",
			ToolPolicy: skillspkg.ToolPolicy{
				Policy: skillspkg.ToolPolicyAllowlist,
				Items:  []string{"open_doc"},
			},
		},
	}
	allow, deny, err := resolveSkillToolSets(active, registry)
	if err != nil {
		t.Fatalf("resolveSkillToolSets failed: %v", err)
	}
	if deny != nil {
		t.Fatalf("expected nil deny set, got %#v", deny)
	}
	if _, ok := allow["skill:skill_skill_review:open_doc"]; !ok {
		t.Fatalf("expected stable tool key in allow set, got %#v", allow)
	}
	if _, ok := allow["open_doc"]; ok {
		t.Fatalf("did not expect original tool name in allow set, got %#v", allow)
	}
}

func TestResolveSkillToolSetsFallsBackWithoutBridgeBindings(t *testing.T) {
	active := &activeSkillRuntime{
		Skill: skillspkg.Skill{
			Name: "review",
			ToolPolicy: skillspkg.ToolPolicy{
				Policy: skillspkg.ToolPolicyAllowlist,
				Items:  []string{"read_file"},
			},
		},
	}
	allow, deny, err := resolveSkillToolSets(active, nil)
	if err != nil {
		t.Fatalf("resolveSkillToolSets failed: %v", err)
	}
	if deny != nil {
		t.Fatalf("expected nil deny set, got %#v", deny)
	}
	if _, ok := allow["read_file"]; !ok {
		t.Fatalf("expected read_file in allow set, got %#v", allow)
	}
}

func TestResolveSkillToolSetsDenylistIncludesOriginalAndStableKeys(t *testing.T) {
	registry := &toolspkg.Registry{}
	if err := registry.Register(skillRuntimeTestTool{name: "skill:skill_review:open_doc"}, toolspkg.RegisterOptions{
		Source:       toolspkg.RegistrationSourceExtension,
		ExtensionID:  "skill.review",
		OriginalName: "open_doc",
	}); err != nil {
		t.Fatalf("register extension tool failed: %v", err)
	}

	active := &activeSkillRuntime{
		Skill: skillspkg.Skill{
			Name: "review",
			ToolPolicy: skillspkg.ToolPolicy{
				Policy: skillspkg.ToolPolicyDenylist,
				Items:  []string{"open_doc"},
			},
		},
	}
	allow, deny, err := resolveSkillToolSets(active, registry)
	if err != nil {
		t.Fatalf("resolveSkillToolSets failed: %v", err)
	}
	if allow != nil {
		t.Fatalf("expected nil allow set for denylist, got %#v", allow)
	}
	if _, ok := deny["skill:skill_review:open_doc"]; !ok {
		t.Fatalf("expected stable key in deny set, got %#v", deny)
	}
	if _, ok := deny["open_doc"]; !ok {
		t.Fatalf("expected original key in deny set, got %#v", deny)
	}
}
