package app

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestDispatchCLIRequiresAllHandlers(t *testing.T) {
	err := DispatchCLI(nil, bytes.NewBuffer(nil), io.Discard, io.Discard, DispatchHandlers{})
	if err == nil {
		t.Fatal("expected handler validation error")
	}
}

func TestDispatchCLIRoutesSubcommands(t *testing.T) {
	type call struct {
		name string
		args []string
	}
	var calls []call
	handlers := DispatchHandlers{
		RunTUI: func(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			calls = append(calls, call{name: "tui", args: append([]string(nil), args...)})
			return nil
		},
		RunOneShot: func(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			calls = append(calls, call{name: "run", args: append([]string(nil), args...)})
			return nil
		},
		RunWorker: func(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			calls = append(calls, call{name: "worker", args: append([]string(nil), args...)})
			return nil
		},
		RunInstall: func(args []string, stdout, stderr io.Writer) error {
			calls = append(calls, call{name: "install", args: append([]string(nil), args...)})
			return nil
		},
		RunMCP: func(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			calls = append(calls, call{name: "mcp", args: append([]string(nil), args...)})
			return nil
		},
		RenderUsage: func(w io.Writer) {
			calls = append(calls, call{name: "help"})
		},
	}

	tests := []struct {
		name     string
		args     []string
		wantCall call
	}{
		{name: "default no args -> tui", args: nil, wantCall: call{name: "tui", args: nil}},
		{name: "chat -> tui args[1:]", args: []string{"chat", "-model", "x"}, wantCall: call{name: "tui", args: []string{"-model", "x"}}},
		{name: "tui -> tui args[1:]", args: []string{"tui", "-stream", "true"}, wantCall: call{name: "tui", args: []string{"-stream", "true"}}},
		{name: "run -> oneshot", args: []string{"run", "-prompt", "hello"}, wantCall: call{name: "run", args: []string{"-prompt", "hello"}}},
		{name: "worker -> sandbox worker", args: []string{"worker", "--sandbox-stdio"}, wantCall: call{name: "worker", args: []string{"--sandbox-stdio"}}},
		{name: "install -> installer", args: []string{"install", "-to", "bin"}, wantCall: call{name: "install", args: []string{"-to", "bin"}}},
		{name: "mcp -> mcp handler", args: []string{"mcp", "list"}, wantCall: call{name: "mcp", args: []string{"list"}}},
		{name: "help -> render usage", args: []string{"help"}, wantCall: call{name: "help"}},
		{name: "unknown -> tui passthrough", args: []string{"custom", "arg"}, wantCall: call{name: "tui", args: []string{"custom", "arg"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls = calls[:0]
			if err := DispatchCLI(tc.args, bytes.NewBuffer(nil), io.Discard, io.Discard, handlers); err != nil {
				t.Fatal(err)
			}
			if len(calls) != 1 {
				t.Fatalf("expected one call, got %#v", calls)
			}
			got := calls[0]
			if got.name != tc.wantCall.name || !reflect.DeepEqual(got.args, tc.wantCall.args) {
				t.Fatalf("unexpected call, got=%#v want=%#v", got, tc.wantCall)
			}
		})
	}
}

func TestDispatchCLIPropagatesHandlerError(t *testing.T) {
	expected := errors.New("tui failed")
	handlers := DispatchHandlers{
		RunTUI: func(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			return expected
		},
		RunOneShot:  func(args []string, stdin io.Reader, stdout, stderr io.Writer) error { return nil },
		RunWorker:   func(args []string, stdin io.Reader, stdout, stderr io.Writer) error { return nil },
		RunInstall:  func(args []string, stdout, stderr io.Writer) error { return nil },
		RunMCP:      func(args []string, stdin io.Reader, stdout, stderr io.Writer) error { return nil },
		RenderUsage: func(w io.Writer) {},
	}
	err := DispatchCLI([]string{"chat"}, bytes.NewBuffer(nil), io.Discard, io.Discard, handlers)
	if !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}
