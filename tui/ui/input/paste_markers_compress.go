package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (m *model) compressPastedText(input string) (string, pastedContent, error) {
	m.ensurePastedContentState()
	normalized := normalizeNewlines(input)
	lines := strings.Split(normalized, "\n")
	lineCount := countPastedDisplayLines(normalized)
	id := strconv.Itoa(m.nextPasteID)
	m.nextPasteID++
	now := time.Now().UTC()

	preview := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		preview = line
		break
	}
	if preview == "" {
		preview = "(empty)"
	}
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}

	content := pastedContent{
		ID:      id,
		Content: normalized,
		Lines:   lineCount,
		Time:    now,
		Preview: preview,
	}
	if err := m.storePastedContent(content); err != nil {
		return "", pastedContent{}, err
	}
	m.lastCompressedPasteAt = now
	return fmt.Sprintf("[Paste #%s ~%d lines]", id, lineCount), content, nil
}

func (m *model) applyLongPastedTextPipeline(before, after, source string) (string, string) {
	if m == nil {
		return after, ""
	}
	class, prefix, inserted, suffix := classifyInputMutation(before, after, source)
	if chain, ok := extractLeadingCompressedMarker(before); ok {
		afterTrimmed := strings.TrimSpace(after)
		if strings.HasPrefix(afterTrimmed, chain) {
			rawTail := strings.TrimPrefix(afterTrimmed, chain)
			tail := strings.TrimSpace(rawTail)
			if tail == "" {
				chainValue := strings.TrimSpace(chain)
				if strings.HasPrefix(after, chainValue) {
					visibleTail := strings.TrimPrefix(after, chainValue)
					if strings.TrimSpace(visibleTail) == "" &&
						shouldHoldCompressedMarker(before, after, source, m.lastPasteAt, m.inputBurstSize) {
						return chainValue, ""
					}
				}
			}
			if tail != "" && !compressedPasteMarkerPattern.MatchString(tail) && !compressedPasteMarkerChainPrefixPattern.MatchString(tail) {
				safeTail := len(extractImagePathsFromChunk(tail, m.workspace)) == 0 &&
					len(extractInlineImagePathSpans(tail)) == 0
				if safeTail && shouldMergeIntoLatestMarker(source, m.lastCompressedPasteAt) {
					merged, ok, err := m.mergeTailIntoLatestMarker(chain, rawTail)
					if err != nil {
						return after, err.Error()
					}
					if ok {
						return merged, ""
					}
				}
				if safeTail && m.isLongPastedText(tail) {
					marker, content, err := m.compressPastedText(tail)
					if err != nil {
						return after, err.Error()
					}
					updated := strings.TrimSpace(chain) + marker
					note := fmt.Sprintf("Detected another pasted block and compressed it as %s (%d lines).",
						marker, content.Lines)
					return updated, note
				}
				if shouldHoldCompressedMarker(before, after, source, m.lastPasteAt, m.inputBurstSize) {
					return strings.TrimSpace(chain), ""
				}
				return after, ""
			}
		}
	}
	if class == inputMutationPasteFilled {
		candidate := strings.ReplaceAll(inserted, ctrlVMarkerRune, "")
		if strings.TrimSpace(candidate) != "" && m.shouldCompressPastedText(candidate, source) {
			marker, content, err := m.compressPastedText(candidate)
			if err != nil {
				return after, err.Error()
			}
			updated := after[:prefix] + marker + after[len(after)-suffix:]
			note := fmt.Sprintf("Long pasted text (%d lines) compressed as %s. Use [Paste #%s], [Paste #%s line10], or [Paste #%s line10~line20].",
				content.Lines, marker, content.ID, content.ID, content.ID)
			return updated, note
		}
	}
	if strings.Contains(after, "[Paste #") || strings.Contains(after, "[Pasted #") {
		return after, ""
	}
	if !m.isLongPastedText(after) {
		return after, ""
	}
	marker, content, err := m.compressPastedText(after)
	if err != nil {
		return after, err.Error()
	}
	note := fmt.Sprintf("Long pasted text (%d lines) compressed as %s. Use [Paste #%s], [Paste #%s line10], or [Paste #%s line10~line20].",
		content.Lines, marker, content.ID, content.ID, content.ID)
	return marker, note
}
