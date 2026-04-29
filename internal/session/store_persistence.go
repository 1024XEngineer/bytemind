package session

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
)

const (
	legacySnapshotEventType = "session_snapshot"
	legacySchemaVersion     = 1
)

func loadLegacySessionFile(files *storagepkg.SessionFileStore, path string) (*Session, error) {
	if strings.ToLower(filepath.Ext(path)) != ".jsonl" {
		return nil, errors.New("unsupported session file extension")
	}
	return loadJSONLSession(files, path)
}

func loadJSONLSession(files *storagepkg.SessionFileStore, path string) (*Session, error) {
	rawPayload, err := storagepkg.ReadLatestJSONLSnapshot(files, path, legacySnapshotEventType)
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(rawPayload, &sess); err != nil {
		return nil, errors.New("no valid session snapshot found")
	}
	normalizeLoadedSession(&sess, path)
	return &sess, nil
}

func writeSessionSnapshot(files *storagepkg.SessionFileStore, path string, session *Session) error {
	return storagepkg.WriteJSONLSnapshot(files, path, legacySnapshotEventType, legacySchemaVersion, *session, session.UpdatedAt)
}
