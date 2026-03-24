package planner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"forgecli/internal/workspace"
)

type Plan struct {
	Summary    string
	Steps      []string
	TargetFile string
	SearchHits []workspace.SearchHit
}

type Planner interface {
	BuildPlan(ctx context.Context, task string, ws *workspace.Workspace) (Plan, error)
}

type MockPlanner struct{}

func (p MockPlanner) BuildPlan(_ context.Context, task string, ws *workspace.Workspace) (Plan, error) {
	files, err := ws.ListFiles(250)
	if err != nil {
		return Plan{}, err
	}
	if len(files) == 0 {
		return Plan{}, errors.New("workspace does not contain any files")
	}

	target := detectMentionedFile(task, files)
	hits := make([]workspace.SearchHit, 0, 5)
	if target == "" {
		for _, keyword := range extractKeywords(task) {
			searchHits, err := ws.Search(keyword, 5)
			if err != nil {
				return Plan{}, err
			}
			hits = appendUniqueHits(hits, searchHits, 5)
			if target == "" {
				target = pickBestHitFile(searchHits)
			}
		}
	}
	if target == "" {
		target = firstEditableFile(files)
	}
	if target == "" {
		return Plan{}, errors.New("could not select a target file for the demo change")
	}

	summary := fmt.Sprintf("准备围绕 %s 完成任务：%s", target, shorten(task, 80))
	steps := []string{
		"读取目标文件并确认当前内容",
		"生成可审阅的 diff，等待用户审批",
		"写入后执行验证命令并输出总结",
	}

	return Plan{Summary: summary, Steps: steps, TargetFile: target, SearchHits: hits}, nil
}

func detectMentionedFile(task string, files []string) string {
	taskValue := strings.ToLower(strings.ReplaceAll(task, "\\", "/"))

	for _, file := range files {
		lowerFile := strings.ToLower(file)
		if strings.Contains(taskValue, lowerFile) {
			return file
		}
	}

	for _, file := range files {
		lowerBase := strings.ToLower(filepath.Base(file))
		if strings.Contains(taskValue, lowerBase) {
			return file
		}
	}
	return ""
}

func extractKeywords(task string) []string {
	re := regexp.MustCompile(`[A-Za-z0-9._/-]{3,}`)
	matches := re.FindAllString(task, -1)
	seen := make(map[string]struct{}, len(matches))
	keywords := make([]string, 0, len(matches))
	for _, item := range matches {
		lower := strings.ToLower(item)
		if isNoiseWord(lower) {
			continue
		}
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		keywords = append(keywords, lower)
	}
	sort.Strings(keywords)
	return keywords
}

func isNoiseWord(value string) bool {
	switch value {
	case "with", "from", "that", "this", "file", "task", "replace", "update", "fix", "add", "forge":
		return true
	default:
		return false
	}
}

func appendUniqueHits(base []workspace.SearchHit, incoming []workspace.SearchHit, limit int) []workspace.SearchHit {
	seen := make(map[string]struct{}, len(base))
	for _, hit := range base {
		key := fmt.Sprintf("%s:%d", hit.Path, hit.Line)
		seen[key] = struct{}{}
	}
	for _, hit := range incoming {
		key := fmt.Sprintf("%s:%d", hit.Path, hit.Line)
		if _, ok := seen[key]; ok {
			continue
		}
		base = append(base, hit)
		seen[key] = struct{}{}
		if limit > 0 && len(base) >= limit {
			return base
		}
	}
	return base
}

func pickBestHitFile(hits []workspace.SearchHit) string {
	for _, hit := range hits {
		if isPreferredFile(hit.Path) {
			return hit.Path
		}
	}
	if len(hits) > 0 {
		return hits[0].Path
	}
	return ""
}

func firstEditableFile(files []string) string {
	for _, file := range files {
		if isPreferredFile(file) {
			return file
		}
	}
	for _, file := range files {
		switch strings.ToLower(filepath.Ext(file)) {
		case ".go", ".md", ".txt", ".yaml", ".yml", ".json", ".html":
			return file
		}
	}
	return files[0]
}

func isPreferredFile(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, "_test.go") {
		return false
	}
	switch filepath.Ext(lower) {
	case ".go", ".md", ".txt", ".yaml", ".yml", ".json", ".html":
		return true
	default:
		return false
	}
}

func shorten(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max < 4 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
