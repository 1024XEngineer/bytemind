package extensions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"bytemind/internal/llm"
	skillspkg "bytemind/internal/skills"
	toolspkg "bytemind/internal/tools"
)

func TestStableToolKeyNormalizesSegments(t *testing.T) {
	key, err := StableToolKey(ExtensionSkill, " Skill.Review/One ", "Open Doc!!")
	if err != nil {
		t.Fatalf("StableToolKey failed: %v", err)
	}
	if key != "skill:skill_review_one:open_doc" {
		t.Fatalf("unexpected stable key: %q", key)
	}
}

func TestStableToolKeyAppliesLengthLimit(t *testing.T) {
	extensionID := strings.Repeat("x", 128)
	toolName := strings.Repeat("y", 128)
	key, err := StableToolKey(ExtensionMCP, extensionID, toolName)
	if err != nil {
		t.Fatalf("StableToolKey failed: %v", err)
	}
	if len(key) > maxStableToolKeyLength {
		t.Fatalf("stable key exceeded max length: %d", len(key))
	}
}

func TestStableToolKeyForMCPStripsExtensionPrefix(t *testing.T) {
	key, err := StableToolKey(ExtensionMCP, "mcp.docs", "open_doc")
	if err != nil {
		t.Fatalf("StableToolKey failed: %v", err)
	}
	if key != "mcp:docs:open_doc" {
		t.Fatalf("unexpected mcp stable key: %q", key)
	}
}

func TestStableToolKeyTruncationKeepsValidUTF8(t *testing.T) {
	extensionID := strings.Repeat("技能", 80)
	toolName := strings.Repeat("工具", 80)
	key, err := StableToolKey(ExtensionSkill, extensionID, toolName)
	if err != nil {
		t.Fatalf("StableToolKey failed: %v", err)
	}
	if len(key) > maxStableToolKeyLength {
		t.Fatalf("stable key exceeded max length: %d", len(key))
	}
	if !utf8.ValidString(key) {
		t.Fatalf("stable key must remain valid UTF-8, got %q", key)
	}
}

func TestRegisterBridgedToolRegistersStableKeyAndMetadata(t *testing.T) {
	registry := &toolspkg.Registry{}
	binding, err := RegisterBridgedTool(registry, ExtensionTool{
		Source:      ExtensionSkill,
		ExtensionID: "skill.review",
		Tool:        bridgeTestTool{name: "open_doc"},
	})
	if err != nil {
		t.Fatalf("RegisterBridgedTool failed: %v", err)
	}
	if binding.StableKey != "skill:skill_review:open_doc" {
		t.Fatalf("unexpected stable key: %q", binding.StableKey)
	}
	resolved, ok := registry.Get(binding.StableKey)
	if !ok {
		t.Fatalf("expected bridged tool %q", binding.StableKey)
	}
	if resolved.Definition.Function.Name != binding.StableKey {
		t.Fatalf("unexpected resolved tool key: %q", resolved.Definition.Function.Name)
	}
	metas := registry.FindByOriginalName("open_doc")
	if len(metas) != 1 {
		t.Fatalf("expected one metadata item, got %d", len(metas))
	}
	if metas[0].StableToolKey != binding.StableKey {
		t.Fatalf("unexpected metadata stable key: %q", metas[0].StableToolKey)
	}
	if metas[0].Source != toolspkg.RegistrationSourceExtension {
		t.Fatalf("unexpected metadata source: %q", metas[0].Source)
	}
	if metas[0].ExtensionID != "skill.review" {
		t.Fatalf("unexpected metadata extension id: %q", metas[0].ExtensionID)
	}
	if metas[0].OriginalToolName != "open_doc" {
		t.Fatalf("unexpected original name: %q", metas[0].OriginalToolName)
	}
}

