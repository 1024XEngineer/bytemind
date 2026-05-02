package services

import (
	"strings"
	"time"

	tuiapi "github.com/1024XEngineer/bytemind/tui/api"
)

type InputPolicy struct{}

func NewInputPolicy() *InputPolicy {
	return &InputPolicy{}
}

func (p *InputPolicy) Evaluate(req tuiapi.PastePolicyInput, hasPathLikeContent bool) tuiapi.Result[tuiapi.PastePolicyDecision] {
	trimmed := strings.TrimSpace(req.Input)
	if trimmed == "" {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{})
	}
	if hasPathLikeContent {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{Reason: "path_like_content"})
	}
	if isLongPastedText(req.Input) {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{ShouldCompress: true, Reason: "long_text"})
	}
	rapidBurst := !req.LastInputAt.IsZero() &&
		time.Since(req.LastInputAt) <= req.PasteBurstWindow &&
		req.InputBurstSize >= req.BurstImmediateMin
	if rapidBurst && len(trimmed) >= req.BurstImmediateMin && looksLikePastedFragment(trimmed) {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{ShouldCompress: true, Reason: "rapid_burst"})
	}
	if len(trimmed) < req.PasteQuickChars {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{Reason: "too_short"})
	}
	if isPasteLikeSource(req.Source) {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{ShouldCompress: true, Reason: "paste_source"})
	}
	if !req.LastPasteAt.IsZero() && time.Since(req.LastPasteAt) <= 2*req.PasteSubmitGuard {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{ShouldCompress: true, Reason: "recent_paste"})
	}
	if isSplitPasteContinuation(trimmed, req.Source, req.LastPasteAt, req.ContinuationWindow) {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{ShouldCompress: true, Reason: "continuation"})
	}
	if !req.LastInputAt.IsZero() && time.Since(req.LastInputAt) <= req.PasteBurstWindow && req.InputBurstSize >= req.BurstCharThreshold {
		return tuiapi.Ok(tuiapi.PastePolicyDecision{ShouldCompress: true, Reason: "burst_threshold"})
	}
	return tuiapi.Ok(tuiapi.PastePolicyDecision{
		ShouldCompress: req.InputBurstSize >= req.BurstCharThreshold,
		Reason:         "final_threshold",
	})
}

func isLongPastedText(input string) bool {
	normalized := strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n")
	trimmed := strings.TrimSpace(normalized)
	if trimmed == "" {
		return false
	}
	lines := strings.Split(normalized, "\n")
	lineCount := len(lines)
	newlineCount := strings.Count(normalized, "\n")

	if lineCount > 10 || len(normalized) > 500 {
		return true
	}
	if lineCount <= 2 && len(normalized) >= 180 {
		return true
	}
	if newlineCount >= 3 && len(normalized) >= 80 {
		return true
	}
	return false
}

func isPasteLikeSource(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	return source == "ctrl+v" || source == "ctrl+shift+v" || strings.Contains(source, "paste")
}

func looksLikePastedFragment(value string) bool {
	if strings.ContainsAny(value, "\r\n\t ") {
		return true
	}
	return len(value) >= 64
}

func isSplitPasteContinuation(input, source string, lastPasteAt time.Time, continuationWindow time.Duration) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || strings.Contains(trimmed, "[Paste #") || strings.Contains(trimmed, "[Pasted #") {
		return false
	}
	if !lastPasteAt.IsZero() && time.Since(lastPasteAt) <= continuationWindow {
		return true
	}
	return strings.Contains(trimmed, "\n") || (isPasteLikeSource(source) && len(trimmed) >= 80)
}
