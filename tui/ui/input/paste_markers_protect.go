package tui

import (
	"strings"
	"time"
)

func isMarkerDeletionSource(source string) bool {
	key := normalizeKeyName(source)
	return key == "backspace" || key == "delete" || key == "ctrl+h"
}

func dropLatestCompressedMarker(chain string) string {
	matches := compressedPasteMarkerAnyPattern.FindAllStringIndex(chain, -1)
	if len(matches) == 0 {
		return strings.TrimSpace(chain)
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return strings.TrimSpace(chain)
	}
	updated := chain[:last[0]] + chain[last[1]:]
	return strings.TrimSpace(updated)
}

func shouldHoldCompressedMarker(before, after, source string, lastPasteAt time.Time, burst int) bool {
	rawAfter := after
	before = strings.TrimSpace(before)
	after = strings.TrimSpace(after)
	marker, ok := extractLeadingCompressedMarker(before)
	if !ok {
		return false
	}
	if strings.HasPrefix(rawAfter, marker) {
		rawTail := strings.TrimPrefix(rawAfter, marker)
		if rawTail != "" && strings.TrimSpace(rawTail) == "" {
			if isPasteLikeSource(source) || burst >= 8 {
				return true
			}
			if !lastPasteAt.IsZero() && time.Since(lastPasteAt) <= pasteContinuationWindow {
				return true
			}
		}
	}
	if len(after) <= len(marker) || !strings.HasPrefix(after, marker) {
		return false
	}
	tail := strings.TrimSpace(strings.TrimPrefix(after, marker))
	if tail == "" {
		return false
	}
	if compressedPasteMarkerPattern.MatchString(tail) || compressedPasteMarkerChainPrefixPattern.MatchString(tail) {
		return false
	}
	if len(tail) >= 24 || strings.Contains(tail, "\n") {
		return true
	}
	if isPasteLikeSource(source) || burst >= 8 {
		return true
	}
	if !lastPasteAt.IsZero() && time.Since(lastPasteAt) <= pasteContinuationWindow {
		return true
	}
	return false
}

func (m *model) protectCompressedMarkerChain(before, after, source string) (string, bool) {
	if m == nil {
		return after, false
	}
	beforeChain, ok := extractLeadingCompressedMarker(before)
	if !ok {
		return after, false
	}
	beforeTrimmed := strings.TrimSpace(before)
	afterTrimmed := strings.TrimSpace(after)
	if afterTrimmed == beforeTrimmed {
		return after, false
	}
	if afterTrimmed == "" {
		return after, false
	}
	if isMarkerDeletionSource(source) && !strings.HasPrefix(afterTrimmed, beforeChain) {
		beforeTail := strings.TrimSpace(strings.TrimPrefix(beforeTrimmed, beforeChain))
		reduced := dropLatestCompressedMarker(beforeChain)
		if reduced == "" {
			return beforeTail, true
		}
		if beforeTail != "" {
			return reduced + beforeTail, true
		}
		return reduced, true
	}
	if strings.HasPrefix(afterTrimmed, beforeChain) {
		return after, false
	}

	afterChain, afterHasChain := extractLeadingCompressedMarker(afterTrimmed)
	if afterHasChain {
		beforeIDs := compressedMarkerIDs(beforeChain)
		afterIDs := compressedMarkerIDs(afterChain)
		if sameMarkerIDPrefix(beforeIDs, afterIDs) {
			if strings.TrimSpace(afterChain) == strings.TrimSpace(beforeChain) ||
				isPasteLikeSource(source) ||
				source == "paste-enter" {
				return after, false
			}
			tail := strings.TrimSpace(strings.TrimPrefix(afterTrimmed, afterChain))
			restored := strings.TrimSpace(beforeChain)
			if tail != "" {
				restored += tail
			}
			return restored, true
		}
		tail := strings.TrimSpace(strings.TrimPrefix(afterTrimmed, afterChain))
		restored := strings.TrimSpace(beforeChain)
		if tail != "" {
			restored += tail
		}
		return restored, true
	}

	if isPasteLikeSource(source) {
		return strings.TrimSpace(beforeChain) + afterTrimmed, true
	}
	return strings.TrimSpace(beforeChain), true
}
