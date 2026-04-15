package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderSessionsModal() string {
	lines := []string{modalTitleStyle.Render("Recent Sessions"), mutedStyle.Render("Up/Down to select, Enter to resume, Esc to close"), ""}
	if len(m.sessions) == 0 {
		lines = append(lines, "No sessions available.")
	} else {
		for i, summary := range m.sessions {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == m.sessionCursor {
				prefix = "> "
				style = style.Foreground(colorAccent).Bold(true)
			}
			line := fmt.Sprintf("%s%s  %s  %d msgs", prefix, shortID(summary.ID), summary.UpdatedAt.Local().Format("2006-01-02 15:04"), summary.MessageCount)
			lines = append(lines, style.Render(line))
			lines = append(lines, mutedStyle.Render("   "+summary.Workspace))
			if summary.LastUserMessage != "" {
				lines = append(lines, mutedStyle.Render("   "+summary.LastUserMessage))
			}
			lines = append(lines, "")
		}
	}
	return modalBoxStyle.Width(min(96, max(56, m.width-12))).Render(strings.Join(lines, "\n"))
}

func (m *model) newSession() error {
	next := session.New(m.workspace)
	if err := m.store.Save(next); err != nil {
		return err
	}
	m.sess = next
	m.screen = screenLanding
	m.plan = planpkg.State{}
	m.mode = modeBuild
	m.chatItems = nil
	m.toolRuns = nil
	m.streamingIndex = -1
	m.statusNote = "Started a new session."
	m.chatAutoFollow = true
	m.restoreTokenUsageFromSession(next)
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.inputImageRefs = make(map[int]llm.AssetID, 8)
	m.inputImageMentions = make(map[string]llm.AssetID, 8)
	m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	m.nextImageID = nextSessionImageID(next)
	m.ensureSessionImageAssets()
	m.pastedContents = make(map[string]pastedContent, maxStoredPastedContents)
	m.pastedOrder = make([]string, 0, maxStoredPastedContents)
	m.nextPasteID = 1
	m.pastedStateLoaded = false
	m.lastCompressedPasteAt = time.Time{}
	m.ensurePastedContentState()
	m.pendingBTW = nil
	m.interrupting = false
	m.interruptSafe = false
	m.runCancel = nil
	m.activeRunID = 0
	m.input.Reset()
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}

func (m *model) resumeSession(prefix string) error {
	id, err := resolveSessionID(m.sessions, prefix)
	if err != nil {
		return err
	}
	next, err := m.store.Load(id)
	if err != nil {
		return err
	}
	if !sameWorkspace(m.workspace, next.Workspace) {
		return fmt.Errorf("session %s belongs to workspace %s", next.ID, next.Workspace)
	}
	m.sess = next
	m.screen = screenChat
	m.plan = copyPlanState(next.Plan)
	m.mode = toAgentMode(next.Mode)
	m.chatItems, m.toolRuns = rebuildSessionTimeline(next)
	m.streamingIndex = -1
	m.statusNote = "Resumed session " + shortID(next.ID)
	m.chatAutoFollow = true
	m.restoreTokenUsageFromSession(next)
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.inputImageRefs = make(map[int]llm.AssetID, 8)
	m.inputImageMentions = make(map[string]llm.AssetID, 8)
	m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	m.nextImageID = nextSessionImageID(next)
	m.ensureSessionImageAssets()
	m.pastedContents = make(map[string]pastedContent, maxStoredPastedContents)
	m.pastedOrder = make([]string, 0, maxStoredPastedContents)
	m.nextPasteID = 1
	m.pastedStateLoaded = false
	m.lastCompressedPasteAt = time.Time{}
	m.ensurePastedContentState()
	m.syncInputImageRefs("")
	m.pendingBTW = nil
	m.interrupting = false
	m.interruptSafe = false
	m.runCancel = nil
	m.activeRunID = 0
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}

func resolveSessionID(summaries []session.Summary, prefix string) (string, error) {
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
