package tui

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bytemind/internal/assets"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	"bytemind/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
)

type fakeClipboardImageReader struct {
	mediaType string
	data      []byte
	fileName  string
	err       error
}

func (f fakeClipboardImageReader) ReadImage(context.Context) (string, []byte, string, error) {
	return f.mediaType, f.data, f.fileName, f.err
}

func newImagePipelineModel(t *testing.T) *model {
	t.Helper()

	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	imageStore, err := assets.NewFileAssetStore(t.TempDir())
	if err != nil {
		t.Fatalf("new image store: %v", err)
	}

	input := textarea.New()
	input.Focus()

	m := &model{
		store:              store,
		sess:               session.New(workspace),
		imageStore:         imageStore,
		workspace:          workspace,
		input:              input,
		inputImageRefs:     make(map[int]llm.AssetID, 8),
		inputImageMentions: make(map[string]llm.AssetID, 8),
		orphanedImages:     make(map[llm.AssetID]time.Time, 8),
		nextImageID:        1,
	}
	m.ensureSessionImageAssets()
	return m
}

func mustIngestTestImage(t *testing.T, m *model, marker string) string {
	t.Helper()
	placeholder, note, ok := m.ingestImageBinary("image/png", marker+".png", []byte("png-"+marker))
	if !ok {
		t.Fatalf("ingest failed: %s", note)
	}
	return placeholder
}

func findAssetIDByImageID(t *testing.T, m *model, imageID int) llm.AssetID {
	t.Helper()
	assetID, _, ok := m.findAssetByImageID(imageID)
	if !ok {
		t.Fatalf("asset for image id %d not found", imageID)
	}
	return assetID
}

func TestApplyInputImagePipelineConvertsPastedPathToPlaceholder(t *testing.T) {
	m := newImagePipelineModel(t)
	imagePath := filepath.Join(m.workspace, "case.png")
	if err := os.WriteFile(imagePath, []byte("png-from-file"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	updated, note := m.applyInputImagePipeline("", imagePath, "ctrl+v")
	if updated != "[Image #1]" {
		t.Fatalf("expected image placeholder, got %q", updated)
	}
	if !strings.Contains(note, "Attached 1 image") {
		t.Fatalf("expected attach note, got %q", note)
	}

	assetID := findAssetIDByImageID(t, m, 1)
	blob, err := m.imageStore.GetImageByAssetID(context.Background(), corepkg.SessionID(m.sess.ID), assetID)
	if err != nil {
		t.Fatalf("read stored image: %v", err)
	}
	if !bytes.Equal(blob.Data, []byte("png-from-file")) {
		t.Fatalf("unexpected stored bytes: %q", string(blob.Data))
	}
}

func TestParseClipboardImageOutputJSONPayload(t *testing.T) {
	raw := "{\"media_type\":\"image/jpeg\",\"file_name\":\"copied.jpg\",\"data\":\"" + base64.StdEncoding.EncodeToString([]byte("jpeg-bytes")) + "\"}\n"
	mediaType, data, fileName, err := parseClipboardImageOutput(raw)
	if err != nil {
		t.Fatalf("parseClipboardImageOutput: %v", err)
	}
	if mediaType != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %q", mediaType)
	}
	if fileName != "copied.jpg" {
		t.Fatalf("expected copied.jpg, got %q", fileName)
	}
	if !bytes.Equal(data, []byte("jpeg-bytes")) {
		t.Fatalf("unexpected decoded bytes %q", string(data))
	}
}

func TestParseClipboardImageOutputLegacyBase64(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	mediaType, data, fileName, err := parseClipboardImageOutput(raw)
	if err != nil {
		t.Fatalf("parseClipboardImageOutput: %v", err)
	}
	if mediaType != "image/png" {
		t.Fatalf("expected image/png fallback, got %q", mediaType)
	}
	if fileName != "clipboard.png" {
		t.Fatalf("expected clipboard.png fallback, got %q", fileName)
	}
	if !bytes.Equal(data, []byte("png-bytes")) {
		t.Fatalf("unexpected decoded bytes %q", string(data))
	}
}

func TestParseClipboardImageOutputPrefersLastNonEmptyLine(t *testing.T) {
	raw := "warning line\n\n{\"media_type\":\"image/png\",\"file_name\":\"clip.png\",\"data\":\"" + base64.StdEncoding.EncodeToString([]byte("ok")) + "\"}\n"
	_, data, _, err := parseClipboardImageOutput(raw)
	if err != nil {
		t.Fatalf("parseClipboardImageOutput: %v", err)
	}
	if !bytes.Equal(data, []byte("ok")) {
		t.Fatalf("unexpected decoded bytes %q", string(data))
	}
}

func TestExtractImagePathsFromChunkRejectsMixedTokens(t *testing.T) {
	workspace := t.TempDir()
	imagePath := filepath.Join(workspace, "ok.png")
	textPath := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write png fixture: %v", err)
	}
	if err := os.WriteFile(textPath, []byte("txt"), 0o644); err != nil {
		t.Fatalf("write txt fixture: %v", err)
	}

	got := extractImagePathsFromChunk(imagePath+" "+textPath, workspace)
	if got != nil {
		t.Fatalf("expected mixed non-image tokens to be rejected, got %#v", got)
	}
}

