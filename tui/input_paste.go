package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

const (
	pastedContentMetaKey        = "pasted_contents"
	maxStoredPastedContents     = 10
	longPasteLineThreshold      = 10
	longPasteCharThreshold      = 500
	pasteQuickCharThreshold     = 80
	pasteBurstImmediateMinChars = 12
	flattenedPasteCharThreshold = 180
	pasteBurstCharThreshold     = 120
	pasteBurstWindow            = 80 * time.Millisecond
	pasteContinuationWindow     = 1500 * time.Millisecond
	maxSinglePastedCharLength   = 200000
	pasteSoftWrapWidth          = 72
)

var pastedRefPattern = regexp.MustCompile(`\[(?:Paste|Pasted)(?:\s+#(\d+))?(?:\s+~(\d+)\s+lines)?(?:\s+line(\d+)(?:~line(\d+))?)?\]`)
var compressedPasteMarkerPattern = regexp.MustCompile(`^\[(?:Paste|Pasted)\s+#\d+\s+~\d+\s+lines\]$`)
var compressedPasteMarkerPrefixPattern = regexp.MustCompile(`^\[(?:Paste|Pasted)\s+#\d+\s+~\d+\s+lines\]`)
var compressedPasteMarkerChainPrefixPattern = regexp.MustCompile(`^(?:(?:\[(?:Paste|Pasted)\s+#\d+\s+~\d+\s+lines\])\s*)+`)
var compressedPasteMarkerAnyPattern = regexp.MustCompile(`\[(?:Paste|Pasted)\s+#\d+\s+~\d+\s+lines\]`)
var compressedPasteMarkerDetailsPattern = regexp.MustCompile(`\[(?:Paste|Pasted)\s+#(\d+)\s+~(\d+)\s+lines\]`)

const (
	pasteRefGroupID        = 2
	pasteRefGroupLineCount = 4
	pasteRefGroupLineStart = 6
	pasteRefGroupLineEnd   = 8
)

type pastedContent struct {
	ID      string    `json:"id"`
	Content string    `json:"content"`
	Lines   int       `json:"lines"`
	Time    time.Time `json:"time"`
	Preview string    `json:"preview"`
}

type persistedPastedContents struct {
	Version  int                      `json:"version"`
	NextID   int                      `json:"next_id"`
	Order    []string                 `json:"order"`
	Contents map[string]pastedContent `json:"contents"`
}

func normalizeNewlines(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	return input
}

func trimTrailingPasteTerminators(input string) string {
	return normalizeNewlines(input)
}

func schedulePasteFinalize(id int) tea.Cmd {
	return tea.Tick(pasteAggregateDebounce, func(time.Time) tea.Msg {
		return pasteFinalizeMsg{ID: id}
	})
}

func (m *model) beginPasteSession(source string) {
	if m == nil {
		return
	}
	if m.pasteSession.active {
		return
	}
	now := time.Now()
	m.pasteSession = pasteSessionState{
		active:      true,
		startedAt:   now,
		lastEventAt: now,
		sourceKind:  source,
		baseInput:   m.input.Value(),
	}
}

func (m *model) syncPasteSessionPreview() {
	if m == nil || !m.pasteSession.active {
		return
	}
	preview := m.pasteSession.baseInput + normalizeNewlines(m.pasteSession.bufferedText)
	if m.input.Value() != preview {
		m.setInputValue(preview)
	}
	m.syncInputOverlays()
}

func shouldPreviewPasteSession(source string) bool {
	source = strings.TrimSpace(strings.ToLower(source))
	return source == "paste-key"
}

func (m *model) appendPasteSessionFragment(fragment, source string) int {
	if m == nil {
		return 0
	}
	if !m.pasteSession.active {
		m.beginPasteSession(source)
	}
	now := time.Now()
	m.pasteSession.lastEventAt = now
	if strings.TrimSpace(source) != "" {
		m.pasteSession.sourceKind = source
	}
	normalized := normalizeNewlines(fragment)
	if normalized != "" {
		m.pasteSession.bufferedText += normalized
		if strings.Contains(normalized, "\n") {
			m.pasteSession.sawMultiline = true
		}
		m.lastInputAt = now
		m.inputBurstSize = max(m.inputBurstSize, len([]rune(strings.TrimSpace(normalized))))
	}
	m.pasteSession.finalizeID++
	m.lastPasteAt = now
	m.armPasteSubmitGuard(now)
	return m.pasteSession.finalizeID
}

