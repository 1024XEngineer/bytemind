package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgecli/internal/config"
)

type scriptTerminal struct {
	builder   strings.Builder
	approvals []bool
}

func (t *scriptTerminal) Printf(format string, args ...any) {
	fmt.Fprintf(&t.builder, format, args...)
}

func (t *scriptTerminal) Println(args ...any) {
	fmt.Fprintln(&t.builder, args...)
}

func (t *scriptTerminal) PromptYesNo(_ string) (bool, error) {
	if len(t.approvals) == 0 {
		return false, nil
	}
	value := t.approvals[0]
	t.approvals = t.approvals[1:]
	return value, nil
}

func TestAppRunHappyPath(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/sample\n\ngo 1.23.0\n",
		"greeter.go": `package sample

func Greeting() string {
    return "hello"
}
`,
		"greeter_test.go": `package sample

import "testing"

func TestGreeting(t *testing.T) {
    if Greeting() != "hello, forge" {
        t.Fatalf("unexpected greeting: %s", Greeting())
    }
}
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	terminal := &scriptTerminal{approvals: []bool{true, true}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(config.Default(), terminal, logger)

	err := application.Run(context.Background(), Params{
		RepoPath:      repo,
		Task:          `In greeter.go replace "hello" with "hello, forge"`,
		VerifyCommand: `go test ./...`,
	})
	if err != nil {
		t.Fatalf("expected happy path to succeed, got %v", err)
	}

	content, err := os.ReadFile(filepath.Join(repo, "greeter.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"hello, forge"`) {
		t.Fatalf("expected file to be updated, got: %s", string(content))
	}
}

func TestAppDoesNotWriteWhenApprovalIsDenied(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "demo.go")
	initial := `package sample

func Message() string {
    return "hello"
}
`

	files := map[string]string{
		"go.mod":  "module example.com/sample\n\ngo 1.23.0\n",
		"demo.go": initial,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	terminal := &scriptTerminal{approvals: []bool{false}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(config.Default(), terminal, logger)

	err := application.Run(context.Background(), Params{
		RepoPath: repo,
		Task:     `In demo.go replace "hello" with "goodbye"`,
	})
	if err != nil {
		t.Fatalf("expected denied approval to end without error, got %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != initial {
		t.Fatalf("expected file to remain unchanged, got: %s", string(content))
	}
}

func TestAppSkipsDuplicateReplaceWhenTargetAlreadyMatches(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "greeter.go")
	initial := `package sample

func Greeting() string {
    return "hello, forge"
}
`

	files := map[string]string{
		"go.mod":     "module example.com/sample\n\ngo 1.23.0\n",
		"greeter.go": initial,
		"greeter_test.go": `package sample

import "testing"

func TestGreeting(t *testing.T) {
    if Greeting() != "hello, forge" {
        t.Fatalf("unexpected greeting: %s", Greeting())
    }
}
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	terminal := &scriptTerminal{approvals: []bool{true}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(config.Default(), terminal, logger)

	err := application.Run(context.Background(), Params{
		RepoPath:      repo,
		Task:          `replace hello with hello, forge`,
		VerifyCommand: `go test ./...`,
	})
	if err != nil {
		t.Fatalf("expected no-op run to succeed, got %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != initial {
		t.Fatalf("expected file to remain unchanged, got: %s", string(content))
	}
	if !strings.Contains(terminal.builder.String(), "无需重复修改") {
		t.Fatalf("expected output to mention no-op, got: %s", terminal.builder.String())
	}
}
