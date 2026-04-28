package tools

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type systemSandboxPolicy struct {
	FileIsolation    bool
	ProcessIsolation bool
	NetworkIsolation bool
}

type SystemSandboxBackendCapabilities struct {
	Name                   string
	Known                  bool
	ShellNetworkIsolation  bool
	WorkerNetworkIsolation bool
}

type systemSandboxLaunchSpec struct {
	ArgPrefix []string
	Policy    systemSandboxPolicy
}

type systemSandboxPlatformBackend interface {
	Name() string
	Probe(lookPath func(string) (string, error)) (string, error)
	ShellLaunchSpec() systemSandboxLaunchSpec
	WorkerLaunchSpec() systemSandboxLaunchSpec
}

type linuxUnshareSystemSandboxBackend struct{}

func (linuxUnshareSystemSandboxBackend) Name() string {
	return "linux_unshare"
}

func (linuxUnshareSystemSandboxBackend) Probe(lookPath func(string) (string, error)) (string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	runner, err := lookPath("unshare")
	if err != nil || strings.TrimSpace(runner) == "" {
		return "", errors.New("linux backend \"unshare\" is unavailable")
	}
	return runner, nil
}

func (linuxUnshareSystemSandboxBackend) ShellLaunchSpec() systemSandboxLaunchSpec {
	return systemSandboxLaunchSpec{
		ArgPrefix: append(linuxSystemSandboxNamespaceArgs(), "sh", "-lc"),
		Policy: systemSandboxPolicy{
			FileIsolation:    true,
			ProcessIsolation: true,
			NetworkIsolation: true,
		},
	}
}

func (linuxUnshareSystemSandboxBackend) WorkerLaunchSpec() systemSandboxLaunchSpec {
	return systemSandboxLaunchSpec{
		ArgPrefix: append([]string(nil), linuxSystemSandboxWorkerArgs()...),
		Policy: systemSandboxPolicy{
			FileIsolation:    true,
			ProcessIsolation: true,
			NetworkIsolation: false,
		},
	}
}

type systemSandboxRuntimeBackend struct {
	Enabled           bool
	Name              string
	Runner            string
	Shell             systemSandboxLaunchSpec
	Worker            systemSandboxLaunchSpec
	RequiredCapable   bool
	CapabilityLevel   string
	UnavailableReason string
}

func resolveSystemSandboxRuntimeBackend(mode, goos string, lookPath func(string) (string, error)) (systemSandboxRuntimeBackend, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == systemSandboxModeOff {
		return systemSandboxRuntimeBackend{}, nil
	}

	backend := systemSandboxBackendForOS(goos)
	if backend == nil {
		if mode == systemSandboxModeRequired {
			return systemSandboxRuntimeBackend{}, fmt.Errorf("system sandbox mode required but no backend is available on %s", goos)
		}
		return systemSandboxRuntimeBackend{
			UnavailableReason: fmt.Sprintf("no backend is available on %s", strings.TrimSpace(goos)),
		}, nil
	}

	runner, err := backend.Probe(lookPath)
	if err != nil || strings.TrimSpace(runner) == "" {
		if mode == systemSandboxModeRequired {
			if err != nil {
				return systemSandboxRuntimeBackend{}, fmt.Errorf("system sandbox mode required but %s", err.Error())
			}
			return systemSandboxRuntimeBackend{}, errors.New("system sandbox mode required but backend is unavailable")
		}
		reason := "backend is unavailable"
		if err != nil {
			reason = strings.TrimSpace(err.Error())
		}
		if reason == "" {
			reason = "backend probe returned an empty runner path"
		}
		return systemSandboxRuntimeBackend{
			Name:              backend.Name(),
			UnavailableReason: reason,
		}, nil
	}

	resolved := systemSandboxRuntimeBackend{
		Enabled: true,
		Name:    backend.Name(),
		Runner:  runner,
		Shell:   backend.ShellLaunchSpec(),
		Worker:  backend.WorkerLaunchSpec(),
	}
	resolved.RequiredCapable = requiredSystemSandboxCapabilitiesSatisfied(resolved)
	if mode == systemSandboxModeRequired &&
		!resolved.RequiredCapable &&
		strings.EqualFold(strings.TrimSpace(goos), "windows") {
		resolved.RequiredCapable = requiredWindowsRequiredCapabilitiesSatisfied(resolved)
	}
	resolved.CapabilityLevel = systemSandboxCapabilityLevel(mode, resolved)
	if mode == systemSandboxModeRequired && !resolved.RequiredCapable {
		return systemSandboxRuntimeBackend{}, fmt.Errorf("system sandbox mode required but backend %q lacks required file/process isolation capabilities", resolved.Name)
	}
	return resolved, nil
}

func systemSandboxCapabilityLevel(mode string, backend systemSandboxRuntimeBackend) string {
	if !backend.Enabled {
		return "none"
	}
	if requiredSystemSandboxCapabilitiesSatisfied(backend) {
		return "full"
	}
	if strings.EqualFold(strings.TrimSpace(mode), systemSandboxModeRequired) &&
		requiredWindowsRequiredCapabilitiesSatisfied(backend) {
		return "guarded"
	}
	if strings.EqualFold(strings.TrimSpace(backend.Name), "windows_job_object") {
		return "guarded"
	}
	return "partial"
}