func (m *model) clearPasteSession() {
	if m == nil {
		return
	}
	m.pasteSession = pasteSessionState{}
}

func (m *model) hasActivePasteSession() bool {
	return m != nil && m.pasteSession.active
}

func (m *model) shouldFinalizePasteImmediately(source, fragment string) bool {
	if m == nil || !m.pasteSession.active {
		return false
	}
	if !isPasteLikeSource(source) {
		return false
	}
	normalized := normalizeNewlines(fragment)
	if strings.Count(normalized, "\n") < 2 {
		return false
	}
	return m.isLongPastedText(normalized) || strings.Count(normalized, "\n") >= longPasteLineThreshold
}

func (m *model) finalizePasteSession(id int) {
	if m == nil || !m.pasteSession.active {
		return
	}
	if id > 0 && id != m.pasteSession.finalizeID {
		return
	}

	base := m.pasteSession.baseInput
	content := normalizeNewlines(m.pasteSession.bufferedText)
	source := m.pasteSession.sourceKind
	m.clearPasteSession()

	candidate := strings.ReplaceAll(content, ctrlVMarkerRune, "")
	if candidate == "" {
		m.setInputValue(base)
		m.syncInputOverlays()
		return
	}

	now := time.Now()
	if source == "paste-key" && !m.shouldCompressPastedText(candidate, source) && !m.isLongPastedText(candidate) {
		m.setInputValue(base + candidate)
		if strings.Contains(candidate, "\n") || isPasteLikeSource(source) {
			m.lastPasteAt = now
			m.armPasteSubmitGuard(now)
		}
		m.lastInputAt = now
		m.inputBurstSize = max(1, len([]rune(candidate)))
		m.syncInputOverlays()
		return
	}
	if (source != "paste-key" && strings.Contains(candidate, "\n")) || m.shouldCompressPastedText(candidate, source) || (isPasteLikeSource(source) && m.isLongPastedText(candidate)) {
		marker, stored, err := m.compressPastedText(candidate)
		if err != nil {
			m.statusNote = err.Error()
			m.setInputValue(base)
			m.syncInputOverlays()
			return
		}
		m.setInputValue(base + marker)
		m.lastPasteAt = now
		m.armPasteSubmitGuard(now)
		m.statusNote = fmt.Sprintf("Long pasted text (%d lines) compressed as %s. Use [Paste #%s], [Paste #%s line10], or [Paste #%s line10~line20].",
			stored.Lines, marker, stored.ID, stored.ID, stored.ID)
		m.syncInputOverlays()
		return
	}

	m.setInputValue(base + candidate)
	if isPasteLikeSource(source) || strings.Contains(candidate, "\n") {
		m.lastPasteAt = now
		m.armPasteSubmitGuard(now)
	}
	m.lastInputAt = now
	m.inputBurstSize = max(1, len([]rune(candidate)))
	m.syncInputOverlays()
}

func (m *model) ingestPasteFragment(fragment, source string) tea.Cmd {
	if m == nil {
		return nil
	}
	if m.sessionsOpen || m.helpOpen || m.commandOpen || m.approval != nil {
		return nil
	}
	candidate := strings.ReplaceAll(normalizeNewlines(fragment), ctrlVMarkerRune, "")
	if candidate == "" {
		return nil
	}
	id := m.appendPasteSessionFragment(candidate, source)
	if m.shouldFinalizePasteImmediately(source, candidate) {
		m.finalizePasteSession(id)
		return nil
	}
	if shouldPreviewPasteSession(source) {
		m.syncPasteSessionPreview()
	}
	return schedulePasteFinalize(id)
}

func (m model) handlePastePayload(payload string) (tea.Model, tea.Cmd) {
	return m, m.ingestPasteFragment(trimTrailingPasteTerminators(payload), "paste-payload")
}

