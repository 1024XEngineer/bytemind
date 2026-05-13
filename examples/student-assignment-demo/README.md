# Student Assignment Demo

这是 ByteMind 路演用的“客户同学大作业”演示仓库，用来串联三类用户故事：

1. 仓库理解：让 ByteMind 解释 CourseHub 的模块边界。
2. Plan 执行：根据截图反馈改一个小体验问题。
3. Debug 闭环：拉取 issue，复现 bug，修复，测试，提交。

故事背景：

一位同学做了一个课程作业管理小工具 CourseHub，用来记录课程作业、搜索作业、查看是否逾期。项目快交了，但他遇到两个问题：

- 空搜索结果页文案太冷，用户看不懂自己搜了什么。
- 搜索功能有 bug：输入课程名或大小写不同的关键词时，明明存在的作业查不到。

路演时可以这样演示：

1. 让 ByteMind 理解仓库结构和业务目标。
2. 让 ByteMind 读取 `issues/feature-17-empty-state.md`，根据截图反馈走 Plan/Build。
3. 让 ByteMind 读取 `issues/issue-42-search.md`，运行 `go test ./...` 复现失败。
4. 让 ByteMind 解释搜索链路和失败原因。
5. 让 ByteMind 修复 bug。
6. 重新运行测试，并展示 `git diff`。

推荐演示问题：

```text
这是一个客户同学的课程大作业仓库。请先理解项目结构，然后说明它的模块边界和主要业务流程。
```

当前预期：

```powershell
go test ./...
```

初始状态应当失败；修复搜索逻辑后应当通过。

Plan 执行演示建议：

```text
请读取 issues/feature-17-empty-state.md，并结合 assets/search-empty-state-feedback.png。先切到 Plan 模式给出计划，我确认后再 Build 修改。
```

Debug 演示建议：

```text
请读取 issues/issue-42-search.md，像接到 GitHub issue 一样先 review，再复现、定位、修改、测试，最后给出 commit message。
```
