package planner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"forgecli/internal/workspace"
)

func TestMockPlannerPrefersSourceFileOverTestFile(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"greeter.go": `package sample

func Greeting() string {
    return "hello"
}
`,
		"greeter_test.go": `package sample

import "testing"

func TestGreeting(t *testing.T) {
    if Greeting() != "hello, forge" {
        t.Fatalf("unexpected greeting: %s", Greeting())
    }
}
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ws, err := workspace.Open(repo, []string{".git"}, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := MockPlanner{}.BuildPlan(context.Background(), "replace hello with hello, forge", ws)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetFile != "greeter.go" {
		t.Fatalf("expected greeter.go, got %s", plan.TargetFile)
	}
}

func TestMockPlannerPrefersExactMentionedPath(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"README.md":       "demo\n",
		".gocache/sample": "cached\n",
	}

	for name, content := range files {
		full := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ws, err := workspace.Open(repo, []string{".git", ".gocache"}, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := MockPlanner{}.BuildPlan(context.Background(), `In README.md replace "demo" with "mvp"`, ws)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetFile != "README.md" {
		t.Fatalf("expected README.md, got %s", plan.TargetFile)
	}
}
