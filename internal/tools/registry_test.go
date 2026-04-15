package tools

import (
	"slices"
	"strings"
	"testing"

	planpkg "bytemind/internal/plan"
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
