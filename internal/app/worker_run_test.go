package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunWorkerArgsRequiresSandboxStdioFlag(t *testing.T) {
	err := RunWorkerArgs(nil, bytes.NewBuffer(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected missing --sandbox-stdio flag to fail")
	}
	if !strings.Contains(err.Error(), "--sandbox-stdio") {
		t.Fatalf("expected --sandbox-stdio hint, got %v", err)
	}
}

func TestRunWorkerArgsExecutesWorkerProtocol(t *testing.T) {
	request := `{"version":"v1","tool_name":"unknown_tool","raw_args":{},"execution":{}}`
	var out bytes.Buffer

	err := RunWorkerArgs([]string{"--sandbox-stdio"}, strings.NewReader(request), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run worker args: %v", err)
	}
	if !strings.Contains(out.String(), `"error"`) {
		t.Fatalf("expected worker json error response, got %q", out.String())
	}
}