func TestExtractInlineImagePathSpansFindsPathInsideMixedText(t *testing.T) {
	workspace := t.TempDir()
	imagePath := filepath.Join(workspace, "inline.jpg")
	if err := os.WriteFile(imagePath, []byte("jpg"), 0o644); err != nil {
		t.Fatalf("write jpg fixture: %v", err)
	}

	text := imagePath + "图片在描述什么？"
	spans := extractInlineImagePathSpans(text)
	if len(spans) != 1 {
		t.Fatalf("expected one inline path span, got %#v", spans)
	}
	if spans[0].Path != filepath.Clean(imagePath) {
		t.Fatalf("unexpected span path: %q", spans[0].Path)
	}
}

func TestBuildPromptInputExpandsReferencedPlaceholdersOnly(t *testing.T) {
	m := newImagePipelineModel(t)
	first := mustIngestTestImage(t, m, "first")
	_ = mustIngestTestImage(t, m, "second")

	input, display, err := m.buildPromptInput("inspect " + first + " only")
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if display != "inspect [Image #1] only" {
		t.Fatalf("unexpected display text: %q", display)
	}
	if len(input.Assets) != 1 {
		t.Fatalf("expected one referenced asset, got %d", len(input.Assets))
	}
	if _, ok := input.Assets[findAssetIDByImageID(t, m, 1)]; !ok {
		t.Fatalf("expected image #1 asset to be included")
	}
	if _, ok := input.Assets[findAssetIDByImageID(t, m, 2)]; ok {
		t.Fatalf("did not expect unreferenced image #2 asset")
	}

	imageParts := 0
	for _, part := range input.UserMessage.Parts {
		if part.Type == llm.PartImageRef && part.Image != nil {
			imageParts++
		}
	}
	if imageParts != 1 {
		t.Fatalf("expected one image_ref part, got %d", imageParts)
	}
}

func TestBuildPromptInputExpandsMentionedImageReference(t *testing.T) {
	m := newImagePipelineModel(t)
	_ = mustIngestTestImage(t, m, "mention")
	assetID := findAssetIDByImageID(t, m, 1)
	m.bindMentionImageAsset("2.1.jpg", assetID)

	input, display, err := m.buildPromptInput("这张图 @2.1.jpg 讲了什么")
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if display != "这张图 @2.1.jpg 讲了什么" {
		t.Fatalf("unexpected display text: %q", display)
	}
	if len(input.Assets) != 1 {
		t.Fatalf("expected one referenced asset, got %d", len(input.Assets))
	}
	if _, ok := input.Assets[assetID]; !ok {
		t.Fatalf("expected mentioned image asset to be included")
	}

	imageParts := 0
	for _, part := range input.UserMessage.Parts {
		if part.Type == llm.PartImageRef && part.Image != nil && part.Image.AssetID == assetID {
			imageParts++
		}
	}
	if imageParts != 1 {
		t.Fatalf("expected one image_ref part from @mention, got %d", imageParts)
	}
}

func TestBuildPromptInputIncludesHistoricalImageAssets(t *testing.T) {
	m := newImagePipelineModel(t)
	_ = mustIngestTestImage(t, m, "history")
	assetID := findAssetIDByImageID(t, m, 1)
	m.sess.Messages = append(m.sess.Messages, llm.Message{
		Role: llm.RoleUser,
		Parts: []llm.Part{
			{Type: llm.PartText, Text: &llm.TextPart{Value: "old image "}},
			{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: assetID}},
		},
	})

	input, _, err := m.buildPromptInput("继续分析")
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if len(input.Assets) != 1 {
		t.Fatalf("expected historical image asset included, got %d", len(input.Assets))
	}
	if _, ok := input.Assets[assetID]; !ok {
		t.Fatalf("expected historical asset %q in request assets", assetID)
	}
}

