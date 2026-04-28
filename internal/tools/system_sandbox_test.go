package tools

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSystemSandboxRuntimeWithSkipsWhenSandboxDisabled(t *testing.T) {
	if err := validateSystemSandboxRuntimeWith(false, "required", "windows", func(string) (string, error) {
		t.Fatal("lookPath should not be called when sandbox is disabled")
		return "", nil
	}); err != nil {
		t.Fatalf("expected disabled sandbox to skip system sandbox validation, got %v", err)
	}
}

func TestValidateSystemSandboxRuntimeWithSkipsWhenModeOff(t *testing.T) {
	if err := validateSystemSandboxRuntimeWith(true, "off", "linux", func(string) (string, error) {
		t.Fatal("lookPath should not be called when mode is off")
		return "", nil
	}); err != nil {
		t.Fatalf("expected mode=off to skip validation, got %v", err)
	}
}

func TestValidateSystemSandboxRuntimeWithFailsClosedForRequiredModeWithoutBackend(t *testing.T) {
	err := validateSystemSandboxRuntimeWith(true, "required", "freebsd", func(string) (string, error) {
		t.Fatal("lookPath should not be called on unsupported OS")
		return "", nil
	})
	if err == nil {
		t.Fatal("expected required mode to fail closed when backend is unavailable")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSystemSandboxRuntimeWithWindowsRequiredPassesWithReadonlyGuardCapabilities(t *testing.T) {
	err := validateSystemSandboxRuntimeWith(true, "required", "windows", func(string) (string, error) {
		t.Fatal("lookPath should not be called for windows job-object backend")
		return "", nil
	})
	if err != nil {
		t.Fatalf("expected windows required mode to pass runtime validation, got %v", err)
	}
}

func TestValidateSystemSandboxRuntimeWithBestEffortFallsBackWithoutBackend(t *testing.T) {
	err := validateSystemSandboxRuntimeWith(true, "best_effort", "linux", func(string) (string, error) {
		return "", errors.New("missing unshare")
	})
	if err != nil {
		t.Fatalf("expected best_effort mode to allow fallback, got %v", err)
	}
}

func TestValidateSystemSandboxRuntimeWithRequiredPassesWhenBackendAvailable(t *testing.T) {
	err := validateSystemSandboxRuntimeWith(true, "required", "linux", func(name string) (string, error) {
		if name != "unshare" {
			t.Fatalf("unexpected binary lookup: %q", name)
		}
		return "/usr/bin/unshare", nil
	})
	if err != nil {
		t.Fatalf("expected required mode to pass with backend available, got %v", err)
	}
}

func TestResolveSystemSandboxRuntimeStatusWithBestEffortFallback(t *testing.T) {
	status, err := resolveSystemSandboxRuntimeStatusWith(true, "best_effort", "freebsd", func(string) (string, error) {
		t.Fatal("lookPath should not be called for unsupported OS backend")
		return "", nil
	})
	if err != nil {
		t.Fatalf("resolve status: %v", err)
	}
	if status.Mode != "best_effort" {
		t.Fatalf("expected mode best_effort, got %#v", status)
	}
	if !status.Fallback {
		t.Fatalf("expected fallback=true, got %#v", status)
	}
	if status.BackendEnabled {
		t.Fatalf("expected backend disabled, got %#v", status)
	}
	if status.RequiredCapable {
		t.Fatalf("expected required_capable=false without active backend, got %#v", status)
	}
	if status.CapabilityLevel != "none" {
		t.Fatalf("expected capability_level=none without active backend, got %#v", status)
	}
	if !strings.Contains(strings.ToLower(status.Message), "fallback") {
		t.Fatalf("expected fallback message, got %#v", status)
	}
}

func TestResolveSystemSandboxRuntimeStatusWithWindowsBackendActive(t *testing.T) {
	status, err := resolveSystemSandboxRuntimeStatusWith(true, "best_effort", "windows", func(string) (string, error) {
		t.Fatal("lookPath should not be called for windows job-object backend")
		return "", nil
	})
	if err != nil {
		t.Fatalf("resolve status: %v", err)
	}
	if !status.BackendEnabled {
		t.Fatalf("expected windows backend enabled, got %#v", status)
	}
	if status.BackendName != "windows_job_object" {
		t.Fatalf("expected windows_job_object backend, got %#v", status)
	}
	if status.Fallback {
		t.Fatalf("expected fallback=false for windows backend, got %#v", status)
	}
	if status.RequiredCapable {
		t.Fatalf("expected required_capable=false for windows best_effort backend, got %#v", status)
	}
	if status.CapabilityLevel != "guarded" {
		t.Fatalf("expected capability_level=guarded for windows best_effort backend, got %#v", status)
	}
}

func TestResolveSystemSandboxRuntimeStatusWithWindowsRequiredBackendActive(t *testing.T) {
	status, err := resolveSystemSandboxRuntimeStatusWith(true, "required", "windows", func(string) (string, error) {
		t.Fatal("lookPath should not be called for windows job-object backend")
		return "", nil
	})
	if err != nil {
		t.Fatalf("resolve status: %v", err)
	}
	if !status.BackendEnabled {
		t.Fatalf("expected windows backend enabled, got %#v", status)
	}
	if status.BackendName != "windows_job_object" {
		t.Fatalf("expected windows_job_object backend, got %#v", status)
	}
	if status.Fallback {
		t.Fatalf("expected fallback=false for windows required backend, got %#v", status)
	}
	if !status.RequiredCapable {
		t.Fatalf("expected required_capable=true for windows required backend, got %#v", status)
	}
	if status.CapabilityLevel != "guarded" {
		t.Fatalf("expected capability_level=guarded for windows required backend, got %#v", status)
	}
}

func TestResolveSystemSandboxRuntimeStatusWithRequiredBackendActive(t *testing.T) {
	status, err := resolveSystemSandboxRuntimeStatusWith(true, "required", "linux", func(name string) (string, error) {
		if name != "unshare" {
			t.Fatalf("unexpected binary lookup: %q", name)
		}
		return "/usr/bin/unshare", nil
	})
	if err != nil {
		t.Fatalf("resolve status: %v", err)
	}
	if !status.BackendEnabled {
		t.Fatalf("expected backend enabled, got %#v", status)
	}
	if status.BackendName != "linux_unshare" {
		t.Fatalf("expected linux_unshare backend, got %#v", status)
	}
	if status.Fallback {
		t.Fatalf("expected fallback=false, got %#v", status)
	}
	if !status.RequiredCapable {
		t.Fatalf("expected required_capable=true for linux required backend, got %#v", status)
	}
	if status.CapabilityLevel != "full" {
		t.Fatalf("expected capability_level=full for linux required backend, got %#v", status)
	}
}
