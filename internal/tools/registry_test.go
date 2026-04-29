package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

func TestDefaultRegistryDefinitionsAreSortedAndComplete(t *testing.T) {
	registry := DefaultRegistry()
	defs := registry.Definitions()
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Function.Name)
	}

	want := []string{
		"apply_patch",
		"list_files",
		"read_file",
		"replace_in_file",
		"run_shell",
		"search_text",
		"update_plan",
		"web_fetch",
		"web_search",
		"write_file",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("unexpected definition order or contents: got=%v want=%v", names, want)
	}
}

func TestRegistryResolveForModeReturnsUnknownToolError(t *testing.T) {
	registry := DefaultRegistry()
	_, err := registry.ResolveForMode(planpkg.ModeBuild, "missing_tool")
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
	if !strings.Contains(err.Error(), `unknown tool "missing_tool"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefinitionsForModeWithFiltersRestrictsTools(t *testing.T) {
	registry := DefaultRegistry()
	defs := registry.DefinitionsForModeWithFilters("build", []string{"read_file", "search_text"}, []string{"search_text"})
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Function.Name)
	}
	want := []string{"read_file"}
	if !slices.Equal(names, want) {
		t.Fatalf("unexpected filtered tools: got=%v want=%v", names, want)
	}
}

func TestDefaultRegistryDefinitionsForPlanModeIncludeWebTools(t *testing.T) {
	registry := DefaultRegistry()
	defs := registry.DefinitionsForMode(planpkg.ModePlan)
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Function.Name)
	}

	for _, allowed := range []string{"list_files", "read_file", "search_text", "web_search", "web_fetch", "update_plan", "run_shell"} {
		if !slices.Contains(names, allowed) {
			t.Fatalf("expected %q in plan mode definitions, got %v", allowed, names)
		}
	}
	if slices.Contains(names, "write_file") {
		t.Fatalf("did not expect write_file in plan mode definitions: %v", names)
	}
}

func TestRegistryRegisterRejectsDuplicateName(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "dup_tool"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	err := registry.Register(testTool{name: "dup_tool"}, RegisterOptions{Source: RegistrationSourceExtension, ExtensionID: "skill.demo"})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Fatalf("unexpected error type: %T", err)
	}
	if regErr.Code != RegistryErrorDuplicateName {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
	if regErr.ConflictWith.Source != RegistrationSourceBuiltin {
		t.Fatalf("unexpected conflict source: %+v", regErr.ConflictWith)
	}
}

func TestRegistryGetReturnsClonedDefinition(t *testing.T) {
	registry := &Registry{}
	tool := testTool{
		name: "clone_tool",
		parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
	}
	if err := registry.Register(tool, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	resolved, ok := registry.Get("clone_tool")
	if !ok {
		t.Fatal("expected tool")
	}
	resolved.Definition.Function.Parameters["type"] = "array"
	props := resolved.Definition.Function.Parameters["properties"].(map[string]any)
	props["extra"] = map[string]any{"type": "number"}
	resolvedAgain, ok := registry.Get("clone_tool")
	if !ok {
		t.Fatal("expected tool on second get")
	}
	if resolvedAgain.Definition.Function.Parameters["type"] != "object" {
		t.Fatalf("registry definition mutated: %#v", resolvedAgain.Definition.Function.Parameters)
	}
	propsAgain := resolvedAgain.Definition.Function.Parameters["properties"].(map[string]any)
	if _, exists := propsAgain["extra"]; exists {
		t.Fatalf("registry nested parameters mutated: %#v", propsAgain)
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	registry := &Registry{}
	const count = 32
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := testToolName(i)
			if err := registry.Register(testTool{name: name}, RegisterOptions{Source: RegistrationSourceExtension, ExtensionID: "skill.concurrent"}); err != nil {
				t.Errorf("register %s: %v", name, err)
				return
			}
			registry.Get(name)
			registry.List()
			if err := registry.Unregister(name); err != nil {
				t.Errorf("unregister %s: %v", name, err)
			}
		}(i)
	}
	wg.Wait()
	if got := registry.List(); len(got) != 0 {
		t.Fatalf("expected empty registry, got %d tools", len(got))
	}
}

func TestRegistryGetReturnsClonedMapStringSliceValues(t *testing.T) {
	registry := &Registry{}
	tool := testTool{
		name: "typed_map_clone_tool",
		parameters: map[string]any{
			"type":   "object",
			"groups": map[string][]string{"alpha": {"a", "b"}},
		},
	}
	if err := registry.Register(tool, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	resolved, ok := registry.Get("typed_map_clone_tool")
	if !ok {
		t.Fatal("expected tool")
	}
	resolved.Definition.Function.Parameters["groups"].(map[string][]string)["alpha"][0] = "mutated"
	resolvedAgain, ok := registry.Get("typed_map_clone_tool")
	if !ok {
		t.Fatal("expected tool on second get")
	}
	if got := resolvedAgain.Definition.Function.Parameters["groups"].(map[string][]string)["alpha"][0]; got != "a" {
		t.Fatalf("registry typed map slice mutated: %#v", resolvedAgain.Definition.Function.Parameters["groups"])
	}
}

func TestRegistryGetReturnsClonedTypedSchemaValues(t *testing.T) {
	registry := &Registry{}
	tool := testTool{
		name: "typed_clone_tool",
		parameters: map[string]any{
			"type":     "object",
			"required": []string{"path"},
			"properties": map[string]any{
				"path": map[string]any{
					"type": "string",
					"enum": []string{"a", "b"},
				},
			},
		},
	}
	if err := registry.Register(tool, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	resolved, ok := registry.Get("typed_clone_tool")
	if !ok {
		t.Fatal("expected tool")
	}
	resolved.Definition.Function.Parameters["required"].([]string)[0] = "mutated"
	resolvedProps := resolved.Definition.Function.Parameters["properties"].(map[string]any)
	resolvedProps["path"].(map[string]any)["enum"].([]string)[0] = "mutated"
	resolvedAgain, ok := registry.Get("typed_clone_tool")
	if !ok {
		t.Fatal("expected tool on second get")
	}
	if got := resolvedAgain.Definition.Function.Parameters["required"].([]string)[0]; got != "path" {
		t.Fatalf("registry required slice mutated: %#v", resolvedAgain.Definition.Function.Parameters["required"])
	}
	propsAgain := resolvedAgain.Definition.Function.Parameters["properties"].(map[string]any)
	if got := propsAgain["path"].(map[string]any)["enum"].([]string)[0]; got != "a" {
		t.Fatalf("registry enum slice mutated: %#v", propsAgain["path"])
	}
}

func TestRegistryGetRejectsBlankName(t *testing.T) {
	registry := &Registry{}
	if _, ok := registry.Get("   "); ok {
		t.Fatal("expected blank lookup to fail")
	}
}

func TestRegistrySpecRejectsUnknownName(t *testing.T) {
	registry := &Registry{}
	if _, ok := registry.Spec("missing"); ok {
		t.Fatal("expected missing spec lookup to fail")
	}
}

func TestRegistryUnregisterRejectsBlankName(t *testing.T) {
	registry := &Registry{}
	err := registry.Unregister("   ")
	if err == nil {
		t.Fatal("expected invalid tool key error")
	}
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Fatalf("unexpected error type: %T", err)
	}
	if regErr.Code != RegistryErrorInvalidToolKey {
		t.Fatalf("unexpected code: %s", regErr.Code)
	}
}

func TestRegistryListReturnsSortedSnapshot(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "z_tool"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register z_tool failed: %v", err)
	}
	if err := registry.Register(testTool{name: "a_tool"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register a_tool failed: %v", err)
	}
	items := registry.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(items))
	}
	if got := []string{items[0].Definition.Function.Name, items[1].Definition.Function.Name}; !slices.Equal(got, []string{"a_tool", "z_tool"}) {
		t.Fatalf("unexpected order: %v", got)
	}
	items[0].Definition.Function.Parameters["type"] = "array"
	itemsAgain := registry.List()
	if itemsAgain[0].Definition.Function.Parameters["type"] != "object" {
		t.Fatalf("registry list snapshot mutated: %#v", itemsAgain[0].Definition.Function.Parameters)
	}
}

func TestRegistryFindByOriginalName(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "alpha_tool"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register alpha_tool failed: %v", err)
	}
	if err := registry.Register(testTool{name: "skill:skill_demo:open_doc"}, RegisterOptions{
		Source:       RegistrationSourceExtension,
		ExtensionID:  "skill.demo",
		OriginalName: "open_doc",
	}); err != nil {
		t.Fatalf("register bridged tool failed: %v", err)
	}
	metas := registry.FindByOriginalName("open_doc")
	if len(metas) != 1 {
		t.Fatalf("expected one metadata record, got %d", len(metas))
	}
	if metas[0].ToolKey != "skill:skill_demo:open_doc" {
		t.Fatalf("unexpected tool key: %q", metas[0].ToolKey)
	}
	if metas[0].OriginalToolName != "open_doc" {
		t.Fatalf("unexpected original tool name: %q", metas[0].OriginalToolName)
	}
	if metas[0].Source != RegistrationSourceExtension {
		t.Fatalf("unexpected source: %q", metas[0].Source)
	}
	if got := registry.FindByOriginalName("missing"); len(got) != 0 {
		t.Fatalf("expected no metadata for missing original name, got %#v", got)
	}
}

func TestRegistryRegisterRejectsCaseInsensitiveOriginalNameConflict(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "read_file"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register builtin failed: %v", err)
	}
	err := registry.Register(testTool{name: "skill:skill_demo:read_file"}, RegisterOptions{
		Source:       RegistrationSourceExtension,
		ExtensionID:  "skill.demo",
		OriginalName: "Read_File",
	})
	if err == nil {
		t.Fatal("expected duplicate original-name error")
	}
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Fatalf("unexpected error type: %T", err)
	}
	if regErr.Code != RegistryErrorDuplicateName {
		t.Fatalf("unexpected error code: %s", regErr.Code)
	}
	if regErr.OriginalToolName != "read_file" {
		t.Fatalf("expected normalized original name, got %q", regErr.OriginalToolName)
	}
}

func TestRegistryRegisterAllowsShadowBuiltinOriginalNameWhenOptedIn(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "read_file"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register builtin failed: %v", err)
	}
	if err := registry.Register(testTool{name: "skill:skill_demo:read_file"}, RegisterOptions{
		Source:                         RegistrationSourceExtension,
		ExtensionID:                    "skill.demo",
		OriginalName:                   "read_file",
		AllowOriginalNameShadowBuiltin: true,
	}); err != nil {
		t.Fatalf("register extension shadow tool failed: %v", err)
	}

	metas := registry.FindByOriginalName("read_file")
	if len(metas) != 2 {
		t.Fatalf("expected two metadata records for read_file, got %#v", metas)
	}
}

func TestRegistryRegisterShadowBuiltinAllowsExistingExtensionConflictAcrossExtensions(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "read_file"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register builtin failed: %v", err)
	}
	if err := registry.Register(testTool{name: "skill:skill_review:read_file"}, RegisterOptions{
		Source:                         RegistrationSourceExtension,
		ExtensionID:                    "skill.review",
		OriginalName:                   "read_file",
		AllowOriginalNameShadowBuiltin: true,
	}); err != nil {
		t.Fatalf("register first extension shadow tool failed: %v", err)
	}
	if err := registry.Register(testTool{name: "skill:skill_plan:read_file"}, RegisterOptions{
		Source:                         RegistrationSourceExtension,
		ExtensionID:                    "skill.plan",
		OriginalName:                   "read_file",
		AllowOriginalNameShadowBuiltin: true,
	}); err != nil {
		t.Fatalf("register second extension shadow tool failed: %v", err)
	}
	metas := registry.FindByOriginalName("read_file")
	if len(metas) != 3 {
		t.Fatalf("expected builtin + 2 extension records, got %#v", metas)
	}
}

func TestRegistryRegisterShadowBuiltinRejectsExistingExtensionConflictWithinSameExtension(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "read_file"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register builtin failed: %v", err)
	}
	if err := registry.Register(testTool{name: "skill:skill_review:read_file"}, RegisterOptions{
		Source:                         RegistrationSourceExtension,
		ExtensionID:                    "skill.review",
		OriginalName:                   "read_file",
		AllowOriginalNameShadowBuiltin: true,
	}); err != nil {
		t.Fatalf("register first extension shadow tool failed: %v", err)
	}
	err := registry.Register(testTool{name: "skill:skill_review_v2:read_file"}, RegisterOptions{
		Source:                         RegistrationSourceExtension,
		ExtensionID:                    "skill.review",
		OriginalName:                   "read_file",
		AllowOriginalNameShadowBuiltin: true,
	})
	if err == nil {
		t.Fatal("expected duplicate original-name error within same extension")
	}
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Fatalf("expected RegistryError, got %T", err)
	}
	if regErr.Code != RegistryErrorDuplicateName {
		t.Fatalf("expected duplicate_name, got %s", regErr.Code)
	}
	if regErr.ConflictWith.Source != RegistrationSourceExtension {
		t.Fatalf("expected extension conflict source, got %+v", regErr.ConflictWith)
	}
	if regErr.ConflictWith.ExtensionID != "skill.review" {
		t.Fatalf("expected same extension conflict, got %+v", regErr.ConflictWith)
	}
}

func TestRegistryFindByOriginalNameIsCaseInsensitive(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "skill:skill_demo:open_doc"}, RegisterOptions{
		Source:       RegistrationSourceExtension,
		ExtensionID:  "skill.demo",
		OriginalName: "Open_Doc",
	}); err != nil {
		t.Fatalf("register bridged tool failed: %v", err)
	}
	metas := registry.FindByOriginalName("OPEN_doc")
	if len(metas) != 1 {
		t.Fatalf("expected one metadata record, got %d", len(metas))
	}
	if metas[0].OriginalToolName != "open_doc" {
		t.Fatalf("expected normalized original name, got %q", metas[0].OriginalToolName)
	}
}

func TestRegistryFindByExtensionID(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(testTool{name: "read_file"}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register builtin failed: %v", err)
	}
	if err := registry.Register(testTool{name: "skill:skill_demo:open_doc"}, RegisterOptions{
		Source:       RegistrationSourceExtension,
		ExtensionID:  "skill.demo",
		OriginalName: "open_doc",
	}); err != nil {
		t.Fatalf("register extension tool failed: %v", err)
	}
	if err := registry.Register(testTool{name: "skill:skill_other:search_doc"}, RegisterOptions{
		Source:       RegistrationSourceExtension,
		ExtensionID:  "skill.other",
		OriginalName: "search_doc",
	}); err != nil {
		t.Fatalf("register second extension tool failed: %v", err)
	}
	metas := registry.FindByExtensionID("skill.demo")
	if len(metas) != 1 {
		t.Fatalf("expected one metadata for extension, got %d", len(metas))
	}
	if metas[0].ToolKey != "skill:skill_demo:open_doc" {
		t.Fatalf("unexpected tool key: %q", metas[0].ToolKey)
	}
	if got := registry.FindByExtensionID("missing"); len(got) != 0 {
		t.Fatalf("expected no metadata for missing extension, got %#v", got)
	}
}

func TestRegistryResolveForModeRejectsUnavailableTool(t *testing.T) {
	registry := &Registry{}
	if err := registry.Register(invalidSpecTool{
		testTool: testTool{name: "build_only_tool"},
		spec:     ToolSpec{Name: "build_only_tool", AllowedModes: []planpkg.AgentMode{planpkg.ModeBuild}},
	}, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	_, err := registry.ResolveForMode(planpkg.ModePlan, "build_only_tool")
	if err == nil {
		t.Fatal("expected permission error")
	}
}

func TestRegistryDefinitionsUsesBuildMode(t *testing.T) {
	registry := DefaultRegistry()
	defs := registry.Definitions()
	for _, def := range defs {
		if def.Function.Name == "write_file" {
			return
		}
	}
	t.Fatal("expected build definitions to include write_file")
}

func TestRegistryErrorHelpersAndCloneCoverage(t *testing.T) {
	if (&RegistryError{}).Error() != "" {
		t.Fatal("expected zero-value error string to be empty")
	}
	if (*RegistryError)(nil).Error() != "" {
		t.Fatal("expected nil registry error string to be empty")
	}
	if (*RegistryError)(nil).Unwrap() != nil {
		t.Fatal("expected nil registry error unwrap to be nil")
	}
	cause := fmt.Errorf("boom")
	if (&RegistryError{Cause: cause}).Unwrap() != cause {
		t.Fatal("expected unwrap to return cause")
	}
	if normalizeRegistrationSource(" builtin ") != RegistrationSourceBuiltin {
		t.Fatal("expected builtin source to normalize")
	}
	if normalizeRegistrationSource(" extension ") != RegistrationSourceExtension {
		t.Fatal("expected extension source to normalize")
	}
	if normalizeRegistrationSource("unknown") != "" {
		t.Fatal("expected unknown source to normalize to empty")
	}
	if cloneAnySlice(nil) != nil {
		t.Fatal("expected nil any slice clone to stay nil")
	}
	if cloneAnyMap(nil) != nil {
		t.Fatal("expected nil any map clone to stay nil")
	}
	if cloneAny(nil) != nil {
		t.Fatal("expected nil cloneAny input to stay nil")
	}
	if cloneAny([]string{"a"}).([]string)[0] != "a" {
		t.Fatal("expected []string clone to preserve values")
	}
	if cloneAny(map[string]string{"k": "v"}).(map[string]string)["k"] != "v" {
		t.Fatal("expected map[string]string clone to preserve values")
	}
}

type uncomparableTool struct {
	name    string
	payload map[string]string
}

func (t uncomparableTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        t.name,
			Description: "uncomparable test tool",
			Parameters:  map[string]any{"type": "object", "payload": t.payload},
		},
	}
}

func (uncomparableTool) Run(context.Context, json.RawMessage, *ExecutionContext) (string, error) {
	return "ok", nil
}

type testTool struct {
	name       string
	parameters map[string]any
}

type invalidSpecTool struct {
	testTool
	spec ToolSpec
}

func (t invalidSpecTool) Spec() ToolSpec {
	return t.spec
}

func (t testTool) Definition() llm.ToolDefinition {
	parameters := t.parameters
	if parameters == nil {
		parameters = map[string]any{"type": "object"}
	}
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        t.name,
			Description: "test tool",
			Parameters:  parameters,
		},
	}
}

func (testTool) Run(context.Context, json.RawMessage, *ExecutionContext) (string, error) {
	return "ok", nil
}

func testToolName(i int) string {
	return fmt.Sprintf("tool_concurrent_%d", i)
}
