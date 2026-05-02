package app

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/session"
)

const DefaultSessionListLimit = 8

func ResolveSessionID(store *session.Store, prefix string) (string, error) {
	summaries, _, err := store.List(0)
	if err != nil {
		return "", err
	}

	matches := make([]string, 0, 4)
	for _, item := range summaries {
		if item.ID == prefix {
			return item.ID, nil
		}
		if strings.HasPrefix(item.ID, prefix) {
			matches = append(matches, item.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("session not found: %s", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("session prefix %q matched multiple sessions", prefix)
	}
}

func ParseSessionListLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultSessionListLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("/sessions limit must be a positive integer")
	}
	return limit, nil
}

func SameWorkspace(a, b string) bool {
	return strings.EqualFold(normalizeWorkspacePath(a), normalizeWorkspacePath(b))
}

func normalizeWorkspacePath(path string) string {
	normalized, err := filepath.Abs(path)
	if err != nil {
		normalized = path
	}
	normalized = filepath.Clean(normalized)
	if resolved, err := filepath.EvalSymlinks(normalized); err == nil {
		normalized = filepath.Clean(resolved)
	}
	return normalized
}

func CreateSession(store *session.Store, workspace string) (*session.Session, error) {
	sess := session.New(workspace)
	if err := store.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func ResumeSessionInWorkspace(store *session.Store, currentWorkspace, sessionPrefix string) (*session.Session, error) {
	id, err := ResolveSessionID(store, sessionPrefix)
	if err != nil {
		return nil, err
	}
	sess, err := store.Load(id)
	if err != nil {
		return nil, err
	}
	if !SameWorkspace(currentWorkspace, sess.Workspace) {
		return nil, fmt.Errorf("session %s belongs to workspace %s, current workspace is %s", sess.ID, sess.Workspace, currentWorkspace)
	}
	return sess, nil
}
