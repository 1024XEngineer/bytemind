package session

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) List(limit int) ([]Summary, []string, error) {
	sources, err := s.sessionSources()
	if err != nil {
		return nil, nil, err
	}
	return s.summariesFromSources(sources, limit)
}

func (s *Store) ListInWorkspace(workspace string, limit int) ([]Summary, []string, error) {
	if strings.TrimSpace(workspace) == "" {
		return s.List(limit)
	}
	sources, err := s.sessionSourcesInWorkspace(workspace)
	if err != nil {
		return nil, nil, err
	}
	return s.summariesFromSources(sources, limit)
}

func (s *Store) summariesFromSources(sources []sessionSource, limit int) ([]Summary, []string, error) {
	warnings := make([]string, 0)
	seenIDs := make(map[string]struct{}, len(sources))

	// Phase 1: lightweight metadata scan to get UpdatedAt without full event replay.
	// This lets us skip expensive replay for sessions that won't make the limit cutoff.
	type sourceMeta struct {
		source    sessionSource
		updatedAt time.Time
		hasSnap   bool
	}
	metas := make([]sourceMeta, 0, len(sources))
	for _, source := range sources {
		if _, ok := seenIDs[source.paths.SessionID]; ok {
			continue
		}
		seenIDs[source.paths.SessionID] = struct{}{}

		var updatedAt time.Time
		hasSnap := false
		if source.kind == sourceKindEvents && source.paths.Snapshot != "" {
			if snap, err := readSnapshotFile(source.paths.Snapshot); err == nil {
				updatedAt = snap.Session.UpdatedAt
				hasSnap = true
			}
		}
		metas = append(metas, sourceMeta{
			source:    source,
			updatedAt: updatedAt,
			hasSnap:   hasSnap,
		})
	}

	// Sort by session UpdatedAt descending (snapshot value or fallback to source file mtime)
	sort.Slice(metas, func(i, j int) bool {
		left := metas[i].updatedAt
		if left.IsZero() {
			left = metas[i].source.updatedAt
		}
		right := metas[j].updatedAt
		if right.IsZero() {
			right = metas[j].source.updatedAt
		}
		return left.After(right)
	})

	// Phase 2: determine how many to fully replay.
	// Process top sessions with enough buffer for sort stability.
	replayCount := limit
	if replayCount <= 0 {
		replayCount = len(metas)
	}
	buffer := replayCount / 2
	if buffer < 10 {
		buffer = 10
	}
	target := replayCount + buffer
	if target > len(metas) {
		target = len(metas)
	}

	seenIDs = make(map[string]struct{}, target)
	summaries := make([]Summary, 0, limit)
	for _, meta := range metas[:target] {
		source := meta.source
		if _, ok := seenIDs[source.paths.SessionID]; ok {
			continue
		}
		seenIDs[source.paths.SessionID] = struct{}{}

		var (
			sess *Session
			err  error
			name string
		)
		switch source.kind {
		case sourceKindEvents:
			sess, _, _, err = s.replayFromEventStore(source.paths)
			name = filepath.Base(source.paths.Dir)
		case sourceKindLegacy:
			sess, err = loadLegacySessionFile(s.files, source.paths.Legacy)
			name = filepath.Base(source.paths.Legacy)
		default:
			continue
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped corrupted session file %s: %v", name, err))
			continue
		}
		if strings.TrimSpace(sess.ID) == "" {
			warnings = append(warnings, fmt.Sprintf("skipped corrupted session file %s: missing session id", name))
			continue
		}
		seenIDs[sess.ID] = struct{}{}

		timeline := sessionTimeline(sess)
		metrics := CountMessageMetrics(timeline)
		preview := summarizeMessage(lastUserMessage(timeline), 72)
		title := summarizeMessage(sessionTitle(sess), 72)
		summaries = append(summaries, Summary{
			ID:                            sess.ID,
			Workspace:                     sess.Workspace,
			Title:                         title,
			Preview:                       preview,
			CreatedAt:                     sess.CreatedAt,
			UpdatedAt:                     sess.UpdatedAt,
			LastUserMessage:               preview,
			MessageCount:                  metrics.RawMessageCount,
			RawMessageCount:               metrics.RawMessageCount,
			UserEffectiveInputCount:       metrics.UserEffectiveInputCount,
			AssistantEffectiveOutputCount: metrics.AssistantEffectiveOutputCount,
			ZeroMsgSession:                IsZeroMessageSession(metrics),
			NoReplySession:                IsNoReplySession(metrics),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, warnings, nil
}
