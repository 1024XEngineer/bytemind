package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	sessionPageSize = 8
	sessionPageMax  = 10
)

func sessionListFetchLimit() int {
	return sessionPageSize * sessionPageMax
}

func (m model) renderSessionsModal() string {
	total := len(m.sessions)
	pageCount := m.sessionPageCount()
	pageIndex := m.sessionCurrentPage()

	lines := []string{
		modalTitleStyle.Render("Recent Sessions"),
		mutedStyle.Render(fmt.Sprintf("Page %d/%d \u00b7 Total %d", pageIndex+1, pageCount, total)),
		mutedStyle.Render("Up/Down move, Left/Right page, Enter resume, Delete remove, Esc close"),
		"",
	}
	if total == 0 {
		lines = append(lines, "No sessions available.")
		return modalBoxStyle.Width(min(96, max(56, m.width-12))).Render(strings.Join(lines, "\n"))
	}

	start, end := m.sessionPageBounds(pageIndex)
	for i := start; i < end; i++ {
		summary := m.sessions[i]
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == m.sessionCursor {
			prefix = "> "
			style = style.Foreground(colorAccent).Bold(true)
		}
		rawCount := summary.RawMessageCount
		if rawCount == 0 {
			rawCount = summary.MessageCount
		}
		line := fmt.Sprintf("%s%s  %s  raw:%d", prefix, summary.ID, summary.UpdatedAt.Local().Format("2006-01-02 15:04"), rawCount)
		lines = append(lines, style.Render(line))
		lines = append(lines, mutedStyle.Render("   "+summary.Workspace))
		lines = append(lines, mutedStyle.Render("   "+displaySessionTitle(summary)))
		lines = append(lines, "")
	}
	return modalBoxStyle.Width(min(96, max(56, m.width-12))).Render(strings.Join(lines, "\n"))
}

func displaySessionTitle(summary session.Summary) string {
	title := strings.TrimSpace(summary.Title)
	if title != "" {
		return title
	}
	preview := strings.TrimSpace(summary.Preview)
	if preview != "" {
		return preview
	}
	preview = strings.TrimSpace(summary.LastUserMessage)
	if preview != "" {
		return preview
	}
	return "(no title yet)"
}

func (m model) handleSessionsModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.sessionsOpen = false
	case "up", "k":
		m.moveSessionCursorInPage(-1)
	case "down", "j":
		m.moveSessionCursorInPage(1)
	case "left":
		m.moveSessionPage(-1)
	case "right":
		m.moveSessionPage(1)
	case "delete":
		if len(m.sessions) == 0 {
			return m, nil
		}
		if err := m.deleteSelectedSession(); err != nil {
			m.statusNote = err.Error()
			return m, nil
		}
		return m, m.loadSessionsCmd()
	case "enter":
		if m.busy || len(m.sessions) == 0 {
			return m, nil
		}
		if err := m.resumeSession(m.sessions[m.sessionCursor].ID); err != nil {
			m.statusNote = err.Error()
		} else {
			m.sessionsOpen = false
		}
	}
	return m, nil
}

func (m *model) newSession() error {
	if m.store == nil {
		return fmt.Errorf("session store is unavailable")
	}
	if _, err := m.cleanupZeroMsgSessions(); err != nil {
		return err
	}
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
	if _, err := m.cleanupZeroMsgSessions(); err != nil {
		return err
	}
	if err := m.reloadSessions(); err != nil {
		return err
	}
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

func (m *model) openSessionsModal() error {
	if _, err := m.cleanupZeroMsgSessions(); err != nil {
		return err
	}
	m.sessionsOpen = true
	m.statusNote = "Opened recent sessions."
	return nil
}

func (m *model) cleanupZeroMsgSessions() (session.CleanupResult, error) {
	result := session.CleanupResult{}
	if m.store == nil {
		return result, nil
	}
	activeID := ""
	if m.sess != nil {
		activeID = m.sess.ID
	}
	return m.store.CleanupZeroMessageSessions(m.workspace, activeID)
}

func (m *model) reloadSessions() error {
	if m.store == nil {
		m.sessions = nil
		m.sessionCursor = 0
		return nil
	}
	summaries, _, err := m.store.List(sessionListFetchLimit())
	if err != nil {
		return err
	}
	m.sessions = summaries
	m.clampSessionCursor()
	return nil
}

func (m *model) deleteSelectedSession() error {
	if len(m.sessions) == 0 {
		return nil
	}
	if m.store == nil {
		return fmt.Errorf("session store is unavailable")
	}
	m.clampSessionCursor()
	targetIndex := m.sessionCursor
	target := m.sessions[targetIndex]
	if m.sess != nil && strings.TrimSpace(target.ID) == strings.TrimSpace(m.sess.ID) {
		if m.busy {
			return fmt.Errorf("cannot delete the active session while a run is in progress")
		}
		if err := m.newSession(); err != nil {
			return fmt.Errorf("failed to switch to a new session before deleting active session: %w", err)
		}
	}
	if err := m.store.DeleteInWorkspace(target.Workspace, target.ID); err != nil {
		return err
	}
	m.sessions = append(m.sessions[:targetIndex], m.sessions[targetIndex+1:]...)
	if len(m.sessions) == 0 {
		m.sessionCursor = 0
	} else if targetIndex >= len(m.sessions) {
		m.sessionCursor = len(m.sessions) - 1
	} else {
		m.sessionCursor = targetIndex
	}
	m.statusNote = "Deleted session " + shortID(target.ID)
	return nil
}

func (m *model) clampSessionCursor() {
	if len(m.sessions) == 0 {
		m.sessionCursor = 0
		return
	}
	m.sessionCursor = clamp(m.sessionCursor, 0, len(m.sessions)-1)
}

func (m model) sessionPageCount() int {
	if len(m.sessions) == 0 {
		return 1
	}
	return (len(m.sessions) + sessionPageSize - 1) / sessionPageSize
}

func (m model) sessionCurrentPage() int {
	if len(m.sessions) == 0 {
		return 0
	}
	cursor := clamp(m.sessionCursor, 0, len(m.sessions)-1)
	return cursor / sessionPageSize
}

func (m model) sessionPageBounds(page int) (int, int) {
	if len(m.sessions) == 0 {
		return 0, 0
	}
	page = clamp(page, 0, m.sessionPageCount()-1)
	start := page * sessionPageSize
	end := min(start+sessionPageSize, len(m.sessions))
	return start, end
}

func (m *model) moveSessionCursorInPage(delta int) {
	if len(m.sessions) == 0 || delta == 0 {
		return
	}
	m.clampSessionCursor()
	page := m.sessionCurrentPage()
	start, end := m.sessionPageBounds(page)
	if delta < 0 {
		m.sessionCursor = max(start, m.sessionCursor-1)
		return
	}
	m.sessionCursor = min(end-1, m.sessionCursor+1)
}

func (m *model) moveSessionPage(delta int) {
	if len(m.sessions) == 0 || delta == 0 {
		return
	}
	m.clampSessionCursor()
	currentPage := m.sessionCurrentPage()
	nextPage := clamp(currentPage+delta, 0, m.sessionPageCount()-1)
	if nextPage == currentPage {
		return
	}
	offset := m.sessionCursor % sessionPageSize
	start, end := m.sessionPageBounds(nextPage)
	target := start + offset
	if target >= end {
		target = end - 1
	}
	m.sessionCursor = target
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
