package plan

import (
	"testing"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

func TestNormalizeModeMapsLegacyLabelsToBuild(t *testing.T) {
	legacy := []string{"default", "acceptEdits", "bypassPermissions"}
	for _, raw := range legacy {
		if got := NormalizeMode(raw); got != ModeBuild {
			t.Fatalf("NormalizeMode(%q) = %q, want %q", raw, got, ModeBuild)
		}
	}
}

func TestAgentModeIsCoreSessionModeAlias(t *testing.T) {
	var mode AgentMode = corepkg.SessionModePlan
	if mode != ModePlan {
		t.Fatalf("expected alias assignment to preserve mode, got %q", mode)
	}

	var coreMode corepkg.SessionMode = ModeBuild
	if coreMode != corepkg.SessionModeBuild {
		t.Fatalf("expected reverse assignment to preserve build mode, got %q", coreMode)
	}
}
