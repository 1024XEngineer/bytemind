package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) beginPasteTransaction(payload, source string) {
	if m == nil {
		return
	}
	candidate := strings.ReplaceAll(normalizeNewlines(payload), ctrlVMarkerRune, "")
	if strings.TrimSpace(candidate) == "" {
		m.clearPasteTransaction()
		return
	}
	m.pasteTransaction = pasteTransactionState{
		Active:             true,
		Source:             strings.TrimSpace(source),
		Payload:            candidate,
		Consumed:           0,
		AwaitTrailingEnter: shouldGuardTrailingPasteEnter(source),
	}
}

func (m *model) beginOrAppendPasteTransaction(payload, source string) {
	if m == nil {
		return
	}
	candidate := strings.ReplaceAll(normalizeNewlines(payload), ctrlVMarkerRune, "")
	if candidate == "" {
		return
	}
	source = strings.TrimSpace(source)
	tx := m.pasteTransaction
	if tx.Active && tx.Consumed == 0 && shouldAppendPasteTransactionPayload(tx.Source, source) {
		tx.Payload += candidate
		if strings.TrimSpace(tx.Source) == "" {
			tx.Source = source
		}
		m.pasteTransaction = tx
		return
	}
	if strings.TrimSpace(candidate) == "" {
		return
	}
	m.beginPasteTransaction(candidate, source)
}

func (m *model) clearPasteTransaction() {
	if m == nil {
		return
	}
	m.pasteTransaction = pasteTransactionState{}
}

func (m *model) consumePasteEchoKey(msg tea.KeyMsg) bool {
	if m == nil || !m.pasteTransaction.Active || msg.Paste {
		return false
	}
	if isCtrlVControlKey(msg) {
		// Some terminals emit Ctrl+V key markers before/within the echoed
		// key stream. Ignore them so they do not tear down the active
		// transaction and accidentally re-enable Enter submit.
		return true
	}
	fragment, ok := pasteEchoFragmentFromKey(msg)
	if !ok {
		return false
	}
	fragment = normalizeNewlines(fragment)
	if fragment == "" {
		return false
	}
	remaining := remainingPasteTransactionPayload(m.pasteTransaction.Payload, m.pasteTransaction.Consumed)
	if remaining == "" {
		if m.pasteTransaction.AwaitTrailingEnter && msg.Type == tea.KeyEnter && !msg.Alt {
			m.clearPasteTransaction()
			return true
		}
		m.clearPasteTransaction()
		return false
	}
	if strings.HasPrefix(remaining, fragment) {
		m.pasteTransaction.Consumed += len([]rune(fragment))
		if m.pasteTransaction.Consumed >= len([]rune(m.pasteTransaction.Payload)) {
			if !m.pasteTransaction.AwaitTrailingEnter {
				m.clearPasteTransaction()
			}
		}
		return true
	}
	m.clearPasteTransaction()
	return false
}

func shouldAppendPasteTransactionPayload(currentSource, nextSource string) bool {
	current := strings.ToLower(strings.TrimSpace(currentSource))
	next := strings.ToLower(strings.TrimSpace(nextSource))
	if current == "" || next == "" {
		return false
	}
	if current != next {
		return false
	}
	switch current {
	case "paste-key", "rune-burst-paste":
		return true
	default:
		return false
	}
}

func shouldGuardTrailingPasteEnter(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case "clipboard-capture":
		return true
	default:
		return false
	}
}

func isCtrlVControlKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlV {
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && string(msg.Runes[0]) == ctrlVMarkerRune {
		return true
	}
	return normalizeKeyName(msg.String()) == "ctrl+v"
}

func pasteEchoFragmentFromKey(msg tea.KeyMsg) (string, bool) {
	if msg.Type == tea.KeyEnter && !msg.Alt {
		return "\n", true
	}
	if msg.Type == tea.KeyTab && !msg.Alt {
		return "\t", true
	}
	if msg.Type == tea.KeySpace && !msg.Alt {
		return " ", true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		return string(msg.Runes), true
	}
	return "", false
}

func remainingPasteTransactionPayload(payload string, consumedRunes int) string {
	runes := []rune(payload)
	if consumedRunes <= 0 {
		return payload
	}
	if consumedRunes >= len(runes) {
		return ""
	}
	return string(runes[consumedRunes:])
}