func (m *model) ensurePastedContentState() {
	if m == nil || m.pastedStateLoaded {
		return
	}
	if m.pastedContents == nil {
		m.pastedContents = make(map[string]pastedContent, maxStoredPastedContents)
	}
	if m.pastedOrder == nil {
		m.pastedOrder = make([]string, 0, maxStoredPastedContents)
	}
	if m.nextPasteID <= 0 {
		m.nextPasteID = 1
	}
	m.pastedStateLoaded = true

	if m.sess == nil || m.sess.Conversation.Meta == nil {
		return
	}
	raw, ok := m.sess.Conversation.Meta[pastedContentMetaKey]
	if !ok || raw == nil {
		return
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return
	}
	var persisted persistedPastedContents
	if err := json.Unmarshal(blob, &persisted); err != nil {
		return
	}
	if len(persisted.Contents) == 0 {
		return
	}

	m.pastedContents = make(map[string]pastedContent, len(persisted.Contents))
	for id, content := range persisted.Contents {
		id = strings.TrimSpace(id)
		if id == "" || strings.TrimSpace(content.Content) == "" {
			continue
		}
		content.ID = id
		m.pastedContents[id] = content
	}

	order := make([]string, 0, len(persisted.Order))
	seen := make(map[string]struct{}, len(persisted.Order))
	for _, id := range persisted.Order {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := m.pastedContents[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		order = append(order, id)
	}
	if len(order) < len(m.pastedContents) {
		missing := make([]string, 0, len(m.pastedContents)-len(order))
		for id := range m.pastedContents {
			if _, ok := seen[id]; ok {
				continue
			}
			missing = append(missing, id)
		}
		sort.Strings(missing)
		order = append(order, missing...)
	}
	m.pastedOrder = order

	m.nextPasteID = persisted.NextID
	if m.nextPasteID <= 0 {
		m.nextPasteID = 1
	}
	for _, id := range m.pastedOrder {
		if n, err := strconv.Atoi(id); err == nil && n >= m.nextPasteID {
			m.nextPasteID = n + 1
		}
	}
}

func (m *model) persistPastedContentState() error {
	if m == nil || m.sess == nil {
		return nil
	}
	if m.sess.Conversation.Meta == nil {
		m.sess.Conversation.Meta = make(map[string]any, 4)
	}
	payload := persistedPastedContents{
		Version:  1,
		NextID:   m.nextPasteID,
		Order:    append([]string(nil), m.pastedOrder...),
		Contents: m.pastedContents,
	}
	m.sess.Conversation.Meta[pastedContentMetaKey] = payload
	if m.store != nil {
		return m.store.Save(m.sess)
	}
	return nil
}

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
	if isLikelyPathInput(trimmed) || len(extractImagePathsFromChunk(input, m.workspace)) > 0 || len(extractInlineImagePathSpans(input)) > 0 {
		return false
	}
	pasteContext := m.hasPasteCompressionContext(source)
	if !pasteContext && !isSplitPasteContinuation(input, source, m.lastPasteAt) {
		return false
	}
	if m.isLongPastedText(input) {
		return true
	}
	if trimmed == "" {
		return false
	}
	rapidBurst := pasteContext &&
		!m.lastInputAt.IsZero() &&
		time.Since(m.lastInputAt) <= pasteBurstWindow &&
		m.inputBurstSize >= pasteBurstImmediateMinChars
	if rapidBurst && len(trimmed) >= pasteBurstImmediateMinChars && looksLikePastedFragment(trimmed) {
		return true
	}
	if len(trimmed) < pasteQuickCharThreshold {
		return false
	}
	if isPasteLikeSource(source) {
		return true
	}
	if !m.lastPasteAt.IsZero() && time.Since(m.lastPasteAt) <= 2*pasteSubmitGuard {
		return true
	}
	if isSplitPasteContinuation(input, source, m.lastPasteAt) {
		return true
	}
	if pasteContext && !m.lastInputAt.IsZero() && time.Since(m.lastInputAt) <= pasteBurstWindow && m.inputBurstSize >= pasteBurstCharThreshold {
		return true
	}
	return pasteContext && m.inputBurstSize >= pasteBurstCharThreshold
}

