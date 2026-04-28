# 单次执行模式

Run 模式（`bytemind run`）以非交互方式执行一个完整任务，完成后自动退出。不需要多轮对话，通过 `-prompt` 一次性传入任务描述。

```bash
bytemind run -prompt "更新 README 安装章节"
```

## 适用场景

| 场景            | 示例                        |
| --------------- | --------------------------- |
| CI 流水线自动化 | 生成 changelog、更新版本号  |
| 脚本化文档更新  | 代码变更后自动重生 API 文档 |
| 单次重构        | 全库重命名某个符号          |
| 批处理          | 对多个文件应用统一转换      |

:::tip Chat 模式 vs Run 模式
需要迭代反馈、逐步审批或任务中途调整时，用 **Chat 模式**。任务已经明确、希望一次性完成时，用 **Run 模式**。
:::

## 命令行选项

```bash
bytemind run -prompt "<任务>"                    # 基本用法
bytemind run -prompt "<任务>" -max-iterations 64  # 提高迭代上限
bytemind run -prompt "<任务>" -config ./my.json   # 自定义配置
```

| 参数              | 说明             | 默认値   |
| ----------------- | ---------------- | -------- |
| `-prompt`         | 任务描述（必填） | —        |
| `-max-iterations` | 最大工具调用轮次 | 32       |
| `-config`         | 配置文件路径     | 自动检测 |

## Run 模式中的审批

默认情况下，Run 模式仍然使用 `approval_mode: interactive`，高风险操作会**阻塞等待你的输入**。如果希望完全自动化，配置 `full_access`：

```json
{
  "approval_mode": "full_access"
}
```

或利用环境变量：

```bash
BYTEMIND_APPROVAL_MODE=full_access bytemind run -prompt "重新生成所有 API 文档"
```

:::warning
`away_policy` 已降级为兼容字段，不再影响运行时行为。仅在旧系统仍要求该字段时保留即可。
:::

## 实应示例

**更新文档**

```bash
bytemind run -prompt "根据当前源码重新生成 docs/api.md 中的 API 参考文档"
```

**CI 中自动化清理代码**

```bash
BYTEMIND_APPROVAL_MODE=full_access \
  bytemind run -prompt "删除 src/ 目录下所有 TODO 注释并记录已删除的内容"
```

**版本号升级**

```bash
bytemind run -prompt "将 go.mod、README.md 和 cmd/version.go 中的版本号更新为 v1.2.0"
```

## 相关页面

- [聊天模式](/zh/usage/chat-mode) — 交互式、多轮对话
- [配置](/zh/configuration) — 审批模式与兼容字段
- [CLI 命令](/zh/reference/cli-commands) — 完整参数参考
