package tui

import (
	"context"
	"time"

	"bytemind/internal/llm"
	"bytemind/internal/session"
	tuiruntime "bytemind/internal/tui/runtime"
)

func (m *model) ensureSessionImageAssets() {
	if m == nil {
		return
	}
	m.runtimeAPI().EnsureSessionImageAssets(m.sess)
}

func (m *model) handleEmptyClipboardPaste() string {
	if m == nil || m.clipboard == nil {
		return "Clipboard image is unavailable in current environment."
	}
	mediaType, data, fileName, err := m.clipboard.ReadImage(context.Background())
	if err != nil {
		return err.Error()
	}
	controller := m.imageInputService()
	if controller == nil {
		return "Clipboard image is unavailable in current environment."
	}
	updated, note := controller.AttachClipboard(m.input.Value(), mediaType, fileName, data, m.ingestImageBinary)
	if updated != m.input.Value() {
		m.setInputValue(updated)
		m.syncInputImageRefs(updated)
	}
	return note
}

func (m *model) ingestImageFromPath(path string) (string, string, bool) {
	if m == nil {
		return "", "image ingest failed: session unavailable", false
	}
	if m.imageStore == nil {
		return "", "image ingest failed: image store unavailable", false
	}
	m.runtimeAPI().EnsureSessionImageAssets(m.sess)
	if m.nextImageID <= 0 {
		m.nextImageID = m.runtimeAPI().NextSessionImageID(m.sess)
	}
	meta, err := m.runtimeAPI().PutSessionImageFromPath(m.sess, m.nextImageID, path)
	if err != nil {
		return "", err.Error(), false
	}
	if m.inputImageRefs == nil {
		m.inputImageRefs = make(map[int]llm.AssetID, 8)
	}
	if m.orphanedImages == nil {
		m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	}
	m.inputImageRefs[meta.ImageID] = meta.AssetID
	delete(m.orphanedImages, meta.AssetID)
	m.nextImageID = meta.ImageID + 1
	return placeholderForImageID(meta.ImageID), "", true
}

func (m *model) ingestImageBinary(mediaType, fileName string, data []byte) (string, string, bool) {
	if m == nil {
		return "", "image ingest failed: session unavailable", false
	}
	if m.imageStore == nil {
		return "", "image ingest failed: image store unavailable", false
	}
	m.runtimeAPI().EnsureSessionImageAssets(m.sess)
	if m.nextImageID <= 0 {
		m.nextImageID = m.runtimeAPI().NextSessionImageID(m.sess)
	}
	meta, err := m.runtimeAPI().PutSessionImage(m.sess, m.nextImageID, mediaType, fileName, data)
	if err != nil {
		return "", err.Error(), false
	}
	if m.inputImageRefs == nil {
		m.inputImageRefs = make(map[int]llm.AssetID, 8)
	}
	if m.orphanedImages == nil {
		m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	}
	m.inputImageRefs[meta.ImageID] = meta.AssetID
	delete(m.orphanedImages, meta.AssetID)
	m.nextImageID = meta.ImageID + 1
	return placeholderForImageID(meta.ImageID), "", true
}

func (m *model) hydrateHistoricalRequestAssets(current map[llm.AssetID]llm.ImageAsset) map[llm.AssetID]llm.ImageAsset {
	if m == nil {
		return current
	}
	converted := make(map[string]tuiruntime.ImagePayload, len(current))
	for assetID, payload := range current {
		converted[string(assetID)] = tuiruntime.ImagePayload{MediaType: payload.MediaType, Data: payload.Data}
	}
	hydrated := m.runtimeAPI().HydrateHistoricalAssets(m.sess, converted)
	if len(hydrated) == 0 {
		return nil
	}
	result := make(map[llm.AssetID]llm.ImageAsset, len(hydrated))
	for assetID, payload := range hydrated {
		result[llm.AssetID(assetID)] = llm.ImageAsset{MediaType: payload.MediaType, Data: payload.Data}
	}
	return result
}

func (m *model) findAssetByImageID(imageID int) (llm.AssetID, session.ImageAssetMeta, bool) {
	if m == nil {
		return "", session.ImageAssetMeta{}, false
	}
	ref, ok := m.runtimeAPI().FindSessionAssetByImageID(m.sess, imageID)
	if !ok {
		return "", session.ImageAssetMeta{}, false
	}
	return ref.AssetID, ref.Meta, true
}
