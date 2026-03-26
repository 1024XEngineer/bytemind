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
	Content     string
}

type Manager struct {
	skills    map[string]*Skill
	skillsDir string
}

func NewManager(skillsDir string) *Manager {
	return &Manager{
		skills:    make(map[string]*Skill),
		skillsDir: skillsDir,
	}
}

func (m *Manager) Load() error {
	entries, err := os.ReadDir(m.skillsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(m.skillsDir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}

		content := string(data)
		name := extractSkillName(content, entry.Name())
		desc := extractSkillDescription(content)

		skill := &Skill{
			Name:        name,
			Description: desc,
			Content:     content,
		}

		m.skills[name] = skill
		m.skills[entry.Name()] = skill
	}

	return nil
}

func (m *Manager) Get(name string) *Skill {
	name = strings.TrimPrefix(name, "/")
	return m.skills[name]
}

func (m *Manager) List() []*Skill {
	var list []*Skill
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

func (m *Manager) Has(name string) bool {
	name = strings.TrimPrefix(name, "/")
	_, ok := m.skills[name]
	return ok
}

func extractSkillName(content, defaultName string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "name:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
	}
	return defaultName
}

func extractSkillDescription(content string) string {
	lines := strings.Split(content, "\n")
	descStarted := false
	var desc []string

	for _, line := range lines {
		if strings.HasPrefix(line, "description:") {
			descStarted = true
			desc = append(desc, strings.TrimSpace(strings.TrimPrefix(line, "description:")))
			continue
		}
		if descStarted {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				desc = append(desc, strings.TrimSpace(line))
			} else {
				break
			}
		}
	}

	return strings.Join(desc, " ")
}

var skillDescriptions = map[string]string{
	"browse":                "浏览器自动化与页面快照",
	"careful":               "危险操作提醒",
	"codex":                 "使用 Codex 做二次审查",
	"design-consultation":   "设计咨询与设计系统",
	"design-review":         "设计审查与修复",
	"document-release":      "发布后的文档同步",
	"freeze":                "限制编辑目录范围",
	"guard":                 "同时启用 careful 和 freeze",
	"gstack-upgrade":        "升级 gstack",
	"investigate":           "问题调查与根因分析",
	"office-hours":          "产品想法梳理与挑战",
	"plan-ceo-review":       "CEO 视角审计划",
	"plan-design-review":    "设计视角审计划",
	"plan-eng-review":       "工程视角审计划",
	"qa":                    "QA 测试并修复问题",
	"qa-only":               "只做 QA 报告",
	"retro":                 "迭代回顾",
	"review":                "代码审查",
	"setup-browser-cookies": "导入浏览器 Cookies",
	"ship":                  "测试、发布与提 PR",
	"unfreeze":              "解除编辑范围限制",
}

func (m *Manager) PrintHelp() string {
	var sb strings.Builder
	sb.WriteString("\n可用技能：\n")

	skills := m.List()
	for _, s := range skills {
		desc := skillDescriptions[s.Name]
		if desc == "" {
			desc = s.Description
			if len(desc) > 50 {
				desc = desc[:50] + "..."
			}
		}
		sb.WriteString(fmt.Sprintf("  /%-22s %s\n", s.Name, desc))
	}

	sb.WriteString("\n使用方法:\n")
	sb.WriteString("  /技能名               进入该技能，后续任务默认使用它\n")
	sb.WriteString("  /技能名 <任务>        用该技能直接执行任务\n")
	sb.WriteString("  /clear-skill          退出当前技能\n")
	return sb.String()
}
