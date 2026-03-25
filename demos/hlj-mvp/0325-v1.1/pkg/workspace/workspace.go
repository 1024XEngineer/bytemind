package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

	cleanRoot := filepath.Clean(absPath)
	info, err := os.Stat(cleanRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", cleanRoot)
	}

	return &Workspace{Root: cleanRoot}, nil
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

	for _, name := range trustedFiles {
		if _, err := os.Stat(filepath.Join(w.Root, name)); err == nil {
			return true
		}
	}

	return false
}

func (w *Workspace) IsPathSafe(target string) bool {
	_, err := w.ResolvePath(target)
	return err == nil
}

func (w *Workspace) ResolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(w.Root, candidate)
	}

	cleanCandidate := filepath.Clean(candidate)
	resolvedCandidate, err := resolveForContainment(cleanCandidate)
	if err != nil {
		return "", err
	}

	resolvedRoot, err := resolveForContainment(w.Root)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil {
		return "", err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", os.ErrPermission
	}

	return cleanCandidate, nil
}

func (w *Workspace) ListFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(w.Root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(w.Root, path)
		if err != nil {
			return nil
		}

		if rel == "." {
			return nil
		}

		if shouldSkipPath(rel, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
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

	normalizedPattern := normalizeGlobPattern(pattern)
	if normalizedPattern == "" || normalizedPattern == "." {
		return allFiles, nil
	}

	var matched []string
	for _, relPath := range allFiles {
		matchRel := globMatches(normalizedPattern, relPath)
		matchBase := globMatches(normalizedPattern, filepath.Base(relPath))
		if matchRel || matchBase {
			matched = append(matched, relPath)
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
	for _, relPath := range allFiles {
		fullPath, err := w.ResolvePath(relPath)
		if err != nil {
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		if isProbablyBinary(data) {
			continue
		}

		content := string(data)
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, line))
			}
		}
	}

	return results, nil
}

func (w *Workspace) ReadFile(path string) (string, error) {
	fullPath, err := w.ResolvePath(path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (w *Workspace) WriteFile(path, content string) error {
	fullPath, err := w.ResolvePath(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, []byte(content), 0644)
}

func (w *Workspace) Remove(path string) error {
	fullPath, err := w.ResolvePath(path)
	if err != nil {
		return err
	}

	return os.Remove(fullPath)
}

func (w *Workspace) CreateDirectory(path string) error {
	fullPath, err := w.ResolvePath(path)
	if err != nil {
		return err
	}

	return os.MkdirAll(fullPath, 0755)
}

func (w *Workspace) FileExists(path string) bool {
	fullPath, err := w.ResolvePath(path)
	if err != nil {
		return false
	}

	_, statErr := os.Stat(fullPath)
	return statErr == nil
}

func shouldSkipPath(rel string, isDir bool) bool {
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) == 0 {
		return false
	}

	if strings.HasPrefix(parts[0], ".") {
		return true
	}

	skipNames := map[string]struct{}{
		".git":         {},
		".gocache":     {},
		"node_modules": {},
		"vendor":       {},
		"dist":         {},
		"build":        {},
		"__pycache__":  {},
	}

	for _, part := range parts {
		if _, ok := skipNames[part]; ok {
			return true
		}
	}

	if isDir && strings.HasPrefix(filepath.Base(rel), ".") {
		return true
	}

	if !isDir && shouldSkipFileByExtension(rel) {
		return true
	}

	return false
}

func shouldSkipFileByExtension(rel string) bool {
	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".exe", ".dll", ".so", ".dylib", ".bin", ".class", ".jar", ".pyc", ".pyd",
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf", ".zip", ".gz", ".tar":
		return true
	default:
		return false
	}
}

func isProbablyBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}

	if bytes.IndexByte(sample, 0) >= 0 {
		return true
	}

	nonPrintable := 0
	for _, b := range sample {
		if b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b < 32 || b > 126 {
			nonPrintable++
		}
	}

	return nonPrintable*5 > len(sample)
}

func normalizeGlobPattern(pattern string) string {
	trimmed := strings.TrimSpace(pattern)
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	trimmed = strings.TrimPrefix(trimmed, "./")
	return path.Clean(trimmed)
}

func globMatches(pattern, relPath string) bool {
	normalizedPath := strings.ReplaceAll(relPath, "\\", "/")
	regex := globPatternToRegexp(pattern)
	return regex.MatchString(normalizedPath)
}

func globPatternToRegexp(pattern string) *regexp.Regexp {
	var builder strings.Builder
	builder.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '*' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				builder.WriteString(".*")
				i++
				continue
			}
			builder.WriteString(`[^/]*`)
			continue
		}
		if ch == '?' {
			builder.WriteString(`[^/]`)
			continue
		}
		builder.WriteString(regexp.QuoteMeta(string(ch)))
	}

	builder.WriteString("$")
	return regexp.MustCompile(builder.String())
}

func resolveForContainment(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	current := cleanPath
	var suffix []string

	for {
		info, err := os.Lstat(current)
		if err == nil {
			_ = info
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}

			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}

			return filepath.Clean(resolved), nil
		}

		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("unable to resolve path %q", path)
		}

		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
