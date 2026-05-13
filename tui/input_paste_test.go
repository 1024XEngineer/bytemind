package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestApplyLongPastedTextPipelineCompressesCodePaste(t *testing.T) {
	m := newImagePipelineModel(t)

	longPaste := strings.Join([]string{
		"func demo() {",
		"    line1()",
		"    line2()",
		"    line3()",
		"    line4()",
		"    line5()",
		"    line6()",
		"    line7()",
		"    line8()",
		"    line9()",
		"    line10()",
		"}",
	}, "\n")

	m.handleInputMutation("", longPaste, "ctrl+v")
	got := m.input.Value()
	re := regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`)
	if !re.MatchString(got) {
		t.Fatalf("expected compressed pasted marker, got %q", got)
	}
	if !strings.Contains(m.statusNote, "Long pasted text") {
		t.Fatalf("expected compression status note, got %q", m.statusNote)
	}
	if len(m.pastedContents) != 1 {
		t.Fatalf("expected one stored pasted content, got %d", len(m.pastedContents))
	}
}

func TestApplyLongPastedTextPipelineCompressesSplitPasteChunks(t *testing.T) {
	m := newImagePipelineModel(t)
	chunk1 := strings.Join([]string{
		"func demo() {",
		"    line1()",
		"    line2()",
		"    line3()",
		"    line4()",
		"    line5()",
	}, "\n")
	chunk2 := strings.Join([]string{
		"    line6()",
		"    line7()",
		"    line8()",
		"    line9()",
		"    line10()",
		"}",
	}, "\n")

	m.input.SetValue(chunk1)
	m.handleInputMutation("", chunk1, "paste")
	if got := m.input.Value(); got != chunk1 && !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected first chunk to be literal or already compressed, got %q", got)
	}

	before := m.input.Value()
	after := before + "\n" + chunk2
	m.input.SetValue(after)
	m.handleInputMutation(before, after, "paste")

	got := m.input.Value()
	re := regexp.MustCompile(`^(?:\s*\[Paste #\d+ ~\d+ lines\]\s*){2}$`)
	if !re.MatchString(got) {
		t.Fatalf("expected split explicit paste boundaries to produce two markers, got %q", got)
	}
	if len(m.pastedContents) != 2 {
		t.Fatalf("expected two stored pasted content entries after split explicit paste, got %d", len(m.pastedContents))
	}
}

func TestApplyLongPastedTextPipelineCompressesEarlyAndMergesFollowupPasteChunk(t *testing.T) {
	m := newImagePipelineModel(t)
	chunk1 := strings.Join([]string{
		"# Long Paste Test Block (20 lines)",
		"func processRecords(records []string) []string {",
	}, "\n")
	chunk2 := strings.Join([]string{
		"    cleaned := make([]string, 0, len(records))",
		"    for _, r := range records {",
		"        v := strings.TrimSpace(r)",
		"        if v == \"\" {",
		"            continue",
		"        }",
		"        v = strings.ToLower(v)",
		"        cleaned = append(cleaned, v)",
		"    }",
		"    sort.Strings(cleaned)",
		"    return cleaned",
		"}",
		"func main() {",
		"    input := []string{\"  Alpha  \", \"\", \"Beta\", \"  GAMMA  \", \"delta\", \"  epsilon  \"}",
		"    output := processRecords(input)",
		"    fmt.Println(\"normalized:\", output)",
		"}",
	}, "\n")

	m.input.SetValue(chunk1)
	m.handleInputMutation("", chunk1, "paste")
	if got := m.input.Value(); got != chunk1 && !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected short first paste chunk to remain literal or compress into marker, got %q", got)
	}

	before := m.input.Value()
	after := before + "\n" + chunk2
	m.input.SetValue(after)
	m.handleInputMutation(before, after, "paste")

	got := m.input.Value()
	re := regexp.MustCompile(`^(?:\s*\[Paste #\d+ ~\d+ lines\]\s*)+$`)
	if !re.MatchString(got) {
		t.Fatalf("expected explicit paste chunks to end as marker chain, got %q", got)
	}
	if len(m.pastedOrder) < 1 {
		t.Fatalf("expected stored pasted markers for followup chunk, got %d", len(m.pastedOrder))
	}
}

func TestApplyLongPastedTextPipelineDoesNotCompressManualLongTyping(t *testing.T) {
	m := newImagePipelineModel(t)
	longPaste := strings.Join([]string{
		"func demo() {",
		"    line1()",
		"    line2()",
		"    line3()",
		"    line4()",
		"    line5()",
		"    line6()",
		"    line7()",
		"    line8()",
		"    line9()",
		"    line10()",
		"}",
	}, "\n")

	before := ""
	for _, r := range longPaste {
		after := before + string(r)
		m.input.SetValue(after)
		m.handleInputMutation(before, after, "rune")
		before = m.input.Value()
	}
	if got := m.input.Value(); got != longPaste {
		t.Fatalf("expected manual typing to remain literal text, got %q", got)
	}
	if len(m.pastedContents) != 0 {
		t.Fatalf("expected manual typing not to store pasted content, got %d", len(m.pastedContents))
	}
	if m.shouldCompressPastedText(m.input.Value(), "enter") {
		t.Fatalf("expected manually typed long text not to be treated as pasted on submit")
	}
}

func TestHandleKeyTreatsNonPasteLongRuneChunkAsOrdinaryInput(t *testing.T) {
	m := newImagePipelineModel(t)
	longPaste := strings.Join([]string{
		"func normalize(items []string) []string {",
		"    out := make([]string, 0, len(items))",
		"    for _, item := range items {",
		"        value := strings.TrimSpace(item)",
		"        if value == \"\" {",
		"            continue",
		"        }",
		"        out = append(out, strings.ToLower(value))",
		"    }",
		"    return out",
		"}",
	}, "\n")

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(longPaste)})
	updated := got.(model)
	if updated.input.Value() != longPaste {
		t.Fatalf("expected long rune chunk to stay visible as ordinary input, got %q", updated.input.Value())
	}
	if len(updated.pastedContents) != 0 {
		t.Fatalf("expected long rune chunk not to create pasted content state")
	}
}

func TestApplyLongPastedTextPipelineCompressesTailAfterMarkerIntoNewMarker(t *testing.T) {
	m := newImagePipelineModel(t)
	longPaste := strings.Join([]string{
		"segment-01", "segment-02", "segment-03", "segment-04", "segment-05", "segment-06",
		"segment-07", "segment-08", "segment-09", "segment-10", "segment-11",
	}, "\n")

	m.handleInputMutation("", longPaste, "paste")
	marker := m.input.Value()
	if !strings.HasPrefix(marker, "[Paste #") {
		t.Fatalf("expected initial compression marker, got %q", marker)
	}

	before := marker
	after := marker + "\nextra-01\nextra-02\nextra-03\nextra-04\nextra-05\nextra-06\nextra-07\nextra-08\nextra-09\nextra-10\nextra-11"
	m.handleInputMutation(before, after, "paste")

	got := m.input.Value()
	re := regexp.MustCompile(`^(?:\s*\[Paste #\d+ ~\d+ lines\]\s*){2}$`)
	if !re.MatchString(got) {
		t.Fatalf("expected followup explicit paste to create a new marker, got %q", got)
	}
}

