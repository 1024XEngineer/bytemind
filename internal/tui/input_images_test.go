package tui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bytemind/internal/assets"
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
	blob, err := m.imageStore.GetImageByAssetID(context.Background(), m.sess.ID, assetID)
	if err != nil {
		t.Fatalf("read stored image: %v", err)
	}
	if !bytes.Equal(blob.Data, []byte("png-from-file")) {
		t.Fatalf("unexpected stored bytes: %q", string(blob.Data))
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
	if err := m.imageStore.DeleteSessionImages(context.Background(), m.sess.ID); err != nil {
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
	if err := m.imageStore.DeleteSessionImages(context.Background(), m.sess.ID); err != nil {
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
