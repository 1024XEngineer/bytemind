package tui

import (
	"fmt"
	"strings"
	"time"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	tuiruntime "bytemind/internal/tui/runtime"

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
	next, err := m.runtimeAPI().NewSession(m.workspace)
	if err != nil {
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
	_ = m.refreshSkillCatalog()
	m.input.Reset()
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}

func (m *model) resumeSession(prefix string) error {
	id, err := tuiruntime.ResolveSessionID(m.sessions, prefix)
	if err != nil {
		return err
	}
	next, err := m.runtimeAPI().ResumeSession(m.workspace, id)
	if err != nil {
		return err
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
	_ = m.refreshSkillCatalog()
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}
