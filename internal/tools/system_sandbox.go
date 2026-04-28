package tools

import (
	"fmt"
	"runtime"
	"strings"
)

// SystemSandboxRuntimeStatus describes system sandbox runtime state after
// configuration and platform/backend probing are applied.
type SystemSandboxRuntimeStatus struct {
	SandboxEnabled         bool
	RequestedMode          string
	Mode                   string
	GOOS                   string
	BackendEnabled         bool
	BackendName            string
	RequiredCapable        bool
	CapabilityLevel        string
	ShellNetworkIsolation  bool
	WorkerNetworkIsolation bool
	Fallback               bool
	Message                string
}

// ValidateSystemSandboxRuntime verifies whether the configured system sandbox mode
// is usable in the current runtime. It is fail-closed for required mode.
func ValidateSystemSandboxRuntime(sandboxEnabled bool, mode string) error {
	return validateSystemSandboxRuntimeWith(sandboxEnabled, mode, runtime.GOOS, runShellLookPath)
}

// ResolveSystemSandboxRuntimeStatus reports the effective runtime status of system
// sandbox mode under the current process OS and backend availability.
func ResolveSystemSandboxRuntimeStatus(sandboxEnabled bool, mode string) (SystemSandboxRuntimeStatus, error) {
	return resolveSystemSandboxRuntimeStatusWith(sandboxEnabled, mode, runtime.GOOS, runShellLookPath)
}

func validateSystemSandboxRuntimeWith(
	sandboxEnabled bool,
	mode string,
	goos string,
	lookPath func(string) (string, error),
) error {
	_, err := resolveSystemSandboxRuntimeStatusWith(sandboxEnabled, mode, goos, lookPath)
	return err
}

func resolveSystemSandboxRuntimeStatusWith(
	sandboxEnabled bool,
	mode string,
	goos string,
	lookPath func(string) (string, error),
) (SystemSandboxRuntimeStatus, error) {
	status := SystemSandboxRuntimeStatus{
		SandboxEnabled:  sandboxEnabled,
		RequestedMode:   strings.TrimSpace(mode),
		GOOS:            strings.TrimSpace(goos),
		CapabilityLevel: "none",
	}
	if status.GOOS == "" {
		status.GOOS = runtime.GOOS
	}
	if !sandboxEnabled {
		status.Mode = systemSandboxModeOff
		status.Message = "system sandbox is disabled"
		return status, nil
	}
	status.Mode = normalizeSystemSandboxMode(&ExecutionContext{SystemSandboxMode: mode})
	if status.Mode == systemSandboxModeOff {
		status.Message = "system sandbox mode is off"
		return status, nil
	}
	backend, err := resolveSystemSandboxRuntimeBackend(status.Mode, status.GOOS, lookPath)
	if err != nil {
		return status, fmt.Errorf("system sandbox backend unavailable for mode %q: %w", status.Mode, err)
	}
	if backend.Enabled {
		status.BackendEnabled = true
		status.BackendName = strings.TrimSpace(backend.Name)
		status.RequiredCapable = backend.RequiredCapable
		status.CapabilityLevel = strings.TrimSpace(backend.CapabilityLevel)
		status.ShellNetworkIsolation = backend.Shell.Policy.NetworkIsolation
		status.WorkerNetworkIsolation = backend.Worker.Policy.NetworkIsolation
		if status.CapabilityLevel == "" {
			status.CapabilityLevel = systemSandboxCapabilityLevel(status.Mode, backend)
		}
		status.Message = fmt.Sprintf("system sandbox backend %q is active", status.BackendName)
		return status, nil
	}
	status.CapabilityLevel = "none"
	if status.Mode == systemSandboxModeBestEffort {
		status.Fallback = true
		reason := strings.TrimSpace(backend.UnavailableReason)
		if reason == "" {
			reason = fmt.Sprintf("no backend available on %s", status.GOOS)
		}
		status.Message = fmt.Sprintf("system sandbox best_effort fallback: %s", reason)
		return status, nil
	}
	status.Message = "system sandbox backend is inactive"
	return status, nil
}
