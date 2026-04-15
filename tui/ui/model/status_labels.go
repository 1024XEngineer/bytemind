package tui

import (
	"fmt"
	"strings"
	"time"

	"bytemind/internal/session"
)

func (m model) chatPanelWidth() int {
	return max(20, m.width)
}

func (m model) chatPanelInnerWidth() int {
	width := m.chatPanelWidth() - panelStyle.GetHorizontalFrameSize()
	return max(12, width)
}

func (m model) currentPhaseLabel() string {
	if phase := m.planPhaseLabel(); phase != "none" {
		return phase
	}
	if strings.TrimSpace(m.phase) != "" {
		return strings.TrimSpace(m.phase)
	}
	return "idle"
}

func (m model) currentSessionLabel() string {
	if m.sess == nil {
		return "none"
	}
	return shortID(m.sess.ID)
}

func (m model) autoFollowLabel() string {
	if m.chatAutoFollow {
		return "auto"
	}
	return "manual"
}

func (m model) currentModelLabel() string {
	if model := strings.TrimSpace(m.cfg.Provider.Model); model != "" {
		return model
	}
	return "-"
}

func (m model) currentSkillLabel() string {
	active := m.activeSkillSnapshot()
	if active == nil {
		return "none"
	}
	name := strings.TrimSpace(active.Name)
	if name == "" {
		return "none"
	}
	return name
}

func (m model) sessionText() string {
	if m.sess == nil {
		return "No active session."
	}
	return strings.Join([]string{
		fmt.Sprintf("Session ID: %s", m.sess.ID),
		fmt.Sprintf("Workspace: %s", m.sess.Workspace),
		fmt.Sprintf("Updated: %s", m.sess.UpdatedAt.Local().Format("2006-01-02 15:04:05")),
		fmt.Sprintf("Messages: %d", len(m.sess.Messages)),
	}, "\n")
}

func formatUserMeta(model string, at time.Time) string {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "-"
	}
	return fmt.Sprintf("> you @ %s [%s]", model, at.Format("15:04:05"))
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func initialScreen(sess *session.Session) screenKind {
	if sess == nil || len(sess.Messages) == 0 {
		return screenLanding
	}
	return screenChat
}
