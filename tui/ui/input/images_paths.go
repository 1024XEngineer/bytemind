package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

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
			matches = append(matches, imagePathSpan{Start: loc[0], End: loc[1], Path: resolved})
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
