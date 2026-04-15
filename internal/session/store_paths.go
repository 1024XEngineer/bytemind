package session

import (
	"errors"
	"strings"

	storagepkg "bytemind/internal/storage"
)

func (s *Store) pathForSession(session *Session) (string, error) {
	if strings.TrimSpace(session.ID) == "" {
		return "", errors.New("session id is required")
	}
	return s.files.SessionPath(storagepkg.WorkspaceProjectID(session.Workspace), session.ID)
}

func (s *Store) findSessionPath(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.New("session id is required")
	}
	return s.files.FindNewestByName(id + ".jsonl")
}

func (s *Store) sessionPaths() ([]string, error) {
	return s.files.ListByExt(".jsonl")
}
