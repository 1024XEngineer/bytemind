package session

import (
	"path/filepath"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

func normalizeLoadedSession(sess *Session, path string) {
	if strings.TrimSpace(sess.ID) == "" {
		sess.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if sess.CreatedAt.IsZero() && !sess.UpdatedAt.IsZero() {
		sess.CreatedAt = sess.UpdatedAt
	}
	if sess.UpdatedAt.IsZero() && !sess.CreatedAt.IsZero() {
		sess.UpdatedAt = sess.CreatedAt
	}
	sess.Title = strings.TrimSpace(sess.Title)
	if sess.Title == "" {
		sess.Title = sessionTitle(sess)
	}
	normalizeSessionConversation(sess)
	if sess.Mode == "" {
		sess.Mode = planpkg.ModeBuild
	}
	sess.Plan = planpkg.NormalizeState(sess.Plan)
	sess.ActiveSkill = normalizeActiveSkill(sess.ActiveSkill)
	if len(sess.Plan.Steps) > 0 && sess.Plan.UpdatedAt.IsZero() {
		sess.Plan.UpdatedAt = sess.UpdatedAt
	}
}

func normalizeActiveSkill(raw *ActiveSkill) *ActiveSkill {
	if raw == nil {
		return nil
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		return nil
	}

	args := make(map[string]string, len(raw.Args))
	for key, value := range raw.Args {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		args[key] = value
	}
	if len(args) == 0 {
		args = nil
	}

	return &ActiveSkill{
		Name:        name,
		Args:        args,
		ActivatedAt: raw.ActivatedAt,
	}
}

func normalizeSessionConversation(sess *Session) {
	if sess == nil {
		return
	}
	if len(sess.Messages) > 0 {
		sess.Conversation.Timeline = sess.Messages
	} else if len(sess.Conversation.Timeline) > 0 {
		sess.Messages = sess.Conversation.Timeline
	} else {
		sess.Messages = make([]llm.Message, 0, 32)
		sess.Conversation.Timeline = make([]llm.Message, 0, 32)
	}
	for i := range sess.Conversation.Timeline {
		sess.Conversation.Timeline[i].Normalize()
	}
	for i := range sess.Messages {
		sess.Messages[i].Normalize()
	}
}
