package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/session"
)

func TestApplyPatchToolUpdatesFile(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": "*** Begin Patch\n*** Update File: sample.txt\n@@\n alpha\n-beta\n+gamma\n*** End Patch",
	})
	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha\ngamma\n" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
}

func TestApplyPatchToolPreservesCRLFLineEndings(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\r\nbeta\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": "*** Begin Patch\n*** Update File: sample.txt\n@@\n alpha\n-beta\n+gamma\n*** End Patch",
	})
	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "alpha\r\ngamma\r\n" {
		t.Fatalf("expected CRLF to be preserved, got %q", got)
	}
}

func TestApplyPatchToolUsesHeaderToDisambiguateRepeatedBlock(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	content := "alpha\nbeta\nalpha\nbeta\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": "*** Begin Patch\n*** Update File: sample.txt\n@@ -3,2 +3,2 @@\n alpha\n-beta\n+gamma\n*** End Patch",
	})
	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha\nbeta\nalpha\ngamma\n" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
}

func TestApplyPatchToolAddsFileUsingPatchLineEnding(t *testing.T) {
	workspace := t.TempDir()
	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": strings.Join([]string{
			"*** Begin Patch",
			"*** Add File: sample.txt",
			"+alpha",
			"+beta",
			"*** End Patch",
		}, "\r\n"),
	})

	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "sample.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "alpha\r\nbeta\r\n" {
		t.Fatalf("expected added file to use patch line endings, got %q", got)
	}
}

func FuzzApplyStructuredPatchWithHeader(f *testing.F) {
	seeds := []struct {
		original string
		patch    string
	}{
		{
			original: "one\ntwo\nthree\n",
			patch:    "@@ -2,1 +2,1 @@\n-two\n+TWO",
		},
		{
			original: "alpha\nbeta\ngamma\n",
			patch:    "@@ -1,2 +1,2 @@\n alpha\n-beta\n+delta",
		},
	}
	for _, seed := range seeds {
		f.Add(seed.original, seed.patch)
	}

	f.Fuzz(func(t *testing.T, original, patchChunk string) {
		updated, err := applyStructuredPatch(original, strings.Split(normalizePatchText(patchChunk), "\n"))
		if err != nil {
			return
		}
		if strings.Contains(updated, "\x00") {
			t.Fatalf("updated content should remain text, got %q", updated)
		}
	})
}

func TestApplyPatchToolRejectsAmbiguousRepeatedBlockWithoutHeader(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	content := "alpha\nbeta\nalpha\nbeta\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": "*** Begin Patch\n*** Update File: sample.txt\n alpha\n-beta\n+gamma\n*** End Patch",
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)})
	if err == nil {
		t.Fatal("expected ambiguous patch to fail")
	}
	if !strings.Contains(err.Error(), "ambiguous patch hunk") {
		t.Fatalf("expected ambiguous patch error, got %v", err)
	}
}

func TestApplyPatchToolRejectsHeaderLineCountMismatch(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": "*** Begin Patch\n*** Update File: sample.txt\n@@ -1,3 +1,1 @@\n-alpha\n-beta\n*** End Patch",
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)})
	if err == nil {
		t.Fatal("expected header mismatch to fail")
	}
	if !strings.Contains(err.Error(), "old count mismatch") {
		t.Fatalf("expected header mismatch error, got %v", err)
	}
}

func TestApplyPatchToolRejectsHeaderWhenContentMatchesDifferentLocation(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "sample.txt")
	content := "alpha\nbeta\nalpha\nbeta\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": "*** Begin Patch\n*** Update File: sample.txt\n@@ -2,2 +2,2 @@\n alpha\n-beta\n+gamma\n*** End Patch",
	})
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)})
	if err == nil {
		t.Fatal("expected mismatched header to fail")
	}
	if !strings.Contains(err.Error(), "matched line(s) 1, 3") {
		t.Fatalf("expected mismatch location detail, got %v", err)
	}
}

func TestApplyPatchToolSupportsMoveTo(t *testing.T) {
	workspace := t.TempDir()
	oldPath := filepath.Join(workspace, "old.txt")
	if err := os.WriteFile(oldPath, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": "*** Begin Patch\n*** Update File: old.txt\n*** Move to: nested/new.txt\n@@\n alpha\n-beta\n+gamma\n*** End Patch",
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old path removed, got err=%v", err)
	}
	newPath := filepath.Join(workspace, "nested", "new.txt")
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha\ngamma\n" {
		t.Fatalf("unexpected moved file content: %q", string(data))
	}
	if !strings.Contains(result, "nested/new.txt") {
		t.Fatalf("expected result to mention new path, got %s", result)
	}
}

func TestApplyStructuredPatchPreservesTrailingNewlineState(t *testing.T) {
	updated, err := applyStructuredPatch("alpha\nbeta", []string{"@@ -1,2 +1,2 @@", " alpha", "-beta", "+gamma"})
	if err != nil {
		t.Fatal(err)
	}
	if updated != "alpha\ngamma" {
		t.Fatalf("expected no trailing newline to be preserved, got %q", updated)
	}
}

func BenchmarkApplyStructuredPatch(b *testing.B) {
	original := strings.Repeat("alpha\nbeta\ngamma\n", 200)
	chunk := []string{"@@ -400,2 +400,2 @@", " alpha", "-beta", "+delta"}
	for i := 0; i < b.N; i++ {
		if _, err := applyStructuredPatch(original, chunk); err != nil {
			b.Fatal(err)
		}
	}
}

func ExampleApplyPatchTool_Run() {
	workspace, err := os.MkdirTemp("", "apply-patch-example")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(workspace)

	path := filepath.Join(workspace, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		fmt.Println("error:", err)
		return
	}

	tool := ApplyPatchTool{}
	payload, _ := json.Marshal(map[string]any{
		"patch": "*** Begin Patch\n*** Update File: sample.txt\n@@\n alpha\n-beta\n+gamma\n*** End Patch",
	})
	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Workspace: workspace, Session: session.New(workspace)}); err != nil {
		fmt.Println("error:", err)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Print(string(data))
	// Output:
	// alpha
	// gamma
}
