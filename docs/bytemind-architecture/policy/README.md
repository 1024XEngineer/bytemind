# policy 模块设计

## 模块定位
- 负责权限决策与安全防护（allow/deny/ask、风险分级、路径/命令限制）。
- 不负责执行具体业务动作。

## 边界
- 做：五层权限模型计算、固定优先级决策、风险分级、拒绝原因编码。
- 不做：工具实际执行、任务调度、会话存储。

## 内部实现逻辑
- 聚合五层输入：会话模式、工具白黑名单、工具级策略、风险规则、路径命令规则。
- 按固定优先级进行决策并输出可解释原因：`explicit deny > explicit allow > risk rule > mode default > fallback ask`。

## 对外契约
- 暴露统一决策引擎接口。
- 输入：`mode/tool/path/command` 及 `allowedTools/deniedTools/allowedWritePaths/deniedWritePaths/allowedCommands/deniedCommands`。
- 输出：决策结果、风险等级、原因码。

## 依赖关系
- 允许依赖：规则配置与审计接口。
- 禁止依赖：`agent`、`tools` 的实现细节。

## 可观测性
- 拒绝率、询问率、高风险拦截率。
- 决策原因码分布与审计事件。

## 测试策略
- 优先级冲突测试。
- 边界与逃逸样例测试。
- 敏感文件与高危命令回归测试。
