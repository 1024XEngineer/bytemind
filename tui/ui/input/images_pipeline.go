package tui

import (
	"fmt"
	"strings"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/llm"
	tuiapi "bytemind/internal/tui/api"
	tuiruntime "bytemind/internal/tui/runtime"
)

func (m *model) applyInputImagePipeline(before, after, source string) (string, string) {
	controller := m.imageInputService()
	if m == nil || controller == nil {
		return after, ""
	}
	updated, note := controller.ProcessMutation(before, after, source, m.workspace, m.ingestImageFromPath)
	if updated != after {
		m.syncInputImageRefs(updated)
	}
	return updated, note
}

func (m *model) applyWholeInputImagePathFallback(text, source string) (string, string) {
	controller := m.imageInputService()
	if m == nil || controller == nil {
		return text, ""
	}
	updated, note := controller.ProcessWholeInputFallback(text, source, m.lastPasteAt, pasteSubmitGuard, m.ingestImageFromPath)
	if updated != text {
		m.syncInputImageRefs(updated)
	}
	return updated, note
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
		ref, ok := m.runtimeAPI().FindSessionAssetByImageID(m.sess, id)
		if ok {
			m.inputImageRefs[id] = ref.AssetID
			referencedAssets[ref.AssetID] = struct{}{}
			delete(m.orphanedImages, ref.AssetID)
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

func (m *model) buildPromptInput(raw string) (agent.RunPromptInput, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return agent.RunPromptInput{}, "", fmt.Errorf("prompt is empty")
	}
	m.syncInputImageRefs(raw)
	builder := m.promptBuildService()
	if builder == nil {
		return agent.RunPromptInput{}, "", fmt.Errorf("prompt builder unavailable")
	}
	result := builder.Build(tuiapi.PromptBuildRequest{
		RawInput:        raw,
		MentionBindings: m.inputImageMentions,
	}, tuiruntime.PastedState{
		NextID:   m.nextPasteID,
		Order:    append([]string(nil), m.pastedOrder...),
		Contents: clonePastedContents(m.pastedContents),
	})
	if !result.Success {
		return agent.RunPromptInput{}, "", fmt.Errorf("%s", result.Error)
	}
	return result.Data.Prompt, result.Data.DisplayText, nil
}

func clonePastedContents(contents map[string]pastedContent) map[string]tuiruntime.PastedContent {
	if len(contents) == 0 {
		return map[string]tuiruntime.PastedContent{}
	}
	cloned := make(map[string]tuiruntime.PastedContent, len(contents))
	for id, content := range contents {
		cloned[id] = tuiruntime.PastedContent(content)
	}
	return cloned
}
