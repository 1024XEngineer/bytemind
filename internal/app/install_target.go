package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/config"
)

func resolveInstallTarget(dirValue, nameValue string) (string, error) {
	dir := strings.TrimSpace(dirValue)
	if dir == "" {
		home, err := config.ResolveHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "bin")
	} else {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		dir = abs
	}

	name := strings.TrimSpace(nameValue)
	if name == "" {
		name = defaultBinaryName(runtime.GOOS)
	}
	if strings.ContainsAny(name, `/\`) {
		return "", errors.New("install -name must be a file name, not a path")
	}

	return filepath.Join(dir, name), nil
}

func defaultBinaryName(goos string) string {
	if strings.EqualFold(goos, "windows") {
		return "bytemind.exe"
	}
	return "bytemind"
}

func installBinary(sourcePath, targetPath string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	targetPath = strings.TrimSpace(targetPath)
	if sourcePath == "" || targetPath == "" {
		return errors.New("source and target path are required")
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("source executable is a directory: %s", sourcePath)
	}

	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	tempPath := targetPath + ".tmp"
	_ = os.Remove(tempPath)
	targetFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, sourceInfo.Mode())
	if err != nil {
		return err
	}

	copyOK := false
	defer func() {
		if !copyOK {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		_ = targetFile.Close()
		return err
	}
	if err := targetFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, sourceInfo.Mode()); err != nil && runtime.GOOS != "windows" {
		return err
	}

	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return err
	}
	copyOK = true
	return nil
}

func printPathHint(w io.Writer, targetDir string) {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(w, "PowerShell (current terminal):")
		fmt.Fprintf(w, "$env:Path = \"%s;\" + $env:Path\n", targetDir)
		fmt.Fprintln(w, "PowerShell (persist for future terminals):")
		fmt.Fprintf(w, "$userPath = [Environment]::GetEnvironmentVariable('Path','User'); if ($userPath -notlike '*%s*') { [Environment]::SetEnvironmentVariable('Path', $userPath + ';%s', 'User') }\n", targetDir, targetDir)
		return
	}
	fmt.Fprintln(w, "Shell (current terminal):")
	fmt.Fprintf(w, "export PATH=\"%s:$PATH\"\n", targetDir)
	fmt.Fprintln(w, "Persist in your shell profile (~/.bashrc, ~/.zshrc, etc.).")
}
