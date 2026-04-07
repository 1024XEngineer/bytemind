package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	itui "bytemind/internal/tui"
)

func TestRunTUIBuildsOptionsAndInvokesProgram(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	writeTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})

	sentinel := errors.New("stop program")
	original := runTUIProgram
	t.Cleanup(func() {
		runTUIProgram = original
	})

	called := false
	runTUIProgram = func(opts itui.Options) error {
		called = true
		if opts.Runner == nil || opts.Store == nil || opts.Session == nil {
			t.Fatalf("expected runner/store/session to be initialized")
		}
		if opts.ImageStore == nil {
			t.Fatalf("expected image store to be initialized")
		}
		if opts.Workspace != workspace {
			t.Fatalf("expected workspace %q, got %q", workspace, opts.Workspace)
		}
		if opts.Config.Provider.Model != "gpt-5.4" {
			t.Fatalf("expected model override to apply, got %q", opts.Config.Provider.Model)
		}
		if !opts.Config.Stream {
			t.Fatalf("expected stream override to apply")
		}
		if opts.Config.MaxIterations != 9 {
			t.Fatalf("expected max-iterations override to apply, got %d", opts.Config.MaxIterations)
		}
		return sentinel
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runTUI([]string{
		"-workspace", workspace,
		"-model", "gpt-5.4",
		"-stream", "true",
		"-max-iterations", "9",
	}, strings.NewReader(""), &stdout, &stderr)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if !called {
		t.Fatal("expected tui program runner to be called")
	}
}

func TestRunTUIRejectsInvalidStreamValue(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	writeTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})

	original := runTUIProgram
	t.Cleanup(func() {
		runTUIProgram = original
	})
	runTUIProgram = func(opts itui.Options) error {
		t.Fatalf("did not expect tui program runner on invalid stream")
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runTUI([]string{"-workspace", workspace, "-stream", "invalid"}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected invalid stream error")
	}
	if !strings.Contains(err.Error(), "invalid -stream value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTUIFailsWhenHomeLayoutCannotBeCreated(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	writeTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})

	blockParent := t.TempDir()
	blockFile := filepath.Join(blockParent, "not-a-dir")
	if err := os.WriteFile(blockFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BYTEMIND_HOME", filepath.Join(blockFile, "child"))

	original := runTUIProgram
	t.Cleanup(func() {
		runTUIProgram = original
	})
	runTUIProgram = func(opts itui.Options) error {
		t.Fatalf("did not expect tui program runner when home layout fails")
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runTUI([]string{"-workspace", workspace}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected home layout creation error")
	}
}

func TestRunTUIFailsWhenImageCachePathIsAFile(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	writeTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
		"stream": false,
	})

	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "image-cache"), []byte("block-dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	original := runTUIProgram
	t.Cleanup(func() {
		runTUIProgram = original
	})
	runTUIProgram = func(opts itui.Options) error {
		t.Fatalf("did not expect tui program runner when image store init fails")
		return nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runTUI([]string{"-workspace", workspace}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected image store initialization error")
	}
}
