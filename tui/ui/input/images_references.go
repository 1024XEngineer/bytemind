package tui

import (
	"fmt"
	"strconv"
	"strings"

	"bytemind/internal/llm"
)

func extractImagePlaceholderIDs(text string) []int {
	matches := imagePlaceholderPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	ids := make([]int, 0, len(matches))
	seen := make(map[int]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id, err := strconv.Atoi(match[1])
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func placeholderForImageID(id int) string {
	return fmt.Sprintf("[Image #%d]", id)
}

func imageIDFromPlaceholder(value string) (int, bool) {
	match := imagePlaceholderPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) < 2 {
		return 0, false
	}
	id, err := strconv.Atoi(match[1])
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func extractMentionImageSpans(text string, bindings map[string]llm.AssetID) []mentionImageSpan {
	if len(bindings) == 0 || strings.TrimSpace(text) == "" {
		return nil
	}
	matches := imageMentionPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	spans := make([]mentionImageSpan, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start, end := match[0], match[1]
		pathStart, pathEnd := match[2], match[3]
		key := normalizeImageMentionPath(text[pathStart:pathEnd])
		if key == "" {
			continue
		}
		assetID, ok := bindings[key]
		if !ok {
			continue
		}
		spans = append(spans, mentionImageSpan{Start: start, End: end, AssetID: assetID, Raw: text[start:end]})
	}
	return spans
}

func extractMentionImageReferenceKeys(text string) map[string]struct{} {
	result := make(map[string]struct{}, 8)
	if strings.TrimSpace(text) == "" {
		return result
	}
	matches := imageMentionPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		key := normalizeImageMentionPath(match[1])
		if key == "" {
			continue
		}
		result[key] = struct{}{}
	}
	return result
}

func collectImageAssetIDsFromMessages(messages []llm.Message) []llm.AssetID {
	if len(messages) == 0 {
		return nil
	}
	ids := make([]llm.AssetID, 0, 8)
	seen := make(map[llm.AssetID]struct{}, 8)
	for _, message := range messages {
		for _, part := range message.Parts {
			if part.Image == nil {
				continue
			}
			assetID := llm.AssetID(strings.TrimSpace(string(part.Image.AssetID)))
			if assetID == "" {
				continue
			}
			if _, ok := seen[assetID]; ok {
				continue
			}
			seen[assetID] = struct{}{}
			ids = append(ids, assetID)
		}
	}
	return ids
}