func (m *model) hasPasteCompressionContext(source string) bool {
	if m == nil {
		return false
	}
	source = strings.TrimSpace(strings.ToLower(source))
	if source == "paste-enter" || isPasteLikeSource(source) {
		return true
	}
	return !m.lastPasteAt.IsZero() && time.Since(m.lastPasteAt) <= pasteContinuationWindow
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

func (m *model) storePastedContent(content pastedContent) error {
	if m == nil {
		return nil
	}
	m.ensurePastedContentState()
	if strings.TrimSpace(content.ID) == "" {
		return fmt.Errorf("invalid pasted content id")
	}
	if len(content.Content) > maxSinglePastedCharLength {
		return fmt.Errorf("pasted content too large (%d chars), please attach a file instead", len(content.Content))
	}
	if _, exists := m.pastedContents[content.ID]; exists {
		m.pastedContents[content.ID] = content
		return m.persistPastedContentState()
	}
	m.pastedContents[content.ID] = content
	m.pastedOrder = append(m.pastedOrder, content.ID)
	if len(m.pastedOrder) > maxStoredPastedContents {
		drop := len(m.pastedOrder) - maxStoredPastedContents
		for i := 0; i < drop; i++ {
			oldest := m.pastedOrder[i]
			delete(m.pastedContents, oldest)
		}
		m.pastedOrder = append([]string(nil), m.pastedOrder[drop:]...)
	}
	return m.persistPastedContentState()
}

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

func countPastedDisplayLines(content string) int {
	normalized := normalizeNewlines(content)
	physicalLines := strings.Split(normalized, "\n")
	if len(physicalLines) > 1 {
		return len(physicalLines)
	}

	single := strings.TrimSpace(normalized)
	if single == "" {
		return 1
	}
	width := runewidth.StringWidth(single)
	if width <= 0 {
		return 1
	}
	estimated := (width + pasteSoftWrapWidth - 1) / pasteSoftWrapWidth
	if estimated < 1 {
		return 1
	}
	return estimated
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
				// Some terminals split one clipboard paste into multiple non-paste rune bursts.
				// Merge those bursts into the latest marker to avoid leaking trailing raw text.
				if safeTail && shouldMergeIntoLatestMarker(source, m.lastCompressedPasteAt) {
					merged, ok, err := m.mergeTailIntoLatestMarker(chain, rawTail)
					if err != nil {
						return after, err.Error()
					}
					if ok {
						return merged, ""
					}
				}
				if safeTail && (m.shouldCompressPastedText(tail, source) || m.isLongPastedText(tail)) {
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
					// Hide transient continuation tails so split paste chunks do not
					// visibly appear/disappear before marker coalescing completes.
					return strings.TrimSpace(chain), ""
				}
				// Keep tail as-is so split paste chunks can accumulate and then
				// compress into the next marker once they cross thresholds.
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
	if !m.shouldCompressPastedText(after, source) {
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
		return time.Since(lastCompressedAt) <= 300*time.Millisecond
	}
	return time.Since(lastCompressedAt) <= 500*time.Millisecond
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

type compressedMarkerLoc struct {
	id    string
	start int
	end   int
	ok    bool
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
		if isPasteLikeSource(source) || burst >= 8 {
			return true
		}
		if !lastPasteAt.IsZero() && time.Since(lastPasteAt) <= pasteContinuationWindow {
			return true
		}
		return false
	}
	if isPasteLikeSource(source) || burst >= 8 {
		return true
	}
	if !lastPasteAt.IsZero() && time.Since(lastPasteAt) <= pasteContinuationWindow {
		return true
	}
	return false
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
		// Allow explicit full deletion of compressed markers.
		return after, false
	}
	if isMarkerDeletionSource(source) && !strings.HasPrefix(afterTrimmed, beforeChain) {
		// Backspace/Delete on marker text removes one whole marker block.
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
			// Marker text is immutable for manual edits. Only allow chain text
			// changes when they likely come from paste chunk coalescing logic.
			if strings.TrimSpace(afterChain) == strings.TrimSpace(beforeChain) ||
				len(afterIDs) > len(beforeIDs) ||
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
		// Keep accidental paste spill while restoring immutable marker chain.
		return strings.TrimSpace(beforeChain) + afterTrimmed, true
	}
	return strings.TrimSpace(beforeChain), true
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

func (m *model) resolvePastedLineReference(input string) (string, error) {
	if m == nil || (!strings.Contains(input, "[Paste") && !strings.Contains(input, "[Pasted")) {
		return input, nil
	}
	m.ensurePastedContentState()
	if len(m.pastedOrder) == 0 {
		return input, nil
	}

	matches := pastedRefPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}

	var out strings.Builder
	last := 0
	for _, idx := range matches {
		start, end := idx[0], idx[1]
		out.WriteString(input[last:start])

		full := input[start:end]
		pasteID := submatchString(input, idx, pasteRefGroupID)
		lineCount := submatchString(input, idx, pasteRefGroupLineCount)
		startLineStr := submatchString(input, idx, pasteRefGroupLineStart)
		endLineStr := submatchString(input, idx, pasteRefGroupLineEnd)

		if strings.TrimSpace(startLineStr) == "" && strings.TrimSpace(lineCount) != "" {
			content, ok := m.findPastedContent(pasteID)
			if !ok {
				out.WriteString(full)
			} else {
				out.WriteString("```\n")
				out.WriteString(content.Content)
				out.WriteString("\n```")
			}
		} else if strings.TrimSpace(startLineStr) == "" {
			content, ok := m.findPastedContent(pasteID)
			if !ok {
				out.WriteString(full)
			} else {
				out.WriteString("```\n")
				out.WriteString(content.Content)
				out.WriteString("\n```")
			}
		} else {
			content, err := m.resolvePastedSelection(pasteID, startLineStr, endLineStr)
			if err != nil {
				out.WriteString(full)
			} else {
				out.WriteString("```\n")
				out.WriteString(content)
				out.WriteString("\n```")
			}
		}
		last = end
	}
	out.WriteString(input[last:])
	return out.String(), nil
}

