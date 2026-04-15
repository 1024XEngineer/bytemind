package tui

import (
	"path/filepath"
	"strings"
)

func mediaTypeFromPath(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
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
