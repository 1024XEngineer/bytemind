package tools

import (
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

func TestDefaultToolSpecDerivesReadOnlyAndPlanModes(t *testing.T) {
	spec := DefaultToolSpec(llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name: "read_file",
		},
	})

	if !spec.ReadOnly {
		t.Fatal("expected read_file to be read-only")
	}
	if !spec.StrictArgs {
		t.Fatal("expected strict args to default true")
	}
	if !modeAllowed(spec, planpkg.ModePlan) {
		t.Fatal("expected read_file to be available in plan mode")
	}
}

func TestNormalizeToolSpecFillsDefaults(t *testing.T) {
	spec := NormalizeToolSpec(ToolSpec{
		Name:         "custom_tool",
		AllowedModes: []planpkg.AgentMode{planpkg.ModeBuild, planpkg.ModeBuild, planpkg.ModePlan},
	})

	if spec.DefaultTimeoutS != 30 {
		t.Fatalf("unexpected default timeout: %d", spec.DefaultTimeoutS)
	}
	if spec.MaxTimeoutS != 300 {
		t.Fatalf("unexpected max timeout: %d", spec.MaxTimeoutS)
	}
	if len(spec.AllowedModes) != 2 {
		t.Fatalf("expected deduplicated modes, got %#v", spec.AllowedModes)
	}
}

func TestNormalizeToolSpecMarksDestructiveFromSafetyClass(t *testing.T) {
	spec := NormalizeToolSpec(ToolSpec{
		Name:        "custom_write_tool",
		SafetyClass: SafetyClassDestructive,
	})
	if !spec.Destructive {
		t.Fatal("expected destructive safety class to imply destructive tool")
	}
}

func TestValidateToolSpecRejectsConflictingFlags(t *testing.T) {
	err := ValidateToolSpec(ToolSpec{
		Name:            "bad_tool",
		ReadOnly:        true,
		Destructive:     true,
		SafetyClass:     SafetyClassSafe,
		AllowedModes:    []planpkg.AgentMode{planpkg.ModeBuild},
		DefaultTimeoutS: 1,
		MaxTimeoutS:     1,
		MaxResultChars:  1,
	})
	if err == nil {
		t.Fatal("expected spec validation error")
	}
}

func TestMergeToolSpecPreservesDefaultsWhenOverrideLeavesFieldsEmpty(t *testing.T) {
	base := DefaultToolSpec(llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name: "run_shell",
		},
	})

	merged := MergeToolSpec(base, ToolSpec{
		MaxResultChars: 1024,
	})

	if merged.Name != "run_shell" {
		t.Fatalf("expected default name to survive merge, got %q", merged.Name)
	}
	if merged.MaxResultChars != 1024 {
		t.Fatalf("expected override max result chars, got %d", merged.MaxResultChars)
	}
	if merged.DefaultTimeoutS != base.DefaultTimeoutS {
		t.Fatalf("expected default timeout to survive merge, got %d", merged.DefaultTimeoutS)
	}
	if !merged.ConcurrencySafe {
		t.Fatal("expected default concurrency safety to survive merge")
	}
}
