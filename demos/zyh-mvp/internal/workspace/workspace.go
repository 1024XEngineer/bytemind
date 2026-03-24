package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

var errStopWalk = errors.New("stop walk")

type Workspace struct {
	Root              string
	ignoreDirs        map[string]struct{}
	sensitivePatterns []string
}

type ReadResult struct {
	Path      string
	Content   []byte
	Hash      string
	Sensitive bool
}

type SearchHit struct {
	Path    string
	Line    int
	Preview string
}

func Open(root string, ignoreDirs, sensitivePatterns []string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("workspace root is required")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}

	realRoot, err := evalSymlinksOrFallback(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace symlinks: %w", err)
	}

	info, err := os.Stat(realRoot)
	if err != nil {
		return nil, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory: %s", realRoot)
	}

	ignore := make(map[string]struct{}, len(ignoreDirs))
	for _, dir := range ignoreDirs {
		ignore[strings.ToLower(dir)] = struct{}{}
	}

	return &Workspace{
		Root:              realRoot,
		ignoreDirs:        ignore,
		sensitivePatterns: sensitivePatterns,
	}, nil
}

func (w *Workspace) Resolve(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(w.Root, filepath.FromSlash(path))
	}

	abs := filepath.Clean(candidate)
	if info, err := os.Stat(abs); err == nil && info != nil {
		realPath, err := evalSymlinksOrFallback(abs)
		if err != nil {
			return "", fmt.Errorf("resolve path symlinks: %w", err)
		}
		abs = realPath
	} else {
		parent := filepath.Dir(abs)
		realParent, parentErr := evalSymlinksOrFallback(parent)
		if parentErr == nil {
			abs = filepath.Join(realParent, filepath.Base(abs))
		}
	}

	if !isSubpath(w.Root, abs) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}

	return abs, nil
}

func (w *Workspace) ListFiles(limit int) ([]string, error) {
	files := make([]string, 0, limit)
	err := filepath.WalkDir(w.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path != w.Root && w.shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(w.Root, path)
		if err != nil {
			return err
		}

		files = append(files, filepath.ToSlash(rel))
		if limit > 0 && len(files) >= limit {
			return errStopWalk
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func (w *Workspace) ReadFile(path string) (ReadResult, error) {
	abs, err := w.Resolve(path)
	if err != nil {
		return ReadResult{}, err
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return ReadResult{}, fmt.Errorf("read file: %w", err)
	}

	return ReadResult{
		Path:      filepath.ToSlash(mustRel(w.Root, abs)),
		Content:   data,
		Hash:      hashBytes(data),
		Sensitive: w.IsSensitive(path),
	}, nil
}

func (w *Workspace) Search(keyword string, limit int) ([]SearchHit, error) {
	needle := strings.ToLower(strings.TrimSpace(keyword))
	if needle == "" {
		return nil, nil
	}

	hits := make([]SearchHit, 0, limit)
	err := filepath.WalkDir(w.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path != w.Root && w.shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(w.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if w.IsSensitive(rel) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !isTextFile(data) || len(data) > 1024*1024 {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for idx, line := range lines {
			if strings.Contains(strings.ToLower(line), needle) {
				hits = append(hits, SearchHit{
					Path:    rel,
					Line:    idx + 1,
					Preview: shorten(line, 120),
				})
				if limit > 0 && len(hits) >= limit {
					return errStopWalk
				}
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return nil, err
	}

	return hits, nil
}

func (w *Workspace) IsSensitive(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	for _, pattern := range w.sensitivePatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func (w *Workspace) shouldSkipDir(name string) bool {
	_, ok := w.ignoreDirs[strings.ToLower(name)]
	return ok
}

func evalSymlinksOrFallback(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if os.IsPermission(err) {
		return filepath.Clean(path), nil
	}
	return "", err
}

func isTextFile(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func shorten(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max < 4 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func isSubpath(root, candidate string) bool {
	rootValue := normalizedPath(root)
	candidateValue := normalizedPath(candidate)
	if candidateValue == rootValue {
		return true
	}
	return strings.HasPrefix(candidateValue, rootValue+string(os.PathSeparator))
}

func normalizedPath(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

func mustRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
