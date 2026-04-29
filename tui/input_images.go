package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/assets"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
)

var imagePlaceholderPattern = regexp.MustCompile(`\[Image #(\d+)\]`)
var imageMentionPattern = regexp.MustCompile(`(?i)@([^\s@]+?\.(?:png|jpe?g|webp|gif))`)
var inlineWindowsImagePathPattern = regexp.MustCompile(`(?i)[a-z]:\\[^\r\n\t"'<>|]*?\.(?:png|jpe?g|webp|gif)`)
var inlineUnixImagePathPattern = regexp.MustCompile(`(?i)/(?:[^\r\n\t"'<>|/]+/)*[^\r\n\t"'<>|/]+\.(?:png|jpe?g|webp|gif)`)

type inputMutationClass string

const (
	inputMutationOrdinary    inputMutationClass = "ordinary"
	inputMutationPasteEmpty  inputMutationClass = "paste_empty"
	inputMutationPasteFilled inputMutationClass = "paste_filled"

	ctrlVKeyName    = "ctrl+v"
	ctrlVMarkerRune = "\x16"
)

type clipboardImageReader interface {
	ReadImage(ctx context.Context) (mediaType string, data []byte, fileName string, err error)
}

type defaultClipboardImageReader struct{}

func (defaultClipboardImageReader) ReadImage(ctx context.Context) (string, []byte, string, error) {
	if runtime.GOOS != "windows" {
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, fmt.Errorf("clipboard image is only supported on windows in this build"))
	}

	script := strings.TrimSpace(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

function Get-MediaTypeFromExtension([string]$path) {
	if ([string]::IsNullOrWhiteSpace($path)) { return '' }
	$ext = [System.IO.Path]::GetExtension($path).ToLowerInvariant()
	if ($ext -eq '.png') { return 'image/png' }
	elseif ($ext -eq '.jpg' -or $ext -eq '.jpeg') { return 'image/jpeg' }
	elseif ($ext -eq '.webp') { return 'image/webp' }
	elseif ($ext -eq '.gif') { return 'image/gif' }
	return ''
}

function Build-Payload([string]$mediaType, [string]$fileName, [byte[]]$bytes) {
	if ([string]::IsNullOrWhiteSpace($mediaType)) { return '' }
	if ($null -eq $bytes -or $bytes.Length -le 0) { return '' }
	if ([string]::IsNullOrWhiteSpace($fileName)) { $fileName = 'clipboard.png' }
	return (@{media_type=$mediaType;file_name=$fileName;data=[Convert]::ToBase64String($bytes)} | ConvertTo-Json -Compress)
}

function Payload-FromPath([string]$path) {
	if ([string]::IsNullOrWhiteSpace($path)) { return '' }
	$candidate = $path.Trim().Trim('"')
	if (-not [System.IO.File]::Exists($candidate)) { return '' }
	$mediaType = Get-MediaTypeFromExtension $candidate
	if ($mediaType -ne '') {
		$bytes = [System.IO.File]::ReadAllBytes($candidate)
		return (Build-Payload $mediaType ([System.IO.Path]::GetFileName($candidate)) $bytes)
	}
	try {
		$img = [System.Drawing.Image]::FromFile($candidate)
		try {
			$ms = New-Object System.IO.MemoryStream
			$img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
			$name = [System.IO.Path]::GetFileNameWithoutExtension($candidate) + '.png'
			return (Build-Payload 'image/png' $name $ms.ToArray())
		} finally {
			$img.Dispose()
		}
	} catch {
		return ''
	}
}

function Payload-FromDataUri([string]$value) {
	if ([string]::IsNullOrWhiteSpace($value)) { return '' }
	$trimmed = $value.Trim()
	if ($trimmed -notmatch '^data:(?<media>image/[A-Za-z0-9.+-]+);base64,(?<data>.+)$') { return '' }
	$mediaType = $Matches['media'].ToLowerInvariant()
	try {
		$bytes = [Convert]::FromBase64String($Matches['data'])
	} catch {
		return ''
	}
	$fileName = 'clipboard.png'
	if ($mediaType -eq 'image/jpeg') { $fileName = 'clipboard.jpg' }
	elseif ($mediaType -eq 'image/webp') { $fileName = 'clipboard.webp' }
	elseif ($mediaType -eq 'image/gif') { $fileName = 'clipboard.gif' }
	return (Build-Payload $mediaType $fileName $bytes)
}

function Payload-FromImageUrl([string]$value) {
	if ([string]::IsNullOrWhiteSpace($value)) { return '' }
	$urlText = $value.Trim()
	if ($urlText -like 'file://*') {
		try {
			$uri = [System.Uri]$urlText
			return (Payload-FromPath $uri.LocalPath)
		} catch {
			return ''
		}
	}
	if ($urlText -notmatch '^https?://') { return '' }
	try {
		$uri = [System.Uri]$urlText
	} catch {
		return ''
	}
	$mediaType = Get-MediaTypeFromExtension $uri.AbsolutePath
	if ($mediaType -eq '') { return '' }
	try {
		$wc = New-Object System.Net.WebClient
		$bytes = $wc.DownloadData($uri)
		$fileName = [System.IO.Path]::GetFileName($uri.AbsolutePath)
		if ([string]::IsNullOrWhiteSpace($fileName)) {
			$fileName = 'clipboard' + [System.IO.Path]::GetExtension($uri.AbsolutePath)
		}
		return (Build-Payload $mediaType $fileName $bytes)
	} catch {
		return ''
	}
}

function Payload-FromHtml([string]$html) {
	if ([string]::IsNullOrWhiteSpace($html)) { return '' }
	$pattern = '(?is)<img[^>]+src\s*=\s*["''](?<src>[^"''>]+)["'']'
	$match = [System.Text.RegularExpressions.Regex]::Match($html, $pattern)
	if (-not $match.Success) { return '' }
	$src = [System.Net.WebUtility]::HtmlDecode($match.Groups['src'].Value)
	$payload = Payload-FromDataUri $src
	if (-not [string]::IsNullOrWhiteSpace($payload)) { return $payload }
	$payload = Payload-FromPath $src
	if (-not [string]::IsNullOrWhiteSpace($payload)) { return $payload }
	$payload = Payload-FromImageUrl $src
	if (-not [string]::IsNullOrWhiteSpace($payload)) { return $payload }
	return ''
}

$dataObj = [System.Windows.Forms.Clipboard]::GetDataObject()
if ($null -eq $dataObj) { return '' }
$payload = ''

if ($dataObj.GetDataPresent('PNG')) {
	$png = $dataObj.GetData('PNG')
	$pngBytes = $null
	if ($png -is [System.IO.MemoryStream]) { $pngBytes = $png.ToArray() }
	elseif ($png -is [byte[]]) { $pngBytes = $png }
	elseif ($png -is [System.IO.Stream]) {
		$ms = New-Object System.IO.MemoryStream
		$png.CopyTo($ms)
		$pngBytes = $ms.ToArray()
	}
	if ($null -ne $pngBytes -and $pngBytes.Length -gt 0) {
		$payload = Build-Payload 'image/png' 'clipboard.png' $pngBytes
		if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
	}
}

if ($dataObj.GetDataPresent([System.Windows.Forms.DataFormats]::FileDrop)) {
	$files = $dataObj.GetData([System.Windows.Forms.DataFormats]::FileDrop)
	$fileList = @()
	if ($files -is [System.Collections.Specialized.StringCollection]) { $fileList = @($files | ForEach-Object { $_ }) }
	elseif ($files -is [string[]]) { $fileList = $files }
	elseif ($files -is [string]) { $fileList = @($files) }
	foreach ($path in $fileList) {
		$payload = Payload-FromPath $path
		if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
	}
}

if ([System.Windows.Forms.Clipboard]::ContainsImage()) {
	$img = [System.Windows.Forms.Clipboard]::GetImage()
	if ($null -ne $img) {
		try {
			$ms = New-Object System.IO.MemoryStream
			$img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
			$payload = Build-Payload 'image/png' 'clipboard.png' $ms.ToArray()
			if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
		} finally {
			$img.Dispose()
		}
	}
}

if ($dataObj.GetDataPresent([System.Windows.Forms.DataFormats]::Text)) {
	$text = [System.Windows.Forms.Clipboard]::GetText()
	$payload = Payload-FromDataUri $text
	if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
	$payload = Payload-FromPath $text
	if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
	$payload = Payload-FromImageUrl $text
	if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
}

if ($dataObj.GetDataPresent([System.Windows.Forms.DataFormats]::Html)) {
	$html = $dataObj.GetData([System.Windows.Forms.DataFormats]::Html).ToString()
	$payload = Payload-FromHtml $html
	if (-not [string]::IsNullOrWhiteSpace($payload)) { $payload; return }
}

return ''
`)

	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", script).CombinedOutput()
	if err != nil {
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, fmt.Errorf("clipboard image read failed: %s", strings.TrimSpace(string(out))))
	}
	mediaType, data, fileName, parseErr := parseClipboardImageOutput(string(out))
	if parseErr != nil {
		if strings.Contains(strings.ToLower(parseErr.Error()), "no image") {
			return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, parseErr)
		}
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeImageDecodeFailed, parseErr)
	}
	if len(data) == 0 {
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeImageDecodeFailed, fmt.Errorf("clipboard image is empty"))
	}
	if strings.TrimSpace(mediaType) == "" {
		mediaType = "image/png"
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "clipboard.png"
	}
	return mediaType, data, fileName, nil
}

func parseClipboardImageOutput(raw string) (string, []byte, string, error) {
	encoded := normalizeClipboardOutput(raw)
	if encoded == "" {
		return "", nil, "", llm.WrapError("clipboard", llm.ErrorCodeClipboardUnavailable, fmt.Errorf("clipboard has no image"))
	}

	type payload struct {
		MediaType string `json:"media_type"`
		FileName  string `json:"file_name"`
		Data      string `json:"data"`
	}
	if strings.HasPrefix(encoded, "{") {
		var p payload
		if err := json.Unmarshal([]byte(encoded), &p); err == nil && strings.TrimSpace(p.Data) != "" {
			decoded, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(p.Data))
			if decodeErr != nil {
				return "", nil, "", decodeErr
			}
			return strings.TrimSpace(p.MediaType), decoded, strings.TrimSpace(p.FileName), nil
		}
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, "", err
	}
	return "image/png", data, "clipboard.png", nil
}

func normalizeClipboardOutput(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func nextSessionImageID(sess *session.Session) int {
	if sess == nil {
		return 1
	}
	maxID := 0
	for _, meta := range sess.Conversation.Assets.Images {
		if meta.ImageID > maxID {
			maxID = meta.ImageID
		}
	}
	if maxID < 0 {
		return 1
	}
	return maxID + 1
}

func (m *model) ensureSessionImageAssets() {
	if m == nil || m.sess == nil {
		return
	}
	if m.sess.Conversation.Assets.Images == nil {
		m.sess.Conversation.Assets.Images = make(map[llm.AssetID]session.ImageAssetMeta, 8)
	}
}

func (m *model) applyInputImagePipeline(before, after, source string) (string, string) {
	class, prefix, inserted, suffix := classifyInputMutation(before, after, source)
	if class != inputMutationPasteFilled {
		return after, ""
	}

	paths := extractImagePathsFromChunk(inserted, m.workspace)
	if len(paths) > 0 {
		placeholders := make([]string, 0, len(paths))
		notes := make([]string, 0, len(paths))
		for _, path := range paths {
			placeholder, note, ok := m.ingestImageFromPath(path)
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

		replacement := strings.Join(placeholders, " ")
		updated := after[:prefix] + replacement + after[len(after)-suffix:]
		m.syncInputImageRefs(updated)
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
		placeholder, note, ok := m.ingestImageFromPath(span.Path)
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
	m.syncInputImageRefs(updated)
	note := fmt.Sprintf("Attached %d image(s): %s", len(attached), strings.Join(attached, ", "))
	if len(notes) > 0 {
		note += "; " + notes[0]
	}
	return updated, note
}

func (m *model) applyWholeInputImagePathFallback(text, source string) (string, string) {
	if strings.TrimSpace(text) == "" {
		return text, ""
	}
	pasteLike := isCtrlVKey(source) || strings.Contains(strings.ToLower(source), "paste")
	if !pasteLike {
		if (m.lastPasteAt.IsZero() || time.Since(m.lastPasteAt) > 2*pasteSubmitGuard) &&
			!(m.inputBurstSize >= 4 && isLikelyPathInput(strings.TrimSpace(text))) {
			return text, ""
		}
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
		placeholder, note, ok := m.ingestImageFromPath(span.Path)
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
	m.syncInputImageRefs(updated)
	note := fmt.Sprintf("Attached %d image(s): %s", len(attached), strings.Join(attached, ", "))
	if len(notes) > 0 {
		note += "; " + notes[0]
	}
	return updated, note
}

func (m *model) handleEmptyClipboardPaste() string {
	if m == nil || m.clipboard == nil {
		return "Clipboard image is unavailable in current environment."
	}
	mediaType, data, fileName, err := m.clipboard.ReadImage(context.Background())
	if err != nil {
		return err.Error()
	}
	placeholder, note, ok := m.ingestImageBinary(mediaType, fileName, data)
	if !ok {
		return note
	}

	current := m.input.Value()
	updated := placeholder
	if strings.TrimSpace(current) != "" {
		updated = current + " " + placeholder
	}
	m.setInputValue(updated)
	m.syncInputImageRefs(updated)
	if note != "" {
		return note
	}
	return "Attached image from clipboard: " + placeholder
}

func (m *model) ingestImageFromPath(path string) (string, string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Sprintf("failed to read image %s: %v", path, err), false
	}
	mediaType, ok := mediaTypeFromPath(path)
	if !ok {
		return "", fmt.Sprintf("unsupported image format: %s", filepath.Ext(path)), false
	}
	return m.ingestImageBinary(mediaType, filepath.Base(path), data)
}

func (m *model) ingestImageBinary(mediaType, fileName string, data []byte) (string, string, bool) {
	if m == nil || m.sess == nil {
		return "", "image ingest failed: session unavailable", false
	}
	if m.imageStore == nil {
		return "", "image ingest failed: image store unavailable", false
	}
	if len(data) == 0 {
		return "", "image ingest failed: empty image payload", false
	}

	m.ensureSessionImageAssets()
	if m.nextImageID <= 0 {
		m.nextImageID = nextSessionImageID(m.sess)
	}
	imageID := m.nextImageID
	meta, err := m.imageStore.PutImage(context.Background(), assets.PutImageInput{
		SessionID: corepkg.SessionID(m.sess.ID),
		ImageID:   imageID,
		MediaType: mediaType,
		FileName:  fileName,
		Data:      data,
	})
	if err != nil {
		return "", err.Error(), false
	}

	assetID := meta.AssetID
	if strings.TrimSpace(string(assetID)) == "" {
		assetID = assets.AssetID(corepkg.SessionID(m.sess.ID), meta.ImageID)
	}
	m.sess.Conversation.Assets.Images[assetID] = session.ImageAssetMeta{
		ImageID:   meta.ImageID,
		MediaType: meta.MediaType,
		FileName:  meta.FileName,
		CachePath: meta.CachePath,
		ByteSize:  meta.ByteSize,
		Width:     meta.Width,
		Height:    meta.Height,
	}
	if m.store != nil {
		if err := m.store.Save(m.sess); err != nil {
			return "", err.Error(), false
		}
	}

	if m.inputImageRefs == nil {
		m.inputImageRefs = make(map[int]llm.AssetID, 8)
	}
	if m.orphanedImages == nil {
		m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	}
	m.inputImageRefs[meta.ImageID] = assetID
	delete(m.orphanedImages, assetID)
	m.nextImageID = meta.ImageID + 1

	return placeholderForImageID(meta.ImageID), "", true
}

func (m *model) syncInputImageRefs(text string) {
	if m == nil {
		return
	}
	if m.inputImageRefs == nil {
		m.inputImageRefs = make(map[int]llm.AssetID, 8)
	}
	if m.inputImageMentions == nil {
		m.inputImageMentions = make(map[string]llm.AssetID, 8)
	}
	if m.orphanedImages == nil {
		m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	}

	referencedIDs := extractImagePlaceholderIDs(text)
	referencedSet := make(map[int]struct{}, len(referencedIDs))
	referencedAssets := make(map[llm.AssetID]struct{}, len(referencedIDs))
	for _, id := range referencedIDs {
		referencedSet[id] = struct{}{}
		assetID, _, ok := m.findAssetByImageID(id)
		if ok {
			m.inputImageRefs[id] = assetID
			referencedAssets[assetID] = struct{}{}
			delete(m.orphanedImages, assetID)
		}
	}

	for id, assetID := range m.inputImageRefs {
		if _, ok := referencedSet[id]; ok {
			continue
		}
		delete(m.inputImageRefs, id)
		m.orphanedImages[assetID] = time.Now().UTC()
	}

	mentionRefs := extractMentionImageReferenceKeys(text)
	for key := range mentionRefs {
		assetID, ok := m.inputImageMentions[key]
		if !ok {
			continue
		}
		referencedAssets[assetID] = struct{}{}
		delete(m.orphanedImages, assetID)
	}
	for key, assetID := range m.inputImageMentions {
		if _, ok := mentionRefs[key]; ok {
			continue
		}
		delete(m.inputImageMentions, key)
		m.orphanedImages[assetID] = time.Now().UTC()
	}
}

func (m *model) buildPromptInput(raw string) (RunPromptInput, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RunPromptInput{}, "", fmt.Errorf("prompt is empty")
	}
	m.syncInputImageRefs(raw)
	resolvedRaw, err := m.resolvePromptPastedInput(raw)
	if err != nil {
		return RunPromptInput{}, "", err
	}

	placeholderMatches := imagePlaceholderPattern.FindAllStringSubmatchIndex(resolvedRaw, -1)
	mentionMatches := extractMentionImageSpans(resolvedRaw, m.inputImageMentions)
	if len(placeholderMatches) == 0 && len(mentionMatches) == 0 {
		assets := m.hydrateHistoricalRequestAssets(nil)
		return RunPromptInput{
			UserMessage: llm.NewUserTextMessage(resolvedRaw),
			Assets:      assets,
			DisplayText: raw,
		}, raw, nil
	}

	type imageSpan struct {
		Start    int
		End      int
		AssetID  llm.AssetID
		Fallback string
	}
	spans := make([]imageSpan, 0, len(placeholderMatches)+len(mentionMatches))
	for _, match := range placeholderMatches {
		start, end := match[0], match[1]
		idStart, idEnd := match[2], match[3]
		imageID, err := strconv.Atoi(raw[idStart:idEnd])
		if err != nil {
			continue
		}
		assetID, _, ok := m.findAssetByImageID(imageID)
		if !ok {
			spans = append(spans, imageSpan{
				Start:    start,
				End:      end,
				Fallback: fmt.Sprintf("[Image #%d unavailable]", imageID),
			})
			continue
		}
		spans = append(spans, imageSpan{
			Start:    start,
			End:      end,
			AssetID:  assetID,
			Fallback: fmt.Sprintf("[Image #%d unavailable]", imageID),
		})
	}
	for _, match := range mentionMatches {
		spans = append(spans, imageSpan{
			Start:    match.Start,
			End:      match.End,
			AssetID:  match.AssetID,
			Fallback: match.Raw,
		})
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start == spans[j].Start {
			return spans[i].End > spans[j].End
		}
		return spans[i].Start < spans[j].Start
	})

	filtered := make([]imageSpan, 0, len(spans))
	lastEnd := -1
	for _, span := range spans {
		if span.Start < lastEnd {
			continue
		}
		filtered = append(filtered, span)
		lastEnd = span.End
	}

	parts := make([]llm.Part, 0, len(filtered)*2+1)
	assetPayloads := make(map[llm.AssetID]llm.ImageAsset, len(filtered))
	appendTextPart := func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		parts = append(parts, llm.Part{Type: llm.PartText, Text: &llm.TextPart{Value: text}})
	}

	last := 0
	for _, span := range filtered {
		appendTextPart(resolvedRaw[last:span.Start])
		if m.imageStore == nil || strings.TrimSpace(string(span.AssetID)) == "" {
			appendTextPart(span.Fallback)
			last = span.End
			continue
		}
		blob, err := m.imageStore.GetImageByAssetID(context.Background(), corepkg.SessionID(m.sess.ID), span.AssetID)
		if err != nil {
			appendTextPart(span.Fallback)
			last = span.End
			continue
		}
		assetPayloads[span.AssetID] = llm.ImageAsset{MediaType: blob.MediaType, Data: blob.Data}
		parts = append(parts, llm.Part{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: span.AssetID}})
		last = span.End
	}
	appendTextPart(resolvedRaw[last:])

	if len(parts) == 0 {
		parts = append(parts, llm.Part{Type: llm.PartText, Text: &llm.TextPart{Value: resolvedRaw}})
	}

	userMessage := llm.Message{Role: llm.RoleUser, Parts: parts}
	userMessage.Normalize()
	if err := llm.ValidateMessage(userMessage); err != nil {
		return RunPromptInput{}, "", err
	}
	assetPayloads = m.hydrateHistoricalRequestAssets(assetPayloads)

	return RunPromptInput{
		UserMessage: userMessage,
		Assets:      assetPayloads,
		DisplayText: raw,
	}, raw, nil
}

func (m *model) hydrateHistoricalRequestAssets(current map[llm.AssetID]llm.ImageAsset) map[llm.AssetID]llm.ImageAsset {
	if m == nil || m.sess == nil || m.imageStore == nil {
		return current
	}
	imageAssetIDs := collectImageAssetIDsFromMessages(m.sess.Messages)
	if len(imageAssetIDs) == 0 {
		return current
	}
	if current == nil {
		current = make(map[llm.AssetID]llm.ImageAsset, len(imageAssetIDs))
	}
	for _, assetID := range imageAssetIDs {
		if strings.TrimSpace(string(assetID)) == "" {
			continue
		}
		if _, ok := current[assetID]; ok {
			continue
		}
		blob, err := m.imageStore.GetImageByAssetID(context.Background(), corepkg.SessionID(m.sess.ID), assetID)
		if err != nil || len(blob.Data) == 0 {
			continue
		}
		current[assetID] = llm.ImageAsset{
			MediaType: blob.MediaType,
			Data:      blob.Data,
		}
	}
	if len(current) == 0 {
		return nil
	}
	return current
}

func collectImageAssetIDsFromMessages(messages []llm.Message) []llm.AssetID {
	if len(messages) == 0 {
		return nil
	}
	seen := make(map[llm.AssetID]struct{}, 8)
	assetIDs := make([]llm.AssetID, 0, 8)
	for i := range messages {
		msg := messages[i]
		msg.Normalize()
		for _, part := range msg.Parts {
			if part.Type != llm.PartImageRef || part.Image == nil {
				continue
			}
			assetID := part.Image.AssetID
			if strings.TrimSpace(string(assetID)) == "" {
				continue
			}
			if _, ok := seen[assetID]; ok {
				continue
			}
			seen[assetID] = struct{}{}
			assetIDs = append(assetIDs, assetID)
		}
	}
	if len(assetIDs) == 0 {
		return nil
	}
	return assetIDs
}

func (m *model) findAssetByImageID(imageID int) (llm.AssetID, session.ImageAssetMeta, bool) {
	if m == nil || m.sess == nil {
		return "", session.ImageAssetMeta{}, false
	}
	for assetID, meta := range m.sess.Conversation.Assets.Images {
		if meta.ImageID == imageID {
			return assetID, meta, true
		}
	}
	return "", session.ImageAssetMeta{}, false
}

func classifyInputMutation(before, after, source string) (inputMutationClass, int, string, int) {
	prefix, inserted, suffix := insertionDiff(before, after)
	cleanInserted := strings.ReplaceAll(inserted, ctrlVMarkerRune, "")
	sourceTrimmed := strings.TrimSpace(source)
	pasteSignal := isCtrlVKey(source) || strings.Contains(strings.ToLower(source), "paste") || strings.Contains(cleanInserted, "\n")
	if !pasteSignal && strings.TrimSpace(cleanInserted) != "" {
		// Bubble Tea reports ordinary key-runes by echoing the inserted text in
		// msg.String(). Treat those as normal typing, even if the terminal batches
		// multiple runes in one event (for example IME commits).
		sameAsTypedChunk := sourceTrimmed != "" && sourceTrimmed == cleanInserted
		if !sameAsTypedChunk && shouldRecordPasteSignal(source) {
			pasteSignal = true
		}
	}
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
		if _, ok := mediaTypeFromPath(resolved); !ok {
			continue
		}
		paths = append(paths, resolved)
	}

	if candidateCount == 0 || len(paths) != candidateCount {
		return nil
	}
	return paths
}

func splitPathTokens(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	result := make([]string, 0, 8)
	var b strings.Builder
	quote := rune(0)
	for _, r := range raw {
		switch {
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '\n' || r == '\r' || r == '\t' || r == ' '):
			if b.Len() > 0 {
				result = append(result, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		result = append(result, b.String())
	}
	return result
}

func mediaTypeFromPath(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".webp":
		return "image/webp", true
	case ".gif":
		return "image/gif", true
	default:
		return "", false
	}
}

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

type mentionImageSpan struct {
	Start   int
	End     int
	AssetID llm.AssetID
	Raw     string
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
		spans = append(spans, mentionImageSpan{
			Start:   start,
			End:     end,
			AssetID: assetID,
			Raw:     text[start:end],
		})
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

func normalizeImageMentionPath(path string) string {
	path = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(path), "@"))
	if path == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	cleaned = filepath.ToSlash(cleaned)
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "." || cleaned == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned
}

type imagePathSpan struct {
	Start int
	End   int
	Path  string
}

type imagePlaceholderSpan struct {
	Start int
	End   int
}

func extractInlineImagePathSpans(chunk string) []imagePathSpan {
	chunk = strings.TrimSpace(chunk)
	if chunk == "" {
		return nil
	}

	matches := make([]imagePathSpan, 0, 4)
	appendMatches := func(pattern *regexp.Regexp) {
		for _, loc := range pattern.FindAllStringIndex(chunk, -1) {
			if len(loc) != 2 || loc[1] <= loc[0] {
				continue
			}
			raw := chunk[loc[0]:loc[1]]
			resolved := filepath.Clean(raw)
			info, err := os.Stat(resolved)
			if err != nil || info.IsDir() {
				continue
			}
			if _, ok := mediaTypeFromPath(resolved); !ok {
				continue
			}
			matches = append(matches, imagePathSpan{
				Start: loc[0],
				End:   loc[1],
				Path:  resolved,
			})
		}
	}
	appendMatches(inlineWindowsImagePathPattern)
	appendMatches(inlineUnixImagePathPattern)

	if len(matches) == 0 {
		return nil
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Start == matches[j].Start {
			return matches[i].End < matches[j].End
		}
		return matches[i].Start < matches[j].Start
	})
	filtered := make([]imagePathSpan, 0, len(matches))
	lastEnd := -1
	for _, span := range matches {
		if span.Start < lastEnd {
			continue
		}
		filtered = append(filtered, span)
		lastEnd = span.End
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func isImagePlaceholderDeletionSource(source string) bool {
	key := normalizeKeyName(source)
	return key == "backspace" || key == "delete" || key == "ctrl+h" || key == "ctrl+d"
}

func extractImagePlaceholderSpans(value string) []imagePlaceholderSpan {
	matches := imagePlaceholderPattern.FindAllStringIndex(value, -1)
	if len(matches) == 0 {
		return nil
	}
	spans := make([]imagePlaceholderSpan, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 || match[0] >= match[1] {
			continue
		}
		spans = append(spans, imagePlaceholderSpan{Start: match[0], End: match[1]})
	}
	return spans
}

func overlapsRange(startA, endA, startB, endB int) bool {
	return startA < endB && startB < endA
}

func removeImagePlaceholderSpans(value string, spans []imagePlaceholderSpan) string {
	if len(spans) == 0 {
		return value
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start == spans[j].Start {
			return spans[i].End < spans[j].End
		}
		return spans[i].Start < spans[j].Start
	})

	var b strings.Builder
	cursor := 0
	for _, span := range spans {
		start := clamp(span.Start, 0, len(value))
		end := clamp(span.End, start, len(value))
		if start < cursor {
			continue
		}
		if start > cursor {
			b.WriteString(value[cursor:start])
		}
		cursor = end
	}
	if cursor < len(value) {
		b.WriteString(value[cursor:])
	}
	return b.String()
}

func (m *model) protectImagePlaceholderDeletion(before, after, source string) (string, bool) {
	if m == nil || !isImagePlaceholderDeletionSource(source) {
		return after, false
	}
	if before == after || len(after) >= len(before) {
		return after, false
	}
	if len(before)-len(after) != 1 {
		return after, false
	}
	if !strings.Contains(before, "[Image #") {
		return after, false
	}

	prefix, _, suffix := insertionDiff(before, after)
	affectedStart := clamp(prefix, 0, len(before))
	affectedEnd := clamp(len(before)-suffix, affectedStart, len(before))
	if affectedEnd <= affectedStart {
		return after, false
	}

	spans := extractImagePlaceholderSpans(before)
	if len(spans) == 0 {
		return after, false
	}

	toRemove := make([]imagePlaceholderSpan, 0, 1)
	for _, span := range spans {
		if overlapsRange(affectedStart, affectedEnd, span.Start, span.End) {
			toRemove = append(toRemove, span)
		}
	}
	if len(toRemove) == 0 {
		return after, false
	}

	locked := removeImagePlaceholderSpans(before, toRemove)
	return locked, locked != after
}
