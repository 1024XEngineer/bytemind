package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"
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
	re := regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`)
	if !re.MatchString(got) {
		t.Fatalf("expected split paste to compress into marker, got %q", got)
	}
	if len(m.pastedContents) != 1 {
		t.Fatalf("expected one stored pasted content after split paste, got %d", len(m.pastedContents))
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
	if got := m.input.Value(); !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected first chunk to compress immediately, got %q", got)
	}

	before := m.input.Value()
	after := before + "\n" + chunk2
	m.input.SetValue(after)
	m.handleInputMutation(before, after, "paste")

	got := m.input.Value()
	re := regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`)
	if !re.MatchString(got) {
		t.Fatalf("expected followup paste chunk to merge into one marker, got %q", got)
	}
	if len(m.pastedOrder) != 1 {
		t.Fatalf("expected one stored marker after merged paste chunks, got %d", len(m.pastedOrder))
	}
}

func TestApplyLongPastedTextPipelineCompressesBurstTypedPasteFallback(t *testing.T) {
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
		if strings.HasPrefix(before, "[Paste #") {
			break
		}
	}
	if !strings.HasPrefix(m.input.Value(), "[Paste #") {
		t.Fatalf("expected burst fallback compression, got %q", m.input.Value())
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
	re := regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`)
	if !re.MatchString(got) {
		t.Fatalf("expected immediate followup paste chunk to merge into latest marker, got %q", got)
	}
}

func TestApplyLongPastedTextPipelineAllowsTailAccumulationThenCompresses(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Join([]string{
		"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11",
	}, "\n")
	second := strings.Join([]string{
		"b1", "b2", "b3", "b4", "b5", "b6", "b7", "b8", "b9", "b10", "b11",
	}, "\n")

	m.handleInputMutation("", first, "paste")
	before := m.input.Value()
	if !strings.HasPrefix(before, "[Paste #") {
		t.Fatalf("expected first marker, got %q", before)
	}

	// Simulate a terminal that emits pasted text as many tiny rune updates.
	for _, r := range second {
		after := before + string(r)
		m.input.SetValue(after)
		m.handleInputMutation(before, after, "rune")
		before = m.input.Value()
	}

	re := regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`)
	if !re.MatchString(before) {
		t.Fatalf("expected accumulated split chunks to stay in one marker, got %q", before)
	}
	if len(m.pastedOrder) != 1 {
		t.Fatalf("expected one stored entry for split chunks from same paste burst, got %d", len(m.pastedOrder))
	}
}

