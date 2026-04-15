package tui

import (
	"strings"
	"time"

	tuiapi "bytemind/internal/tui/api"
	tuiservices "bytemind/internal/tui/services"
)

func (m *model) isLongPastedText(input string) bool {
	normalized := normalizeNewlines(input)
	trimmed := strings.TrimSpace(normalized)
	if trimmed == "" {
		return false
	}
	if isLikelyPathInput(trimmed) {
		return false
	}

	lines := strings.Split(normalized, "\n")
	lineCount := len(lines)
	newlineCount := strings.Count(normalized, "\n")

	if lineCount > longPasteLineThreshold || len(normalized) > longPasteCharThreshold {
		return true
	}
	if lineCount <= 2 && len(normalized) >= flattenedPasteCharThreshold {
		return true
	}
	if newlineCount >= 3 && len(normalized) >= pasteQuickCharThreshold {
		return true
	}
	return false
}

func isCtrlVSource(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	return source == "ctrl+v" || source == "ctrl+shift+v"
}

func isPasteLikeSource(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	return isCtrlVSource(source) || strings.Contains(source, "paste")
}

func (m *model) shouldCompressPastedText(input, source string) bool {
	if m == nil {
		return false
	}
	trimmed := strings.TrimSpace(input)
	hasPathLikeContent := isLikelyPathInput(trimmed) || len(extractImagePathsFromChunk(input, m.workspace)) > 0 || len(extractInlineImagePathSpans(input)) > 0
	if m.inputPolicy == nil {
		m.inputPolicy = tuiservices.NewInputPolicy()
	}
	result := m.inputPolicy.Evaluate(tuiapi.PastePolicyInput{
		Input:              input,
		Source:             source,
		Workspace:          m.workspace,
		LastInputAt:        m.lastInputAt,
		LastPasteAt:        m.lastPasteAt,
		InputBurstSize:     m.inputBurstSize,
		PasteSubmitGuard:   pasteSubmitGuard,
		PasteBurstWindow:   pasteBurstWindow,
		PasteQuickChars:    pasteQuickCharThreshold,
		BurstImmediateMin:  pasteBurstImmediateMinChars,
		BurstCharThreshold: pasteBurstCharThreshold,
		ContinuationWindow: pasteContinuationWindow,
	}, hasPathLikeContent)
	if !result.Success {
		return false
	}
	return result.Data.ShouldCompress
}

func looksLikePastedFragment(value string) bool {
	if strings.ContainsAny(value, "\r\n\t ") {
		return true
	}
	return len(value) >= 64
}

func isLikelyPathInput(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	if len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	if strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, `/`) || strings.HasPrefix(value, `./`) || strings.HasPrefix(value, `../`) {
		return true
	}
	separatorCount := strings.Count(value, `\`) + strings.Count(value, `/`)
	if separatorCount >= 3 && !strings.Contains(value, "\n") {
		return true
	}
	return false
}

func isSplitPasteContinuation(input, source string, lastPasteAt time.Time) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || isLikelyPathInput(trimmed) {
		return false
	}
	if !isPasteLikeSource(source) {
		return false
	}
	if strings.Contains(trimmed, "[Paste #") || strings.Contains(trimmed, "[Pasted #") {
		return false
	}
	if !lastPasteAt.IsZero() && time.Since(lastPasteAt) <= pasteContinuationWindow {
		return true
	}
	return strings.Contains(trimmed, "\n") || len(trimmed) >= pasteQuickCharThreshold
}
