package tui

import (
	"os"
	"runtime"
	"testing"
)

func TestShouldEnableMouseCaptureFromEnv(t *testing.T) {
	t.Setenv("BYTEMIND_ENABLE_MOUSE", "off")
	if shouldEnableMouseCapture() {
		t.Fatalf("expected mouse capture disabled when env is off")
	}

	t.Setenv("BYTEMIND_ENABLE_MOUSE", "on")
	if !shouldEnableMouseCapture() {
		t.Fatalf("expected mouse capture enabled when env is on")
	}
}

func TestShouldUseInputTTYRespectsOSAndEnv(t *testing.T) {
	t.Setenv("BYTEMIND_WINDOWS_INPUT_TTY", "1")
	got := shouldUseInputTTY()
	want := runtime.GOOS == "windows"
	if got != want {
		t.Fatalf("expected shouldUseInputTTY=%v on %s, got %v", want, runtime.GOOS, got)
	}

	t.Setenv("BYTEMIND_WINDOWS_INPUT_TTY", "0")
	if shouldUseInputTTY() {
		t.Fatalf("expected shouldUseInputTTY false when env is disabled")
	}
}

func TestApplyAutoMouseYOffsetFromEnvironment(t *testing.T) {
	t.Setenv("BYTEMIND_WINDOWS_INPUT_TTY", "0")
	t.Setenv("WT_SESSION", "session")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("BYTEMIND_MOUSE_Y_OFFSET", "")

	applyAutoMouseYOffset()
	got := os.Getenv("BYTEMIND_MOUSE_Y_OFFSET")
	if runtime.GOOS == "windows" {
		if got != "2" {
			t.Fatalf("expected BYTEMIND_MOUSE_Y_OFFSET=2 on windows host, got %q", got)
		}
	} else if got != "" {
		t.Fatalf("expected BYTEMIND_MOUSE_Y_OFFSET unchanged on non-windows, got %q", got)
	}
}

func TestApplyAutoMouseYOffsetRespectsExplicitOverride(t *testing.T) {
	t.Setenv("BYTEMIND_WINDOWS_INPUT_TTY", "0")
	t.Setenv("WT_SESSION", "session")
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("BYTEMIND_MOUSE_Y_OFFSET", "5")

	applyAutoMouseYOffset()
	if got := os.Getenv("BYTEMIND_MOUSE_Y_OFFSET"); got != "5" {
		t.Fatalf("expected explicit BYTEMIND_MOUSE_Y_OFFSET to remain 5, got %q", got)
	}
}