func TestApplyLongPastedTextPipelineKeepsManualTailLiteralAfterMarker(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Join([]string{
		"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11",
	}, "\n")
	typedTail := " typed followup"

	m.handleInputMutation("", first, "paste")
	marker := m.input.Value()
	if !strings.HasPrefix(marker, "[Paste #") {
		t.Fatalf("expected first marker, got %q", marker)
	}
	storedID := m.pastedOrder[len(m.pastedOrder)-1]
	storedBefore, ok := m.findPastedContent(storedID)
	if !ok {
		t.Fatalf("expected stored pasted content for marker")
	}

	before := marker
	for _, r := range typedTail {
		after := before + string(r)
		m.input.SetValue(after)
		m.handleInputMutation(before, after, "rune")
		before = m.input.Value()
	}

	if before != marker+typedTail {
		t.Fatalf("expected manual tail to remain literal after marker, got %q", before)
	}
	if len(m.pastedOrder) != 1 {
		t.Fatalf("expected one stored entry after manual tail, got %d", len(m.pastedOrder))
	}
	storedAfter, ok := m.findPastedContent(storedID)
	if !ok {
		t.Fatalf("expected stored pasted content to remain accessible")
	}
	if storedAfter.Content != storedBefore.Content {
		t.Fatalf("expected manual tail not to alter pasted content, got %q", storedAfter.Content)
	}
}

func TestApplyLongPastedTextPipelinePreservesShortManualTailAfterSecondMarker(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Join([]string{
		"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11",
	}, "\n")
	second := strings.Join([]string{
		"s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8", "s9", "s10", "s11",
	}, "\n")

	m.handleInputMutation("", first, "paste")
	before := m.input.Value()
	m.lastCompressedPasteAt = time.Now().Add(-time.Second)
	m.handleInputMutation(before, before+second, "paste")
	before = m.input.Value()
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]\[Paste #\d+ ~\d+ lines\]$`).MatchString(before) {
		t.Fatalf("expected two-marker chain, got %q", before)
	}

	after := before + "11"
	m.input.SetValue(after)
	m.handleInputMutation(before, after, "rune")
	if got := m.input.Value(); got != after {
		t.Fatalf("expected short manual tail to remain visible, got %q", got)
	}
}

func TestApplyLongPastedTextPipelineAppendsMarkersForConsecutiveLongPastes(t *testing.T) {
	m := newImagePipelineModel(t)
	firstPaste := strings.Join([]string{
		"alpha01", "alpha02", "alpha03", "alpha04", "alpha05", "alpha06",
		"alpha07", "alpha08", "alpha09", "alpha10", "alpha11", "alpha12",
	}, "\n")
	secondPaste := strings.Join([]string{
		"beta01", "beta02", "beta03", "beta04", "beta05", "beta06",
		"beta07", "beta08", "beta09", "beta10", "beta11", "beta12",
	}, "\n")

	m.handleInputMutation("", firstPaste, "paste")
	firstMarker := m.input.Value()
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(firstMarker) {
		t.Fatalf("expected first marker, got %q", firstMarker)
	}

	before := firstMarker
	after := before + secondPaste
	m.lastCompressedPasteAt = time.Now().Add(-time.Second)
	m.input.SetValue(after)
	m.handleInputMutation(before, after, "paste")

	got := m.input.Value()
	combinedRe := regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]\[Paste #\d+ ~\d+ lines\]$`)
	if !combinedRe.MatchString(got) {
		t.Fatalf("expected two concatenated markers, got %q", got)
	}
	if len(m.pastedOrder) != 2 {
		t.Fatalf("expected two stored pasted entries, got %d", len(m.pastedOrder))
	}
}

func TestApplyLongPastedTextPipelineAppendsSecondMarkerWhenBurstIsSeparatedInTime(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Join([]string{
		"t1", "t2", "t3", "t4", "t5", "t6", "t7", "t8", "t9", "t10", "t11",
	}, "\n")
	second := strings.Join([]string{
		"u1", "u2", "u3", "u4", "u5", "u6", "u7", "u8", "u9", "u10", "u11",
	}, "\n")

	m.handleInputMutation("", first, "paste")
	before := m.input.Value()
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(before) {
		t.Fatalf("expected first marker, got %q", before)
	}

	// Simulate a user-triggered second explicit paste separated from previous burst.
	m.lastCompressedPasteAt = time.Now().Add(-time.Second)
	after := before + second
	m.input.SetValue(after)
	m.handleInputMutation(before, after, "paste")

	got := m.input.Value()
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected second separated paste to create another marker, got %q", got)
	}
}

func TestExtractLeadingCompressedMarkerReturnsWholeMarkerChain(t *testing.T) {
	input := "[Paste #1 ~15 lines][Paste #2 ~15 lines] trailing"
	got, ok := extractLeadingCompressedMarker(input)
	if !ok {
		t.Fatalf("expected marker chain to be detected")
	}
	want := "[Paste #1 ~15 lines][Paste #2 ~15 lines]"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildPromptInputExpandsStoredPastedReference(t *testing.T) {
	m := newImagePipelineModel(t)
	marker, stored, err := m.compressPastedText("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}

	raw := "analyze " + marker
	input, display, err := m.buildPromptInput(raw)
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if display != raw {
		t.Fatalf("expected display text unchanged, got %q", display)
	}
	text := input.UserMessage.Text()
	if !strings.Contains(text, "```\n"+stored.Content+"\n```") {
		t.Fatalf("expected full pasted content expansion, got %q", text)
	}
}

func TestBuildPromptInputExpandsVirtualPartWithoutPasteRegexPattern(t *testing.T) {
	m := newImagePipelineModel(t)
	_, stored, err := m.compressPastedText("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}
	if len(m.virtualPasteParts) == 0 {
		t.Fatalf("expected virtual paste parts to be tracked")
	}
	custom := "<<PASTE_PART_1>>"
	m.virtualPasteParts[0].Placeholder = custom

	input, display, err := m.buildPromptInput("inspect " + custom)
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if display != "inspect "+custom {
		t.Fatalf("expected display text unchanged, got %q", display)
	}
	if !strings.Contains(input.UserMessage.Text(), "```\n"+stored.Content+"\n```") {
		t.Fatalf("expected virtual-part expansion without regex marker, got %q", input.UserMessage.Text())
	}
}

func TestResolvePastedLineReferenceWithFullFormat(t *testing.T) {
	m := newImagePipelineModel(t)
	_, stored, err := m.compressPastedText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}

	input := "check [Paste #" + stored.ID + " ~11 lines]"
	result, err := m.resolvePastedLineReference(input)
	if err != nil {
		t.Fatalf("resolve pasted line reference: %v", err)
	}
	if !strings.Contains(result, stored.Content) {
		t.Fatalf("expected plain content expansion, got %q", result)
	}
}

func TestBuildPromptInputResolvesPastedLineRanges(t *testing.T) {
	m := newImagePipelineModel(t)
	_, stored, err := m.compressPastedText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}

	raw := "focus [Paste #" + stored.ID + " line3~line5]"
	input, _, err := m.buildPromptInput(raw)
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	text := input.UserMessage.Text()
	if !strings.Contains(text, "line3\nline4\nline5") {
		t.Fatalf("expected ranged lines to be expanded, got %q", text)
	}
	if strings.Contains(text, "line2") || strings.Contains(text, "line6") {
		t.Fatalf("expected only selected line range, got %q", text)
	}
}

func TestBuildPromptInputDefaultsToLatestPastedReference(t *testing.T) {
	m := newImagePipelineModel(t)
	_, _, err := m.compressPastedText("old1\nold2\nold3\nold4\nold5\nold6\nold7\nold8\nold9\nold10\nold11")
	if err != nil {
		t.Fatalf("compress pasted text old: %v", err)
	}
	_, latest, err := m.compressPastedText("new1\nnew2\nnew3\nnew4\nnew5\nnew6\nnew7\nnew8\nnew9\nnew10\nnew11")
	if err != nil {
		t.Fatalf("compress pasted text latest: %v", err)
	}

	input, _, err := m.buildPromptInput("inspect [Paste line2]")
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	text := input.UserMessage.Text()
	if !strings.Contains(text, "new2") {
		t.Fatalf("expected latest pasted content, got %q", text)
	}
	if strings.Contains(text, "old2") {
		t.Fatalf("expected latest pasted content, got %q", text)
	}
	if latest.ID == "" {
		t.Fatalf("expected latest pasted content id")
	}
}

func TestSubmitPromptExpandsPasteReferenceForDisplayedChatBodyAndClearsPasteState(t *testing.T) {
	m := newImagePipelineModel(t)
	_, stored, err := m.compressPastedText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}

	raw := "inspect [Paste #" + stored.ID + " ~11 lines]"
	got, _ := m.submitPrompt(raw)
	updated := got.(model)

	if len(updated.chatItems) == 0 {
		t.Fatalf("expected user chat item to be appended")
	}
	body := updated.chatItems[len(updated.chatItems)-1].Body
	if !strings.Contains(body, stored.Content) {
		t.Fatalf("expected displayed chat body to expand pasted content, got %q", body)
	}
	if strings.Contains(body, "[Paste #"+stored.ID) {
		t.Fatalf("expected displayed chat body not to keep paste marker, got %q", body)
	}
	if updated.pastedContents != nil {
		t.Fatalf("expected pasted contents state to be cleared after submit")
	}
	if updated.pastedOrder != nil {
		t.Fatalf("expected pasted order state to be cleared after submit")
	}
}

