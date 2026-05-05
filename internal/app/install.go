package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var installCommandLookPath = exec.LookPath

func RunInstall(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	installDir := fs.String("to", "", "Install directory. Defaults to ~/bin.")
	binaryName := fs.String("name", "", "Binary name. Defaults to bytemind (bytemind.exe on Windows).")
	addToPath := fs.Bool("add-to-path", true, "Automatically add install directory to user PATH when possible.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("install does not accept positional args: %s", strings.Join(fs.Args(), " "))
	}

	sourcePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	targetPath, err := resolveInstallTarget(*installDir, *binaryName)
	if err != nil {
		return err
	}
	if err := installBinary(sourcePath, targetPath); err != nil {
		return err
	}

	targetDir := filepath.Dir(targetPath)
	commandName := installCommandName(targetPath)
	fmt.Fprintf(stdout, "Installed Bytemind to %s\n", targetPath)
	printPathShadowWarning(stdout, commandName, targetPath)
	if pathContainsDir(os.Getenv("PATH"), targetDir) {
		fmt.Fprintf(stdout, "PATH already includes this directory in this terminal. You can now run: %s\n", commandName)
		return nil
	}
	if *addToPath {
		changed, err := addInstallDirToUserPath(targetDir)
		if err == nil {
			if changed {
				fmt.Fprintln(stdout, "Added install directory to user PATH.")
				fmt.Fprintf(stdout, "Open a new terminal, then run: %s\n", commandName)
			} else {
				fmt.Fprintln(stdout, "Install directory already exists in user PATH.")
			}
			return nil
		}
		fmt.Fprintf(stdout, "Automatic PATH update failed: %v\n", err)
	}

	fmt.Fprintf(stdout, "Add this directory to PATH to run Bytemind from anywhere:\n%s\n", targetDir)
	printPathHint(stdout, targetDir)
	return nil
}

func installCommandName(targetPath string) string {
	name := filepath.Base(strings.TrimSpace(targetPath))
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(name), ".exe") {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "bytemind"
	}
	return name
}

func printPathShadowWarning(w io.Writer, commandName, targetPath string) {
	resolvedPath, err := installCommandLookPath(commandName)
	if err != nil {
		return
	}
	resolvedPath = strings.TrimSpace(resolvedPath)
	if resolvedPath == "" || sameCommandPath(resolvedPath, targetPath) {
		return
	}

	fmt.Fprintf(w, "Warning: %s on PATH resolves to %s, not the installed binary.\n", commandName, resolvedPath)
	fmt.Fprintf(w, "Run %s directly, or move %s earlier in PATH.\n", targetPath, filepath.Dir(targetPath))
}

func sameCommandPath(a, b string) bool {
	aPath := normalizeComparablePath(a)
	bPath := normalizeComparablePath(b)
	if aPath == "" || bPath == "" {
		return false
	}
	if aPath == bPath {
		return true
	}

	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	if aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo) {
		return true
	}

	aReal, aErr := filepath.EvalSymlinks(a)
	bReal, bErr := filepath.EvalSymlinks(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return normalizeComparablePath(aReal) == normalizeComparablePath(bReal)
}

func normalizeComparablePath(path string) string {
	path = strings.TrimSpace(strings.Trim(path, `"`))
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return normalizePathEntry(path, runtime.GOOS == "windows")
}
