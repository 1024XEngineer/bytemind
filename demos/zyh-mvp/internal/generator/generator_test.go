package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgecli/internal/config"
)

func TestGenerateHTMLFallsBackAndWritesFile(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "landing.html")
	service := New(config.Default(), nil)
	path, err := service.GenerateFile(context.Background(), "create a simple landing page", output)
	if err != nil {
		t.Fatal(err)
	}
	if path != output {
		t.Fatalf("unexpected output path: %s", path)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	if !strings.Contains(strings.ToLower(html), "<html") {
		t.Fatalf("expected html output, got: %s", html)
	}
}

func TestGenerateContentForGoFile(t *testing.T) {
	service := New(config.Default(), nil)
	content, err := service.GenerateContent(context.Background(), "write a tiny go program that prints hello", "hello.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "package main") {
		t.Fatalf("expected go file content, got: %s", content)
	}
}

func TestGenerateContentForTodoHTML(t *testing.T) {
	service := New(config.Default(), nil)
	content, err := service.GenerateContent(context.Background(), "make a todo web page", "todo.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(content), "<html") {
		t.Fatalf("expected html file content, got: %s", content)
	}
}

func TestStripCodeFence(t *testing.T) {
	got := stripCodeFence("```html\n<html></html>\n```", ".html")
	if got != "<html></html>" {
		t.Fatalf("unexpected result: %q", got)
	}
}
