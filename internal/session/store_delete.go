package session

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type CleanupResult struct {
	DeletedIDs []string
}

func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}
	source, err := s.findSessionSource(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	switch source.kind {
	case sourceKindEvents:
		if err := os.RemoveAll(source.paths.Dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	case sourceKindLegacy:
		if err := os.Remove(source.paths.Legacy); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteInWorkspace(workspace, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("session id is required")
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return s.Delete(id)
	}
	paths, err := s.pathForWorkspaceSession(workspace, id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(paths.Dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(paths.Legacy); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) CleanupZeroMessageSessions(workspace, activeSessionID string) (CleanupResult, error) {
	result := CleanupResult{DeletedIDs: make([]string, 0, 8)}
	workspace = strings.TrimSpace(workspace)
	activeSessionID = strings.TrimSpace(activeSessionID)

	summaries, _, err := s.List(0)
	if err != nil {
		return result, err
	}
	for _, summary := range summaries {
		id := strings.TrimSpace(summary.ID)
		if id == "" || id == activeSessionID {
			continue
		}
		if workspace != "" && !sameWorkspacePath(workspace, summary.Workspace) {
			continue
		}
		if !summary.ZeroMsgSession {
			continue
		}
		if err := s.DeleteInWorkspace(summary.Workspace, id); err != nil {
			return result, err
		}
		result.DeletedIDs = append(result.DeletedIDs, id)
	}
	return result, nil
}

func sameWorkspacePath(left, right string) bool {
	leftAbs, err := filepath.Abs(left)
	if err == nil {
		left = leftAbs
	}
	rightAbs, err := filepath.Abs(right)
	if err == nil {
		right = rightAbs
	}
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
