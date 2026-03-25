package skills

import (
	"fmt"
	"os"
	"path/filepath"
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
	for _, s := range m.skills {
		list = append(list, s)
	}
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
	"office-hours":          "创业头脑风暴 - 验证产品想法",
	"plan-ceo-review":       "CEO 评审 - 战略层面审查计划",
	"plan-eng-review":       "工程评审 - 架构和数据流审查",
	"plan-design-review":    "设计评审 - UI/UX 层面审查计划",
	"design-consultation":   "设计咨询 - 创建完整设计系统",
	"browse":                "浏览器 - 无头浏览器测试",
	"qa":                    "QA 测试 - 找 bug 并修复",
	"qa-only":               "QA 报告 - 仅报告 bug 不修复",
	"review":                "代码审查 - PR 预合并审查",
	"investigate":           "调试调查 - 根因分析",
	"design-review":         "视觉审查 - 修复视觉问题",
	"ship":                  "发布 - 一键测试+部署+PR",
	"document-release":      "文档更新 - 同步文档到最新代码",
	"retro":                 "回顾 - 每周团队回顾",
	"careful":               "安全警告 - 危险命令二次确认",
	"freeze":                "冻结 - 锁定编辑目录",
	"guard":                 "守卫 - 激活所有安全限制",
	"unfreeze":              "解冻 - 解除目录限制",
	"gstack-upgrade":        "升级 - 更新 gstack 到最新版本",
	"codex":                 "Codex - OpenAI Codex CLI 包装",
	"setup-browser-cookies": "Cookies - 导入浏览器 Cookies",
}

func (m *Manager) PrintHelp() string {
	var sb strings.Builder
	sb.WriteString("\n可用 Skills:\n")

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

	sb.WriteString("\n使用方法: /skill-name (如 /qa)\n")
	return sb.String()
}
