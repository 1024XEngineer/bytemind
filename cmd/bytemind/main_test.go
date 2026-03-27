package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/session"
)

func TestCompleteSlashCommand(t *testing.T) {
	completed, suggestions := completeSlashCommand("/he")
	if len(suggestions) != 0 {
		t.Fatalf("expected unique completion, got suggestions %#v", suggestions)
	}
	if completed != "/help" {
		t.Fatalf("expected /help, got %q", completed)
	}
}

func TestCompleteSlashCommandReturnsSuggestionsForAmbiguousPrefix(t *testing.T) {
	completed, suggestions := completeSlashCommand("/sess")
	if completed != "/sess" {
		t.Fatalf("expected input to remain unchanged, got %q", completed)
	}
	if len(suggestions) != 2 || suggestions[0] != "/session" || suggestions[1] != "/sessions" {
		t.Fatalf("unexpected suggestions: %#v", suggestions)
	}
}

func TestResolveSessionIDSupportsUniquePrefix(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first := session.New(`E:\\repo`)
	first.ID = "20260324-120000-abcd"
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}

	second := session.New(`E:\\repo`)
	second.ID = "20260324-130000-efgh"
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveSessionID(store, "20260324-1300")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != second.ID {
		t.Fatalf("expected %q, got %q", second.ID, resolved)
	}
}

func TestHandleSlashCommandRejectsResumeAcrossWorkspaces(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	current := session.New(`E:\\repo-a`)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}

	other := session.New(`E:\\repo-b`)
	other.ID = "other"
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	next, shouldExit, handled, err := handleSlashCommand(&out, store, current, "/resume other")
	if err == nil {
		t.Fatal("expected cross-workspace resume to fail")
	}
	if !strings.Contains(err.Error(), "belongs to workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled || shouldExit {
		t.Fatalf("expected handled command without exit, got handled=%v shouldExit=%v", handled, shouldExit)
	}
	if next != current {
		t.Fatal("expected current session to remain active")
	}
}

func TestHandleSlashCommandResumesSessionWithinWorkspace(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	current := session.New(workspace)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}

	resumed := session.New(filepath.Join(workspace, "."))
	resumed.ID = "resume-me"
	if err := store.Save(resumed); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	next, shouldExit, handled, err := handleSlashCommand(&out, store, current, "/resume resume")
	if err != nil {
		t.Fatal(err)
	}
	if shouldExit || !handled {
		t.Fatalf("expected handled resume without exit, got handled=%v shouldExit=%v", handled, shouldExit)
	}
	if next.ID != resumed.ID {
		t.Fatalf("expected resumed session %q, got %#v", resumed.ID, next)
	}
	if !strings.Contains(out.String(), "resumed") {
		t.Fatalf("expected resumed output, got %q", out.String())
	}
}

func TestSameWorkspaceNormalizesPaths(t *testing.T) {
	workspace := t.TempDir()
	if !sameWorkspace(workspace, filepath.Join(workspace, ".")) {
		t.Fatal("expected normalized paths to match")
	}
}

func TestHandleSlashCommandCreatesNewSession(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	current := session.New(`E:\\repo`)
	current.ID = "current"
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	next, shouldExit, handled, err := handleSlashCommand(&out, store, current, "/new")
	if err != nil {
		t.Fatal(err)
	}
	if shouldExit || !handled {
		t.Fatalf("expected handled new command without exit, got handled=%v shouldExit=%v", handled, shouldExit)
	}
	if next.ID == current.ID {
		t.Fatalf("expected a new session id, got %#v", next)
	}
	if !strings.Contains(out.String(), "new session") {
		t.Fatalf("expected output to mention new session, got %q", out.String())
	}
}

func TestPrintSessionsShowsFullSessionID(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(`E:\\repo`)
	sess.ID = "20260327-074655-abcd1234"
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := printSessions(&out, store, sess.ID, 8); err != nil {
		t.Fatal(err)
	}
	output := out.String()
	if !strings.Contains(output, sess.ID) {
		t.Fatalf("expected full session id in output, got %q", output)
	}
}

func TestHandleSlashCommandReportsUnknownCommandSuggestions(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	current := session.New(`E:\\repo`)
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	_, shouldExit, handled, err := handleSlashCommand(&out, store, current, "/wat")
	if err != nil {
		t.Fatal(err)
	}
	if shouldExit || !handled {
		t.Fatalf("expected handled unknown command without exit, got handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(out.String(), "unknown command") || !strings.Contains(out.String(), "/help") {
		t.Fatalf("expected suggestions for unknown command, got %q", out.String())
	}
}

func TestRunChatAcceptsWorkspaceFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runChat([]string{"-workspace", `E:\\repo`}, strings.NewReader(""), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected chat to fail later because config is incomplete")
	}
	if strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapCreatesSessionInWorkspace(t *testing.T) {
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

	runner, store, sess, err := bootstrap("", "", "", "", "", 0, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if runner == nil || store == nil || sess == nil {
		t.Fatal("expected bootstrap to return runner, store, and session")
	}
	if sess.Workspace != workspace {
		t.Fatalf("expected workspace %q, got %q", workspace, sess.Workspace)
	}
	if strings.TrimSpace(sess.ID) == "" {
		t.Fatal("expected session id to be created")
	}
}

func TestRunOneShotRejectsMissingPrompt(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runOneShot(nil, strings.NewReader(""), &stdout, &stderr)
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

	writeTestConfig(t, workspace, map[string]any{
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
	err := runOneShot([]string{"inspect", "repo"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Task complete.") {
		t.Fatalf("expected final answer in stdout, got %q", stdout.String())
	}
}

func TestRunChatRejectsInvalidStreamValue(t *testing.T) {
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

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runChat([]string{"-stream", "nope"}, strings.NewReader("/quit\n"), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected invalid stream error")
	}
	if !strings.Contains(err.Error(), "invalid -stream value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunChatRejectsNegativeMaxIterations(t *testing.T) {
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

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runChat([]string{"-max-iterations", "-1"}, strings.NewReader("/quit\n"), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected negative max-iterations error")
	}
	if !strings.Contains(err.Error(), "-max-iterations must be greater than 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapRejectsMissingAPIKey(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("BYTEMIND_API_KEY", "")
	writeTestConfig(t, workspace, map[string]any{
		"provider": map[string]any{
			"type":     "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"model":    "gpt-5.4-mini",
		},
		"stream": false,
	})

	_, _, _, err := bootstrap("", "", "", "", "", 0, strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected missing API key error")
	}
	if !strings.Contains(err.Error(), "missing API key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapRejectsExplicitMissingConfigFile(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)

	_, _, _, err := bootstrap(filepath.Join(workspace, "missing-config.json"), "", "", "", "", 0, strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected missing config file error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

func TestBootstrapRejectsExplicitMalformedConfigFile(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	badConfigPath := filepath.Join(workspace, "bad-config.json")
	if err := os.WriteFile(badConfigPath, []byte(`{"provider":`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, _, err := bootstrap(badConfigPath, "", "", "", "", 0, strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected malformed config error")
	}
	if !strings.Contains(err.Error(), "unexpected end of JSON input") && !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeTestConfig(t *testing.T, workspace string, cfg map[string]any) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "config.json"), data, 0o644); err != nil {
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
