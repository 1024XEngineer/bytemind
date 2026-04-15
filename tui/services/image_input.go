package services

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	serviceInlineWindowsImagePathPattern = regexp.MustCompile(`(?i)[a-z]:\\[^\r\n\t"'<>|]*?\.(?:png|jpe?g|webp|gif)`)
	serviceInlineUnixImagePathPattern    = regexp.MustCompile(`(?i)/(?:[^\r\n\t"'<>|/]+/)*[^\r\n\t"'<>|/]+\.(?:png|jpe?g|webp|gif)`)
)

type InputMutationClass string

const (
	InputMutationOrdinary   InputMutationClass = "ordinary"
	InputMutationPasteEmpty InputMutationClass = "paste_empty"
	InputMutationPasteFull  InputMutationClass = "paste_filled"
)

type ImagePathSpan struct {
	Start int
	End   int
	Path  string
}

type ImageInputController struct{}

func NewImageInputController() *ImageInputController {
	return &ImageInputController{}
}

func (c *ImageInputController) ProcessMutation(before, after, source, workspace string, ingestPath func(string) (string, string, bool)) (string, string) {
	class, prefix, inserted, suffix := classifyMutation(before, after, source)
	if class != InputMutationPasteFull {
		return after, ""
	}

	paths := extractImagePathsFromChunk(inserted, workspace)
	if len(paths) > 0 {
		placeholders := make([]string, 0, len(paths))
		notes := make([]string, 0, len(paths))
		for _, path := range paths {
			placeholder, note, ok := ingestPath(path)
			if !ok {
				notes = append(notes, note)
				continue
			}
			placeholders = append(placeholders, placeholder)
		}
		if len(placeholders) == 0 {
			if len(notes) > 0 {
				return after, notes[0]
			}
			return after, ""
		}
		updated := after[:prefix] + strings.Join(placeholders, " ") + after[len(after)-suffix:]
		note := fmt.Sprintf("Attached %d image(s): %s", len(placeholders), strings.Join(placeholders, ", "))
		if len(notes) > 0 {
			note += "; " + notes[0]
		}
		return updated, note
	}

	spans := extractInlineImagePathSpans(inserted)
	if len(spans) == 0 {
		return after, ""
	}

	var transformed strings.Builder
	transformed.Grow(len(inserted))
	attached := make([]string, 0, len(spans))
	notes := make([]string, 0, len(spans))
	last := 0
	for _, span := range spans {
		if span.Start > last {
			transformed.WriteString(inserted[last:span.Start])
		}
		placeholder, note, ok := ingestPath(span.Path)
		if !ok {
			transformed.WriteString(inserted[span.Start:span.End])
			notes = append(notes, note)
		} else {
			transformed.WriteString(placeholder)
			attached = append(attached, placeholder)
		}
		last = span.End
	}
	if last < len(inserted) {
		transformed.WriteString(inserted[last:])
	}
	if len(attached) == 0 {
		if len(notes) > 0 {
			return after, notes[0]
		}
		return after, ""
	}

	updated := after[:prefix] + transformed.String() + after[len(after)-suffix:]
	note := fmt.Sprintf("Attached %d image(s): %s", len(attached), strings.Join(attached, ", "))
	if len(notes) > 0 {
		note += "; " + notes[0]
	}
	return updated, note
}

func (c *ImageInputController) ProcessWholeInputFallback(text, source string, lastPasteAt time.Time, pasteSubmitGuard time.Duration, ingestPath func(string) (string, string, bool)) (string, string) {
	if strings.TrimSpace(text) == "" {
		return text, ""
	}
	pasteLike := isCtrlVKey(source) || strings.Contains(strings.ToLower(source), "paste")
	if !pasteLike && (lastPasteAt.IsZero() || time.Since(lastPasteAt) > 2*pasteSubmitGuard) {
		return text, ""
	}

	spans := extractInlineImagePathSpans(text)
	if len(spans) == 0 {
		return text, ""
	}

	var transformed strings.Builder
	transformed.Grow(len(text))
	attached := make([]string, 0, len(spans))
	notes := make([]string, 0, len(spans))
	last := 0
	for _, span := range spans {
		if span.Start > last {
			transformed.WriteString(text[last:span.Start])
		}
		placeholder, note, ok := ingestPath(span.Path)
		if !ok {
			transformed.WriteString(text[span.Start:span.End])
			notes = append(notes, note)
		} else {
			transformed.WriteString(placeholder)
			attached = append(attached, placeholder)
		}
		last = span.End
	}
	if last < len(text) {
		transformed.WriteString(text[last:])
	}
	if len(attached) == 0 {
		if len(notes) > 0 {
			return text, notes[0]
		}
		return text, ""
	}
	updated := transformed.String()
	note := fmt.Sprintf("Attached %d image(s): %s", len(attached), strings.Join(attached, ", "))
	if len(notes) > 0 {
		note += "; " + notes[0]
	}
	return updated, note
}

