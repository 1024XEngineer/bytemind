package tui

import (
	"os"
	"path/filepath"
	"strings"

	"bytemind/internal/llm"
)

func (m *model) ingestMentionImageCandidate(path string) (assetID llm.AssetID, note string, isImage bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", false
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.workspace, resolved)
	}
	resolved = filepath.Clean(resolved)

	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return "", "", false
	}
	if _, ok := mediaTypeFromPath(resolved); !ok {
		return "", "", false
	}

	placeholder, note, ok := m.ingestImageFromPath(resolved)
	if !ok {
		return "", note, true
	}
	imageID, ok := imageIDFromPlaceholder(placeholder)
	if !ok {
		return "", "image ingest failed: invalid placeholder id", true
	}
	assetID, _, ok = m.findAssetByImageID(imageID)
	if !ok {
		return "", "image ingest failed: asset metadata missing", true
	}
	return assetID, note, true
}
