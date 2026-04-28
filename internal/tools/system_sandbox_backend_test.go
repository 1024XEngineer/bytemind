package tools

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveSystemSandboxRuntimeBackendRequiredFailsOnUnsupportedOS(t *testing.T) {
	_, err := resolveSystemSandboxRuntimeBackend(systemSandboxModeRequired, "freebsd", func(string) (string, error) {
		t.Fatal("lookPath should not be called for unsupported OS")
		return "", nil
	})
	if err == nil {
		t.Fatal("expected required mode to fail on unsupported OS")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSystemSandboxRuntimeBackendBestEffortFallsBackWhenLinuxBackendMissing(t *testing.T) {
	backend, err := resolveSystemSandboxRuntimeBackend(systemSandboxModeBestEffort, "linux", func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err != nil {
		t.Fatalf("expected best_effort fallback, got %v", err)
	}
	if backend.Enabled {
		t.Fatalf("expected disabled backend when probe fails in best_effort, got %#v", backend)
	}
	if !strings.Contains(strings.ToLower(backend.UnavailableReason), "unavailable") {
		t.Fatalf("expected unavailable reason to include probe error, got %#v", backend)
	}
}

func TestResolveSystemSandboxRuntimeBackendWindowsBackendActive(t *testing.T) {
	backend, err := resolveSystemSandboxRuntimeBackend(systemSandboxModeBestEffort, "windows", func(string) (string, error) {
		t.Fatal("lookPath should not be called for windows job-object backend")
		return "", nil
	})
	if err != nil {
		t.Fatalf("expected windows backend to resolve, got %v", err)
	}
	if !backend.Enabled {
		t.Fatalf("expected backend enabled for windows job-object backend, got %#v", backend)
	}
	if backend.Name != "windows_job_object" {
		t.Fatalf("expected windows_job_object backend name, got %#v", backend)
	}
	if strings.TrimSpace(backend.Runner) == "" {
		t.Fatalf("expected non-empty backend runner marker, got %#v", backend)
	}
	if !backend.Shell.Policy.ProcessIsolation || !backend.Worker.Policy.ProcessIsolation {
		t.Fatalf("expected process isolation policy in windows backend, got %#v", backend)
	}
	if backend.Shell.Policy.FileIsolation || backend.Worker.Policy.FileIsolation {
		t.Fatalf("expected file isolation to remain disabled on windows backend, got %#v", backend)
	}
}

func TestResolveSystemSandboxRuntimeBackendWindowsRequiredPassesWithReadonlyGuardCapabilities(t *testing.T) {
	backend, err := resolveSystemSandboxRuntimeBackend(systemSandboxModeRequired, "windows", func(string) (string, error) {
		t.Fatal("lookPath should not be called for windows job-object backend")
		return "", nil
	})
	if err != nil {
		t.Fatalf("expected windows required mode to pass capability gate, got %v", err)
	}
	if !backend.RequiredCapable {
		t.Fatalf("expected windows required mode backend to be required-capable, got %#v", backend)
	}
	if !backend.Enabled || backend.Name != "windows_job_object" {
		t.Fatalf("unexpected windows backend: %#v", backend)
	}
}

func TestResolveSystemSandboxRuntimeBackendBestEffortFallbackIncludesPendingDarwinReason(t *testing.T) {
	backend, err := resolveSystemSandboxRuntimeBackend(systemSandboxModeRequired, "darwin", func(name string) (string, error) {
		if name != "sandbox-exec" {
			t.Fatalf("unexpected binary lookup: %q", name)
		}
		return "/usr/bin/sandbox-exec", nil
	})
	if err != nil {
		t.Fatalf("expected darwin required backend to pass, got %v", err)
	}
	if !backend.Enabled {
		t.Fatalf("expected backend enabled for darwin sandbox-exec, got %#v", backend)
	}
	if backend.Name != "darwin_sandbox_exec" {
		t.Fatalf("expected darwin_sandbox_exec backend name, got %#v", backend)
	}
	if backend.Runner != "/usr/bin/sandbox-exec" {
		t.Fatalf("unexpected runner: %#v", backend)
	}
}

func TestResolveSystemSandboxRuntimeBackendBestEffortFallbackIncludesDarwinProbeReason(t *testing.T) {
	backend, err := resolveSystemSandboxRuntimeBackend(systemSandboxModeBestEffort, "darwin", func(string) (string, error) {
		return "", errors.New("missing sandbox-exec")
	})
	if err != nil {
		t.Fatalf("expected darwin best_effort fallback, got %v", err)
	}
	if backend.Enabled {
		t.Fatalf("expected backend disabled for missing darwin backend, got %#v", backend)
	}
	if backend.Name != "darwin_sandbox_exec" {
		t.Fatalf("expected darwin_sandbox_exec backend name, got %#v", backend)
	}
	if !strings.Contains(strings.ToLower(backend.UnavailableReason), "sandbox-exec") {
		t.Fatalf("expected sandbox-exec probe reason, got %#v", backend)
	}
}

func TestResolveSystemSandboxRuntimeBackendLinuxProfiles(t *testing.T) {
	backend, err := resolveSystemSandboxRuntimeBackend(systemSandboxModeRequired, "linux", func(name string) (string, error) {
		if name != "unshare" {
			t.Fatalf("unexpected binary lookup: %q", name)
		}
		return "/usr/bin/unshare", nil
	})
	if err != nil {
		t.Fatalf("resolve backend: %v", err)
	}
	if !backend.Enabled {
		t.Fatalf("expected enabled backend, got %#v", backend)
	}
	if backend.Name != "linux_unshare" {
		t.Fatalf("unexpected backend name: %#v", backend)
	}
	if !containsString(backend.Shell.ArgPrefix, "--net") {
		t.Fatalf("shell launch should include --net isolation, got %#v", backend.Shell.ArgPrefix)
	}
	if containsString(backend.Worker.ArgPrefix, "--net") {
		t.Fatalf("worker launch should not include --net isolation, got %#v", backend.Worker.ArgPrefix)
	}
	if !backend.Shell.Policy.NetworkIsolation {
		t.Fatalf("expected shell policy network isolation, got %#v", backend.Shell.Policy)
	}
	if backend.Worker.Policy.NetworkIsolation {
		t.Fatalf("expected worker policy without network isolation, got %#v", backend.Worker.Policy)
	}
}

func TestSystemSandboxBackendCapabilitiesForName(t *testing.T) {
	tests := []struct {
		name              string
		backend           string
		wantKnown         bool
		wantShellNetwork  bool
		wantWorkerNetwork bool
	}{
		{
			name:              "linux_unshare",
			backend:           "linux_unshare",
			wantKnown:         true,
			wantShellNetwork:  true,
			wantWorkerNetwork: false,
		},
		{
			name:              "darwin_sandbox_exec",
			backend:           "darwin_sandbox_exec",
			wantKnown:         true,
			wantShellNetwork:  false,
			wantWorkerNetwork: false,
		},
		{
			name:              "windows_job_object",
			backend:           "windows_job_object",
			wantKnown:         true,
			wantShellNetwork:  false,
			wantWorkerNetwork: false,
		},
		{
			name:              "future_worker_net_isolated",
			backend:           "future_worker_net_isolated",
			wantKnown:         true,
			wantShellNetwork:  false,
			wantWorkerNetwork: true,
		},
		{
			name:              "future_shell_net_isolated",
			backend:           "future_shell_net_isolated",
			wantKnown:         true,
			wantShellNetwork:  true,
			wantWorkerNetwork: false,
		},
		{
			name:              "unknown_backend",
			backend:           "unknown_backend",
			wantKnown:         false,
			wantShellNetwork:  false,
			wantWorkerNetwork: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SystemSandboxBackendCapabilitiesForName(tt.backend)
			if got.Known != tt.wantKnown {
				t.Fatalf("Known mismatch: got %v want %v", got.Known, tt.wantKnown)
			}
			if got.ShellNetworkIsolation != tt.wantShellNetwork {
				t.Fatalf("ShellNetworkIsolation mismatch: got %v want %v", got.ShellNetworkIsolation, tt.wantShellNetwork)
			}
			if got.WorkerNetworkIsolation != tt.wantWorkerNetwork {
				t.Fatalf("WorkerNetworkIsolation mismatch: got %v want %v", got.WorkerNetworkIsolation, tt.wantWorkerNetwork)
			}
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
