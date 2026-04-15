package session

import (
	"errors"
	"fmt"
	"time"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

func (s *Store) Save(session *Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	now := time.Now().UTC()
	session.UpdatedAt = now
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.Mode == "" {
		session.Mode = planpkg.ModeBuild
	}
	normalizeSessionConversation(session)
	for i, message := range session.Conversation.Timeline {
		if err := llm.ValidateMessage(message); err != nil {
			return fmt.Errorf("timeline[%d] validation failed: %w", i, err)
		}
	}
	session.Plan = planpkg.NormalizeState(session.Plan)
	session.ActiveSkill = normalizeActiveSkill(session.ActiveSkill)
	if len(session.Plan.Steps) > 0 {
		session.Plan.UpdatedAt = session.UpdatedAt
	}

	target, err := s.pathForSession(session)
	if err != nil {
		return err
	}
	return writeSessionSnapshot(s.files, target, session)
}
