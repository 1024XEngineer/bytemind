package tui

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestWarnSetenvWritesWarningToStderr(t *testing.T) {
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writer
	t.Cleanup(func() {
		os.Stderr = original
	})

	warnSetenv("BYTEMIND_MOUSE_Y_OFFSET", errors.New("boom"))
	_ = writer.Close()

	out, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("read warning output: %v", readErr)
	}
	text := string(out)
	if !strings.Contains(text, "failed to set BYTEMIND_MOUSE_Y_OFFSET") || !strings.Contains(text, "boom") {
		t.Fatalf("expected warning text to include env and cause, got %q", text)
	}
}

func TestWarnSetenvNoopWhenErrorNil(t *testing.T) {
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writer
	t.Cleanup(func() {
		os.Stderr = original
	})

	warnSetenv("BYTEMIND_MOUSE_Y_OFFSET", nil)
	_ = writer.Close()

	out, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("read warning output: %v", readErr)
	}
	if len(out) != 0 {
		t.Fatalf("expected no warning output when err is nil, got %q", string(out))
	}
}