func (m *model) resolvePastedSelection(pasteID, startLineStr, endLineStr string) (string, error) {
	content, ok := m.findPastedContent(pasteID)
	if !ok {
		return "", fmt.Errorf("pasted reference not found")
	}
	if strings.TrimSpace(startLineStr) == "" {
		return content.Content, nil
	}
	startLine, err := strconv.Atoi(startLineStr)
	if err != nil {
		return "", fmt.Errorf("invalid start line")
	}
	endLine := startLine
	if strings.TrimSpace(endLineStr) != "" {
		if v, err := strconv.Atoi(endLineStr); err == nil {
			endLine = v
		}
	}
	return extractLineRange(content.Content, startLine, endLine), nil
}

func (m *model) findPastedContent(pasteID string) (pastedContent, bool) {
	m.ensurePastedContentState()
	if strings.TrimSpace(pasteID) == "" {
		if len(m.pastedOrder) == 0 {
			return pastedContent{}, false
		}
		latestID := m.pastedOrder[len(m.pastedOrder)-1]
		content, ok := m.pastedContents[latestID]
		return content, ok
	}
	content, ok := m.pastedContents[strings.TrimSpace(pasteID)]
	return content, ok
}

func extractLineRange(content string, startLine, endLine int) string {
	normalized := normalizeNewlines(content)
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 {
		return ""
	}
	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 {
		endLine = startLine
	}
	if startLine > len(lines) {
		startLine = len(lines)
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if endLine < startLine {
		endLine = startLine
	}
	return strings.Join(lines[startLine-1:endLine], "\n")
}

func submatchString(input string, indexes []int, groupOffset int) string {
	if len(indexes) <= groupOffset+1 {
		return ""
	}
	start, end := indexes[groupOffset], indexes[groupOffset+1]
	if start < 0 || end < 0 || start > end || end > len(input) {
		return ""
	}
	return input[start:end]
}
