package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimepkg "bytemind/internal/runtime"
)

func TestBootstrapPersistsRuntimeTasksOnlyToUnifiedTaskLog(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".bytemind-home")
	t.Setenv("BYTEMIND_HOME", home)

	configPath := filepath.Join(workspace, "config.json")
	data, err := json.Marshal(map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := Bootstrap(BootstrapRequest{
		Workspace:     workspace,
		ConfigPath:    configPath,
		RequireAPIKey: true,
		Stdin:         strings.NewReader(""),
		Stdout:        ioDiscard{},
	})
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	taskID, err := rt.TaskManager.Submit(context.Background(), runtimepkg.TaskSpec{Name: "persist"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := rt.TaskManager.Cancel(context.Background(), taskID, "test"); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	taskLogPath := filepath.Join(home, "tasks", string(taskID)+".log")
	taskLogContent, err := os.ReadFile(taskLogPath)
	if err != nil {
		t.Fatalf("expected unified task log %q, got error: %v", taskLogPath, err)
	}
	if !bytes.Contains(taskLogContent, []byte(`"type":"task_event.status"`)) {
		t.Fatalf("expected task log to include %q records", "task_event.status")
	}
	if !bytes.Contains(taskLogContent, []byte(`"type":"task_log"`)) {
		t.Fatalf("expected task log to include %q records", "task_log")
	}

	legacyEventFiles, err := filepath.Glob(filepath.Join(home, "runtime", "tasks", "events", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob legacy events failed: %v", err)
	}
	if len(legacyEventFiles) != 0 {
		t.Fatalf("expected no legacy runtime event files, found %d", len(legacyEventFiles))
	}
	legacyLogFiles, err := filepath.Glob(filepath.Join(home, "runtime", "tasks", "logs", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob legacy logs failed: %v", err)
	}
	if len(legacyLogFiles) != 0 {
		t.Fatalf("expected no legacy runtime log files, found %d", len(legacyLogFiles))
	}
}

func TestBootstrapFallsBackToLegacyRuntimeTaskStoreWhenUnifiedInitFails(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, ".bytemind-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tasks"), []byte("not-a-dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BYTEMIND_HOME", home)

	configPath := filepath.Join(workspace, "config.json")
	data, err := json.Marshal(map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := Bootstrap(BootstrapRequest{
		Workspace:     workspace,
		ConfigPath:    configPath,
		RequireAPIKey: true,
		Stdin:         strings.NewReader(""),
		Stdout:        ioDiscard{},
	})
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	taskID, err := rt.TaskManager.Submit(context.Background(), runtimepkg.TaskSpec{Name: "legacy-fallback"})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if err := rt.TaskManager.Cancel(context.Background(), taskID, "test"); err != nil {
		task, getErr := rt.TaskManager.Get(context.Background(), taskID)
		if getErr != nil {
			t.Fatalf("Cancel failed: %v (and Get failed: %v)", err, getErr)
		}
		if task.Status != "failed" || task.ErrorCode != runtimepkg.ErrorCodeTaskExecutionFailed {
			t.Fatalf("Cancel failed: %v (task status=%s error_code=%s)", err, task.Status, task.ErrorCode)
		}
	}

	legacyEventFiles, err := filepath.Glob(filepath.Join(home, "runtime", "tasks", "events", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob legacy events failed: %v", err)
	}
	if len(legacyEventFiles) == 0 {
		t.Fatal("expected legacy runtime event files when unified init fails")
	}
	legacyLogFiles, err := filepath.Glob(filepath.Join(home, "runtime", "tasks", "logs", "*.jsonl"))
	if err != nil {
		t.Fatalf("glob legacy logs failed: %v", err)
	}
	if len(legacyLogFiles) == 0 {
		t.Fatal("expected legacy runtime log files when unified init fails")
	}
}
