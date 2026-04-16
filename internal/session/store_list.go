package session

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Store) List(limit int) ([]Summary, []string, error) {
	paths, err := s.sessionPaths()
	if err != nil {
		return nil, nil, err
	}

	summaries := make([]Summary, 0, len(paths))
	warnings := make([]string, 0)
	for _, path := range paths {
		sess, err := loadSessionFile(s.files, path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped corrupted session file %s: %v", filepath.Base(path), err))
			continue
		}
		if strings.TrimSpace(sess.ID) == "" {
			warnings = append(warnings, fmt.Sprintf("skipped corrupted session file %s: missing session id", filepath.Base(path)))
			continue
		}

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
