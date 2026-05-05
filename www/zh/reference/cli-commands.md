# CLI 命令

ByteMind 默认不带子命令即可启动交互界面；`chat` 是兼容别名，`run` 用于单次非交互任务。

## `bytemind`

启动交互式多轮会话。

```bash
bytemind [参数]
```

| 参数                  | 说明                   | 默认值   |
| --------------------- | ---------------------- | -------- |
| `-config <路径>`      | 指定配置文件路径       | 自动检测 |
| `-max-iterations <n>` | 单任务最大工具调用轮次 | 32       |
| `-workspace <路径>`   | 指定工作区目录         | 当前目录 |
| `-v`                  | 开启详细/调试输出      | false    |

**示例：**

```bash
bytemind
bytemind -max-iterations 64
bytemind -config ~/.bytemind/work.json
bytemind -workspace D:\code\my-project
bytemind -v
```

`bytemind chat` 与 `bytemind tui` 仍可使用，行为等同于默认交互界面。

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

输出已安装的版本后退出。

```bash
bytemind --version
# v0.1.5
```

## 会话斜杠命令

以下命令在 `bytemind` 交互会话内输入，不是在 Shell 中执行：

| 命令                                          | 说明                         |
| --------------------------------------------- | ---------------------------- |
| `/help`                                       | 列出所有可用命令             |
| `/session`                                    | 显示当前会话 ID 与摘要       |
| `/sessions [n]`                               | 列出最近 n 条会话（默认 10） |
| `/resume <id>`                                | 按 ID 或前缀恢复会话         |
| `/new`                                        | 在当前工作区开启新会话       |
| `/plan`                                       | 切换到 Plan 模式             |
| `/build`                                      | 切换到 Build 模式            |
| `/commit <message>`                           | 暂存当前全部改动并创建本地 Git commit |
| `/undo-commit`                                | 回退当前会话里由 `/commit` 创建的最后一个本地 commit |
| `/quit`                                       | 安全退出                     |
| `/bug-investigation [symptom="..."]`          | 激活 Bug 排查技能            |
| `/review [base_ref=<ref>]`                    | 激活代码审查技能             |
| `/github-pr [pr_number=<n>] [base_ref=<ref>]` | 激活 GitHub PR 技能          |
| `/repo-onboarding`                            | 激活仓库入门技能             |
| `/write-rfc [path=<文件>]`                    | 激活 RFC 撰写技能            |

### `/commit <message>`

在 `bytemind` 会话里使用 `/commit`，可以让 ByteMind 把当前工作区改动保存为一个本地 Git commit。

```text
/commit fix(/commit): 调整 /commit 的反馈形式
```

从 Slash 命令面板选择 `/commit` 时，ByteMind 会先把输入框填成 `/commit `，等待你手动填写 commit message。按 Enter 后，ByteMind 会执行 `git add -A`、创建 commit，并反馈 commit hash、message 和包含的文件数量。

### `/undo-commit`

使用 `/undo-commit` 可以回退当前会话里最近一次由 ByteMind `/commit` 创建的 commit。

```text
/undo-commit
```

ByteMind 只会在当前 `HEAD` 仍然是这次会话提交、工作区没有更新的改动、并且 upstream 分支还不包含该提交时执行回退。它使用 `git reset --soft HEAD~1`，所以文件改动仍然会保留在本地。

## 配置加载顺序

未指定 `-config` 时，ByteMind 会先加载全局配置，再加载项目覆盖配置：

1. 用户目录的 `~/.bytemind/config.json`
2. 当前工作区的 `.bytemind/config.json`（可选，覆盖全局配置）

## 相关页面

- [配置参考](/zh/reference/config-reference)
- [环境变量](/zh/reference/env-vars)
- [会话管理](/zh/usage/session-management)