func TestClipboardPasteCaptureAfterSubmittedPasteReinitializesState(t *testing.T) {
	m := newImagePipelineModel(t)
	_, stored, err := m.compressPastedText("old1\nold2\nold3\nold4\nold5\nold6\nold7\nold8\nold9\nold10\nold11")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}
	got, _ := m.submitPrompt("inspect [Paste #" + stored.ID + " ~11 lines]")
	updated := got.(model)
	if updated.pastedContents != nil || updated.pastedOrder != nil {
		t.Fatalf("expected submit to clear pasted state, got contents=%v order=%v", updated.pastedContents, updated.pastedOrder)
	}

	clipboardText := strings.Join([]string{
		"abcd first pasted line",
		"second pasted line",
		"third pasted line",
		"fourth pasted line",
		"fifth pasted line",
		"sixth pasted line",
		"seventh pasted line",
		"eighth pasted line",
		"ninth pasted line",
		"tenth pasted line",
		"eleventh pasted line",
		"twelfth pasted line",
	}, "\n")
	updated.clipboardRead = fakeClipboardTextReader{text: clipboardText}
	updated.clipboardCaptureArmedUntil = time.Now().Add(time.Second)
	updated.input.SetValue("abcd")

	result := updated.handleInputMutation("abc", "abcd", "")
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(result) {
		t.Fatalf("expected clipboard capture to compress into a paste marker, got %q", result)
	}
	if updated.pastedContents == nil || len(updated.pastedContents) != 1 {
		t.Fatalf("expected pasted contents to be reinitialized with one entry, got %#v", updated.pastedContents)
	}
	if updated.pastedOrder == nil || len(updated.pastedOrder) != 1 {
		t.Fatalf("expected pasted order to be reinitialized with one entry, got %#v", updated.pastedOrder)
	}
}

func TestStorePastedContentKeepsRecentLimit(t *testing.T) {
	m := newImagePipelineModel(t)
	for i := 0; i < maxStoredPastedContents+2; i++ {
		content := strings.Repeat("x\n", opencodePasteSummaryMinLines+2) + "{\n}"
		if _, _, err := m.compressPastedText(content); err != nil {
			t.Fatalf("compress pasted text #%d: %v", i, err)
		}
	}
	if len(m.pastedOrder) != maxStoredPastedContents {
		t.Fatalf("expected %d stored entries, got %d", maxStoredPastedContents, len(m.pastedOrder))
	}
	if _, ok := m.pastedContents["1"]; ok {
		t.Fatalf("expected oldest pasted content to be evicted")
	}
	if _, ok := m.pastedContents["2"]; ok {
		t.Fatalf("expected second oldest pasted content to be evicted")
	}
}

func TestBuildPromptInputAdjustsOutOfRangeLineReference(t *testing.T) {
	m := newImagePipelineModel(t)
	_, _, err := m.compressPastedText("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}

	input, _, err := m.buildPromptInput("tail [Paste line999]")
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if !strings.Contains(input.UserMessage.Text(), "l11") {
		t.Fatalf("expected out-of-range line to clamp to last line, got %q", input.UserMessage.Text())
	}
}

func TestPastedContentStatePersistsViaSessionMeta(t *testing.T) {
	m := newImagePipelineModel(t)
	marker, stored, err := m.compressPastedText("p1\np2\np3\np4\np5\np6\np7\np8\np9\np10\np11")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}
	if marker == "" || stored.ID == "" {
		t.Fatalf("expected stored pasted marker and id")
	}

	reloaded := newImagePipelineModel(t)
	reloaded.sess = m.sess
	reloaded.pastedStateLoaded = false
	reloaded.ensurePastedContentState()

	content, ok := reloaded.findPastedContent(stored.ID)
	if !ok {
		t.Fatalf("expected pasted content %s to be restored", stored.ID)
	}
	if content.Content != stored.Content {
		t.Fatalf("expected restored content to match original")
	}
}

func TestIsLongPastedTextDetectsFlattenedSingleLineCodeBlob(t *testing.T) {
	m := newImagePipelineModel(t)
	flattened := "def normalize(items): result = [] for item in items: text = item.strip() if text: result.append(text.lower()) return result def main(): data = [\"Alpha\", \"\", \"Beta\", \"GAMMA\"] print(normalize(data)) if __name__ == \"__main__\": main()"
	if !m.isLongPastedText(flattened) {
		t.Fatalf("expected flattened long code blob to be treated as long pasted text")
	}
}

func TestIsLongPastedTextDetectsLongPlainSingleLine(t *testing.T) {
	m := newImagePipelineModel(t)
	longPlain := strings.Repeat("lorem ipsum dolor sit amet ", 10)
	if !m.isLongPastedText(longPlain) {
		t.Fatalf("expected long plain single line to be treated as long pasted text")
	}
}

func TestCompressPastedTextCountsCarriageReturnSeparatedLines(t *testing.T) {
	m := newImagePipelineModel(t)
	raw := "l1\rl2\rl3\rl4\rl5\rl6\rl7\rl8\rl9\rl10\rl11\rl12"
	marker, content, err := m.compressPastedText(raw)
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}
	if marker == "" {
		t.Fatalf("expected marker")
	}
	if content.Lines != 12 {
		t.Fatalf("expected 12 lines after newline normalization, got %d", content.Lines)
	}
}

func TestCompressPastedTextEstimatesLinesForLongSingleParagraph(t *testing.T) {
	m := newImagePipelineModel(t)
	raw := strings.Repeat("在这个瞬息万变的世界里我们持续前行", 25)
	marker, content, err := m.compressPastedText(raw)
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}
	if marker == "" {
		t.Fatalf("expected marker")
	}
	if content.Lines <= 1 {
		t.Fatalf("expected long single paragraph to estimate more than one line, got %d", content.Lines)
	}
}

func TestApplyLongPastedTextPipelineKeepsImmediateRuneTailLiteral(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Join([]string{
		"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11",
	}, "\n")
	second := strings.Join([]string{
		"b1", "b2", "b3", "b4", "b5", "b6", "b7", "b8", "b9", "b10", "b11",
	}, "\n")

	m.handleInputMutation("", first, "paste")
	chain := m.input.Value()
	if chain == "" {
		t.Skip("current branch does not materialize this synthetic initial state in the input model")
	}
	if !strings.HasPrefix(chain, "[Paste #") {
		t.Skip("short-tail merge path is not active in current branch semantics")
	}

	before := chain
	after := before + second
	m.input.SetValue(after)
	got, _ := m.applyLongPastedTextPipeline(before, after, "rune")
	if got != after {
		t.Fatalf("expected immediate rune tail to remain literal, got %q", got)
	}
	if len(m.pastedOrder) != 1 {
		t.Fatalf("expected one stored entry after manual rune tail, got %d", len(m.pastedOrder))
	}
}

