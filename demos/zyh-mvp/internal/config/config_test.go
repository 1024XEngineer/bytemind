package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadSupportsUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "forgecli.json")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"workspace_root":".","model":{"model":"demo-model"}}`)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkspaceRoot != "." {
		t.Fatalf("unexpected workspace root: %s", cfg.WorkspaceRoot)
	}
	if cfg.Model.Model != "demo-model" {
		t.Fatalf("unexpected model: %s", cfg.Model.Model)
	}
}

func TestDefaultIgnoresGoTempDirs(t *testing.T) {
	cfg := Default()
	if !slices.Contains(cfg.SearchIgnore, ".gocache") {
		t.Fatal("expected .gocache to be ignored")
	}
	if !slices.Contains(cfg.SearchIgnore, ".gotmp") {
		t.Fatal("expected .gotmp to be ignored")
	}
}
