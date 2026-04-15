package tui

import (
	"fmt"
	"strings"

	planpkg "bytemind/internal/plan"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleComposerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isInputNewlineKey(msg) {
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.input.Value() != before {
			m.handleInputMutation(before, m.input.Value(), inputMutationSource(msg))
			m.syncInputOverlays()
		}
		return m, cmd
	}

	ctrlVPasteDetected := isCtrlVPasteKey(msg)
	if ctrlVPasteDetected {
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
			if strings.Contains(note, "Attached image from clipboard") {
				m.syncInputOverlays()
				return m, nil
			}
			if !isClipboardNoImageNote(note) {
				m.syncInputOverlays()
				return m, nil
			}
		}
	}

	if next, cmd, handled := m.handleComposerShortcutKey(msg); handled {
		return next, cmd
	}

	if msg.String() == "enter" && !msg.Paste {
		return m.handleComposerSubmit()
	}

	return m.handleComposerMutation(msg, ctrlVPasteDetected)
}

func (m model) handleComposerShortcutKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+l":
		if !m.busy {
			m.sessionsOpen = true
		}
		return m, m.loadSessionsCmd(), true
	case "alt+v":
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
		m.syncInputOverlays()
		return m, nil, true
	case "ctrl+n":
		if !m.busy && m.screen == screenChat {
			if err := m.newSession(); err != nil {
				m.statusNote = err.Error()
			}
		}
		return m, m.loadSessionsCmd(), true
	case "home":
		m.viewport.GotoTop()
		m.syncCopyViewOffset()
		m.chatAutoFollow = false
		return m, nil, true
	case "end":
		m.viewport.GotoBottom()
		m.syncCopyViewOffset()
		m.chatAutoFollow = true
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m model) handleComposerSubmit() (tea.Model, tea.Cmd) {
	if m.shouldSuppressEnterAfterPaste() {
		if m.busy {
			return m, nil
		}
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.input.Value() != before {
			m.handleInputMutation(before, m.input.Value(), "paste-enter")
			m.syncInputOverlays()
		}
		return m, cmd
	}

	rawValue := m.input.Value()
	if next, handled := m.handleCompressedPasteSubmit(rawValue); handled {
		return next, nil
	}
	if next, handled := m.handleLongPasteCompression(rawValue); handled {
		return next, nil
	}

	value := strings.TrimSpace(rawValue)
	if m.startupGuide.Active && !strings.HasPrefix(value, "/") {
		if err := m.handleStartupGuideSubmission(rawValue); err != nil {
			m.statusNote = err.Error()
		}
		m.screen = screenLanding
		return m, nil
	}
	if value == "" {
		return m, nil
	}
	if isBTWCommand(value) {
		btw, err := extractBTWText(value)
		if err != nil {
			m.statusNote = err.Error()
			return m, nil
		}
		if m.busy {
			return m.submitBTW(btw)
		}
		return m.submitPrompt(btw)
	}
	if value == "/quit" {
		return m, tea.Quit
	}
	if m.busy {
		if strings.HasPrefix(value, "/") {
			m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message> or plain text."
			return m, nil
		}
		return m.submitBTW(value)
	}
	if isContinueExecutionInput(value) && planpkg.HasStructuredPlan(m.plan) {
		state, err := preparePlanForContinuation(m.plan)
		if err != nil {
			m.statusNote = err.Error()
			return m, nil
		}
		m.plan = state
		m.mode = modeBuild
		if m.sess != nil {
			m.sess.Mode = planpkg.ModeBuild
			m.sess.Plan = copyPlanState(state)
			if err := m.runtimeAPI().SaveSession(m.sess); err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
		}
	}
	if strings.HasPrefix(value, "/") {
		m.input.Reset()
		next, cmd, err := m.executeCommand(value)
		if err != nil {
			m.statusNote = err.Error()
			return m, nil
		}
		return next, cmd
	}
	return m.submitPrompt(value)
}

func (m model) handleCompressedPasteSubmit(rawValue string) (tea.Model, bool) {
	if markerChain, ok := extractLeadingCompressedMarker(rawValue); ok {
		tail := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rawValue), markerChain))
		if tail != "" {
			if m.shouldCompressPastedText(tail, "paste-enter") {
				marker, content, err := m.compressPastedText(tail)
				if err != nil {
					m.statusNote = err.Error()
					return m, true
				}
				combined := strings.TrimSpace(markerChain) + marker
				m.setInputValue(combined)
				m.syncInputOverlays()
				m.statusNote = fmt.Sprintf("Detected another pasted block and compressed it as %s (%d lines). Press Enter again to send.", marker, content.Lines)
				return m, true
			}
			if len(tail) >= 24 || strings.Contains(tail, "\n") {
				m.setInputValue(strings.TrimSpace(markerChain))
				m.syncInputOverlays()
				m.statusNote = "Detected continued paste chunk after compressed marker. Kept compressed markers only; press Enter again to send."
				return m, true
			}
		}
	}
	return m, false
}

func (m model) handleLongPasteCompression(rawValue string) (tea.Model, bool) {
	isAlreadyCompressed := strings.Contains(rawValue, "[Paste #") || strings.Contains(rawValue, "[Pasted #")
	if isAlreadyCompressed || !m.shouldCompressPastedText(rawValue, "enter") {
		return m, false
	}
	marker, content, err := m.compressPastedText(rawValue)
	if err != nil {
		m.statusNote = err.Error()
		return m, true
	}
	m.setInputValue(marker)
	m.syncInputOverlays()
	m.statusNote = fmt.Sprintf("Long pasted text (%d lines) compressed as %s. Press Enter again to send. Use [Paste #%s] or [Paste #%s line10~line20] later.", content.Lines, marker, content.ID, content.ID)
	return m, true
}

func (m model) handleComposerMutation(msg tea.KeyMsg, ctrlVPasteDetected bool) (tea.Model, tea.Cmd) {
	mutationSource := inputMutationSource(msg)
	before := m.input.Value()
	var cmd tea.Cmd
	var after string
	if msg.Paste {
		preview := m.input
		preview, cmd = preview.Update(msg)
		after = preview.Value()
		m.handleInputMutation(before, after, mutationSource)
		if m.input.Value() == before && after != before {
			m.input = preview
		}
		after = m.input.Value()
	} else {
		m.input, cmd = m.input.Update(msg)
		after = m.input.Value()
		if after != before {
			m.handleInputMutation(before, after, mutationSource)
			after = m.input.Value()
		}
	}
	triggerClipboardImagePaste := shouldTriggerClipboardImagePaste(before, after, mutationSource)
	if ctrlVPasteDetected {
		triggerClipboardImagePaste = false
	}
	if !triggerClipboardImagePaste && msg.Paste {
		_, inserted, _ := insertionDiff(before, after)
		cleanInserted := strings.TrimSpace(strings.ReplaceAll(inserted, ctrlVMarkerRune, ""))
		if cleanInserted == "" {
			triggerClipboardImagePaste = true
		}
	}
	if triggerClipboardImagePaste {
		if cleaned, changed := stripCtrlVMarker(m.input.Value()); changed {
			m.setInputValue(cleaned)
		}
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
	}
	m.syncInputOverlays()
	return m, cmd
}
