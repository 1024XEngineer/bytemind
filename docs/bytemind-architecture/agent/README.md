# agent 模块设计

## 1. 模块定位
`agent` 是 coding agent 的主闭环编排层，负责请求处理、模型交互、工具协调和结果归并。

## 2. 职责边界
做什么：
- 接收用户输入并读取会话快照。
- 调用 `context` 构建模型请求并执行预算/压缩。
- 调用 `provider` 消费流式模型事件。
- 工具调用前走 `policy` 决策，再调度 `tools`。
- 与 `runtime` 协作执行子代理。

不做什么：
- 不实现工具细节。
- 不实现权限规则本身。
- 不实现任务状态机与底层存储细节。

## 3. 对外接口
- `Engine`：单轮主入口，处理 `TurnRequest` 并输出 `TurnEvent` 流。
- `SessionGateway`：获取会话快照并追加 turn 结果。
- `ContextGateway`：将输入和会话快照构建为模型请求。
- `ModelGateway`：统一模型流式调用入口。
- `PolicyGateway`：执行工具调用前权限决策。
- `ToolGateway`：执行工具并返回工具结果事件流。
- `RuntimeGateway`：创建和等待子代理任务。

## 4. 主闭环
1. 读取 `session` 快照。  
2. 构建 `context`。  
3. 调用 `provider`。  
4. 工具意图先过 `policy`。  
5. 调用 `tools/runtime`。  
6. 写入 `session/storage` 并返回终态。  

## 5. 测试策略
- 主循环契约测试：事件顺序与终态一致性。
- 压缩测试：阈值触发与配对保留。
- 工具编排测试：超时、取消、重试、并发上限。