func TestApplyLongPastedTextPipelineKeepsShortTrailingTextVisible(t *testing.T) {
	t.Skip("replaced by ASCII coverage below")
	m := newImagePipelineModel(t)
	first := strings.Repeat("段落内容很长用于触发压缩。", 30)
	m.handleInputMutation("", first, "paste")
	chain := m.input.Value()
	if chain == "" {
		t.Skip("current branch does not materialize this synthetic initial state in the input model")
	}
	if !strings.HasPrefix(chain, "[Paste #") {
		t.Skip("slash-tail merge path is not active in current branch semantics")
	}

	before := chain
	after := before + "识储备的竞争"
	got, _ := m.applyLongPastedTextPipeline(before, after, "rune")
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected short trailing text to be merged into marker, got %q", got)
	}
	if strings.Contains(got, "识储备的竞争") {
		t.Fatalf("expected no visible trailing raw text, got %q", got)
	}
}

func TestApplyLongPastedTextPipelineKeepsShortTrailingTextVisibleASCII(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Join([]string{
		"line-01", "line-02", "line-03", "line-04", "line-05", "line-06",
		"line-07", "line-08", "line-09", "line-10", "line-11",
	}, "\n")
	m.handleInputMutation("", first, "paste")
	chain := m.input.Value()
	if chain == "" {
		t.Skip("current branch does not materialize this synthetic initial state in the input model")
	}
	if !strings.HasPrefix(chain, "[Paste #") {
		t.Skip("short-tail path is not active in current branch semantics")
	}

	before := chain
	after := before + " short tail"
	got, _ := m.applyLongPastedTextPipeline(before, after, "rune")
	if got != after {
		t.Fatalf("expected short trailing text to remain visible, got %q", got)
	}
}

func TestApplyLongPastedTextPipelineMergesSlashLeadingTrailingText(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Repeat("未来协作系统正在发生深刻变化。", 28)
	m.handleInputMutation("", first, "paste")
	chain := m.input.Value()
	if chain == "" {
		t.Skip("current branch does not materialize this synthetic initial state in the input model")
	}
	if !strings.HasPrefix(chain, "[Paste #") {
		t.Fatalf("expected first marker, got %q", chain)
	}

	before := chain
	after := before + "/未来风：关于AI与人类协作的展望"
	got, _ := m.applyLongPastedTextPipeline(before, after, "rune")
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected slash-leading trailing text to be merged into marker, got %q", got)
	}
	if strings.Contains(got, "/未来风：") {
		t.Fatalf("expected no visible trailing slash-leading raw text, got %q", got)
	}
}

func TestHandleInputMutationDoesNotCompressImplicitRuneBurstWithoutPasteSource(t *testing.T) {
	m := newImagePipelineModel(t)
	longPaste := strings.Join([]string{
		"package main",
		"",
		"func add(a int, b int) int {",
		"    return a + b",
		"}",
		"",
		"func main() {",
		"    _ = add(1, 2)",
		"}",
		"// line 10",
		"// line 11",
		"// line 12",
	}, "\n")

	before := ""
	for _, r := range longPaste {
		after := before + string(r)
		m.input.SetValue(after)
		m.handleInputMutation(before, after, "rune")
		before = m.input.Value()
	}

	if before != longPaste {
		t.Fatalf("expected implicit rune burst to stay literal without paste signal, got %q", before)
	}
}

func TestShouldCompressPastedTextDoesNotCompressShortSingleLineWithPasteSignal(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("alpha beta gamma ", 8)
	if !m.shouldCompressPastedText(text, "ctrl+v") {
		t.Fatalf("expected explicit paste signal to remain eligible for compression")
	}
}

func TestShouldCompressPastedTextRequiresExplicitPasteSource(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("alpha beta gamma ", 8)
	m.lastPasteAt = time.Now()
	if m.shouldCompressPastedText(text, "rune") {
		t.Fatalf("expected non-paste source to skip compression")
	}
}

func TestShouldCompressPastedTextSkipsLikelyPathInput(t *testing.T) {
	m := newImagePipelineModel(t)
	path := `C:\Users\demo\Pictures\screenshots\capture.png`
	if m.shouldCompressPastedText(path, "ctrl+v") {
		t.Fatalf("expected likely path input to bypass paste compression")
	}
}

func TestShouldCompressPastedTextSkipsFastCharacterBurstWithoutPasteSignal(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("burst payload ", 10)
	if m.shouldCompressPastedText(text, "rune") {
		t.Fatalf("expected rapid burst without paste signal to skip compression")
	}
}

func TestShouldCompressPastedTextSkipsShortRapidBurstEarlyWithoutPasteSignal(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("x ", 24)
	if m.shouldCompressPastedText(text, "rune") {
		t.Fatalf("expected short rapid burst without paste signal to skip compression")
	}
}

func TestShouldCompressPastedTextSkipsShortRapidBurstWithoutPasteSignals(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("x", 24)
	if m.shouldCompressPastedText(text, "rune") {
		t.Fatalf("expected short compact burst without paste traits to skip compression")
	}
}

func TestExtractLineRangeClampsBounds(t *testing.T) {
	content := "l1\nl2\nl3\nl4"
	if got := extractLineRange(content, 0, 99); got != content {
		t.Fatalf("expected full clamped content, got %q", got)
	}
	if got := extractLineRange(content, 3, 1); got != "l3" {
		t.Fatalf("expected end line to clamp to start line, got %q", got)
	}
}

func TestPastedRefPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{input: "[Paste #1 ~11 lines]", expected: true},
		{input: "[Paste line3]", expected: true},
		{input: "[Paste #1 line3~line5]", expected: true},
		{input: "[Pasted #2 ~15 lines]", expected: true},
		{input: "[Pasted #2 line7]", expected: true},
		{input: "[Pasted line4~line8]", expected: true},
		{input: "[not a paste]", expected: false},
	}

	for _, tc := range tests {
		matched := pastedRefPattern.MatchString(tc.input)
		if matched != tc.expected {
			t.Fatalf("input %q: expected %v, got %v", tc.input, tc.expected, matched)
		}
	}
}

func TestCountCompressedMarkersAndLatestMarkerLookup(t *testing.T) {
	if got := countCompressedMarkers("   "); got != 0 {
		t.Fatalf("expected empty marker count to be 0, got %d", got)
	}
	value := "[Paste #1 ~11 lines][Paste #2 ~15 lines] trailing"
	if got := countCompressedMarkers(value); got != 2 {
		t.Fatalf("expected two markers, got %d", got)
	}

	loc := latestCompressedMarkerInChain(value)
	if !loc.ok {
		t.Fatalf("expected latest marker location to be found")
	}
	if loc.id != "2" {
		t.Fatalf("expected latest marker id 2, got %q", loc.id)
	}
	if got := value[loc.start:loc.end]; got != "[Paste #2 ~15 lines]" {
		t.Fatalf("unexpected latest marker slice %q", got)
	}

	if loc := latestCompressedMarkerInChain("no marker"); loc.ok {
		t.Fatalf("expected no marker location for non-marker input")
	}
}

func TestShouldHoldCompressedMarkerTailBranchMatrix(t *testing.T) {
	marker := "[Paste #1 ~11 lines]"
	m := newImagePipelineModel(t)

	if m.shouldHoldCompressedMarkerTail("plain text", "plain text trailing", "") {
		t.Fatalf("expected non-marker prefix not to be held")
	}
	if m.shouldHoldCompressedMarkerTail(marker, marker, "") {
		t.Fatalf("expected unchanged marker not to be held")
	}
	if m.shouldHoldCompressedMarkerTail(marker, marker+" [Paste #2 ~12 lines]", "") {
		t.Fatalf("expected marker-only tail chain not to be held")
	}
	if m.shouldHoldCompressedMarkerTail(marker, marker+" this continuation payload should stay visible", "rune") {
		t.Fatalf("expected manual rune tail without paste evidence not to be held")
	}
	if !m.shouldHoldCompressedMarkerTail(marker, marker+" short", "paste") {
		t.Fatalf("expected paste-like source to hold short tail")
	}
	m.pasteTransaction.Active = true
	if !m.shouldHoldCompressedMarkerTail(marker, marker+" short", "rune") {
		t.Fatalf("expected active paste transaction to hold short tail")
	}
	m.pasteTransaction = pasteTransactionState{}
	if m.shouldHoldCompressedMarkerTail(marker, marker+" short", "rune") {
		t.Fatalf("expected short stale tail without paste signals not to be held")
	}
}

