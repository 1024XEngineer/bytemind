package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

func TestListFilesToolRespectsDepthLimitAndHiddenFiles(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "visible.txt"), "visible")
	mustWriteFile(t, filepath.Join(workspace, ".hidden.txt"), "hidden")
	mustMkdirAll(t, filepath.Join(workspace, "dir", "nested"))
	mustWriteFile(t, filepath.Join(workspace, "dir", "nested", "deep.txt"), "deep")

	tool := ListFilesTool{}
	payload, _ := json.Marshal(map[string]any{
		"depth": 1,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Root  string `json:"root"`
		Items []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}

	paths := make([]string, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		paths = append(paths, item.Path)
	}
	if parsed.Root != "." {
		t.Fatalf("expected root '.', got %q", parsed.Root)
	}
	for _, want := range []string{"visible.txt", "dir"} {
		if !slices.Contains(paths, want) {
			t.Fatalf("expected %q in items, got %v", want, paths)
		}
	}
	for _, unwanted := range []string{".hidden.txt", "dir/nested", "dir/nested/deep.txt"} {
		if slices.Contains(paths, unwanted) {
			t.Fatalf("did not expect %q in items, got %v", unwanted, paths)
		}
	}
}

func TestListFilesToolIncludesHiddenAndAppliesLimit(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, ".hidden.txt"), "hidden")
	mustWriteFile(t, filepath.Join(workspace, "a.txt"), "a")
	mustWriteFile(t, filepath.Join(workspace, "b.txt"), "b")

	tool := ListFilesTool{}
	payload, _ := json.Marshal(map[string]any{
		"include_hidden": true,
		"limit":          2,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Items []struct {
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Items) != 2 {
		t.Fatalf("expected 2 items, got %#v", parsed.Items)
	}
	paths := []string{parsed.Items[0].Path, parsed.Items[1].Path}
	if !slices.Contains(paths, ".hidden.txt") {
		t.Fatalf("expected hidden file in limited results, got %v", paths)
	}
}

func TestListFilesToolRejectsEscapedPath(t *testing.T) {
	workspace := t.TempDir()
	tool := ListFilesTool{}
	payload, _ := json.Marshal(map[string]any{
		"path": filepath.Join("..", "outside"),
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err == nil {
		t.Fatal("expected escaped path error")
	}
}

func TestListFilesToolStopsWhenVisitBudgetExceeded(t *testing.T) {
	workspace := t.TempDir()
	for i := 0; i < 12; i++ {
		mustWriteFile(t, filepath.Join(workspace, "dir", "nested", "f"+string(rune('a'+i))+".txt"), "x")
	}
	t.Setenv("BYTEMIND_LIST_FILES_MAX_VISITS", "4")

	tool := ListFilesTool{}
	payload, _ := json.Marshal(map[string]any{
		"depth": 4,
		"limit": 100,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Items []struct {
			Path string `json:"path"`
		} `json:"items"`
		Truncated bool   `json:"truncated"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.Truncated {
		t.Fatalf("expected truncated result, got %s", result)
	}
	if parsed.Reason != "visit_limit" {
		t.Fatalf("expected visit_limit reason, got %q", parsed.Reason)
	}
}
