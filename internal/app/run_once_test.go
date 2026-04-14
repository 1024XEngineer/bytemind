package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOneShotRejectsMissingPrompt(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunOneShot(RunOneShotRequest{
		Args:   []string{},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err == nil {
		t.Fatal("expected missing prompt error")
	}
	if !strings.Contains(err.Error(), "run requires -prompt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunOneShotAcceptsTrailingPromptText(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	server := newOpenAICompletionServer("Task complete.")
	defer server.Close()

	writeRunOneShotTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": server.URL,
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunOneShot(RunOneShotRequest{
		Args:   []string{"inspect", "repo"},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Task complete.") {
		t.Fatalf("expected final answer in stdout, got %q", stdout.String())
	}
}

func TestRunOneShotCompletesToolLoopSmoke(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		requestCount++
		w.Header().Set("Content-Type", "application/json")

		switch requestCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id":   "call-1",
							"type": "function",
							"function": map[string]any{
								"name":      "list_files",
								"arguments": `{"path":".","limit":5}`,
							},
						}},
					},
				}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role":    "assistant",
						"content": "Workspace inspected after tool call.",
					},
				}},
			})
		}
	}))
	defer server.Close()

	writeRunOneShotTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": server.URL,
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunOneShot(RunOneShotRequest{
		Args:   []string{"-prompt", "inspect repo"},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("expected two provider requests, got %d", requestCount)
	}
	output := stdout.String()
	if !strings.Contains(output, "tool>") || !strings.Contains(output, "list_files") || !strings.Contains(output, "listed") {
		t.Fatalf("expected tool execution output, got %q", output)
	}
	if !strings.Contains(output, "Workspace inspected after tool call.") {
		t.Fatalf("expected final answer in stdout, got %q", output)
	}
}

func writeRunOneShotTestConfig(t *testing.T, workspace string, cfg map[string]any) {
	t.Helper()
	t.Setenv("BYTEMIND_HOME", filepath.Join(workspace, ".bytemind-home"))
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	projectConfigDir := filepath.Join(workspace, ".bytemind")
	if err := os.MkdirAll(projectConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectConfigDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func newOpenAICompletionServer(content string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
			}},
		})
	}))
}
