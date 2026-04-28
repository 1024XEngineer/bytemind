package tools

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"
)

func TestRunCommandWithSystemSandboxRejectsNilCommand(t *testing.T) {
	err := runCommandWithSystemSandbox(nil, "windows_job_object", "required")
	if err == nil {
		t.Fatal("expected nil command error")
	}
}

func TestRunCommandWithSystemSandboxUsesWindowsRunnerForWindowsBackend(t *testing.T) {
	original := runCommandWithWindowsJobObjectFn
	defer func() { runCommandWithWindowsJobObjectFn = original }()

	sentinel := errors.New("windows-runner-called")
	calls := 0
	runCommandWithWindowsJobObjectFn = func(cmd *exec.Cmd) error {
		calls++
		if cmd == nil {
			t.Fatal("expected command to be forwarded to windows runner")
		}
		return sentinel
	}

	err := runCommandWithSystemSandbox(&exec.Cmd{}, "windows_job_object", "required")
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one windows runner call, got %d", calls)
	}
}

func TestRunCommandWithSystemSandboxSkipsWindowsRunnerWhenModeOff(t *testing.T) {
	original := runCommandWithWindowsJobObjectFn
	defer func() { runCommandWithWindowsJobObjectFn = original }()

	calls := 0
	runCommandWithWindowsJobObjectFn = func(*exec.Cmd) error {
		calls++
		return nil
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit 0")
	} else {
		cmd = exec.Command("sh", "-lc", "true")
	}

	if err := runCommandWithSystemSandbox(cmd, "windows_job_object", "off"); err != nil {
		t.Fatalf("expected command to run without windows runner in mode=off, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected windows runner to be skipped in mode=off, got %d calls", calls)
	}
}