func requiredSystemSandboxCapabilitiesSatisfied(backend systemSandboxRuntimeBackend) bool {
	if !backend.Enabled {
		return false
	}
	if !backend.Shell.Policy.ProcessIsolation || !backend.Worker.Policy.ProcessIsolation {
		return false
	}
	if !backend.Shell.Policy.FileIsolation || !backend.Worker.Policy.FileIsolation {
		return false
	}
	return true
}

func requiredWindowsRequiredCapabilitiesSatisfied(backend systemSandboxRuntimeBackend) bool {
	if !backend.Enabled {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(backend.Name), "windows_job_object") {
		return false
	}
	// Windows required mode relies on Job Object process isolation and a strict
	// read-only shell command guard in run_shell.
	return backend.Shell.Policy.ProcessIsolation && backend.Worker.Policy.ProcessIsolation
}

func systemSandboxBackendForOS(goos string) systemSandboxPlatformBackend {
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "linux":
		return linuxUnshareSystemSandboxBackend{}
	case "darwin":
		return darwinSandboxExecSystemSandboxBackend{}
	case "windows":
		return windowsJobObjectSystemSandboxBackend{}
	default:
		return nil
	}
}

func systemSandboxBackendByName(name string) systemSandboxPlatformBackend {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "linux_unshare":
		return linuxUnshareSystemSandboxBackend{}
	case "darwin_sandbox_exec":
		return darwinSandboxExecSystemSandboxBackend{}
	case "windows_job_object":
		return windowsJobObjectSystemSandboxBackend{}
	default:
		return nil
	}
}

func SystemSandboxBackendCapabilitiesForName(name string) SystemSandboxBackendCapabilities {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "future_worker_net_isolated":
		return SystemSandboxBackendCapabilities{
			Name:                   normalized,
			Known:                  true,
			ShellNetworkIsolation:  false,
			WorkerNetworkIsolation: true,
		}
	case "future_shell_net_isolated":
		return SystemSandboxBackendCapabilities{
			Name:                   normalized,
			Known:                  true,
			ShellNetworkIsolation:  true,
			WorkerNetworkIsolation: false,
		}
	}

	backend := systemSandboxBackendByName(normalized)
	if backend == nil {
		return SystemSandboxBackendCapabilities{Name: normalized, Known: false}
	}
	shell := backend.ShellLaunchSpec()
	worker := backend.WorkerLaunchSpec()
	return SystemSandboxBackendCapabilities{
		Name:                   normalized,
		Known:                  true,
		ShellNetworkIsolation:  shell.Policy.NetworkIsolation,
		WorkerNetworkIsolation: worker.Policy.NetworkIsolation,
	}
}

type darwinSandboxExecSystemSandboxBackend struct{}

func (darwinSandboxExecSystemSandboxBackend) Name() string {
	return "darwin_sandbox_exec"
}

func (darwinSandboxExecSystemSandboxBackend) Probe(lookPath func(string) (string, error)) (string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	runner, err := lookPath("sandbox-exec")
	if err != nil || strings.TrimSpace(runner) == "" {
		return "", errors.New("darwin backend \"sandbox-exec\" is unavailable")
	}
	return runner, nil
}

func (darwinSandboxExecSystemSandboxBackend) ShellLaunchSpec() systemSandboxLaunchSpec {
	return systemSandboxLaunchSpec{
		Policy: systemSandboxPolicy{
			FileIsolation:    true,
			ProcessIsolation: true,
			NetworkIsolation: false,
		},
	}
}

func (darwinSandboxExecSystemSandboxBackend) WorkerLaunchSpec() systemSandboxLaunchSpec {
	return systemSandboxLaunchSpec{
		Policy: systemSandboxPolicy{
			FileIsolation:    true,
			ProcessIsolation: true,
			NetworkIsolation: false,
		},
	}
}

type windowsJobObjectSystemSandboxBackend struct{}

func (windowsJobObjectSystemSandboxBackend) Name() string {
	return "windows_job_object"
}

func (windowsJobObjectSystemSandboxBackend) Probe(func(string) (string, error)) (string, error) {
	return "windows-job-object", nil
}

func (windowsJobObjectSystemSandboxBackend) ShellLaunchSpec() systemSandboxLaunchSpec {
	return systemSandboxLaunchSpec{
		Policy: systemSandboxPolicy{
			FileIsolation:    false,
			ProcessIsolation: true,
			NetworkIsolation: false,
		},
	}
}

func (windowsJobObjectSystemSandboxBackend) WorkerLaunchSpec() systemSandboxLaunchSpec {
	return systemSandboxLaunchSpec{
		Policy: systemSandboxPolicy{
			FileIsolation:    false,
			ProcessIsolation: true,
			NetworkIsolation: false,
		},
	}
}
