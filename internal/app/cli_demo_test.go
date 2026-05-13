package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunDemoUnknownDemo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := RunDemo([]string{"nonexistent"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown demo")
	}
	if !strings.Contains(stderr.String(), "Unknown demo") {
		t.Errorf("expected 'Unknown demo', got %s", stderr.String())
	}
}