func TestApplyLongPastedTextPipelineHidesTransientTailDuringSplitRuneContinuation(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Join([]string{
		"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11",
	}, "\n")
	second := strings.Join([]string{
		"b1", "b2", "b3", "b4", "b5", "b6", "b7", "b8", "b9", "b10", "b11",
	}, "\n")

	m.handleInputMutation("", first, "paste")
	before := m.input.Value()
	if !strings.HasPrefix(before, "[Paste #") {
		t.Fatalf("expected first marker, got %q", before)
	}

	markerRe := regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`)
	for _, r := range second {
		after := before + string(r)
		m.input.SetValue(after)
		m.handleInputMutation(before, after, "rune")
		before = m.input.Value()
		if !markerRe.MatchString(before) {
			t.Fatalf("expected transient continuation tail to stay hidden, got %q", before)
		}
	}
	if len(m.pastedOrder) != 1 {
		t.Fatalf("expected one stored entry for split chunks from same paste burst, got %d", len(m.pastedOrder))
	}
}

func TestApplyLongPastedTextPipelineTrimsShortTailAfterSecondMarker(t *testing.T) {
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
	m.handleInputMutation(before, after, "rune")
	if got := m.input.Value(); got != before {
		t.Fatalf("expected short residual tail to be trimmed, got %q", got)
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

	// Simulate a user-triggered second paste separated from previous burst.
	m.lastCompressedPasteAt = time.Now().Add(-time.Second)
	after := before + second
	m.input.SetValue(after)
	m.handleInputMutation(before, after, "rune")

	got := m.input.Value()
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected second separated paste to create another marker, got %q", got)
	}
}

func TestShouldHoldCompressedMarkerWithLongTailEvenWithoutPasteSignal(t *testing.T) {
	before := "[Paste #1 ~42 lines]"
	after := before + " this is a long continuation chunk that should be dropped"
	if !shouldHoldCompressedMarker(before, after, "", time.Now(), 1) {
		t.Fatalf("expected long tail after marker to be treated as continuation")
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
	if !strings.Contains(result, "```\n"+stored.Content+"\n```") {
		t.Fatalf("expected full content expansion, got %q", result)
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
	if !strings.Contains(text, "```\nnew2\n```") {
		t.Fatalf("expected latest pasted line expansion, got %q", text)
	}
	if strings.Contains(text, "old2") {
		t.Fatalf("expected latest pasted content, got %q", text)
	}
	if latest.ID == "" {
		t.Fatalf("expected latest pasted content id")
	}
}

func TestStorePastedContentKeepsRecentLimit(t *testing.T) {
	m := newImagePipelineModel(t)
	for i := 0; i < maxStoredPastedContents+2; i++ {
		content := strings.Repeat("x\n", longPasteLineThreshold+1) + "{\n}"
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
	if !strings.Contains(input.UserMessage.Text(), "```\nl11\n```") {
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

func TestApplyLongPastedTextPipelineMergesImmediateRuneTailIntoLatestMarker(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Join([]string{
		"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10", "a11",
	}, "\n")
	second := strings.Join([]string{
		"b1", "b2", "b3", "b4", "b5", "b6", "b7", "b8", "b9", "b10", "b11",
	}, "\n")

	m.handleInputMutation("", first, "paste")
	chain := m.input.Value()
	if !strings.HasPrefix(chain, "[Paste #") {
		t.Fatalf("expected first marker, got %q", chain)
	}

	before := chain
	after := before + second
	got, _ := m.applyLongPastedTextPipeline(before, after, "rune")
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(got) {
		t.Fatalf("expected immediate rune tail to merge into one marker, got %q", got)
	}
	if len(m.pastedOrder) != 1 {
		t.Fatalf("expected one stored entry after merge, got %d", len(m.pastedOrder))
	}
}

func TestApplyLongPastedTextPipelineMergesShortTrailingTextAndHidesTail(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Repeat("段落内容很长用于触发压缩。", 30)
	m.handleInputMutation("", first, "paste")
	chain := m.input.Value()
	if !strings.HasPrefix(chain, "[Paste #") {
		t.Fatalf("expected first marker, got %q", chain)
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

func TestApplyLongPastedTextPipelineMergesSlashLeadingTrailingText(t *testing.T) {
	m := newImagePipelineModel(t)
	first := strings.Repeat("未来协作系统正在发生深刻变化。", 28)
	m.handleInputMutation("", first, "paste")
	chain := m.input.Value()
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

func TestShouldCompressPastedTextQuickThresholdWithPasteSignal(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("alpha beta gamma ", 8)
	if !m.shouldCompressPastedText(text, "ctrl+v") {
		t.Fatalf("expected paste signal + quick threshold to trigger compression")
	}
}

func TestShouldCompressPastedTextQuickThresholdWithRecentPasteWindow(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("alpha beta gamma ", 8)
	m.lastPasteAt = time.Now()
	if !m.shouldCompressPastedText(text, "rune") {
		t.Fatalf("expected recent paste window + quick threshold to trigger compression")
	}
}

func TestShouldCompressPastedTextSkipsLikelyPathInput(t *testing.T) {
	m := newImagePipelineModel(t)
	path := `C:\Users\demo\Pictures\screenshots\capture.png`
	if m.shouldCompressPastedText(path, "ctrl+v") {
		t.Fatalf("expected likely path input to bypass paste compression")
	}
}

func TestShouldCompressPastedTextDetectsFastCharacterBurstWithoutPasteSignal(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("burst payload ", 10)
	m.inputBurstSize = pasteBurstCharThreshold
	m.lastInputAt = time.Now()
	if !m.shouldCompressPastedText(text, "rune") {
		t.Fatalf("expected rapid burst fallback to trigger compression")
	}
}

func TestShouldCompressPastedTextDetectsShortRapidBurstEarly(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("x ", pasteBurstImmediateMinChars+2)
	m.inputBurstSize = pasteBurstImmediateMinChars
	m.lastInputAt = time.Now()
	if !m.shouldCompressPastedText(text, "rune") {
		t.Fatalf("expected short rapid burst to trigger early compression")
	}
}

func TestShouldCompressPastedTextSkipsShortRapidBurstWithoutPasteSignals(t *testing.T) {
	m := newImagePipelineModel(t)
	text := strings.Repeat("x", pasteBurstImmediateMinChars+2)
	m.inputBurstSize = pasteBurstImmediateMinChars
	m.lastInputAt = time.Now()
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

func TestShouldHoldCompressedMarkerBranchMatrix(t *testing.T) {
	now := time.Now()
	marker := "[Paste #1 ~11 lines]"

	if shouldHoldCompressedMarker("plain text", "plain text trailing", "", now, 0) {
		t.Fatalf("expected non-marker prefix not to be held")
	}
	if shouldHoldCompressedMarker(marker, marker, "", now, 0) {
		t.Fatalf("expected unchanged marker not to be held")
	}
	if shouldHoldCompressedMarker(marker, marker+" [Paste #2 ~12 lines]", "", now, 0) {
		t.Fatalf("expected marker-only tail chain not to be held")
	}

	if !shouldHoldCompressedMarker(marker, marker+" this continuation payload should be held", "", now, 0) {
		t.Fatalf("expected long tail after marker to be held")
	}
	if !shouldHoldCompressedMarker(marker, marker+" short", "paste", time.Time{}, 0) {
		t.Fatalf("expected paste-like source to hold short tail")
	}
	if !shouldHoldCompressedMarker(marker, marker+" short", "rune", time.Time{}, 10) {
		t.Fatalf("expected burst size to hold short tail")
	}
	if !shouldHoldCompressedMarker(marker, marker+" short", "rune", now, 0) {
		t.Fatalf("expected recent paste window to hold short tail")
	}
	if shouldHoldCompressedMarker(marker, marker+" short", "rune", time.Time{}, 0) {
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

func TestShouldMergeIntoLatestMarkerAllowsFastPasteChunks(t *testing.T) {
	now := time.Now()
	if !shouldMergeIntoLatestMarker("paste", now.Add(-80*time.Millisecond)) {
		t.Fatalf("expected immediate paste chunk to merge into latest marker")
	}
	if shouldMergeIntoLatestMarker("paste", now.Add(-500*time.Millisecond)) {
		t.Fatalf("expected delayed paste chunk not to merge into latest marker")
	}
}
