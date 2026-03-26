package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Workspace struct {
	Root string
}

func New(root string) (*Workspace, error) {
	absPath, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, err
	}

	return &Workspace{Root: absPath}, nil
}

func (w *Workspace) IsTrusted() bool {
	trustedFiles := []string{
		".forgecli",
		".git",
		"package.json",
		"go.mod",
		"Cargo.toml",
		"pom.xml",
		"requirements.txt",
	}

	for _, f := range trustedFiles {
		if _, err := os.Stat(filepath.Join(w.Root, f)); err == nil {
			return true
		}
	}
	return false
}

func (w *Workspace) IsPathSafe(target string) bool {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absTarget, w.Root)
}

func (w *Workspace) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(w.Root, path)
}

func (w *Workspace) ListFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(w.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(w.Root, path)
		if err != nil {
			return nil
		}

		if strings.HasPrefix(rel, ".") {
			return nil
		}

		skipDirs := []string{"node_modules", "vendor", ".git", "dist", "build", "__pycache__"}
		for _, skip := range skipDirs {
			if strings.Contains(rel, skip) {
				return nil
			}
		}

		files = append(files, rel)
		return nil
	})

	return files, err
}

func (w *Workspace) GlobFiles(pattern string) ([]string, error) {
	allFiles, err := w.ListFiles()
	if err != nil {
		return nil, err
	}

	var matched []string
	for _, f := range allFiles {
		isMatch, _ := filepath.Match(pattern, filepath.Base(f))
		if isMatch {
			matched = append(matched, f)
		}
	}
	return matched, nil
}

func (w *Workspace) GrepFiles(pattern string) ([]string, error) {
	allFiles, err := w.ListFiles()
	if err != nil {
		return nil, err
	}

	var results []string
	for _, f := range allFiles {
		path := w.ResolvePath(f)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				results = append(results, fmt.Sprintf("%s:%d: %s", f, i+1, line))
			}
		}
	}
	return results, nil
}

func (w *Workspace) ReadFile(path string) (string, error) {
	fullPath := w.ResolvePath(path)
	if !w.IsPathSafe(fullPath) {
		return "", os.ErrPermission
	}
	data, err := os.ReadFile(fullPath)
	return string(data), err
}

func (w *Workspace) WriteFile(path, content string) error {
	fullPath := w.ResolvePath(path)
	if !w.IsPathSafe(fullPath) {
		return os.ErrPermission
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, []byte(content), 0644)
}

func (w *Workspace) FileExists(path string) bool {
	fullPath := w.ResolvePath(path)
	_, err := os.Stat(fullPath)
	return err == nil
}
