package storage

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"strings"
	"unicode"
)

func WorkspaceProjectID(workspace string) string {
	value := strings.TrimSpace(workspace)
	if value == "" {
		return "-unknown-project"
	}
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	value = filepath.Clean(value)
	// Keep project-id normalization stable across CI/OS by lowercasing paths.
	value = strings.ToLower(value)
	value = filepath.ToSlash(value)

	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		}
	}

	id := strings.Trim(builder.String(), "-")
	if id == "" {
		id = "unknown-project"
	}
	if len(id) > 96 {
		sum := sha1.Sum([]byte(value))
		id = id[:80] + "-" + hex.EncodeToString(sum[:4])
	}
	return "-" + id
}
