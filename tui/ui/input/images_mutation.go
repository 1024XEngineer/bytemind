package tui

import "strings"

func classifyInputMutation(before, after, source string) (inputMutationClass, int, string, int) {
	prefix, inserted, suffix := insertionDiff(before, after)
	cleanInserted := strings.ReplaceAll(inserted, ctrlVMarkerRune, "")
	pasteSignal := isCtrlVKey(source) || strings.Contains(strings.ToLower(source), "paste") || strings.Contains(cleanInserted, "\n") || len(cleanInserted) > 1
	if shouldTriggerClipboardImagePaste(before, after, source) {
		return inputMutationPasteEmpty, prefix, inserted, suffix
	}
	if pasteSignal && strings.TrimSpace(cleanInserted) != "" {
		return inputMutationPasteFilled, prefix, inserted, suffix
	}
	return inputMutationOrdinary, prefix, inserted, suffix
}

func isCtrlVKey(source string) bool {
	source = strings.TrimSpace(source)
	return strings.EqualFold(source, ctrlVKeyName) ||
		source == ctrlVMarkerRune ||
		source == "["+ctrlVMarkerRune+"]"
}

func shouldTriggerClipboardImagePaste(before, after, source string) bool {
	if !isCtrlVKey(source) {
		return false
	}
	_, inserted, _ := insertionDiff(before, after)
	cleanInserted := strings.ReplaceAll(inserted, ctrlVMarkerRune, "")
	return strings.TrimSpace(cleanInserted) == ""
}

func stripCtrlVMarker(text string) (string, bool) {
	cleaned := strings.ReplaceAll(text, ctrlVMarkerRune, "")
	return cleaned, cleaned != text
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

func lenCommonSuffix(a, b string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			return i
		}
	}
	return limit
}
