package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SessionFileStore owns low-level filesystem concerns for session snapshots.
type SessionFileStore struct {
	root string
}

func NewSessionFileStore(root string) (*SessionFileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("session root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &SessionFileStore{root: root}, nil
}

func (s *SessionFileStore) WriteAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (s *SessionFileStore) SessionPath(projectID, sessionID string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", errors.New("project id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("session id is required")
	}
	return filepath.Join(s.root, projectID, sessionID+".jsonl"), nil
}

func (s *SessionFileStore) Read(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (s *SessionFileStore) FindNewestByName(fileName string) (string, error) {
	if strings.TrimSpace(fileName) == "" {
		return "", errors.New("file name is required")
	}

	matches := make([]string, 0, 2)
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == fileName {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", os.ErrNotExist
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	sort.Slice(matches, func(i, j int) bool {
		leftInfo, leftErr := os.Stat(matches[i])
		rightInfo, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] < matches[j]
		}
		if leftInfo.ModTime().Equal(rightInfo.ModTime()) {
			return matches[i] < matches[j]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})
	return matches[0], nil
}

func (s *SessionFileStore) ListByExt(ext string) ([]string, error) {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		ext = ".jsonl"
	}

	paths := make([]string, 0, 32)
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) == ext {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
