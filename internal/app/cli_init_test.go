package app

import (
	"bytes"
	"io"
	"testing"
)

func TestRunInitDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RunInit panicked: %v", r)
		}
	}()
	// Init reads from os.Stdin, so in non-interactive test env it
	// won't complete. Just verify parsing flags doesn't panic.
	var buf bytes.Buffer
	_ = RunInit([]string{"-workspace", "."}, &buf, io.Discard)
}