func (c *ImageInputController) AttachClipboard(currentInput, mediaType, fileName string, data []byte, ingestBinary func(string, string, []byte) (string, string, bool)) (string, string) {
	placeholder, note, ok := ingestBinary(mediaType, fileName, data)
	if !ok {
		return currentInput, note
	}
	updated := placeholder
	if strings.TrimSpace(currentInput) != "" {
		updated = currentInput + " " + placeholder
	}
	if note != "" {
		return updated, note
	}
	return updated, "Attached image from clipboard: " + placeholder
}

func classifyMutation(before, after, source string) (InputMutationClass, int, string, int) {
	prefix, inserted, suffix := insertionDiff(before, after)
	cleanInserted := strings.ReplaceAll(inserted, "\x16", "")
	pasteSignal := isCtrlVKey(source) || strings.Contains(strings.ToLower(source), "paste") || strings.Contains(cleanInserted, "\n") || len(cleanInserted) > 1
	if shouldTriggerClipboardImagePaste(before, after, source) {
		return InputMutationPasteEmpty, prefix, inserted, suffix
	}
	if pasteSignal && strings.TrimSpace(cleanInserted) != "" {
		return InputMutationPasteFull, prefix, inserted, suffix
	}
	return InputMutationOrdinary, prefix, inserted, suffix
}

func extractImagePathsFromChunk(chunk, workspace string) []string {
	tokens := splitPathTokens(chunk)
	if len(tokens) == 0 {
		return nil
	}
	paths := make([]string, 0, len(tokens))
	candidateCount := 0
	for _, token := range tokens {
		token = strings.TrimSpace(strings.Trim(token, `"'`))
		if token == "" {
			continue
		}
		candidateCount++
		resolved := token
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(workspace, token)
		}
		resolved = filepath.Clean(resolved)
		info, err := os.Stat(resolved)
		if err != nil || info.IsDir() {
			continue
		}
		if !hasImageExt(resolved) {
			continue
		}
		paths = append(paths, resolved)
	}
	if len(paths) == 0 || (candidateCount > 1 && len(paths) != candidateCount) {
		return nil
	}
	return paths
}

func extractInlineImagePathSpans(chunk string) []ImagePathSpan {
	chunk = strings.TrimSpace(chunk)
	if chunk == "" {
		return nil
	}
	matches := make([]ImagePathSpan, 0, 4)
	appendMatches := func(pattern *regexp.Regexp) {
		for _, loc := range pattern.FindAllStringIndex(chunk, -1) {
			if len(loc) != 2 || loc[1] <= loc[0] {
				continue
			}
			raw := chunk[loc[0]:loc[1]]
			resolved := filepath.Clean(raw)
			info, err := os.Stat(resolved)
			if err != nil || info.IsDir() || !hasImageExt(resolved) {
				continue
			}
			matches = append(matches, ImagePathSpan{Start: loc[0], End: loc[1], Path: resolved})
		}
	}
	appendMatches(serviceInlineWindowsImagePathPattern)
	appendMatches(serviceInlineUnixImagePathPattern)
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Start == matches[j].Start {
			return matches[i].End < matches[j].End
		}
		return matches[i].Start < matches[j].Start
	})
	filtered := make([]ImagePathSpan, 0, len(matches))
	lastEnd := -1
	for _, span := range matches {
		if span.Start < lastEnd {
			continue
		}
		filtered = append(filtered, span)
		lastEnd = span.End
	}
	return filtered
}

func splitPathTokens(chunk string) []string {
	fields := strings.FieldsFunc(chunk, func(r rune) bool {
		switch r {
		case '\n', '\r', '\t', ',', ';':
			return true
		default:
			return false
		}
	})
	return fields
}

func hasImageExt(path string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(path)), ".")) {
	case "png", "jpg", "jpeg", "webp", "gif":
		return true
	default:
		return false
	}
}

func isCtrlVKey(source string) bool {
	source = strings.TrimSpace(source)
	return strings.EqualFold(source, "ctrl+v") || source == "\x16" || source == "["+"\x16"+"]"
}

func shouldTriggerClipboardImagePaste(before, after, source string) bool {
	if !isCtrlVKey(source) {
		return false
	}
	_, inserted, _ := insertionDiff(before, after)
	cleanInserted := strings.ReplaceAll(inserted, "\x16", "")
	return strings.TrimSpace(cleanInserted) == ""
}

func insertionDiff(before, after string) (prefix int, inserted string, suffix int) {
	prefix = lenCommonPrefix(before, after)
	beforeTail := before[prefix:]
	afterTail := after[prefix:]
	suffix = lenCommonSuffix(beforeTail, afterTail)
	if suffix > len(afterTail) {
		suffix = len(afterTail)
	}
	if suffix > len(beforeTail) {
		suffix = len(beforeTail)
	}
	inserted = afterTail[:len(afterTail)-suffix]
	return prefix, inserted, suffix
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

func lenCommonSuffix(a, b string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			return i
		}
	}
	return limit
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
