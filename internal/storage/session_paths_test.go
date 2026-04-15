package storage

import (
	"strings"
	"testing"
)

func TestWorkspaceProjectIDProducesStablePathSafeValue(t *testing.T) {
	first := WorkspaceProjectID(`C:\\Users\\Wheat\\Desktop\\Demo Repo`)
	second := WorkspaceProjectID(`C:\\Users\\wheat\\Desktop\\Demo Repo`)
	if first != second {
		t.Fatalf("expected stable id normalization, got %q vs %q", first, second)
	}
	if !strings.HasPrefix(first, "-") {
		t.Fatalf("expected project id to be prefixed with '-', got %q", first)
	}
	if strings.Contains(first, "\\") || strings.Contains(first, "/") || strings.Contains(first, ":") {
		t.Fatalf("expected path-safe project id, got %q", first)
	}
}

func TestWorkspaceProjectIDEmptyWorkspace(t *testing.T) {
	if got := WorkspaceProjectID(""); got != "-unknown-project" {
		t.Fatalf("expected -unknown-project, got %q", got)
	}
}
