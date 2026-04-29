package services

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/agent"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
	tuiapi "github.com/1024XEngineer/bytemind/tui/api"
	tuiruntime "github.com/1024XEngineer/bytemind/tui/runtime"
)

var (
	builderImagePlaceholderPattern = regexp.MustCompile(`\[Image #(\d+)\]`)
	builderImageMentionPattern     = regexp.MustCompile(`(?i)@([^\s@]+?\.(?:png|jpe?g|webp|gif))`)
)

type PromptBuilder struct {
	api  tuiruntime.UIAPI
	sess *session.Session
}

func NewPromptBuilder(api tuiruntime.UIAPI, sess *session.Session) *PromptBuilder {
	return &PromptBuilder{api: api, sess: sess}
}

func (b *PromptBuilder) BindSession(sess *session.Session) {
	b.sess = sess
}

func (b *PromptBuilder) Build(req tuiapi.PromptBuildRequest, pasted tuiruntime.PastedState) tuiapi.Result[tuiapi.PromptBuildResult] {
	raw := strings.TrimSpace(req.RawInput)
	if raw == "" {
		return tuiapi.Invalid[tuiapi.PromptBuildResult]("prompt is empty")
	}
	if b == nil || b.api == nil {
		return tuiapi.Unavailable[tuiapi.PromptBuildResult]("prompt builder")
	}
	resolvedRaw, err := tuiruntime.ResolvePastedLineReference(raw, pasted)
	if err != nil {
		return tuiapi.FailCode[tuiapi.PromptBuildResult](tuiapi.ErrorCodeInvalid, err.Error())
	}

	placeholderMatches := builderImagePlaceholderPattern.FindAllStringSubmatchIndex(resolvedRaw, -1)
	mentionMatches := extractMentionImageSpans(resolvedRaw, req.MentionBindings)
	if len(placeholderMatches) == 0 && len(mentionMatches) == 0 {
		assets := b.hydrateHistoricalRequestAssets(nil)
		return tuiapi.Ok(tuiapi.PromptBuildResult{
			Prompt: agent.RunPromptInput{
				UserMessage: llm.NewUserTextMessage(resolvedRaw),
				Assets:      assets,
				DisplayText: raw,
			},
			DisplayText: raw,
		})
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
		imageID, err := strconv.Atoi(resolvedRaw[idStart:idEnd])
		if err != nil {
			continue
		}
		ref, ok := b.api.FindSessionAssetByImageID(b.sess, imageID)
		if !ok {
			spans = append(spans, imageSpan{Start: start, End: end, Fallback: fmt.Sprintf("[Image #%d unavailable]", imageID)})
			continue
		}
		spans = append(spans, imageSpan{Start: start, End: end, AssetID: ref.AssetID, Fallback: fmt.Sprintf("[Image #%d unavailable]", imageID)})
	}
	for _, match := range mentionMatches {
		spans = append(spans, imageSpan{Start: match.Start, End: match.End, AssetID: match.AssetID, Fallback: match.Raw})
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
		if strings.TrimSpace(string(span.AssetID)) == "" {
			appendTextPart(span.Fallback)
			last = span.End
			continue
		}
		blob, err := b.api.LoadSessionImageAsset(b.sess, string(span.AssetID))
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
		return tuiapi.FailCode[tuiapi.PromptBuildResult](tuiapi.ErrorCodeInvalid, err.Error())
	}
	assetPayloads = b.hydrateHistoricalRequestAssets(assetPayloads)
	return tuiapi.Ok(tuiapi.PromptBuildResult{
		Prompt: agent.RunPromptInput{
			UserMessage: userMessage,
			Assets:      assetPayloads,
			DisplayText: raw,
		},
		DisplayText: raw,
	})
}

func (b *PromptBuilder) hydrateHistoricalRequestAssets(current map[llm.AssetID]llm.ImageAsset) map[llm.AssetID]llm.ImageAsset {
	if b == nil || b.api == nil {
		return current
	}
	converted := make(map[string]tuiruntime.ImagePayload, len(current))
	for assetID, payload := range current {
		converted[string(assetID)] = tuiruntime.ImagePayload{MediaType: payload.MediaType, Data: payload.Data}
	}
	hydrated := b.api.HydrateHistoricalAssets(b.sess, converted)
	if len(hydrated) == 0 {
		return nil
	}
	result := make(map[llm.AssetID]llm.ImageAsset, len(hydrated))
	for assetID, payload := range hydrated {
		result[llm.AssetID(assetID)] = llm.ImageAsset{MediaType: payload.MediaType, Data: payload.Data}
	}
	return result
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
	matches := builderImageMentionPattern.FindAllStringSubmatchIndex(text, -1)
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

func normalizeImageMentionPath(path string) string {
	path = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(path), "@"))
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "./")
	if path == "." || path == "" {
		return ""
	}
	return strings.ToLower(path)
}