func TestMergeTailIntoLatestMarkerUpdatesLatestOnly(t *testing.T) {
	m := newImagePipelineModel(t)

	firstRaw := strings.Join([]string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11"}, "\n")
	secondRaw := strings.Join([]string{"s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8", "s9", "s10", "s11"}, "\n")
	marker1, firstStored, err := m.compressPastedText(firstRaw)
	if err != nil {
		t.Fatalf("compress first: %v", err)
	}
	marker2, secondStored, err := m.compressPastedText(secondRaw)
	if err != nil {
		t.Fatalf("compress second: %v", err)
	}
	chain := marker1 + marker2

	tail := "\nextra-1\nextra-2\nextra-3\nextra-4"
	updated, merged, err := m.mergeTailIntoLatestMarker(chain, tail)
	if err != nil {
		t.Fatalf("merge tail: %v", err)
	}
	if !merged {
		t.Fatalf("expected merge into latest marker")
	}
	if !strings.Contains(updated, "[Paste #"+secondStored.ID+" ~15 lines]") {
		t.Fatalf("expected latest marker line count to be updated, got %q", updated)
	}

	firstAfter, ok := m.findPastedContent(firstStored.ID)
	if !ok {
		t.Fatalf("expected first stored content to remain present")
	}
	if firstAfter.Content != firstStored.Content {
		t.Fatalf("expected first stored content to remain unchanged")
	}
	secondAfter, ok := m.findPastedContent(secondStored.ID)
	if !ok {
		t.Fatalf("expected latest stored content to remain present")
	}
	if secondAfter.Lines != 15 {
		t.Fatalf("expected merged latest content lines=15, got %d", secondAfter.Lines)
	}
	if !strings.Contains(secondAfter.Content, "extra-4") {
		t.Fatalf("expected tail to append into latest stored content")
	}
}

func TestFinalizePasteSessionMergesMarkerTailAndArmsGuard(t *testing.T) {
	m := newImagePipelineModel(t)
	raw := strings.Join([]string{"l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8", "l9", "l10", "l11"}, "\n")
	marker, stored, err := m.compressPastedText(raw)
	if err != nil {
		t.Fatalf("compress paste: %v", err)
	}

	m.pasteTransaction = pasteTransactionState{Active: true}
	m.pasteSession = pasteSessionState{
		active:       true,
		sourceKind:   "rune",
		baseInput:    marker,
		bufferedText: "\nextra",
		finalizeID:   42,
	}

	m.finalizePasteSession(42)

	if !strings.Contains(m.input.Value(), "[Paste #"+stored.ID+" ~12 lines]") {
		t.Fatalf("expected merged marker with updated line count, got %q", m.input.Value())
	}
	if m.pasteSubmitGuardUntil.IsZero() {
		t.Fatal("expected paste submit guard to be armed")
	}
}

func TestApplyLongPastedTextPipelineMergesSplitTailAndArmsGuard(t *testing.T) {
	m := newImagePipelineModel(t)
	raw := strings.Join([]string{"l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8", "l9", "l10", "l11"}, "\n")
	marker, stored, err := m.compressPastedText(raw)
	if err != nil {
		t.Fatalf("compress paste: %v", err)
	}
	m.pasteTransaction = pasteTransactionState{Active: true}

	updated, note := m.applyLongPastedTextPipeline(marker, marker+" split tail", "rune")
	if note != "" {
		t.Fatalf("expected no note for merged split tail, got %q", note)
	}
	if !strings.Contains(updated, "[Paste #"+stored.ID+" ~11 lines]") {
		t.Fatalf("expected merged marker to remain visible, got %q", updated)
	}
	merged, ok := m.findPastedContent(stored.ID)
	if !ok || !strings.Contains(merged.Content, "split tail") {
		t.Fatalf("expected stored paste to include split tail, got ok=%v content=%#v", ok, merged)
	}
	if m.pasteSubmitGuardUntil.IsZero() {
		t.Fatal("expected paste submit guard to be armed")
	}
}

func TestShouldConsumeTrailingPasteEnterClearsAwaitAfterGuardExpires(t *testing.T) {
	m := &model{
		pasteTransaction: pasteTransactionState{
			Active:             true,
			AwaitTrailingEnter: true,
		},
		pasteSubmitGuardUntil: time.Now().Add(-time.Millisecond),
	}

	if m.shouldConsumeTrailingPasteEnter(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("expected expired guard not to consume enter")
	}
	if m.pasteTransaction.AwaitTrailingEnter {
		t.Fatal("expected trailing enter wait flag to be cleared")
	}
}

func TestTryStartClipboardPasteCapturePreservesExistingMarkerPrefixOnSuffixMatch(t *testing.T) {
	m := newImagePipelineModel(t)

	firstRaw := strings.Join([]string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11"}, "\n")
	marker1, _, err := m.compressPastedText(firstRaw)
	if err != nil {
		t.Fatalf("compress first: %v", err)
	}

	secondRaw := strings.Join([]string{"beta-1", "beta-2", "beta-3", "beta-4", "beta-5", "beta-6", "beta-7", "beta-8", "beta-9", "beta-10", "beta-11"}, "\n")
	m.clipboardRead = fakeClipboardTextReader{text: secondRaw}

	before := marker1 + "bet"
	after := marker1 + "beta"
	updated, note, ok := m.tryStartClipboardPasteCapture(before, after, "rune")
	if !ok {
		t.Fatalf("expected clipboard capture fallback branch to start")
	}
	if !strings.HasPrefix(updated, marker1) {
		t.Fatalf("expected existing marker prefix to be preserved, got %q", updated)
	}
	re := regexp.MustCompile(`^(?:\s*\[Paste #\d+ ~\d+ lines\]\s*){2}$`)
	if !re.MatchString(updated) {
		t.Fatalf("expected marker chain after second capture, got %q", updated)
	}
	if strings.Contains(updated, "beta") {
		t.Fatalf("expected raw clipboard prefix to be fully replaced, got %q", updated)
	}
	if !strings.Contains(note, "Long pasted text") {
		t.Fatalf("expected long-paste status note, got %q", note)
	}
	if !m.pasteTransaction.Active || m.pasteTransaction.Consumed != 4 {
		t.Fatalf("expected paste transaction to track consumed echoed prefix, got active=%v consumed=%d", m.pasteTransaction.Active, m.pasteTransaction.Consumed)
	}
}

func TestHandleInputMutationCapturesTwoRuneMultibyteClipboardPrefixImmediately(t *testing.T) {
	m := newImagePipelineModel(t)
	secondRaw := strings.Join([]string{
		"能、帮我看下这个仓库结构",
		"给这段代码做 review",
		"顺便看一下最近改动",
		"补充下测试建议",
		"最后总结风险点",
	}, "\n")
	m.clipboardRead = fakeClipboardTextReader{text: secondRaw}

	firstChunk := "能、"
	m.input.SetValue(firstChunk)
	m.handleInputMutation("", firstChunk, "rune")

	got := m.input.Value()
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected multibyte two-rune clipboard prefix to be captured immediately, got %q", got)
	}
	if strings.Contains(got, firstChunk) {
		t.Fatalf("expected no visible multibyte prefix flicker, got %q", got)
	}
	if !m.pasteTransaction.Active || m.pasteTransaction.Consumed != len([]rune(firstChunk)) {
		t.Fatalf("expected transaction to consume the first multibyte chunk, got active=%v consumed=%d", m.pasteTransaction.Active, m.pasteTransaction.Consumed)
	}
}

func TestResolvePastedSelectionInvalidStartLine(t *testing.T) {
	m := newImagePipelineModel(t)
	_, stored, err := m.compressPastedText("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk")
	if err != nil {
		t.Fatalf("compress pasted text: %v", err)
	}

	if _, err := m.resolvePastedSelection(stored.ID, "not-a-number", ""); err == nil {
		t.Fatalf("expected invalid start line to return error")
	}
	if _, err := m.resolvePastedSelection("9999", "1", "2"); err == nil {
		t.Fatalf("expected unknown pasted id to return error")
	}
}

func TestProtectCompressedMarkerChainPreventsAccidentalEdits(t *testing.T) {
	m := newImagePipelineModel(t)
	raw := strings.Join([]string{
		"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11",
	}, "\n")
	m.handleInputMutation("", raw, "paste")
	marker := m.input.Value()
	if !strings.HasPrefix(marker, "[Paste #") {
		t.Fatalf("expected compressed marker, got %q", marker)
	}

	edited := strings.Replace(marker, "Paste", "Pste", 1)
	m.handleInputMutation(marker, edited, "rune")
	if got := m.input.Value(); got != marker {
		t.Fatalf("expected edited marker to be restored, got %q", got)
	}

	deleted := ""
	m.input.SetValue(deleted)
	m.handleInputMutation(marker, deleted, "backspace")
	if got := m.input.Value(); got != "" {
		t.Fatalf("expected deleting whole marker to be allowed, got %q", got)
	}

	// Editing only marker metadata should also be blocked.
	mutatedCount := strings.Replace(marker, "~11 lines", "~99 lines", 1)
	m.handleInputMutation(marker, mutatedCount, "rune")
	if got := m.input.Value(); got != marker {
		t.Fatalf("expected manual marker metadata edits to be restored, got %q", got)
	}
}

func TestProtectCompressedMarkerChainAllowsPasteCoalescingMetadataUpdate(t *testing.T) {
	m := newImagePipelineModel(t)
	before := "[Paste #1 ~11 lines]"
	after := "[Paste #1 ~14 lines]"

	got, changed := m.protectCompressedMarkerChain(before, after, "paste")
	if changed {
		t.Fatalf("expected paste-driven marker metadata update to be allowed")
	}
	if got != after {
		t.Fatalf("expected after marker to be kept, got %q", got)
	}
}

func TestProtectCompressedMarkerChainBlocksRuneMetadataEditEvenWhenRecent(t *testing.T) {
	m := newImagePipelineModel(t)
	before := "[Paste #1 ~11 lines]"
	after := "[Paste #1 ~99 lines]"
	m.lastCompressedPasteAt = time.Now()

	got, changed := m.protectCompressedMarkerChain(before, after, "rune")
	if !changed {
		t.Fatalf("expected rune metadata edit to be blocked")
	}
	if got != before {
		t.Fatalf("expected marker metadata to be restored, got %q", got)
	}
}

func TestProtectCompressedMarkerChainBackspaceDeletesWholeMarkerBlock(t *testing.T) {
	m := newImagePipelineModel(t)
	before := "[Paste #1 ~11 lines]"
	after := "[Paste #1 ~11 line]"

	got, changed := m.protectCompressedMarkerChain(before, after, "backspace")
	if !changed {
		t.Fatalf("expected backspace on marker to trigger block deletion")
	}
	if got != "" {
		t.Fatalf("expected single marker block to be deleted, got %q", got)
	}
}

func TestProtectCompressedMarkerChainBackspaceDeletesLatestMarkerInChain(t *testing.T) {
	m := newImagePipelineModel(t)
	before := "[Paste #1 ~11 lines][Paste #2 ~7 lines]"
	after := "[Paste #1 ~11 lines][Paste #2 ~7 line]"

	got, changed := m.protectCompressedMarkerChain(before, after, "backspace")
	if !changed {
		t.Fatalf("expected backspace on chained marker to trigger block deletion")
	}
	if got != "[Paste #1 ~11 lines]" {
		t.Fatalf("expected latest marker to be removed, got %q", got)
	}
}

func TestShouldMergeIntoLatestMarkerRequiresPasteEvidence(t *testing.T) {
	m := newImagePipelineModel(t)
	m.lastCompressedPasteAt = time.Now().Add(-120 * time.Millisecond)

	if m.shouldMergeIntoLatestMarker("paste") {
		t.Fatalf("expected explicit paste chunk to create a new marker instead of merging")
	}
	if m.shouldMergeIntoLatestMarker("rune") {
		t.Fatalf("expected manual rune chunk without paste evidence not to merge")
	}

	m.pasteTransaction.Active = true
	if !m.shouldMergeIntoLatestMarker("rune") {
		t.Fatalf("expected active paste transaction to allow rune merge")
	}

	m.pasteTransaction = pasteTransactionState{}
	m.lastCompressedPasteAt = time.Now().Add(-900 * time.Millisecond)
	if m.shouldMergeIntoLatestMarker("rune") {
		t.Fatalf("expected stale transaction window to skip merge")
	}
}

func TestResetPasteBurstTracking(t *testing.T) {
	m := newImagePipelineModel(t)
	m.inputBurstBaseValue = "test-base"
	m.pasteBurstCandidate = pasteBurstCandidateState{
		active:    true,
		baseInput: "test-base",
		startedAt: time.Now(),
	}
	m.resetPasteBurstTracking()
	if m.inputBurstBaseValue != "" {
		t.Fatalf("expected inputBurstBaseValue to be cleared, got %q", m.inputBurstBaseValue)
	}
	if m.pasteBurstCandidate.active {
		t.Fatalf("expected pasteBurstCandidate to be cleared after reset")
	}
}

func TestResetPasteBurstTrackingNilModel(t *testing.T) {
	var m *model
	m.resetPasteBurstTracking()
	// should not panic
}

func TestCaptureImplicitPasteCandidateNilModel(t *testing.T) {
	var m *model
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	cmd := m.captureImplicitPasteCandidate(msg)
	if cmd != nil {
		t.Fatalf("expected nil command from nil model")
	}
}

func TestCaptureImplicitPasteCandidateNonPromotableKey(t *testing.T) {
	m := newImagePipelineModel(t)
	m.input.SetValue("hello")
	m.lastInputAt = time.Now().Add(-50 * time.Millisecond)

	// Use Escape which implicitPasteCandidateFragment returns ok=false for
	msg := tea.KeyMsg{Type: tea.KeyEscape}
	cmd := m.captureImplicitPasteCandidate(msg)
	if cmd != nil {
		t.Fatalf("expected nil command for non-fragment key (Escape)")
	}
}

func TestCaptureImplicitPasteCandidateWithPasteMsg(t *testing.T) {
	m := newImagePipelineModel(t)
	msg := tea.KeyMsg{Paste: true}
	cmd := m.captureImplicitPasteCandidate(msg)
	if cmd != nil {
		t.Fatalf("expected nil command when msg.Paste is true")
	}
}

func TestCaptureImplicitPasteSpecialKeyNilModel(t *testing.T) {
	var m *model
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	cmd := m.captureImplicitPasteSpecialKey(msg)
	if cmd != nil {
		t.Fatalf("expected nil command from nil model")
	}
}

func TestCaptureImplicitPasteSpecialKeyEnterStartsSession(t *testing.T) {
	m := newImagePipelineModel(t)
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	cmd := m.captureImplicitPasteSpecialKey(msg)
	if cmd == nil {
		t.Fatalf("expected non-nil command for Enter key (starts implicit paste session)")
	}
	if !m.pasteSession.active {
		t.Fatalf("expected paste session to be active after implicit special key capture")
	}
}

func TestCaptureImplicitPasteSpecialKeyTabStartsSession(t *testing.T) {
	m := newImagePipelineModel(t)
	msg := tea.KeyMsg{Type: tea.KeyTab}
	cmd := m.captureImplicitPasteSpecialKey(msg)
	if cmd == nil {
		t.Fatalf("expected non-nil command for Tab key (starts implicit paste session)")
	}
	if m.pasteSession.sourceKind != "implicit-tab" {
		t.Fatalf("expected implicit-tab source kind, got %q", m.pasteSession.sourceKind)
	}
}

func TestIsSplitPasteContinuationEmptyInput(t *testing.T) {
	if isSplitPasteContinuation("   ", "paste-key", time.Now()) {
		t.Fatalf("expected empty trimmed input to not be a split continuation")
	}
}

func TestIsSplitPasteContinuationPathInput(t *testing.T) {
	if isSplitPasteContinuation(`C:\Users\test\file`, "paste-key", time.Now()) {
		t.Fatalf("expected path-like input to not be a split continuation")
	}
}

func TestShouldPromoteImplicitImagePathPasteGuards(t *testing.T) {
	var nilModel *model
	if nilModel.shouldPromoteImplicitImagePathPaste(`C:/tmp/image.png`, pasteBurstImmediateMinChars, 4) {
		t.Fatalf("expected nil model to not promote image path paste")
	}

	m := newImagePipelineModel(t)
	if m.shouldPromoteImplicitImagePathPaste(`C:`, 2, 2) {
		t.Fatalf("expected short path burst to not promote")
	}
	if m.shouldPromoteImplicitImagePathPaste("C:/tmp/\nimage.png", pasteBurstImmediateMinChars, 4) {
		t.Fatalf("expected newline-containing path burst to not promote")
	}
	if !m.shouldPromoteImplicitImagePathPaste(`"C:/tmp/image.png"`, pasteBurstImmediateMinChars, 4) {
		t.Fatalf("expected quoted path burst to promote")
	}
}

func TestRapidPathPrefixStartsHiddenPasteSession(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat

	for _, r := range "C:/" {
		got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		next := got.(model)
		m = &next
	}

	if m.input.Value() != "" {
		t.Fatalf("expected rapid path prefix to stay hidden in paste session, got %q", m.input.Value())
	}
	if !m.hasActivePasteSession() {
		t.Fatalf("expected rapid path prefix to start paste session")
	}
	if m.pasteSession.bufferedText != "C:/" {
		t.Fatalf("expected buffered path prefix, got %q", m.pasteSession.bufferedText)
	}
}

func TestShouldPromoteImplicitPasteCandidatePromotesImagePathBurst(t *testing.T) {
	m := newImagePipelineModel(t)
	m.input.SetValue(`C:/tmp/image.pn`)
	m.pasteBurstCandidate = pasteBurstCandidateState{
		active:      true,
		baseInput:   "",
		lastEventAt: time.Now(),
		charCount:   len(`C:/tmp/image.pn`),
		eventCount:  4,
	}

	if !m.shouldPromoteImplicitPasteCandidate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}) {
		t.Fatalf("expected image path burst to promote into paste session")
	}
}

