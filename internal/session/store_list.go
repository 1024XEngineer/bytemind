package session

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Store) List(limit int) ([]Summary, []string, error) {
	sources, err := s.sessionSources()
	if err != nil {
		return nil, nil, err
	}

	summaries := make([]Summary, 0, len(sources))
	warnings := make([]string, 0)
	seenIDs := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		if _, ok := seenIDs[source.paths.SessionID]; ok {
			continue
		}

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
