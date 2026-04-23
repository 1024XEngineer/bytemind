# 工具与审批

ByteMind 支持读取文件、搜索、写入补丁和执行 Shell 命令。

## 安全建议

将 `approval_policy` 保持为 `on-request`，重要操作由你确认。

## 推荐流程

1. 先让 Agent 分析相关文件。
2. 审阅改动建议。
3. 再批准执行命令或写入操作。
