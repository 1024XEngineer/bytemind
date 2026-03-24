package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type Proposal struct {
	NewContent string
	Summary    string
	Noop       bool
}

type Provider interface {
	ProposeChange(task, targetPath string, content []byte) (Proposal, error)
}

type StubProvider struct{}

var (
	replacePatternEn = regexp.MustCompile(`(?i)replace\s+"([^"]+)"\s+with\s+"([^"]+)"`)
	replacePatternZh = regexp.MustCompile(`把\s*"([^"]+)"\s*替换为\s*"([^"]+)"`)
	replacePlainEn   = regexp.MustCompile(`(?i)replace\s+([^\r\n]+?)\s+with\s+([^\r\n]+)$`)
	replacePlainZh   = regexp.MustCompile(`把\s*([^\r\n]+?)\s*替换为\s*([^\r\n]+)$`)
	appendPattern    = regexp.MustCompile(`(?i)append\s+"([^"]+)"`)
)

func (p StubProvider) ProposeChange(task, targetPath string, content []byte) (Proposal, error) {
	current := string(content)

	if oldValue, newValue, ok := parseReplaceInstruction(task, targetPath); ok {
		if isReplacementAlreadySatisfied(current, oldValue, newValue) {
			return Proposal{
				NewContent: current,
				Summary:    fmt.Sprintf("目标文件已经满足 %q -> %q 的替换要求，无需重复修改。", oldValue, newValue),
				Noop:       true,
			}, nil
		}

		if !strings.Contains(current, oldValue) {
			return Proposal{}, fmt.Errorf("target file does not contain %q", oldValue)
		}

		updated := strings.Replace(current, oldValue, newValue, 1)
		if updated == current {
			return Proposal{
				NewContent: current,
				Summary:    fmt.Sprintf("目标文件看起来已经满足 %q -> %q 的替换要求，无需重复修改。", oldValue, newValue),
				Noop:       true,
			}, nil
		}

		return Proposal{
			NewContent: updated,
			Summary:    fmt.Sprintf("已按任务要求把 %q 替换为 %q。", oldValue, newValue),
		}, nil
	}

	if snippet, ok := parseAppendInstruction(task); ok {
		return appendSnippet(targetPath, current, snippet)
	}

	return appendFallback(targetPath, current, task)
}

func parseReplaceInstruction(task, targetPath string) (string, string, bool) {
	for _, pattern := range []*regexp.Regexp{replacePatternEn, replacePatternZh} {
		matches := pattern.FindStringSubmatch(task)
		if len(matches) == 3 {
			return sanitizeReplaceValue(matches[1]), sanitizeReplaceValue(matches[2]), true
		}
	}

	for _, pattern := range []*regexp.Regexp{replacePlainEn, replacePlainZh} {
		matches := pattern.FindStringSubmatch(strings.TrimSpace(task))
		if len(matches) == 3 {
			oldValue := sanitizeReplaceValue(matches[1])
			newValue := sanitizeReplaceValue(matches[2])
			newValue = trimTrailingFileHint(newValue, targetPath)
			if oldValue != "" && newValue != "" {
				return oldValue, newValue, true
			}
		}
	}

	return "", "", false
}

func parseAppendInstruction(task string) (string, bool) {
	matches := appendPattern.FindStringSubmatch(task)
	if len(matches) == 2 {
		return matches[1], true
	}
	return "", false
}

func appendSnippet(targetPath, current, snippet string) (Proposal, error) {
	switch strings.ToLower(filepath.Ext(targetPath)) {
	case ".go":
		comment := "// " + sanitizeSingleLine(snippet)
		if strings.Contains(current, comment) {
			return Proposal{NewContent: current, Summary: "目标注释已存在，无需重复追加。", Noop: true}, nil
		}
		return Proposal{
			NewContent: ensureTrailingNewline(current) + comment + "\n",
			Summary:    fmt.Sprintf("已向 %s 追加一条 Go 注释。", targetPath),
		}, nil
	case ".md", ".txt", ".yaml", ".yml":
		if strings.Contains(current, snippet) {
			return Proposal{NewContent: current, Summary: "目标文本已存在，无需重复追加。", Noop: true}, nil
		}
		return Proposal{
			NewContent: ensureTrailingNewline(current) + snippet + "\n",
			Summary:    fmt.Sprintf("已向 %s 追加一段文本。", targetPath),
		}, nil
	default:
		return Proposal{}, errors.New("append instruction is only supported for .go/.md/.txt/.yaml/.yml files in the stub provider")
	}
}

func appendFallback(targetPath, current, task string) (Proposal, error) {
	switch strings.ToLower(filepath.Ext(targetPath)) {
	case ".go":
		comment := "// forgecli demo change: " + sanitizeSingleLine(task)
		if strings.Contains(current, comment) {
			return Proposal{NewContent: current, Summary: "相同的 demo 注释已存在，无需重复追加。", Noop: true}, nil
		}
		return Proposal{
			NewContent: ensureTrailingNewline(current) + comment + "\n",
			Summary:    "追加了一条 demo 注释，便于演示最小闭环。",
		}, nil
	case ".md", ".txt":
		block := "[forgecli demo] " + sanitizeSingleLine(task)
		if strings.Contains(current, block) {
			return Proposal{NewContent: current, Summary: "相同的 demo 文本已存在，无需重复追加。", Noop: true}, nil
		}
		return Proposal{
			NewContent: ensureTrailingNewline(current) + block + "\n",
			Summary:    "追加了一条 demo 文本，便于演示最小闭环。",
		}, nil
	default:
		return Proposal{}, errors.New("stub provider needs an explicit replace instruction for this file type")
	}
}

func sanitizeReplaceValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func trimTrailingFileHint(value, targetPath string) string {
	trimmed := sanitizeReplaceValue(value)
	lower := strings.ToLower(trimmed)

	suffixes := []string{
		" in " + strings.ToLower(filepath.Base(targetPath)),
		" in " + strings.ToLower(filepath.ToSlash(targetPath)),
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, suffix) {
			return strings.TrimSpace(trimmed[:len(trimmed)-len(suffix)])
		}
	}
	return trimmed
}

func isReplacementAlreadySatisfied(current, oldValue, newValue string) bool {
	if current == "" || oldValue == "" || newValue == "" {
		return false
	}
	if strings.Contains(current, newValue) && !strings.Contains(current, oldValue) {
		return true
	}
	return strings.Contains(current, newValue) && strings.Count(current, oldValue) == strings.Count(current, newValue)
}

func ensureTrailingNewline(value string) string {
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, "\n") {
		return value
	}
	return value + "\n"
}

func sanitizeSingleLine(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
