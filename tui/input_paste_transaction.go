package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const pasteCtrlVControlEchoWindow = 120 * time.Millisecond
const pasteTransactionAppendWindow = 120 * time.Millisecond

func (m *model) beginPasteTransaction(payload, source string) {
	if m == nil {
		return
	}
	candidate := strings.ReplaceAll(normalizeNewlines(payload), ctrlVMarkerRune, "")
	if strings.TrimSpace(candidate) == "" {
		m.clearPasteTransaction()
		return
	}
	now := time.Now()
	m.pasteTransaction = pasteTransactionState{
		Active:             true,
		Source:             strings.TrimSpace(source),
		Payload:            candidate,
		Consumed:           0,
		StartedAt:          now,
		LastEchoAt:         time.Time{},
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
	now := time.Now()
	if tx.Active && tx.Consumed == 0 && shouldAppendPasteTransactionPayload(tx.Source, source) &&
		isPasteTransactionAppendWindowOpen(tx, now) {
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
	if msg.Type == tea.KeyRunes && m.shouldExpireStalePasteTransactionBeforeRunes() {
		// No echoed payload arrived for a while; treat upcoming runes as fresh
		// user input (for example a second paste operation), not stale echo.
		m.clearPasteTransaction()
		return false
	}
	if isCtrlVControlKey(msg) {
		// Some terminals emit Ctrl+V key markers before/within the echoed
		// key stream. Ignore them so they do not tear down the active
		// transaction and accidentally re-enable Enter submit.
		if m.shouldConsumeCtrlVControlKey(msg) {
			m.pasteTransaction.LastEchoAt = time.Now()
			return true
		}
		// If the transaction is stale and a fresh Ctrl+V arrives, allow it to
		// start a new paste boundary instead of swallowing the user's input.
		m.clearPasteTransaction()
		return false
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
		if m.shouldConsumeTrailingPasteEnter(msg) {
			m.clearPasteTransaction()
			m.releasePasteSubmitSuppression()
			return true
		}
		m.clearPasteTransaction()
		return false
	}
	if msg.Type == tea.KeyEnter && !msg.Alt && m.pasteTransaction.AwaitTrailingEnter && !strings.HasPrefix(remaining, "\n") {
		// Terminal variation: some explicit paste flows do not echo payload
		// characters back through key events. In that case the first plain Enter
		// after paste should close the transaction but must not submit.
		m.clearPasteTransaction()
		m.releasePasteSubmitSuppression()
		return true
	}
	if strings.HasPrefix(remaining, fragment) {
		m.pasteTransaction.Consumed += len([]rune(fragment))
		m.pasteTransaction.LastEchoAt = time.Now()
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

func (m *model) shouldExpireStalePasteTransactionBeforeRunes() bool {
	if m == nil {
		return false
	}
	tx := m.pasteTransaction
	if !tx.Active || tx.StartedAt.IsZero() {
		return false
	}
	if tx.Consumed > 0 || !tx.LastEchoAt.IsZero() {
		return false
	}
	return time.Since(tx.StartedAt) > pasteTransactionAppendWindow
}

func (m *model) shouldConsumeCtrlVControlKey(msg tea.KeyMsg) bool {
	if m == nil {
		return false
	}
	tx := m.pasteTransaction
	if tx.StartedAt.IsZero() {
		return false
	}
	now := time.Now()
	isCtrlVMarkerRune := msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && string(msg.Runes[0]) == ctrlVMarkerRune
	if !isCtrlVMarkerRune && msg.Type != tea.KeyCtrlV && normalizeKeyName(msg.String()) != "ctrl+v" {
		return false
	}
	// Treat Ctrl+V as control noise only in a tiny window right after
	// transaction start or right after echoed key fragments. This avoids
	// swallowing a user's intentional second paste.
	if tx.Consumed == 0 && now.Sub(tx.StartedAt) <= pasteCtrlVControlEchoWindow {
		return true
	}
	if !tx.LastEchoAt.IsZero() && now.Sub(tx.LastEchoAt) <= pasteCtrlVControlEchoWindow {
		return true
	}
	return false
}

func (m *model) shouldConsumeTrailingPasteEnter(msg tea.KeyMsg) bool {
	if m == nil {
		return false
	}
	if !m.pasteTransaction.AwaitTrailingEnter {
		return false
	}
	if msg.Type != tea.KeyEnter || msg.Alt {
		return false
	}
	// In terminal mode we cannot reliably distinguish a user Enter from a
	// delayed echoed Enter after paste. Consume the first plain Enter once the
	// payload has been fully echoed, and require the next Enter to submit.
	return true
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
	case "paste-key", "paste-payload", "paste-burst", "rune-burst-paste":
		return true
	default:
		return false
	}
}

func shouldGuardTrailingPasteEnter(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case "clipboard-capture", "ctrl+v", "paste-payload", "paste-key", "paste-burst", "rune-burst-paste":
		return true
	default:
		return false
	}
}

func isPasteTransactionAppendWindowOpen(tx pasteTransactionState, now time.Time) bool {
	if !tx.Active || tx.StartedAt.IsZero() {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	if now.Sub(tx.StartedAt) <= pasteTransactionAppendWindow {
		return true
	}
	if !tx.LastEchoAt.IsZero() && now.Sub(tx.LastEchoAt) <= pasteTransactionAppendWindow {
		return true
	}
	return false
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
