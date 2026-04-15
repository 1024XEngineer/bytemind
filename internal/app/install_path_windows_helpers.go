package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func addInstallDirToUserPath(targetDir string) (bool, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return false, errors.New("install directory is empty")
	}
	if runtime.GOOS != "windows" {
		return false, errors.New("automatic PATH update is currently supported on Windows only")
	}
	return addToWindowsUserPath(targetDir)
}

var windowsUserPathGetter = func() (string, error) {
	cmd := windowsPowerShellCommand("-NoProfile", "-Command", "[Environment]::GetEnvironmentVariable('Path','User')")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read user PATH via PowerShell: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

var windowsUserPathSetter = func(newPath string) error {
	cmd := windowsPowerShellCommand("-NoProfile", "-Command", "[Environment]::SetEnvironmentVariable('Path', $env:BYTEMIND_USER_PATH, 'User')")
	cmd.Env = append(os.Environ(), "BYTEMIND_USER_PATH="+newPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write user PATH via PowerShell: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func windowsPowerShellCommand(args ...string) *exec.Cmd {
	executable := resolveWindowsPowerShellExecutable(exec.LookPath, os.Stat, os.Getenv)
	return exec.Command(executable, args...)
}

func resolveWindowsPowerShellExecutable(
	lookPath func(file string) (string, error),
	statFn func(name string) (os.FileInfo, error),
	getenv func(key string) string,
) string {
	for _, candidate := range windowsPowerShellCandidates(getenv) {
		if isWindowsAbsolutePath(candidate) {
			info, err := statFn(candidate)
			if err == nil && info != nil && !info.IsDir() {
				return candidate
			}
			continue
		}
		resolved, err := lookPath(candidate)
		if err == nil && strings.TrimSpace(resolved) != "" {
			return resolved
		}
	}
	return "powershell"
}

func windowsPowerShellCandidates(getenv func(key string) string) []string {
	candidates := []string{
		"powershell.exe",
		"powershell",
		"pwsh.exe",
		"pwsh",
	}

	appendWindowsRoot := func(root string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		candidates = append(candidates,
			filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
			filepath.Join(root, "Sysnative", "WindowsPowerShell", "v1.0", "powershell.exe"),
		)
	}

	appendWindowsRoot(getenv("SystemRoot"))
	appendWindowsRoot(getenv("WINDIR"))

	if programFiles := strings.TrimSpace(getenv("ProgramFiles")); programFiles != "" {
		candidates = append(candidates, filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe"))
	}

	seen := make(map[string]struct{}, len(candidates))
	uniq := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniq = append(uniq, candidate)
	}
	return uniq
}

func isWindowsAbsolutePath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return true
	}
	if len(path) >= 2 && path[0] == '\\' && path[1] == '\\' {
		return true
	}
	return false
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func addToWindowsUserPath(targetDir string) (bool, error) {
	currentUserPath, err := windowsUserPathGetter()
	if err != nil {
		return false, err
	}
	nextUserPath, changed := appendPathEntry(currentUserPath, targetDir, true)
	if !changed {
		return false, nil
	}
	if err := windowsUserPathSetter(nextUserPath); err != nil {
		return false, err
	}
	return true, nil
}
