package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParseFlagSet(t *testing.T) {
	fs, err := parseFlagSet("x", []string{})
	if err != nil {
		t.Fatalf("parseFlagSet with empty args should succeed: %v", err)
	}
	if fs == nil {
		t.Fatalf("expected non-nil flagset")
	}
	if _, err := parseFlagSet("x", []string{"--bad-flag"}); err == nil {
		t.Fatalf("expected parse error for bad flag")
	}
}

func TestPrintUsage(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	printUsage()
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	text := buf.String()
	if !strings.Contains(text, "Subcommands:") || !strings.Contains(text, "generate-review") {
		t.Fatalf("unexpected usage output: %s", text)
	}
}
