package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunInstallRejectsPositionalArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunInstall([]string{"extra-arg"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected install positional-arg validation error")
	}
	if !strings.Contains(err.Error(), "does not accept positional args") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInstallWarnsWhenCommandIsShadowedInPath(t *testing.T) {
	previousLookPath := installCommandLookPath
	t.Cleanup(func() {
		installCommandLookPath = previousLookPath
	})

	root := t.TempDir()
	targetDir := filepath.Join(root, "bin")
	shadowDir := filepath.Join(root, "old")
	shadowPath := filepath.Join(shadowDir, defaultBinaryName(runtime.GOOS))
	if err := os.MkdirAll(shadowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shadowPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	installCommandLookPath = func(string) (string, error) {
		return shadowPath, nil
	}
	t.Setenv("PATH", strings.Join([]string{shadowDir, targetDir}, string(os.PathListSeparator)))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := RunInstall([]string{"-to", targetDir}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "not the installed binary") {
		t.Fatalf("expected PATH shadow warning, got %q", output)
	}
	if !strings.Contains(output, targetDir) {
		t.Fatalf("expected warning to mention target dir %q, got %q", targetDir, output)
	}
}

func TestRunInstallPrintsPathHintWhenPathUpdateDisabled(t *testing.T) {
	previousLookPath := installCommandLookPath
	t.Cleanup(func() {
		installCommandLookPath = previousLookPath
	})
	installCommandLookPath = func(string) (string, error) {
		return "", os.ErrNotExist
	}

	root := t.TempDir()
	targetDir := filepath.Join(root, "bin")
	t.Setenv("PATH", filepath.Join(root, "other"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := RunInstall([]string{"-to", targetDir, "-add-to-path=false"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Add this directory to PATH") {
		t.Fatalf("expected manual PATH hint, got %q", output)
	}
	if !strings.Contains(output, targetDir) {
		t.Fatalf("expected output to mention target dir %q, got %q", targetDir, output)
	}
}

func TestInstallCommandNameFallsBackForEmptyPath(t *testing.T) {
	if got := installCommandName(""); got != "bytemind" {
		t.Fatalf("expected empty path to fall back to bytemind, got %q", got)
	}
}

func TestSameCommandPathTreatsHardlinksAsSameFile(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "bytemind.exe")
	linked := filepath.Join(dir, "linked-bytemind.exe")
	if err := os.WriteFile(original, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, linked); err != nil {
		t.Skipf("hardlinks are not available: %v", err)
	}
	if !sameCommandPath(original, linked) {
		t.Fatalf("expected hardlinked paths to be treated as the same command: %q %q", original, linked)
	}
}

func TestSameCommandPathBranches(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.exe")
	second := filepath.Join(dir, "second.exe")
	if err := os.WriteFile(first, []byte("first"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o755); err != nil {
		t.Fatal(err)
	}

	if sameCommandPath("", first) {
		t.Fatal("expected empty command path not to match")
	}
	if !sameCommandPath(first, first) {
		t.Fatal("expected identical command paths to match")
	}
	if sameCommandPath(first, second) {
		t.Fatal("expected different files not to match")
	}
	if sameCommandPath(first, filepath.Join(dir, "missing.exe")) {
		t.Fatal("expected missing target path not to match")
	}
}

func TestPrintPathShadowWarningSkipsWhenLookPathFailsOrMatches(t *testing.T) {
	previousLookPath := installCommandLookPath
	t.Cleanup(func() {
		installCommandLookPath = previousLookPath
	})

	var stdout bytes.Buffer
	installCommandLookPath = func(string) (string, error) {
		return "", os.ErrNotExist
	}
	printPathShadowWarning(&stdout, "bytemind", "missing")
	if stdout.Len() != 0 {
		t.Fatalf("expected no output when command lookup fails, got %q", stdout.String())
	}

	target := filepath.Join(t.TempDir(), defaultBinaryName(runtime.GOOS))
	if err := os.WriteFile(target, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	installCommandLookPath = func(string) (string, error) {
		return target, nil
	}
	printPathShadowWarning(&stdout, "bytemind", target)
	if stdout.Len() != 0 {
		t.Fatalf("expected no output when command resolves to target, got %q", stdout.String())
	}
}

func TestDefaultBinaryName(t *testing.T) {
	if got := defaultBinaryName("windows"); got != "bytemind.exe" {
		t.Fatalf("expected windows binary name with .exe, got %q", got)
	}
	if got := defaultBinaryName("linux"); got != "bytemind" {
		t.Fatalf("expected non-windows binary name without extension, got %q", got)
	}
}

func TestResolveInstallTargetUsesUserHomeBinByDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BYTEMIND_HOME", filepath.Join(t.TempDir(), ".bytemind-home"))
	setUserHomeEnv(t, home)

	target, err := resolveInstallTarget("", "custom-bin")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "bin", "custom-bin")
	if !sameInstallPath(target, want) {
		t.Fatalf("expected target %q, got %q", want, target)
	}
}

func TestResolveInstallTargetRejectsNamePath(t *testing.T) {
	_, err := resolveInstallTarget("", "nested/bytemind")
	if err == nil {
		t.Fatal("expected install target resolution to reject path-like name")
	}
}

func TestInstallBinaryCopiesExecutableFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.exe")
	content := []byte("binary-content")
	if err := os.WriteFile(source, content, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "bin", "bytemind.exe")
	if err := installBinary(source, target); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("expected target content %q, got %q", string(content), string(got))
	}
}

func TestPathContainsDirForOS(t *testing.T) {
	pathEnv := strings.Join([]string{"C:/Tools", "C:/Users/Wheat/bin"}, ";")
	if !pathContainsDirForOS(pathEnv, `c:\users\wheat\bin`, true) {
		t.Fatal("expected windows path lookup to be case-insensitive and slash-insensitive")
	}
	if pathContainsDirForOS(pathEnv, `C:\missing`, true) {
		t.Fatal("did not expect missing path entry")
	}
}

func TestAppendPathEntryAvoidsDuplicates(t *testing.T) {
	current := strings.Join([]string{`C:\Tools`, `C:\Users\wheat\bin`}, ";")
	next, changed := appendPathEntry(current, `c:\users\wheat\bin`, true)
	if changed {
		t.Fatalf("expected duplicate path to be ignored, got changed=true next=%q", next)
	}
	if next != current {
		t.Fatalf("expected unchanged path, got %q", next)
	}
}

func TestAppendPathEntryAddsMissingEntry(t *testing.T) {
	current := `C:\Tools`
	next, changed := appendPathEntry(current, `C:\Users\wheat\bin`, true)
	if !changed {
		t.Fatal("expected missing path entry to be appended")
	}
	if !strings.Contains(next, `C:\Users\wheat\bin`) {
		t.Fatalf("expected appended path entry, got %q", next)
	}
}

func TestAddToWindowsUserPathUsesGetterAndSetter(t *testing.T) {
	originalGetter := windowsUserPathGetter
	originalSetter := windowsUserPathSetter
	t.Cleanup(func() {
		windowsUserPathGetter = originalGetter
		windowsUserPathSetter = originalSetter
	})

	windowsUserPathGetter = func() (string, error) {
		return `C:\Tools`, nil
	}
	captured := ""
	windowsUserPathSetter = func(newPath string) error {
		captured = newPath
		return nil
	}

	changed, err := addToWindowsUserPath(`C:\Users\wheat\bin`)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected user path to change")
	}
	if !strings.Contains(captured, `C:\Users\wheat\bin`) {
		t.Fatalf("expected setter to receive appended path, got %q", captured)
	}
}

func TestResolveWindowsPowerShellExecutablePrefersLookPathCandidate(t *testing.T) {
	got := resolveWindowsPowerShellExecutable(
		func(file string) (string, error) {
			if file == "powershell.exe" {
				return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, nil
			}
			return "", errors.New("not found")
		},
		func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		func(key string) string { return "" },
	)
	if !strings.EqualFold(got, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`) {
		t.Fatalf("expected lookPath candidate, got %q", got)
	}
}

func TestResolveWindowsPowerShellExecutableFallsBackToAbsoluteCandidate(t *testing.T) {
	windowsRoot := `C:\Windows`
	expected := filepath.Join(windowsRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	got := resolveWindowsPowerShellExecutable(
		func(file string) (string, error) {
			return "", errors.New("not found")
		},
		func(name string) (os.FileInfo, error) {
			if strings.EqualFold(name, expected) {
				return stubInstallFileInfo{}, nil
			}
			return nil, os.ErrNotExist
		},
		func(key string) string {
			if key == "SystemRoot" {
				return windowsRoot
			}
			return ""
		},
	)
	if !strings.EqualFold(got, expected) {
		t.Fatalf("expected absolute fallback %q, got %q", expected, got)
	}
}

func TestResolveWindowsPowerShellExecutableFallbacksToPowerShellLiteral(t *testing.T) {
	got := resolveWindowsPowerShellExecutable(
		func(file string) (string, error) { return "", errors.New("not found") },
		func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(key string) string { return "" },
	)
	if got != "powershell" {
		t.Fatalf("expected final fallback powershell, got %q", got)
	}
}

func sameInstallPath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func setUserHomeEnv(t *testing.T, home string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
		return
	}
	t.Setenv("HOME", home)
}

type stubInstallFileInfo struct{}

func (stubInstallFileInfo) Name() string       { return "powershell.exe" }
func (stubInstallFileInfo) Size() int64        { return 0 }
func (stubInstallFileInfo) Mode() os.FileMode  { return 0o644 }
func (stubInstallFileInfo) ModTime() time.Time { return time.Time{} }
func (stubInstallFileInfo) IsDir() bool        { return false }
func (stubInstallFileInfo) Sys() any           { return nil }
