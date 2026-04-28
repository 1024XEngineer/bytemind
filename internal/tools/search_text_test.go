package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSearchTextToolFindsMatchesCaseInsensitiveAndSkipsHiddenDirsAndBinary(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "a.txt"), "Alpha\nbeta\n")
	mustWriteFile(t, filepath.Join(workspace, "b.txt"), "alpha again\n")
	mustWriteFile(t, filepath.Join(workspace, ".hidden", "ignored.txt"), "alpha hidden\n")
	mustWriteBinaryFile(t, filepath.Join(workspace, "data.bin"), []byte{'a', 0, 'b'})

	tool := SearchTextTool{}
	payload, _ := json.Marshal(map[string]any{
		"query": "ALPHA",
		"limit": 2,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Query   string `json:"query"`
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Query != "ALPHA" {
		t.Fatalf("expected original query, got %q", parsed.Query)
	}
	if len(parsed.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %#v", parsed.Matches)
	}
	paths := []string{parsed.Matches[0].Path, parsed.Matches[1].Path}
	for _, unwanted := range []string{".hidden/ignored.txt", "data.bin"} {
		if slices.Contains(paths, unwanted) {
			t.Fatalf("did not expect %q in matches, got %v", unwanted, paths)
		}
	}
}

func TestSearchTextToolSupportsCaseSensitiveAndSubpathSearch(t *testing.T) {
	workspace := t.TempDir()
	mustMkdirAll(t, filepath.Join(workspace, "src"))
	mustWriteFile(t, filepath.Join(workspace, "src", "main.go"), "Alpha\nalpha\n")
	mustWriteFile(t, filepath.Join(workspace, "other.txt"), "Alpha elsewhere\n")

	tool := SearchTextTool{}
	payload, _ := json.Marshal(map[string]any{
		"query":          "Alpha",
		"path":           "src",
		"case_sensitive": true,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Matches) != 1 {
		t.Fatalf("expected 1 case-sensitive subpath match, got %#v", parsed.Matches)
	}
	if parsed.Matches[0].Path != "src/main.go" || parsed.Matches[0].Line != 1 {
		t.Fatalf("unexpected match %#v", parsed.Matches[0])
	}
}

func TestSearchTextToolRejectsEscapedPath(t *testing.T) {
	workspace := t.TempDir()
	tool := SearchTextTool{}
	payload, _ := json.Marshal(map[string]any{
		"query": "hello",
		"path":  filepath.Join("..", "outside"),
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err == nil {
		t.Fatal("expected escaped path error")
	}
}

func TestSearchTextToolSkipsCommonLargeDirs(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "src", "main.go"), "alpha here\n")
	mustWriteFile(t, filepath.Join(workspace, "node_modules", "pkg", "index.js"), "alpha from deps\n")

	tool := SearchTextTool{}
	payload, _ := json.Marshal(map[string]any{
		"query": "alpha",
		"limit": 10,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Matches []struct {
			Path string `json:"path"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(parsed.Matches))
	for _, item := range parsed.Matches {
		paths = append(paths, item.Path)
	}
	if !slices.Contains(paths, "src/main.go") {
		t.Fatalf("expected src/main.go in matches, got %v", paths)
	}
	if slices.Contains(paths, "node_modules/pkg/index.js") {
		t.Fatalf("did not expect node_modules match, got %v", paths)
	}
}

func TestSearchTextToolStopsWhenFileBudgetExceeded(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "a.txt"), "nope\n")
	mustWriteFile(t, filepath.Join(workspace, "b.txt"), "alpha\n")
	t.Setenv("BYTEMIND_SEARCH_MAX_FILES", "1")

	tool := SearchTextTool{}
	payload, _ := json.Marshal(map[string]any{
		"query": "alpha",
		"limit": 10,
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Matches      []struct{} `json:"matches"`
		Truncated    bool       `json:"truncated"`
		Reason       string     `json:"reason"`
		FilesVisited int        `json:"files_visited"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.Truncated {
		t.Fatalf("expected truncated result, got %s", result)
	}
	if parsed.Reason != "file_limit" {
		t.Fatalf("expected file_limit reason, got %q", parsed.Reason)
	}
	if len(parsed.Matches) != 0 {
		t.Fatalf("expected zero matches after early stop, got %#v", parsed.Matches)
	}
	if parsed.FilesVisited != 2 {
		t.Fatalf("expected files_visited=2 (second file triggers stop), got %d", parsed.FilesVisited)
	}
}

func TestSearchTextCanUseRipgrepBudgetsDisabledByEnvOverride(t *testing.T) {
	t.Setenv("BYTEMIND_SEARCH_MAX_FILES", "1")
	if searchTextCanUseRipgrepBudgets() {
		t.Fatal("expected ripgrep path to be disabled when custom budgets are configured")
	}
}

func TestSearchTextRipgrepTimeoutFromEnv(t *testing.T) {
	t.Setenv("BYTEMIND_SEARCH_RG_TIMEOUT_MS", "25")
	if got := searchTextRipgrepTimeout(); got != 25*time.Millisecond {
		t.Fatalf("expected 25ms timeout, got %s", got)
	}

	t.Setenv("BYTEMIND_SEARCH_RG_TIMEOUT_MS", "invalid")
	if got := searchTextRipgrepTimeout(); got != defaultSearchTextRipgrepTimeout {
		t.Fatalf("expected invalid timeout to use default, got %s", got)
	}
}

func TestNormalizeRipgrepPathResolvesRelativeAndAbsolutePaths(t *testing.T) {
	workspace := t.TempDir()
	base := filepath.Join(workspace, "src")
	relative := normalizeRipgrepPath(workspace, base, filepath.Join("pkg", "main.go"))
	if relative != "src/pkg/main.go" {
		t.Fatalf("unexpected normalized relative path %q", relative)
	}

	absoluteInput := filepath.Join(workspace, "README.md")
	absolute := normalizeRipgrepPath(workspace, base, absoluteInput)
	if absolute != "README.md" {
		t.Fatalf("unexpected normalized absolute path %q", absolute)
	}
}
