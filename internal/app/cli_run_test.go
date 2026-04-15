package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCLIHelpRendersUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunCLI([]string{"help"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "bytemind chat") || !strings.Contains(output, "bytemind run") {
		t.Fatalf("expected help usage output, got %q", output)
	}
}
