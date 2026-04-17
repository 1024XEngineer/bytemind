package tui

import (
	"strings"
	"time"
)

func (m *model) noteInputMutation(before, after, source string) {
	now := time.Now()
	previousInputAt := m.lastInputAt
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

	if shouldRecordPasteSignal(before, after, source) ||
		shouldRecordImplicitPasteBurst(after, source, previousInputAt, now, m.inputBurstSize) ||
		(m.inputBurstSize >= 4 && isLikelyPathInput(strings.TrimSpace(after))) {
		m.lastPasteAt = now
		m.armPasteSubmitGuard(now)
	}
}

func shouldRecordPasteSignal(before, after, source string) bool {
	if source == "paste-enter" || isPasteLikeSource(source) {
		return true
	}
	if source == "rune" || source == "rapid-enter" {
		return false
	}
	_, inserted, _ := insertionDiff(before, after)
	inserted = strings.ReplaceAll(inserted, ctrlVMarkerRune, "")
	trimmed := strings.TrimSpace(inserted)
	if trimmed == "" {
		return false
	}
	if strings.Contains(inserted, "\n") {
		return true
	}
	return len(inserted) > 1 && len(trimmed) >= pasteBurstImmediateMinChars
}

func shouldRecordImplicitPasteBurst(after, source string, previousInputAt, now time.Time, burst int) bool {
	if source == "paste-enter" || isPasteLikeSource(source) {
		return false
	}
	if previousInputAt.IsZero() || now.Sub(previousInputAt) > 2*pasteBurstWindow {
		return false
	}
	if burst < pasteBurstImmediateMinChars {
		return false
	}
	trimmed := strings.TrimSpace(after)
	if trimmed == "" || strings.Contains(trimmed, "[Paste #") || strings.Contains(trimmed, "[Pasted #") {
		return false
	}
	if source == "rune" && !strings.Contains(after, "\n\n") {
		return false
	}
	if strings.Count(after, "\n") >= 2 {
		return true
	}
	if burst >= pasteBurstCharThreshold && len(trimmed) >= pasteQuickCharThreshold && looksLikePastedFragment(trimmed) {
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
