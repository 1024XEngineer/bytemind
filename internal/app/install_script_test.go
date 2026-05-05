package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func loadInstallScript(t *testing.T, name string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	scriptPath := filepath.Join(repoRoot, "scripts", name)

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return string(content)
}

func TestInstallPSScript_ArchitectureFallbackForLegacyPowerShell(t *testing.T) {
	script := loadInstallScript(t, "install.ps1")

	requiredSnippets := []string{
		`GetProperty("OSArchitecture")`,
		`PROCESSOR_ARCHITEW6432`,
		`PROCESSOR_ARCHITECTURE`,
		`"AMD64" { return "amd64" }`,
		`"ARM64" { return "arm64" }`,
	}

	for _, snippet := range requiredSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("install.ps1 missing legacy-compat architecture logic snippet: %q", snippet)
		}
	}
}

func TestInstallScriptsDefaultToUserHomeBin(t *testing.T) {
	shellScript := loadInstallScript(t, "install.sh")
	if !strings.Contains(shellScript, `INSTALL_DIR="${BYTEMIND_INSTALL_DIR:-$HOME/bin}"`) {
		t.Fatal("install.sh should default BYTEMIND_INSTALL_DIR to $HOME/bin")
	}

	powerShellScript := loadInstallScript(t, "install.ps1")
	if !strings.Contains(powerShellScript, `$installDir = if ($env:BYTEMIND_INSTALL_DIR) { $env:BYTEMIND_INSTALL_DIR } else { Join-Path $HOME "bin" }`) {
		t.Fatal("install.ps1 should default BYTEMIND_INSTALL_DIR to $HOME\\bin")
	}
}

func TestInstallScriptsWarnWhenPathResolutionIsShadowed(t *testing.T) {
	shellScript := loadInstallScript(t, "install.sh")
	for _, snippet := range []string{
		`show_path_resolution_hint`,
		`command -v bytemind`,
		`not ${installed_binary}`,
		`move ${INSTALL_DIR} earlier in PATH`,
	} {
		if !strings.Contains(shellScript, snippet) {
			t.Fatalf("install.sh missing PATH shadow warning snippet: %q", snippet)
		}
	}

	powerShellScript := loadInstallScript(t, "install.ps1")
	for _, snippet := range []string{
		`Show-PathResolutionHint`,
		`Get-Command bytemind`,
		`not $InstalledBinary`,
		`before the older PATH entry`,
	} {
		if !strings.Contains(powerShellScript, snippet) {
			t.Fatalf("install.ps1 missing PATH shadow warning snippet: %q", snippet)
		}
	}
}