func TestApplyInputImagePipelineReplacesInlinePathAndKeepsTrailingText(t *testing.T) {
	m := newImagePipelineModel(t)
	imagePath := filepath.Join(m.workspace, "inline.png")
	if err := os.WriteFile(imagePath, []byte("png-inline"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	updated, note := m.applyInputImagePipeline("", imagePath+"图片在描述什么？", "ctrl+v")
	if updated != "[Image #1]图片在描述什么？" {
		t.Fatalf("expected inline path replaced and text preserved, got %q", updated)
	}
	if !strings.Contains(note, "Attached 1 image") {
		t.Fatalf("expected attach note, got %q", note)
	}
}

func TestBuildPromptInputGracefullyDegradesMissingImageAsset(t *testing.T) {
	m := newImagePipelineModel(t)
	ref := mustIngestTestImage(t, m, "missing")
	if err := m.imageStore.DeleteSessionImages(context.Background(), corepkg.SessionID(m.sess.ID)); err != nil {
		t.Fatalf("delete session images: %v", err)
	}

	input, _, err := m.buildPromptInput("use " + ref)
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if len(input.Assets) != 0 {
		t.Fatalf("expected no binary asset payload when cache is missing, got %d", len(input.Assets))
	}
	if strings.Contains(input.UserMessage.Text(), "[Image #1]") {
		t.Fatalf("expected unavailable marker text, got %q", input.UserMessage.Text())
	}
	if !strings.Contains(input.UserMessage.Text(), "Image #1 unavailable") {
		t.Fatalf("expected unavailable marker text, got %q", input.UserMessage.Text())
	}
}

func TestSyncInputImageRefsMarksOrphansAndRestoresReferences(t *testing.T) {
	m := newImagePipelineModel(t)
	ref := mustIngestTestImage(t, m, "orphan")
	assetID := findAssetIDByImageID(t, m, 1)

	m.syncInputImageRefs(ref)
	if _, ok := m.inputImageRefs[1]; !ok {
		t.Fatalf("expected image #1 to be tracked as input reference")
	}
	if _, orphaned := m.orphanedImages[assetID]; orphaned {
		t.Fatalf("did not expect referenced asset to be orphaned")
	}

	m.syncInputImageRefs("")
	if _, ok := m.inputImageRefs[1]; ok {
		t.Fatalf("expected image #1 input reference to be cleared")
	}
	if _, orphaned := m.orphanedImages[assetID]; !orphaned {
		t.Fatalf("expected orphaned marker after placeholder removal")
	}

	m.syncInputImageRefs(ref)
	if _, ok := m.inputImageRefs[1]; !ok {
		t.Fatalf("expected image #1 reference to be restored")
	}
	if _, orphaned := m.orphanedImages[assetID]; orphaned {
		t.Fatalf("expected orphan marker to clear when placeholder is re-added")
	}
}

func TestSyncInputImageRefsOrphansMentionImageWhenMentionRemoved(t *testing.T) {
	m := newImagePipelineModel(t)
	_ = mustIngestTestImage(t, m, "mention-orphan")
	assetID := findAssetIDByImageID(t, m, 1)
	m.bindMentionImageAsset("2.1.jpg", assetID)

	m.syncInputImageRefs("请看 @2.1.jpg")
	if _, orphaned := m.orphanedImages[assetID]; orphaned {
		t.Fatalf("did not expect mentioned image to be orphaned while referenced")
	}

	m.syncInputImageRefs("请看这个文件")
	if _, orphaned := m.orphanedImages[assetID]; !orphaned {
		t.Fatalf("expected mention image to be marked orphan after mention removal")
	}
}

func TestHandleEmptyClipboardPasteReadsAndAttachesImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard-bytes"),
		fileName:  "clipboard.png",
	}

	note := m.handleEmptyClipboardPaste()
	if !strings.Contains(note, "Attached image from clipboard") {
		t.Fatalf("expected clipboard attach note, got %q", note)
	}
	if m.input.Value() != "[Image #1]" {
		t.Fatalf("expected placeholder inserted into input, got %q", m.input.Value())
	}
}

