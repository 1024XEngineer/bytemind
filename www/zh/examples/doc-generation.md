# 示例：文档生成

本示例展示如何自动化文档生成——包括在交互式 Chat 模式中编写新页面，以及在 Run 模式中在 CI 流水线中全自动化执行。

## 场景 A：新 CLI 命令文档（Chat 模式）

你刚添加了 `bytemind export` 命令，需要编写面向用户的文档，包括用途、参数、示例和常见错误。

```text
读取 cmd/export.go 中 `export` 命令的实现。
为它生成一个面向用户的 Markdown 文档页，包含：
- 命令说明和用途
- 所有参数及其类型、说明和默认值
- 3-4 个实用示例
- 常见错误与解决方法
将输出写入 docs/cli/export.md
```

Agent 读取源码，根据参数名和逻辑推断意图，写出完整的文档页面。

## 场景 B：从源码生成 API 参考（Run 模式）

在 CI 中当代码变更时自动重生 API 参考文档：

```bash
bytemind run -prompt "\
  读取 internal/api/ 中所有导出的函数和类型。\
  在 docs/api-reference.md 中生成参考页面。\
  每个条目包含函数签名、说明、参数、返回值和示例。\
  不要修改任何源文件。\
"
```

加入 CI 流水线：

```yaml
- name: 重新生成 API 文档
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
    BYTEMIND_APPROVAL_MODE: away
  run: |
    bytemind run -prompt "根据 internal/api/ 中的当前源码重新生成 docs/api-reference.md"
```

## 场景 C：功能变更后更新现有文档

对已有功能新增选项后：

```text
对比 internal/config/config.go 中配置系统的当前实现
与 docs/configuration.md 中的现有文档。
识别未文档的字段或已过时的描述。
更新文档以反映当前实现。
```

## 高质量文档的技巧

:::tip 先读源码，再写文档
始终要求 Agent **先读取源码**再写文档。这能生成准确的、按实际实现为根据的文档，而不是猜测。
:::

:::tip 用“不要修改”限定范围
对文档生成任务，始终加上 `不要修改任何源文件`，防止意外的代码变更。
:::

## 期望产出

- 准确的、以源码为小根据的文档
- 任务导向的文字描述配合具体示例
- 包含正确默认值的参数/选项表格
- 常见错误处理指导

## 相关页面

- [单次执行模式](/zh/usage/run-mode) — CI 流水线自动化
- [聊天模式](/zh/usage/chat-mode) — 迭代式文档编写
- [工具与审批](/zh/usage/tools-and-approval) — 写入文件前的 `write_file` 审批
