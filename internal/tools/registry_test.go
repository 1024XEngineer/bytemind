package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

type recordingTool struct {
	name    string
	lastRaw string
	result  string
}

func (t *recordingTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name: t.name,
		},
	}
}

func (t *recordingTool) Run(_ context.Context, raw json.RawMessage, _ *ExecutionContext) (string, error) {
	t.lastRaw = string(raw)
	return t.result, nil
}

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

func TestRegistryExecuteReturnsUnknownToolError(t *testing.T) {
	registry := DefaultRegistry()
	_, err := registry.Execute(context.Background(), "missing_tool", `{}`, &ExecutionContext{})
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
	if !strings.Contains(err.Error(), `unknown tool "missing_tool"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegistryExecuteDefaultsEmptyArgsToJSONObject(t *testing.T) {
	tool := &recordingTool{name: "fake_tool", result: `{"ok":true}`}
	registry := &Registry{tools: map[string]ResolvedTool{}}
	registry.Add(tool)

	result, err := registry.Execute(context.Background(), "fake_tool", "", &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if result != `{"ok":true}` {
		t.Fatalf("unexpected result %q", result)
	}
	if tool.lastRaw != "{}" {
		t.Fatalf("expected empty args to default to {}, got %q", tool.lastRaw)
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

func TestRegistryExecuteRespectsActiveSkillPolicy(t *testing.T) {
	tool := &recordingTool{name: "fake_tool", result: `{"ok":true}`}
	registry := &Registry{tools: map[string]ResolvedTool{}}
	registry.Add(tool)

	_, err := registry.ExecuteForMode(context.Background(), "build", "fake_tool", `{}`, &ExecutionContext{
		AllowedTools: map[string]struct{}{
			"read_file": {},
		},
	})
	if err == nil {
		t.Fatal("expected active skill policy to block disallowed tool")
	}
	if !strings.Contains(err.Error(), "active skill policy") {
		t.Fatalf("unexpected error: %v", err)
	}
}
