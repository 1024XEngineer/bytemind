package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMCPHelpRendersUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := RunMCP([]string{"help"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "bytemind mcp list") || !strings.Contains(output, "bytemind mcp add") {
		t.Fatalf("unexpected help output: %q", output)
	}
}

func TestRunMCPAddAndList(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("BYTEMIND_HOME", t.TempDir())
	writeMCPCLITestConfig(t, workspace)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := RunMCP([]string{
		"add", "local",
		"--cmd", "cmd",
		"--args", "/c,echo,ok",
		"--auto-start=false",
		"--workspace", workspace,
	}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunMCP add failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "local") {
		t.Fatalf("expected add output to include local server, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := RunMCP([]string{"list", "--workspace", workspace}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunMCP list failed: %v", err)
	}
	listOutput := stdout.String()
	if !strings.Contains(listOutput, "ID") || !strings.Contains(listOutput, "local") {
		t.Fatalf("expected list output to include local server, got %q", listOutput)
	}

	stdout.Reset()
	stderr.Reset()
	if err := RunMCP([]string{"show", "local", "--workspace", workspace}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunMCP show failed: %v", err)
	}
	showOutput := stdout.String()
	if !strings.Contains(showOutput, "id: local") || !strings.Contains(showOutput, "command: cmd") {
		t.Fatalf("expected show output to include local id and command, got %q", showOutput)
	}
}

func TestRunMCPAuthRendersGuidance(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := RunMCP([]string{"auth", "github"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("RunMCP auth failed: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Auth guide for `github`") {
		t.Fatalf("expected auth header in output, got %q", output)
	}
	if !strings.Contains(output, "bytemind mcp test github") {
		t.Fatalf("expected auth output to include test hint, got %q", output)
	}
}

func writeMCPCLITestConfig(t *testing.T, workspace string) {
	t.Helper()
	doc := map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
			"api_key":  "test-key",
		},
	}
	path := filepath.Join(workspace, ".bytemind", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config dir failed: %v", err)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal config failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
}
