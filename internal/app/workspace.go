package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultBroadWorkspaceEntryThreshold = 300

var workspaceProjectMarkers = []string{
	".git",
	"go.mod",
	"package.json",
	"pnpm-workspace.yaml",
	"pyproject.toml",
	"Cargo.toml",
	"pom.xml",
	"build.gradle",
	"Makefile",
}

func ResolveWorkspace(workspaceOverride string) (string, error) {
	workspaceOverride = strings.TrimSpace(workspaceOverride)
	if workspaceOverride != "" {
		workspace, err := filepath.Abs(workspaceOverride)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(workspace)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("workspace must be a directory: %s", workspace)
		}
		return workspace, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return "", err
	}

	if projectRoot := DetectProjectRoot(cwd); projectRoot != "" {
		return projectRoot, nil
	}
	if IsBroadWorkspacePath(cwd) {
		return "", fmt.Errorf("current directory %s is too broad for default workspace; rerun with -workspace <project-dir> (or set BYTEMIND_ALLOW_BROAD_WORKSPACE=true)", cwd)
	}
	return cwd, nil
}

func DetectProjectRoot(start string) string {
	current := strings.TrimSpace(start)
	if current == "" {
		return ""
	}
	current = filepath.Clean(current)
	for {
		if hasProjectMarker(current) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func hasProjectMarker(dir string) bool {
	for _, marker := range workspaceProjectMarkers {
		if marker == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func IsBroadWorkspacePath(dir string) bool {
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_ALLOW_BROAD_WORKSPACE")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil && parsed {
			return false
		}
	}
	home, _ := os.UserHomeDir()
	return IsBroadWorkspacePathWithHome(dir, home)
}

func IsBroadWorkspacePathWithHome(dir, home string) bool {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" {
		return false
	}

	if IsFilesystemRoot(dir) {
		return true
	}

	home = strings.TrimSpace(home)
	if home != "" {
		home = filepath.Clean(home)
		if SameWorkspace(dir, home) {
			return true
		}
		for _, name := range []string{"Desktop", "Documents", "Downloads"} {
			if SameWorkspace(dir, filepath.Join(home, name)) {
				return true
			}
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) >= DefaultBroadWorkspaceEntryThreshold
}

func IsFilesystemRoot(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	parent := filepath.Dir(path)
	return parent == path
}
