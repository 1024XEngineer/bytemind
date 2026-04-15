package tui

import (
	"fmt"
	"strings"
	"time"
)

func countCompressedMarkers(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	return len(compressedPasteMarkerAnyPattern.FindAllString(value, -1))
}

func shouldMergeIntoLatestMarker(source string, lastCompressedAt time.Time) bool {
	if lastCompressedAt.IsZero() {
		return false
	}
	if isPasteLikeSource(source) {
		return time.Since(lastCompressedAt) <= 120*time.Millisecond
	}
	return time.Since(lastCompressedAt) <= 300*time.Millisecond
}

func (m *model) mergeTailIntoLatestMarker(chain, tail string) (string, bool, error) {
	if m == nil {
		return chain, false, nil
	}
	rawTail := normalizeNewlines(tail)
	if strings.TrimSpace(rawTail) == "" {
		return chain, false, nil
	}

	loc := latestCompressedMarkerInChain(chain)
	if !loc.ok {
		return chain, false, nil
	}
	content, ok := m.findPastedContent(loc.id)
	if !ok {
		return chain, false, nil
	}

	if strings.TrimSpace(content.Content) == "" {
		content.Content = rawTail
	} else {
		content.Content = content.Content + rawTail
	}
	content.Content = normalizeNewlines(content.Content)
	content.Lines = len(strings.Split(content.Content, "\n"))
	content.Time = time.Now().UTC()

	if err := m.storePastedContent(content); err != nil {
		return chain, false, err
	}
	m.lastCompressedPasteAt = time.Now().UTC()

	updatedMarker := fmt.Sprintf("[Paste #%s ~%d lines]", content.ID, content.Lines)
	updatedChain := chain[:loc.start] + updatedMarker + chain[loc.end:]
	return strings.TrimSpace(updatedChain), true, nil
}

func latestCompressedMarkerInChain(chain string) compressedMarkerLoc {
	matches := compressedPasteMarkerDetailsPattern.FindAllStringSubmatchIndex(chain, -1)
	if len(matches) == 0 {
		return compressedMarkerLoc{}
	}
	last := matches[len(matches)-1]
	if len(last) < 4 {
		return compressedMarkerLoc{}
	}
	idStart, idEnd := last[2], last[3]
	if idStart < 0 || idEnd < 0 || idStart >= idEnd || idEnd > len(chain) {
		return compressedMarkerLoc{}
	}
	return compressedMarkerLoc{
		id:    chain[idStart:idEnd],
		start: last[0],
		end:   last[1],
		ok:    true,
	}
}

func extractLeadingCompressedMarker(value string) (string, bool) {
	value = strings.TrimSpace(value)
	marker := compressedPasteMarkerChainPrefixPattern.FindString(value)
	if marker == "" {
		return "", false
	}
	marker = strings.TrimSpace(marker)
	if !strings.HasPrefix(value, marker) {
		return "", false
	}
	return marker, true
}

func compressedMarkerIDs(value string) []string {
	matches := compressedPasteMarkerDetailsPattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return nil
	}
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id := strings.TrimSpace(match[1])
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func sameMarkerIDPrefix(beforeIDs, afterIDs []string) bool {
	if len(beforeIDs) == 0 || len(afterIDs) < len(beforeIDs) {
		return false
	}
	for i := range beforeIDs {
		if beforeIDs[i] != afterIDs[i] {
			return false
		}
	}
	return true
}
