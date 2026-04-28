# 技能

**技能**是一种可以通过斜杠命令激活的专项工作流程指引。激活后，它会将领域内的结构化指令注入到 Agent 的系统提示词中，引导 Agent 按照特定流程执行任务。

当通用提示词产出的结果质量不理想时，用技能让 Agent 遵循经过验证的结构化方法。

## 内置技能

ByteMind 随附五个内置技能：

### `/bug-investigation`

结构化 Bug 排查，先收集证据再提出修复方案。

```text
/bug-investigation
/bug-investigation symptom="结账页登录用户返回 403"
```

**输出内容：**

- 症状摘要
- 复现步骤
- 证据（日志、调用链、代码位置）
- 带置信度的根因假设
- 最小修复方案和验证计划

### `/review`

专注于正确性、回归风险和测试覆盖盲点的代码审查。

```text
/review
/review base_ref=main
```

**输出内容：**

- 正确性问题和逻辑错误
- 回归风险评估
- 测试覆盖缺失
- 按严重程度排序的建议

### `/github-pr`

分析 PR diff、Review 反馈和合并风险。

```text
/github-pr
/github-pr pr_number=42
/github-pr pr_number=42 base_ref=main
```

### `/repo-onboarding`

快速建立对仓库的理解——目录结构、入口和运行流程。

```text
/repo-onboarding
```

初次接触新仓库时，用它先做一次引导性目览，再开始实际工作。

### `/write-rfc`

生成包含问题语境、方案比较、权衡分析和上线计划的结构化技术提案。

```text
/write-rfc
/write-rfc path=docs/rfc/feature-x.md
```

## 技能作用域

技能来自三个作用域，按以下优先级应用：

| 作用域    | 路径                  | 适用场景               |
| --------- | --------------------- | ---------------------- |
| `builtin` | ByteMind 内置         | 通用工作流             |
| `user`    | `~/.bytemind/skills/` | 跨项目个人工作流       |
| `project` | `.bytemind/skills/`   | 团队层面项目专用工作流 |

同名技能优先级：项目级 > 用户级 > 内置。

## 创建自定义技能

用内置的 Skill Creator 快速创建：

```text
/skill-creator
```

也可手动创建。每个技能需要一个命名目录，包含两个文件：

**`skill.json`**——清单文件：

```json
{
  "name": "my-skill",
  "version": "0.1.0",
  "title": "我的自定义技能",
  "description": "这个技能的用途。",
  "entry": { "slash": "/my-skill" },
  "tools": {
    "policy": "allowlist",
    "items": ["list_files", "read_file", "search_text"]
  }
}
```

**`SKILL.md`**——注入到 Agent 的指令内容：

```markdown
---
name: my-skill
description: 简短描述，显示在技能目录中。
---

# my-skill

## 流程

1. 第一步
2. 第二步
3. 第三步

## 输出约定

- 期望输出 A
- 期望输出 B
```

**工具策略选项：**

| 策略        | 行为                 |
| ----------- | -------------------- |
| `inherit`   | 使用会话默认工具集   |
| `allowlist` | 仅列出的工具可用     |
| `denylist`  | 除列出的工具外均可用 |

## 放置自定义技能

**项目级**——放入 `.bytemind/skills/`：

```
.bytemind/
  skills/
    my-skill/
      skill.json
      SKILL.md
```

**用户级（跨项目个人使用）**——放入 `~/.bytemind/skills/`：

```
~/.bytemind/
  skills/
    my-skill/
      skill.json
      SKILL.md
```

## 相关页面

- [核心概念](/zh/core-concepts) — 技能概述
- [聊天模式](/zh/usage/chat-mode) — 在会话中使用技能
