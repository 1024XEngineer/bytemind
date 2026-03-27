package skills

import "strings"

const BuiltinSkillAuthorName = "skill-author"

var builtinSkills = map[string]*Skill{
	BuiltinSkillAuthorName: {
		Name:        BuiltinSkillAuthorName,
		Description: "根据需求生成、修改和整理项目内 skill",
		Content: `---
name: skill-author
description: 根据用户需求生成、修改和整理项目内 skill。默认只保留精简的 SKILL.md；如果确实需要本地自动化，优先补 Go helper。
---

# Skill Author

- 目标是为当前项目创建或修改可复用 skill，默认写入 skills/<name>/SKILL.md。
- 先确认场景、触发方式、输入、输出和 1 到 2 个具体例子；只有在名称或边界不清楚时才追问。
- skill 名称使用小写字母、数字和连字符，保持短、准、直接。
- SKILL.md 只写必要信息：什么时候用、怎么做、交付什么，不堆长篇背景。
- 默认不要创建 scripts、references、assets、examples、LICENSE.txt 之类的附属目录。
- 如果确实需要本地自动化，优先把复用逻辑写成 Go 代码，放到 cmd/skilltool 或 internal/skilltool。
- 如果是在修改已有 skill，先读取现有内容，再做最小必要调整，不要无意义重写。
- 完成后检查当前项目的 skill loader 能否识别它，并告诉用户如何通过 /<skill> 或 -skill <name> 触发。
`,
	},
}

func Builtin(name string) *Skill {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if name == "" {
		return nil
	}
	return builtinSkills[name]
}