func TestBuildPromptInputFallsBackWhenMentionAssetMissing(t *testing.T) {
	m := newImagePipelineModel(t)
	_ = mustIngestTestImage(t, m, "mention-missing")
	assetID := findAssetIDByImageID(t, m, 1)
	m.bindMentionImageAsset("2.1.jpg", assetID)
	if err := m.imageStore.DeleteSessionImages(context.Background(), corepkg.SessionID(m.sess.ID)); err != nil {
		t.Fatalf("delete session images: %v", err)
	}

	input, _, err := m.buildPromptInput("分析 @2.1.jpg")
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if len(input.Assets) != 0 {
		t.Fatalf("expected missing mention asset not to produce binary payload")
	}
	if !strings.Contains(input.UserMessage.Text(), "@2.1.jpg") {
		t.Fatalf("expected missing mention image to fall back to literal mention text, got %q", input.UserMessage.Text())
	}
}

func TestClassifyInputMutationDetectsCtrlVPasteEmpty(t *testing.T) {
	class, _, _, _ := classifyInputMutation("unchanged", "unchanged", "ctrl+v")
	if class != inputMutationPasteEmpty {
		t.Fatalf("expected ctrl+v empty paste classification, got %q", class)
	}
}

func TestShouldTriggerClipboardImagePasteForCtrlVMarkerOnly(t *testing.T) {
	if !shouldTriggerClipboardImagePaste("", ctrlVMarkerRune, "ctrl+v") {
		t.Fatalf("expected ctrl+v control marker to trigger clipboard image read")
	}
}

func TestShouldTriggerClipboardImagePasteSkipsTextPayload(t *testing.T) {
	if shouldTriggerClipboardImagePaste("", `C:\tmp\a.png`, "ctrl+v") {
		t.Fatalf("expected text payload paste not to trigger clipboard image read")
	}
}

func TestIsCtrlVKeyAcceptsControlMarker(t *testing.T) {
	if !isCtrlVKey(ctrlVMarkerRune) {
		t.Fatalf("expected control marker to be treated as ctrl+v")
	}
	if !isCtrlVKey("[" + ctrlVMarkerRune + "]") {
		t.Fatalf("expected bracketed control marker to be treated as ctrl+v")
	}
}

func TestDefaultClipboardImageReaderReturnsUnavailableOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows behavior only")
	}

	_, _, _, err := defaultClipboardImageReader{}.ReadImage(context.Background())
	if err == nil {
		t.Fatal("expected clipboard reader to fail on non-windows")
	}
	perr, ok := err.(*llm.ProviderError)
	if !ok {
		t.Fatalf("expected provider error, got %T", err)
	}
	if perr.Code != llm.ErrorCodeClipboardUnavailable {
		t.Fatalf("expected clipboard_unavailable code, got %q", perr.Code)
	}
}

func TestApplyWholeInputImagePathFallbackRequiresPasteSignalOrRecentPaste(t *testing.T) {
	m := newImagePipelineModel(t)
	imagePath := filepath.Join(m.workspace, "fallback.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	unchanged, note := m.applyWholeInputImagePathFallback(imagePath, "rune")
	if unchanged != imagePath {
		t.Fatalf("expected no fallback conversion without paste signal, got %q", unchanged)
	}
	if note != "" {
		t.Fatalf("expected no note without conversion, got %q", note)
	}

	m.lastPasteAt = time.Now()
	updated, note := m.applyWholeInputImagePathFallback(imagePath, "rune")
	if updated != "[Image #1]" {
		t.Fatalf("expected fallback conversion with recent paste window, got %q", updated)
	}
	if !strings.Contains(note, "Attached 1 image") {
		t.Fatalf("expected attach note, got %q", note)
	}
}

func TestBindMentionImageAssetRebindMarksPreviousAssetOrphaned(t *testing.T) {
	m := newImagePipelineModel(t)
	_ = mustIngestTestImage(t, m, "first")
	_ = mustIngestTestImage(t, m, "second")
	asset1 := findAssetIDByImageID(t, m, 1)
	asset2 := findAssetIDByImageID(t, m, 2)

	m.bindMentionImageAsset("2.1.jpg", asset1)
	m.bindMentionImageAsset("2.1.jpg", asset2)

	key := normalizeImageMentionPath("2.1.jpg")
	if m.inputImageMentions[key] != asset2 {
		t.Fatalf("expected mention key %q to point to new asset", key)
	}
	if _, ok := m.orphanedImages[asset1]; !ok {
		t.Fatalf("expected previous mention asset %q to be orphaned after rebind", asset1)
	}
}

func TestBuildPromptInputIgnoresForgedInvalidPlaceholderIDs(t *testing.T) {
	m := newImagePipelineModel(t)
	raw := "look [Image #999999999999999999999999]"

	input, display, err := m.buildPromptInput(raw)
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	if display != raw {
		t.Fatalf("expected display text unchanged, got %q", display)
	}
	if len(input.Assets) != 0 {
		t.Fatalf("expected no assets for forged placeholders, got %d", len(input.Assets))
	}
	if input.UserMessage.Text() != raw {
		t.Fatalf("expected forged placeholders to remain literal text, got %q", input.UserMessage.Text())
	}
}

func TestCollectImageAssetIDsFromMessagesDeduplicatesAndSkipsInvalid(t *testing.T) {
	assetA := llm.AssetID("session-a:1")
	assetB := llm.AssetID("session-a:2")
	ids := collectImageAssetIDsFromMessages([]llm.Message{
		{
			Role: llm.RoleUser,
			Parts: []llm.Part{
				{Type: llm.PartText, Text: &llm.TextPart{Value: "text only"}},
				{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: assetA}},
				{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: assetA}},
				{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: "   "}},
			},
		},
		{
			Role: llm.RoleAssistant,
			Parts: []llm.Part{
				{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: assetB}},
			},
		},
	})
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique asset ids, got %#v", ids)
	}
	if ids[0] != assetA || ids[1] != assetB {
		t.Fatalf("unexpected asset id order/content: %#v", ids)
	}
}

