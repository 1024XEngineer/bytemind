package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	Path        string
	Content     string
}

func (s *Skill) Instructions() string {
	return strings.TrimSpace(stripFrontMatter(s.Content))
}

func (s *Skill) DisplayDescription() string {
	if s == nil {
		return "自定义项目技能"
	}
	if value, ok := conciseChineseDescriptions[s.Name]; ok {
		return value
	}
	if value := compactDescription(strings.Trim(s.Description, `"`), 18); value != "" {
		return value
	}
	if strings.TrimSpace(s.Name) != "" {
		return fmt.Sprintf("自定义技能：%s", s.Name)
	}
	return "自定义项目技能"
}

type Manager struct {
	dir    string
	skills map[string]*Skill
}

var conciseChineseDescriptions = map[string]string{
	"skill-author":             "按需求生成或改写项目 skill",
	"pptx":                     "PPT 读写、改稿、套模板",
	"pdf":                      "PDF 提取、合并、OCR、拆分",
	"docx":                     "Word 报告、公文、模板处理",
	"xlsx":                     "Excel 表格、公式、图表处理",
	"transcribe":               "音频转写、说话人区分",
	"internal-comms":           "周报、公告、内部沟通稿",
	"doc-coauthoring":          "协作整理成正式文档",
	"notion-knowledge-capture": "沉淀会议纪要和决策到 Notion",
	"screenshot":               "截图留痕、反馈和操作说明",
	"theme-factory":            "统一文档和演示视觉主题",
}

func NewManager(workspace string) *Manager {
	return &Manager{
		dir:    filepath.Join(workspace, "skills"),
		skills: make(map[string]*Skill),
	}
}

func (m *Manager) Dir() string {
	return m.dir
}

func (m *Manager) Load() error {
	m.skills = make(map[string]*Skill)

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(m.dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}

		content := string(data)
		name := extractSkillName(content, entry.Name())
		skill := &Skill{
			Name:        name,
			Description: extractSkillDescription(content),
			Path:        skillPath,
			Content:     content,
		}

		m.skills[name] = skill
		m.skills[entry.Name()] = skill
	}

	return nil
}

func (m *Manager) Get(name string) *Skill {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if name == "" {
		return nil
	}
	return m.skills[name]
}

func (m *Manager) Has(name string) bool {
	return m.Get(name) != nil
}

func (m *Manager) List() []*Skill {
	if len(m.skills) == 0 {
		return nil
	}
	list := make([]*Skill, 0, len(m.skills))
	seen := map[*Skill]struct{}{}
	for _, skill := range m.skills {
		if _, ok := seen[skill]; ok {
			continue
		}
		seen[skill] = struct{}{}
		list = append(list, skill)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

func extractSkillName(content, defaultName string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name:") {
			value := trimYAMLScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "name:")))
			if value != "" {
				return value
			}
		}
	}
	return defaultName
}

func extractSkillDescription(content string) string {
	lines := strings.Split(content, "\n")
	var description []string
	collecting := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "description:"):
			collecting = true
			description = append(description, strings.TrimSpace(strings.TrimPrefix(trimmed, "description:")))
		case collecting && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")):
			description = append(description, strings.TrimSpace(line))
		case collecting:
			return strings.Join(description, " ")
		}
	}

	return strings.Join(description, " ")
}

func stripFrontMatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return content
}

func compactDescription(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func trimYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return strings.TrimSpace(value)
}
