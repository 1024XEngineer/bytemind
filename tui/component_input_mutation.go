package tui

import (
	"strings"
	"time"
)

func (m *model) noteInputMutation(before, after, source string) {
	now := time.Now()
	gap := time.Duration(0)
	if !m.lastInputAt.IsZero() {
		gap = now.Sub(m.lastInputAt)
	}
	delta := len(after) - len(before)
	if delta < 0 {
		delta = 0
	}

	if now.Sub(m.lastInputAt) <= 80*time.Millisecond {
		m.inputBurstSize += max(1, delta)
	} else {
		m.inputBurstSize = max(1, delta)
	}
	m.lastInputAt = now

	if shouldRecordPasteSignal(source) {
		m.lastPasteAt = now
		m.armClipboardPasteCaptureSignal()
		return
	}
	if m.shouldArmClipboardCaptureFromImplicitMutation(before, after, source, gap, delta) {
		m.armClipboardPasteCaptureSignal()
	}
}

func shouldRecordPasteSignal(source string) bool {
	if isPasteLikeSource(source) {
		return true
	}
	return false
}

func (m *model) handleInputMutation(before, after, source string) string {
	m.noteInputMutation(before, after, source)

	updated, note := m.applyInputImagePipeline(before, after, source)
	if updated == after {
		fallbackUpdated, fallbackNote := m.applyWholeInputImagePathFallback(after, source)
		if fallbackUpdated != after {
			updated = fallbackUpdated
		}
		if strings.TrimSpace(note) == "" {
			note = fallbackNote
		}
	}
	if m.shouldAttemptClipboardPasteCapture(before, updated, source) {
		captured, captureNote, ok := m.tryStartClipboardPasteCapture(before, updated, source)
		if ok {
			updated = captured
			if strings.TrimSpace(note) == "" {
				note = captureNote
			}
		}
	}

	pasteUpdated, pasteNote := m.applyLongPastedTextPipeline(before, updated, source)
	if pasteUpdated != updated {
		updated = pasteUpdated
	}
	if strings.TrimSpace(note) == "" {
		note = pasteNote
	}
	if locked, changed := m.protectCompressedMarkerChain(before, updated, source); changed {
		updated = locked
		if strings.TrimSpace(note) == "" {
			note = "Paste marker is locked to prevent accidental edits."
		}
	}
	if locked, changed := m.protectImagePlaceholderDeletion(before, updated, source); changed {
		updated = locked
	}

	if updated != after {
		m.setInputValue(updated)
	}
	if strings.TrimSpace(note) != "" {
		m.statusNote = note
	}

	return updated
}

func lenCommonPrefix(a, b string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}