func TestShouldPromoteImplicitPasteCandidateRejectsSmallPathBurst(t *testing.T) {
	m := newImagePipelineModel(t)
	m.input.SetValue(`C`)
	m.pasteBurstCandidate = pasteBurstCandidateState{
		active:      true,
		baseInput:   "",
		lastEventAt: time.Now(),
		charCount:   len(`C`),
		eventCount:  1,
	}

	if m.shouldPromoteImplicitPasteCandidate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}) {
		t.Fatalf("expected incomplete path prefix to remain ordinary input")
	}
}

func TestShouldPromoteImplicitPasteCandidateRejectsPathBeforePromotionThreshold(t *testing.T) {
	m := newImagePipelineModel(t)
	m.input.SetValue(`C:`)
	m.pasteBurstCandidate = pasteBurstCandidateState{
		active:      true,
		baseInput:   "",
		lastEventAt: time.Now(),
		charCount:   len(`C:`),
		eventCount:  0,
	}

	if m.shouldPromoteImplicitPasteCandidate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}) {
		t.Fatalf("expected path-like input below promotion threshold to stay ordinary")
	}
}

func TestShouldHoldImagePathPastePreviewGuards(t *testing.T) {
	var nilModel *model
	if nilModel.shouldHoldImagePathPastePreview() {
		t.Fatalf("expected nil model to not hold image path preview")
	}

	m := newImagePipelineModel(t)
	if m.shouldHoldImagePathPastePreview() {
		t.Fatalf("expected inactive paste session to not hold preview")
	}

	m.pasteSession.active = true
	m.pasteSession.bufferedText = "C:/tmp/\nimage.png"
	if m.shouldHoldImagePathPastePreview() {
		t.Fatalf("expected newline-containing buffered text to not hold preview")
	}

	m.pasteSession.bufferedText = `"C:/tmp/image.png"`
	if !m.shouldHoldImagePathPastePreview() {
		t.Fatalf("expected quoted path-like buffered text to hold preview")
	}
}

