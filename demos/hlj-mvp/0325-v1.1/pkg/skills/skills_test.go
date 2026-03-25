package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintHelpIncludesInvocationPatterns(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "review")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("name: review\ndescription: review code\n"), 0644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(root)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}

	help := manager.PrintHelp()
	for _, expected := range []string{
		"/技能名",
		"/技能名 <任务>",
		"退出当前技能",
	} {
		if !strings.Contains(help, expected) {
			t.Fatalf("expected help to contain %q, got %s", expected, help)
		}
	}
}
