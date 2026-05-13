package app

import "testing"

func TestRunInitPanicFree(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RunInit panicked: %v", r)
		}
	}()
	// RunInit reads stdin interactively, so in test mode it will
	// likely error. We just verify no panic.
	_ = RunInit([]string{"-workspace", "."}, nil, nil)
}