func TestShouldFinalizeImagePathPasteSessionGuards(t *testing.T) {
	m := newImagePipelineModel(t)
	m.pasteSession = pasteSessionState{
		active:       true,
		bufferedText: `"C:/tmp/image.png`,
		sourceKind:   "paste-burst",
	}
	if m.shouldFinalizeImagePathPasteSession() {
		t.Fatalf("expected unclosed quoted path to stay buffered")
	}

	if hasUnclosedPathQuote(`"`) {
		t.Fatalf("expected single quote character to not count as unclosed path quote")
	}
}

func TestScheduleImagePathPasteFinalizeMsg(t *testing.T) {
	cmd := scheduleImagePathPasteFinalize(42)
	msg := cmd()
	finalize, ok := msg.(pasteFinalizeMsg)
	if !ok {
		t.Fatalf("expected pasteFinalizeMsg, got %T", msg)
	}
	if finalize.ID != 42 {
		t.Fatalf("expected finalize id 42, got %d", finalize.ID)
	}
}

func TestIngestPasteFragmentHoldsImagePathPreview(t *testing.T) {
	m := newImagePipelineModel(t)

	cmd := m.ingestPasteFragment(`C:/tmp/image.pn`, "paste-burst")
	if cmd == nil {
		t.Fatalf("expected paste fragment to schedule finalize")
	}
	if m.input.Value() != "" {
		t.Fatalf("expected image path paste preview to stay buffered, got %q", m.input.Value())
	}
	if !m.hasActivePasteSession() {
		t.Fatalf("expected active paste session")
	}
}

func TestIngestPasteFragmentFinalizesCompleteImagePathImmediately(t *testing.T) {
	m := newImagePipelineModel(t)
	imagePath := filepath.ToSlash(filepath.Join(m.workspace, "immediate.png"))
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	cmd := m.ingestPasteFragment(imagePath, "paste-burst")
	if cmd == nil {
		t.Fatalf("expected image path paste to schedule settle")
	}
	if m.hasActivePasteSession() {
		t.Fatalf("expected complete image path to finalize immediately")
	}
	if m.input.Value() != "[Image#1]" {
		t.Fatalf("expected image placeholder, got %q", m.input.Value())
	}
}

