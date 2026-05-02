package session

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
)

const (
	eventsFileName   = "events.jsonl"
	snapshotFileName = "snapshot.json"
)

type sourceKind string

const (
	sourceKindEvents sourceKind = "events"
	sourceKindLegacy sourceKind = "legacy"
)

type sessionPaths struct {
	ProjectID string
	SessionID string
	Dir       string
	Events    string
	Snapshot  string
	Legacy    string
}

type sessionSource struct {
	kind      sourceKind
	updatedAt time.Time
	paths     sessionPaths
}

func (s *Store) pathForSession(session *Session) (sessionPaths, error) {
	return s.pathForWorkspaceSession(session.Workspace, session.ID)
}

func (s *Store) pathForWorkspaceSession(workspace, id string) (sessionPaths, error) {
	normalizedID, err := normalizeSessionID(id)
	if err != nil {
		return sessionPaths{}, err
	}
	projectID := storagepkg.WorkspaceProjectID(workspace)
	root := s.files.Root()
	dir := filepath.Join(root, projectID, normalizedID)
	legacy, err := s.files.SessionPath(projectID, normalizedID)
	if err != nil {
		return sessionPaths{}, err
	}
	return sessionPaths{
		ProjectID: projectID,
		SessionID: normalizedID,
		Dir:       dir,
		Events:    filepath.Join(dir, eventsFileName),
		Snapshot:  filepath.Join(dir, snapshotFileName),
		Legacy:    legacy,
	}, nil
}

func (s *Store) findSessionSource(id string) (sessionSource, error) {
	normalizedID, err := normalizeSessionID(id)
	if err != nil {
		return sessionSource{}, err
	}

	sources, err := s.sessionSources()
	if err != nil {
		return sessionSource{}, err
	}

	var newestEvents *sessionSource
	var newestLegacy *sessionSource
	for i := range sources {
		source := sources[i]
		if source.paths.SessionID != normalizedID {
			continue
		}
		switch source.kind {
		case sourceKindEvents:
			if newestEvents == nil || source.updatedAt.After(newestEvents.updatedAt) {
				copy := source
				newestEvents = &copy
			}
		case sourceKindLegacy:
			if newestLegacy == nil || source.updatedAt.After(newestLegacy.updatedAt) {
				copy := source
				newestLegacy = &copy
			}
		}
	}

	if newestEvents != nil {
		return *newestEvents, nil
	}
	if newestLegacy != nil {
		return *newestLegacy, nil
	}
	return sessionSource{}, os.ErrNotExist
}

func (s *Store) sessionSources() ([]sessionSource, error) {
	root := strings.TrimSpace(s.files.Root())
	if root == "" {
		return nil, errors.New("session root is required")
	}

	projectEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	sources := make([]sessionSource, 0, 32)
	for _, projectEntry := range projectEntries {
		if !projectEntry.IsDir() {
			continue
		}
		projectID := projectEntry.Name()
		projectDir := filepath.Join(root, projectID)
		entries, err := os.ReadDir(projectDir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := strings.TrimSpace(entry.Name())
			if name == "" {
				continue
			}
			if entry.IsDir() {
				paths := sessionPaths{
					ProjectID: projectID,
					SessionID: name,
					Dir:       filepath.Join(projectDir, name),
				}
				paths.Events = filepath.Join(paths.Dir, eventsFileName)
				paths.Snapshot = filepath.Join(paths.Dir, snapshotFileName)

				eventsInfo, eventsErr := os.Stat(paths.Events)
				snapshotInfo, snapshotErr := os.Stat(paths.Snapshot)
				if eventsErr != nil && snapshotErr != nil {
					continue
				}
				paths.Legacy = filepath.Join(projectDir, name+".jsonl")
				sources = append(sources, sessionSource{
					kind:      sourceKindEvents,
					updatedAt: newestModTime(eventsInfo, snapshotInfo),
					paths:     paths,
				})
				continue
			}

			if strings.EqualFold(filepath.Ext(name), ".jsonl") && !strings.EqualFold(name, eventsFileName) {
				legacyPath := filepath.Join(projectDir, name)
				info, err := os.Stat(legacyPath)
				if err != nil {
					return nil, err
				}
				sessionID := strings.TrimSuffix(name, filepath.Ext(name))
				sources = append(sources, sessionSource{
					kind:      sourceKindLegacy,
					updatedAt: info.ModTime().UTC(),
					paths: sessionPaths{
						ProjectID: projectID,
						SessionID: sessionID,
						Legacy:    legacyPath,
					},
				})
			}
		}
	}

	sort.Slice(sources, func(i, j int) bool {
		if sources[i].paths.SessionID == sources[j].paths.SessionID {
			return sources[i].updatedAt.After(sources[j].updatedAt)
		}
		return sources[i].paths.SessionID < sources[j].paths.SessionID
	})
	return sources, nil
}

func newestModTime(first, second os.FileInfo) time.Time {
	var left time.Time
	if first != nil {
		left = first.ModTime().UTC()
	}
	var right time.Time
	if second != nil {
		right = second.ModTime().UTC()
	}
	if right.After(left) {
		return right
	}
	return left
}
