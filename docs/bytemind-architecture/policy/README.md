# policy 模块设计

## 1. 模块定位
`policy` 是权限与安全决策中心，输出 `allow/deny/ask` 及风险等级。

## 2. 职责边界
做什么：
- 执行五层权限模型。
- 按固定优先级处理规则冲突。
- 进行路径、命令、敏感文件风险判定。

不做什么：
- 不执行业务动作。
- 不承担主循环和任务调度。

## 3. 对外接口
- `Engine`：策略决策主入口。
- `Rule`：按阶段归属的可组合原子规则。
- `RuleSet`：规则注册与枚举。
- `PriorityResolver`：固定优先级决策归并器。
- `PathGuard`：路径访问安全判定。
- `CommandGuard`：命令执行安全判定。
- `SensitiveFileGuard`：敏感文件读取防护。

## 4. 决策优先级
`hard_deny > explicit_deny > risk_rule > explicit_allow(仅低中风险可生效) > mode_default > fallback_ask`

## 5. 五层权限模型
- 会话模式层：`default/acceptEdits/bypassPermissions(受控)/plan`。
- 工具白黑名单层：`allowedTools/deniedTools`。
- 工具级策略层：读默认放行，写与命令默认询问。
- 风险层：`low/medium/high`。
- 路径命令层：`allowedWritePaths/deniedWritePaths/allowedCommands/deniedCommands`。

## 6. 测试策略
- 冲突优先级与边界样例。
- 高危命令、敏感文件、路径逃逸回归。