func TestRegisterBridgedToolRejectsBuiltinOriginalNameConflict(t *testing.T) {
	registry := toolspkg.DefaultRegistry()
	_, err := RegisterBridgedTool(registry, ExtensionTool{
		Source:      ExtensionSkill,
		ExtensionID: "skill.review",
		Tool:        bridgeTestTool{name: "read_file"},
	})
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	var regErr *toolspkg.RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != toolspkg.RegistryErrorDuplicateName {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.ConflictWith.Source != toolspkg.RegistrationSourceBuiltin {
		t.Fatalf("unexpected conflict source: %+v", regErr.ConflictWith)
	}
	if regErr.ConflictWith.OriginalToolName != "read_file" {
		t.Fatalf("unexpected conflict original name: %q", regErr.ConflictWith.OriginalToolName)
	}
}

func TestRegisterBridgedToolWithOptionsAllowsBuiltinOriginalNameShadow(t *testing.T) {
	registry := toolspkg.DefaultRegistry()
	binding, err := RegisterBridgedToolWithOptions(registry, ExtensionTool{
		Source:      ExtensionSkill,
		ExtensionID: "skill.review",
		Tool:        bridgeTestTool{name: "read_file"},
	}, BridgeRegisterOptions{
		AllowOriginalNameShadowBuiltin: true,
	})
	if err != nil {
		t.Fatalf("expected bridge registration to allow builtin shadow, got %v", err)
	}
	if binding.StableKey == "" {
		t.Fatal("expected stable key")
	}
	if _, ok := registry.Get(binding.StableKey); !ok {
		t.Fatalf("expected bridged tool %q to be registered", binding.StableKey)
	}
}

func TestRegisterBridgedToolAllowsCrossExtensionOriginalNameReuse(t *testing.T) {
	registry := &toolspkg.Registry{}
	_, err := RegisterBridgedTool(registry, ExtensionTool{
		Source:      ExtensionSkill,
		ExtensionID: "skill.first",
		Tool:        bridgeTestTool{name: "open_doc"},
	})
	if err != nil {
		t.Fatalf("first RegisterBridgedTool failed: %v", err)
	}
	_, err = RegisterBridgedTool(registry, ExtensionTool{
		Source:      ExtensionSkill,
		ExtensionID: "skill.second",
		Tool:        bridgeTestTool{name: "open_doc"},
	})
	if err != nil {
		t.Fatalf("expected cross-extension reuse to succeed, got %v", err)
	}
	metas := registry.FindByOriginalName("open_doc")
	if len(metas) != 2 {
		t.Fatalf("expected two extension records for open_doc, got %#v", metas)
	}
}

func TestRegisterBridgedToolPropagatesSchemaValidationError(t *testing.T) {
	registry := &toolspkg.Registry{}
	_, err := RegisterBridgedTool(registry, ExtensionTool{
		Source:      ExtensionSkill,
		ExtensionID: "skill.review",
		Tool:        bridgeInvalidSpecTool{bridgeTestTool{name: "invalid_schema"}},
	})
	if err == nil {
		t.Fatal("expected schema error")
	}
	var regErr *toolspkg.RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != toolspkg.RegistryErrorInvalidSchema {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
}

func TestResolvePolicyToolSetsMapsOriginalNamesToStableKeys(t *testing.T) {
	allow, deny, err := ResolvePolicyToolSets(skillspkg.ToolPolicy{
		Policy: skillspkg.ToolPolicyAllowlist,
		Items:  []string{"open_doc", "read_file"},
	}, []BridgeBinding{{
		Source:       ExtensionSkill,
		ExtensionID:  "skill.review",
		OriginalName: "open_doc",
		StableKey:    "skill:skill_review:open_doc",
	}})
	if err != nil {
		t.Fatalf("ResolvePolicyToolSets failed: %v", err)
	}
	if deny != nil {
		t.Fatalf("expected nil deny set, got %#v", deny)
	}
	if _, ok := allow["skill:skill_review:open_doc"]; !ok {
		t.Fatalf("expected mapped stable key in allow set: %#v", allow)
	}
	if _, ok := allow["read_file"]; !ok {
		t.Fatalf("expected builtin tool in allow set: %#v", allow)
	}
	if _, ok := allow["open_doc"]; ok {
		t.Fatalf("did not expect legacy name after mapping: %#v", allow)
	}
}

func TestResolvePolicyToolSetsMapsAliasesCaseInsensitively(t *testing.T) {
	allow, deny, err := ResolvePolicyToolSets(skillspkg.ToolPolicy{
		Policy: skillspkg.ToolPolicyAllowlist,
		Items:  []string{"OPEN_DOC"},
	}, []BridgeBinding{{
		Source:       ExtensionSkill,
		ExtensionID:  "skill.review",
		OriginalName: "open_doc",
		StableKey:    "skill:skill_review:open_doc",
	}})
	if err != nil {
		t.Fatalf("ResolvePolicyToolSets failed: %v", err)
	}
	if deny != nil {
		t.Fatalf("expected nil deny set, got %#v", deny)
	}
	if _, ok := allow["skill:skill_review:open_doc"]; !ok {
		t.Fatalf("expected mapped stable key in allow set: %#v", allow)
	}
}

func TestResolvePolicyToolSetsDenylistIncludesOriginalAndStableKeys(t *testing.T) {
	allow, deny, err := ResolvePolicyToolSets(skillspkg.ToolPolicy{
		Policy: skillspkg.ToolPolicyDenylist,
		Items:  []string{"OPEN_DOC"},
	}, []BridgeBinding{{
		Source:       ExtensionSkill,
		ExtensionID:  "skill.review",
		OriginalName: "open_doc",
		StableKey:    "skill:skill_review:open_doc",
	}})
	if err != nil {
		t.Fatalf("ResolvePolicyToolSets failed: %v", err)
	}
	if allow != nil {
		t.Fatalf("expected nil allow set, got %#v", allow)
	}
	if _, ok := deny["skill:skill_review:open_doc"]; !ok {
		t.Fatalf("expected stable key in deny set: %#v", deny)
	}
	if _, ok := deny["open_doc"]; !ok {
		t.Fatalf("expected original key in deny set: %#v", deny)
	}
}

func TestResolvePolicyToolSetsRejectsAmbiguousAliases(t *testing.T) {
	_, _, err := ResolvePolicyToolSets(skillspkg.ToolPolicy{
		Policy: skillspkg.ToolPolicyAllowlist,
		Items:  []string{"open_doc"},
	}, []BridgeBinding{
		{
			Source:       ExtensionSkill,
			ExtensionID:  "skill.first",
			OriginalName: "open_doc",
			StableKey:    "skill:skill_first:open_doc",
		},
		{
			Source:       ExtensionMCP,
			ExtensionID:  "mcp.docs",
			OriginalName: "open_doc",
			StableKey:    "mcp:mcp_docs:open_doc",
		},
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var extErr *ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeConflict {
		t.Fatalf("unexpected error code: %s", extErr.Code)
	}
}

func TestResolvePolicyToolSetsRejectsAmbiguousAliasesIgnoringCase(t *testing.T) {
	_, _, err := ResolvePolicyToolSets(skillspkg.ToolPolicy{
		Policy: skillspkg.ToolPolicyAllowlist,
		Items:  []string{"open_doc"},
	}, []BridgeBinding{
		{
			Source:       ExtensionSkill,
			ExtensionID:  "skill.first",
			OriginalName: "Open_Doc",
			StableKey:    "skill:skill_first:open_doc",
		},
		{
			Source:       ExtensionMCP,
			ExtensionID:  "mcp.docs",
			OriginalName: "open_doc",
			StableKey:    "mcp:mcp_docs:open_doc",
		},
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var extErr *ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeConflict {
		t.Fatalf("unexpected error code: %s", extErr.Code)
	}
}

type bridgeTestTool struct {
	name string
}

func (t bridgeTestTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        t.name,
			Description: "bridge test tool",
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

func (bridgeTestTool) Run(context.Context, json.RawMessage, *toolspkg.ExecutionContext) (string, error) {
	return "ok", nil
}

type bridgeInvalidSpecTool struct {
	bridgeTestTool
}

func (t bridgeInvalidSpecTool) Spec() toolspkg.ToolSpec {
	return toolspkg.ToolSpec{
		Name:        t.name,
		SafetyClass: "invalid",
	}
}
