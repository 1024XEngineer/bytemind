package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRejectsEscapingPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.txt"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := Open(root, []string{".git"}, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ws.Resolve("../outside.txt"); err == nil {
		t.Fatal("expected resolve to reject escaping path")
	}
}
