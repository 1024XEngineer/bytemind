# CLI 命令

ByteMind 提供两个顶级命令：`chat` 和 `run`。

## `bytemind chat`

启动交互式多轮会话。

```bash
bytemind chat [参数]
```

| 参数                  | 说明                   | 默认值   |
| --------------------- | ---------------------- | -------- |
| `-config <路径>`      | 指定配置文件路径       | 自动检测 |
| `-max-iterations <n>` | 单任务最大工具调用轮次 | 32       |
| `-v`                  | 开启详细/调试输出      | false    |

**示例：**

```bash
bytemind chat
bytemind chat -max-iterations 64
bytemind chat -config ~/.bytemind/work.json
bytemind chat -v
```

## `bytemind run`

以非交互方式执行单次任务后退出。

```bash
bytemind run -prompt "<任务>" [参数]
```

| 参数                  | 说明                   | 默认值   |
| --------------------- | ---------------------- | -------- |
| `-prompt <文本>`      | 任务描述（**必填**）   | —        |
| `-config <路径>`      | 指定配置文件路径       | 自动检测 |
| `-max-iterations <n>` | 单任务最大工具调用轮次 | 32       |
| `-v`                  | 开启详细/调试输出      | false    |

**示例：**

```bash
bytemind run -prompt "更新 README 安装章节"
bytemind run -prompt "全库重命名 Foo 为 Bar" -max-iterations 64
```

## `bytemind --version`

输出已安装的版本和构建信息后退出。

```bash
bytemind --version
# ByteMind v0.4.0 (go1.24.0 darwin/arm64)
```

## 会话斜杠命令

以下命令在 `bytemind chat` 会话内输入，不是在 Shell 中执行：

| 命令                                          | 说明                         |
| --------------------------------------------- | ---------------------------- |
| `/help`                                       | 列出所有可用命令             |
| `/session`                                    | 显示当前会话 ID 与摘要       |
| `/sessions [n]`                               | 列出最近 n 条会话（默认 10） |
| `/resume <id>`                                | 按 ID 或前缀恢复会话         |
| `/new`                                        | 在当前工作区开启新会话       |
| `/plan`                                       | 切换到 Plan 模式             |
| `/build`                                      | 切换到 Build 模式            |
| `/quit`                                       | 安全退出                     |
| `/bug-investigation [symptom="..."]`          | 激活 Bug 排查技能            |
| `/review [base_ref=<ref>]`                    | 激活代码审查技能             |
| `/github-pr [pr_number=<n>] [base_ref=<ref>]` | 激活 GitHub PR 技能          |
| `/repo-onboarding`                            | 激活仓库入门技能             |
| `/write-rfc [path=<文件>]`                    | 激活 RFC 撰写技能            |

## 配置加载顺序

未指定 `-config` 时，ByteMind 按以下顺序查找配置：

1. 当前目录的 `.bytemind/config.json`
2. 当前目录的 `config.json`
3. 主目录的 `~/.bytemind/config.json`

## 相关页面

- [配置参考](/zh/reference/config-reference)
- [环境变量](/zh/reference/env-vars)
- [会话管理](/zh/usage/session-management)
