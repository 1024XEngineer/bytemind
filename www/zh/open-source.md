# 开源参与

ByteMind 是采用 **MIT License** 开源的软件。

## 仓库信息

- **GitHub**：https://github.com/1024XEngineer/bytemind
- **Issue**：https://github.com/1024XEngineer/bytemind/issues
- **Releases**：https://github.com/1024XEngineer/bytemind/releases

## 许可证

ByteMind 采用 MIT 许可证，可自由用于个人、商业和内部项目。完整条款见 [`LICENSE`](https://github.com/1024XEngineer/bytemind/blob/main/LICENSE)。

## 参与贡献

欢迎任何形式的贡献。发起 Pull Request 前，请遵循以下指引：

### 开始之前

- **Bug 修复**：先开 Issue 说明现象、期望行为和复现步骤，避免重复工作。
- **功能添加**：先通过 Discussion 或 Issue 对齐范围，再投入实现。
- **文档修改**：错别字、断链或事实错误可直接提 PR。

### 开发环境搭建

需要 Go 1.24 以上。

```bash
git clone https://github.com/1024XEngineer/bytemind.git
cd bytemind
go build ./...
go test ./...
```

### Pull Request 规范

1. **保持变更聚焦**——每个 PR 只做一件完整的事
2. **行为变更要同步更新测试**，尤其是 `internal/agent/` 或 runner 流程相关变更
3. **提交前运行完整测试**：`go test ./...`
4. **包含清晰的变更说明**，带验证步骤或复现步骤
5. **关联相关 Issue**（如 `Fixes #123`）

### 代码库结构概览

| 包                 | 职责                               |
| ------------------ | ---------------------------------- |
| `cmd/bytemind`     | CLI 入口                           |
| `internal/agent`   | Agent runner、提示词组装、工具调度 |
| `internal/app`     | TUI 应用、斜杠命令处理             |
| `internal/config`  | 配置加载与默认值                   |
| `internal/skills`  | 技能目录与加载器                   |
| `internal/session` | 会话持久化                         |
| `internal/tools`   | 工具实现                           |

涉及提示词组装或 Agent runner 流程的变更，请先阅读仓库根目录的 `AGENTS.md`。

## 反馈与支持

- **Bug 反馈**：[GitHub Issues](https://github.com/1024XEngineer/bytemind/issues)
- **功能建议**：[GitHub Discussions](https://github.com/1024XEngineer/bytemind/discussions)
- **安全问题**：请通过 GitHub Security 页签下的私人通道报告，而不要公开提 Issue
