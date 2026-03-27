package skills

import "strings"

const BuiltinSkillAuthorName = "skill-author"

var builtinSkills = map[string]*Skill{
	BuiltinSkillAuthorName: {
		Name:        BuiltinSkillAuthorName,
		Description: "根据需求生成、修改项目内 skill",
		Content: `---
name: skill-author
description: 根据用户需求生成、修改和整理项目内 skill，帮助用户把可复用流程沉淀为 skills/<name>/SKILL.md，并在需要时补充 scripts、references、assets。
---

# Skill Author

- 目标是为当前项目创建或修改可复用 skill，默认写入 skills/<name>/SKILL.md。
- 先理解用户要解决的场景、触发方式、输入输出和 1-2 个具体示例；只有在名称、范围或关键行为不清楚时才追问。
- skill 名称使用小写字母、数字和连字符，尽量短，直接表达用途。
- 保持 SKILL.md 简洁，优先写清“做什么”和“什么时候用”。
- 只有在明显需要复用或提高稳定性时，才新增 scripts/、references/、assets/。
- 不要在 skill 目录里额外创建 README、CHANGELOG、安装指南之类的冗余文档。
- 如果是修改已有 skill，先读取现有内容，再做最小必要调整。
- 如果用户提到“tool”，优先考虑把可重复逻辑沉淀为 skill 内脚本或资源，而不是把说明写得很长。
- 完成后检查生成结果是否能被当前项目的 skill loader 识别，并告诉用户如何触发这个 skill。
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
