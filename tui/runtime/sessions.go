package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"bytemind/internal/session"
)

type CompactSessionResult struct {
	Summary string
	Changed bool
}

func (s *Service) NewSession(workspace string) (*session.Session, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	next := session.New(workspace)
	if err := s.store.Save(next); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *Service) ResumeSession(workspace, id string) (*session.Session, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	next, err := s.store.Load(id)
	if err != nil {
		return nil, err
	}
	if !sameWorkspace(workspace, next.Workspace) {
		return nil, fmt.Errorf("session %s belongs to workspace %s", next.ID, next.Workspace)
	}
	return next, nil
}

func (s *Service) SaveSession(sess *session.Session) error {
	if s == nil || s.store == nil || sess == nil {
		return nil
	}
	return s.store.Save(sess)
}

func (s *Service) ListSessions(limit int) ([]session.Summary, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	summaries, _, err := s.store.List(limit)
	return summaries, err
}

func (s *Service) CompactSession(sess *session.Session) (CompactSessionResult, error) {
	if err := s.requireRunner(); err != nil {
		return CompactSessionResult{}, err
	}
	if sess == nil {
		return CompactSessionResult{}, fmt.Errorf("session is unavailable")
	}
	type sessionCompactor interface {
		CompactSession(ctx context.Context, sess *session.Session) (string, bool, error)
	}
	compactor, ok := any(s.runner).(sessionCompactor)
	if !ok {
		return CompactSessionResult{}, fmt.Errorf("compact is unavailable in this build")
	}
	summary, changed, err := compactor.CompactSession(context.Background(), sess)
	if err != nil {
		return CompactSessionResult{}, err
	}
	return CompactSessionResult{Summary: summary, Changed: changed}, nil
}

func ResolveSessionID(summaries []session.Summary, prefix string) (string, error) {
	matches := make([]string, 0, 4)
	for _, summary := range summaries {
		if summary.ID == prefix {
			return summary.ID, nil
		}
		if strings.HasPrefix(summary.ID, prefix) {
			matches = append(matches, summary.ID)
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

func sameWorkspace(a, b string) bool {
	left, err := filepath.Abs(a)
	if err != nil {
		left = a
	}
	right, err := filepath.Abs(b)
	if err != nil {
		right = b
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
