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
- `Rule`：可组合原子规则。
- `RuleSet`：规则注册与枚举。
- `PathGuard`：路径访问安全判定。
- `CommandGuard`：命令执行安全判定。
- `SensitiveFileGuard`：敏感文件读取防护。

## 4. 决策优先级
`explicit deny > explicit allow > risk rule > mode default > fallback ask`

## 5. 测试策略
- 冲突优先级与边界样例。
- 高危命令、敏感文件、路径逃逸回归。