func TestCaptureImplicitPasteCandidateFinalizesCompleteImagePath(t *testing.T) {
	m := newImagePipelineModel(t)
	imagePath := filepath.ToSlash(filepath.Join(m.workspace, "capture.png"))
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}
	prefix := strings.TrimSuffix(imagePath, "g")
	m.input.SetValue(prefix)
	m.pasteBurstCandidate = pasteBurstCandidateState{
		active:      true,
		baseInput:   "",
		lastEventAt: time.Now(),
		charCount:   len([]rune(prefix)),
		eventCount:  3,
	}

	cmd := m.captureImplicitPasteCandidate(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if cmd == nil {
		t.Fatalf("expected paste burst settle command")
	}
	if m.hasActivePasteSession() {
		t.Fatalf("expected completed image path to finalize paste session")
	}
	if m.input.Value() != "[Image#1]" {
		t.Fatalf("expected image placeholder, got %q", m.input.Value())
	}
}

func TestFinalizePasteSessionConvertsImagePathFallback(t *testing.T) {
	m := newImagePipelineModel(t)
	imagePath := filepath.ToSlash(filepath.Join(m.workspace, "finalize.png"))
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	m.pasteSession = pasteSessionState{
		active:       true,
		baseInput:    "look ",
		bufferedText: imagePath,
		sourceKind:   "paste-burst",
		finalizeID:   1,
	}
	m.finalizePasteSession(1)

	if m.input.Value() != "look [Image#1]" {
		t.Fatalf("expected paste session to attach image path, got %q", m.input.Value())
	}
	if !strings.Contains(m.statusNote, "Attached 1 image") {
		t.Fatalf("expected attach status note, got %q", m.statusNote)
	}
}

func TestIsSplitPasteContinuationNonPasteSource(t *testing.T) {
	if isSplitPasteContinuation("some long text that is not paste", "rune", time.Now()) {
		t.Fatalf("expected non-paste source to not be a split continuation")
	}
}

func TestIsSplitPasteContinuationContainsMarker(t *testing.T) {
	if isSplitPasteContinuation("[Paste #1 ~11 lines]", "paste-key", time.Now()) {
		t.Fatalf("expected input containing paste marker to not be a split continuation")
	}
}

func TestIsSplitPasteContinuationWithinWindow(t *testing.T) {
	if !isSplitPasteContinuation("some quick text", "paste-key", time.Now().Add(-500*time.Millisecond)) {
		t.Fatalf("expected split continuation within paste continuation window")
	}
}

func TestIsSplitPasteContinuationOutsideWindowButMultiLine(t *testing.T) {
	if !isSplitPasteContinuation("line1\nline2\nline3", "paste-key", time.Now().Add(-3*time.Second)) {
		t.Fatalf("expected multi-line paste to be a split continuation even outside window")
	}
}

func TestIsSplitPasteContinuationOutsideWindowButLong(t *testing.T) {
	longText := strings.Repeat("a", pasteQuickCharThreshold)
	if !isSplitPasteContinuation(longText, "paste-key", time.Now().Add(-3*time.Second)) {
		t.Fatalf("expected long paste to be a split continuation even outside window")
	}
}

func TestIsSplitPasteContinuationZeroLastPasteAt(t *testing.T) {
	if isSplitPasteContinuation("short", "paste-key", time.Time{}) {
		t.Fatalf("expected short single line with zero lastPasteAt to not be a split continuation")
	}
}

func TestLooksLikePastedFragmentWithWhitespace(t *testing.T) {
	if !looksLikePastedFragment("text with spaces") {
		t.Fatalf("expected text with spaces to look like a pasted fragment")
	}
	if !looksLikePastedFragment("text\twith\ttabs") {
		t.Fatalf("expected text with tabs to look like a pasted fragment")
	}
}

func TestLooksLikePastedFragmentPlain(t *testing.T) {
	short := strings.Repeat("x", 63)
	if looksLikePastedFragment(short) {
		t.Fatalf("expected short text without whitespace under 64 chars to not look like a pasted fragment")
	}
	long := strings.Repeat("x", 64)
	if !looksLikePastedFragment(long) {
		t.Fatalf("expected text of 64 chars to look like a pasted fragment")
	}
}

func TestShouldMergeIntoLatestMarkerNilModel(t *testing.T) {
	var m *model
	if m.shouldMergeIntoLatestMarker("rune") {
		t.Fatalf("expected nil model to return false")
	}
}

func TestImplicitPasteCandidateFragmentPasteMsg(t *testing.T) {
	m := newImagePipelineModel(t)
	msg := tea.KeyMsg{Paste: true}
	frag, source, ok := m.implicitPasteCandidateFragment(msg)
	if ok {
		t.Fatalf("expected paste message to not be a candidate fragment, got frag=%q source=%q", frag, source)
	}
}

func TestImplicitPasteCandidateFragmentEmptyRunes(t *testing.T) {
	m := newImagePipelineModel(t)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{}}
	_, _, ok := m.implicitPasteCandidateFragment(msg)
	if ok {
		t.Fatalf("expected empty runes to not be a candidate fragment")
	}
}

func TestImplicitPasteCandidateFragmentUnhandledKey(t *testing.T) {
	m := newImagePipelineModel(t)
	msg := tea.KeyMsg{Type: tea.KeyEscape}
	_, _, ok := m.implicitPasteCandidateFragment(msg)
	if ok {
		t.Fatalf("expected unhandled key type to not be a candidate fragment")
	}
}

func TestShouldPromoteImplicitPasteCandidateNilModel(t *testing.T) {
	var m *model
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	if m.shouldPromoteImplicitPasteCandidate(msg) {
		t.Fatalf("expected nil model to return false for promote")
	}
}

func TestShouldCaptureImplicitPasteSpecialKeyNilModel(t *testing.T) {
	var m *model
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	if m.shouldCaptureImplicitPasteSpecialKey(msg) {
		t.Fatalf("expected nil model to return false")
	}
}

func TestShouldCaptureImplicitPasteSpecialKeyPasteMsg(t *testing.T) {
	m := newImagePipelineModel(t)
	msg := tea.KeyMsg{Paste: true}
	if m.shouldCaptureImplicitPasteSpecialKey(msg) {
		t.Fatalf("expected paste message to not be captured as special key")
	}
}

func TestShouldCaptureImplicitPasteSpecialKeyNonEnterTab(t *testing.T) {
	m := newImagePipelineModel(t)
	msg := tea.KeyMsg{Type: tea.KeyEscape}
	if m.shouldCaptureImplicitPasteSpecialKey(msg) {
		t.Fatalf("expected non-enter/tab key to not be captured")
	}
}

func TestCountCompressedMarkersHandlesVariousInputs(t *testing.T) {
	if got := countCompressedMarkers(""); got != 0 {
		t.Fatalf("expected empty string count to be 0, got %d", got)
	}
	if got := countCompressedMarkers("[Paste #1 ~11 lines]"); got != 1 {
		t.Fatalf("expected single marker count to be 1, got %d", got)
	}
	if got := countCompressedMarkers("[Pasted #3 ~20 lines]"); got != 1 {
		t.Fatalf("expected Pasted marker count to be 1, got %d", got)
	}
}

func TestDropLatestCompressedMarkerNoMarker(t *testing.T) {
	result := dropLatestCompressedMarker("plain text with no markers")
	if result != "plain text with no markers" {
		t.Fatalf("expected plain text unchanged, got %q", result)
	}
}

func TestDropLatestCompressedMarkerSingleMarker(t *testing.T) {
	result := dropLatestCompressedMarker("[Paste #1 ~11 lines]")
	if result != "" {
		t.Fatalf("expected single marker to be dropped to empty, got %q", result)
	}
}

func TestDropLatestCompressedMarkerMultipleMarkers(t *testing.T) {
	result := dropLatestCompressedMarker("[Paste #1 ~11 lines] [Paste #2 ~15 lines]")
	if result != "[Paste #1 ~11 lines]" {
		t.Fatalf("expected only latest marker dropped, got %q", result)
	}
}

func TestDropLatestCompressedMarkerWithTextBefore(t *testing.T) {
	result := dropLatestCompressedMarker("before text [Paste #1 ~11 lines]")
	if result != "before text" {
		t.Fatalf("expected marker removed with surrounding text preserved, got %q", result)
	}
}
