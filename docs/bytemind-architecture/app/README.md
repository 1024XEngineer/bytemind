# app 模块设计

## 1. 模块定位
`app` 是系统装配根，负责配置加载、依赖注入和进程生命周期管理。

## 2. 职责边界
做什么：
- 加载并校验配置。
- 装配 `agent/session/context/provider/tools/runtime/policy/storage/extensions`。
- 管理启动、就绪、优雅退出。

不做什么：
- 不承载业务编排逻辑。
- 不实现权限、任务、工具、存储的业务细节。

## 3. 对外接口
- `ConfigLoader`：加载并归一化配置源（文件/环境变量/参数）。
- `Component`：统一组件生命周期（`Start/Stop/Ready`）。
- `Bootstrapper`：根据配置构建模块集合 `ModuleSet`。
- `LifecycleManager`：按依赖拓扑启动和关闭模块。
- `Application`：应用进程入口（`Run/Shutdown`）。

## 4. 启停策略
- 启动顺序建议：`storage -> session -> policy -> tools/extensions -> context -> provider -> runtime -> agent`。
- 关闭顺序建议：按启动逆序关闭并刷盘。

## 5. 测试策略
- 配置测试：默认值与非法配置校验。
- 装配测试：依赖缺失与回滚完整性。
- 生命周期测试：幂等启动/关闭、超时退出。