func TestSplitPathTokensHandlesQuotedPathsWithSpaces(t *testing.T) {
	tokens := splitPathTokens(`"a b/c.png" 'd e/f.jpg' plain.webp`)
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %#v", tokens)
	}
	if tokens[0] != "a b/c.png" || tokens[1] != "d e/f.jpg" || tokens[2] != "plain.webp" {
		t.Fatalf("unexpected token split result: %#v", tokens)
	}
}

func TestIngestImageBinaryRejectsInvalidRuntimeState(t *testing.T) {
	t.Run("nil model", func(t *testing.T) {
		var m *model
		_, note, ok := m.ingestImageBinary("image/png", "x.png", []byte("png"))
		if ok {
			t.Fatal("expected ingest to fail for nil model")
		}
		if !strings.Contains(note, "session unavailable") {
			t.Fatalf("unexpected note: %q", note)
		}
	})

	t.Run("missing image store", func(t *testing.T) {
		m := newImagePipelineModel(t)
		m.imageStore = nil
		_, note, ok := m.ingestImageBinary("image/png", "x.png", []byte("png"))
		if ok {
			t.Fatal("expected ingest to fail without image store")
		}
		if !strings.Contains(note, "image store unavailable") {
			t.Fatalf("unexpected note: %q", note)
		}
	})

	t.Run("empty payload", func(t *testing.T) {
		m := newImagePipelineModel(t)
		_, note, ok := m.ingestImageBinary("image/png", "x.png", nil)
		if ok {
			t.Fatal("expected ingest to fail for empty payload")
		}
		if !strings.Contains(note, "empty image payload") {
			t.Fatalf("unexpected note: %q", note)
		}
	})
}

func TestHandleEmptyClipboardPasteReturnsReaderError(t *testing.T) {
	m := newImagePipelineModel(t)
	m.clipboard = fakeClipboardImageReader{
		err: llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, os.ErrNotExist),
	}

	note := m.handleEmptyClipboardPaste()
	if !strings.Contains(note, "file does not exist") && !strings.Contains(strings.ToLower(note), "not exist") {
		t.Fatalf("expected clipboard error message to propagate, got %q", note)
	}
}

func TestNormalizeImageMentionPathCleansRelativeSegments(t *testing.T) {
	got := normalizeImageMentionPath("@./images/../2.1.jpg")
	if got == "" {
		t.Fatal("expected normalized mention path")
	}
	if strings.Contains(got, "..") {
		t.Fatalf("expected cleaned mention path, got %q", got)
	}
	if strings.HasPrefix(got, "./") {
		t.Fatalf("expected normalized path without ./ prefix, got %q", got)
	}
}

func TestExtractMentionImageSpansOnlyReturnsBoundMentions(t *testing.T) {
	bindings := map[string]llm.AssetID{
		normalizeImageMentionPath("a.png"): "sess:1",
	}
	spans := extractMentionImageSpans("check @a.png and @b.png", bindings)
	if len(spans) != 1 {
		t.Fatalf("expected only bound mention span, got %#v", spans)
	}
	if spans[0].AssetID != "sess:1" {
		t.Fatalf("unexpected bound asset id: %q", spans[0].AssetID)
	}
}
