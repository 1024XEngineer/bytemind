package session

import (
	"errors"
	"path/filepath"
	"strings"
)

func normalizeSessionID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", errors.New("session id is required")
	}
	if id == "." || id == ".." {
		return "", errors.New("invalid session id")
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return "", errors.New("invalid session id")
	}
	if filepath.IsAbs(id) || filepath.VolumeName(id) != "" {
		return "", errors.New("invalid session id")
	}
	if cleaned := filepath.Clean(id); cleaned != id {
		return "", errors.New("invalid session id")
	}
	return id, nil
}

// FlattenSubAgentSessionID converts a subagent session ID (which contains '/')
// into a filesystem-safe ID by replacing '/' with '_'.
func FlattenSubAgentSessionID(raw string) string {
	return strings.ReplaceAll(strings.TrimSpace(raw), "/", "_")
}

// UnflattenSubAgentSessionID reverses FlattenSubAgentSessionID.
func UnflattenSubAgentSessionID(raw string) string {
	return strings.ReplaceAll(raw, "_", "/")
}
