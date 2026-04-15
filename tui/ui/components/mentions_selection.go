package tui

import (
	"path/filepath"
	"strings"
	"time"

	"bytemind/internal/llm"
	"bytemind/internal/mention"
)

func (m *model) recordRecentMention(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	if m.mentionRecent == nil {
		m.mentionRecent = make(map[string]int, 16)
	}
	m.mentionSeq++
	m.mentionRecent[path] = m.mentionSeq
}

func (m model) hasRecentMention(path string) bool {
	if m.mentionRecent == nil {
		return false
	}
	return m.mentionRecent[path] > 0
}

func (m *model) applyMentionSelection(selected mention.Candidate) {
	m.recordRecentMention(selected.Path)

	if assetID, note, isImage := m.ingestMentionImageCandidate(selected.Path); isImage {
		if strings.TrimSpace(string(assetID)) != "" {
			m.bindMentionImageAsset(selected.Path, assetID)
			nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
			m.setInputValue(nextValue)
			if strings.TrimSpace(note) != "" {
				m.statusNote = note
			} else {
				m.statusNote = "Attached image: @" + filepath.ToSlash(strings.TrimSpace(selected.Path))
			}
			m.closeMentionPalette()
			m.syncInputOverlays()
			return
		}
		if strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
	}

	nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
	m.setInputValue(nextValue)
	m.statusNote = "Inserted mention: " + selected.Path
	m.closeMentionPalette()
	m.syncInputOverlays()
}

func (m *model) bindMentionImageAsset(path string, assetID llm.AssetID) {
	if m == nil {
		return
	}
	key := normalizeImageMentionPath(path)
	if key == "" || strings.TrimSpace(string(assetID)) == "" {
		return
	}
	if m.inputImageMentions == nil {
		m.inputImageMentions = make(map[string]llm.AssetID, 8)
	}
	if m.orphanedImages == nil {
		m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	}
	if prev, ok := m.inputImageMentions[key]; ok && prev != assetID {
		m.orphanedImages[prev] = time.Now().UTC()
	}
	m.inputImageMentions[key] = assetID
	delete(m.orphanedImages, assetID)
}
