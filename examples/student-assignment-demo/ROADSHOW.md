# 路演演示脚本

## 一条故事线

客户同学正在做课程大作业 CourseHub。项目已经能跑，但他第一次使用 ByteMind，不知道怎么快速理解仓库、根据需求修改、处理 GitHub issue。

## Act 1：理解 ByteMind 和客户仓库

讲法：

> 这位同学根据文档站安装并配置 ByteMind，选择 DeepSeek 或其他可用模型。然后他把自己的 GitHub 大作业仓库交给 ByteMind，先不改代码，只让它理解项目。

演示提示词：

```text
请先理解这个课程大作业仓库，不要修改文件。请说明项目结构、核心业务对象、模块边界，以及如果要 debug 搜索功能应该从哪里看起。
```

可以让 ByteMind 关注：

- `README.md`
- `cmd/coursehub/main.go`
- `internal/course/assignment.go`
- `internal/course/search.go`

## Act 2：Plan -> Build -> 权限控制 -> 截图反馈修改

讲法：

> 客户同学给了一个截图反馈：空搜索结果页只有英文提示，看不出搜索词。ByteMind 先进入 Plan 模式收敛方案，经用户确认后再进入 Build。修改文件时会触发权限控制。

演示提示词：

```text
请读取 issues/feature-17-empty-state.md，并结合 assets/search-empty-state-feedback.png。先给出 Plan，不要直接修改。我确认后再执行。
```

确认计划后：

```text
同意这个计划，请进入 Build，实现文案修改并补测试。
```

建议演示点：

- Plan 中应明确修改 `internal/course/display.go`。
- Build 中展示写文件/跑测试的权限控制。
- 修改后展示测试和 diff。

## Act 3：Debug issue -> review -> 修改 -> commit

讲法：

> 这个同学又拉了一个 GitHub issue：搜索存在的课程作业时结果为空。ByteMind 先 review issue，再复现失败，最后做最小修复并提交。

演示提示词：

```text
请读取 issues/issue-42-search.md，像接到 GitHub issue 一样先 review，再复现、定位、修改、测试。最后给出适合的 commit message。
```

## 原始 Debug 流程

1. 打开 `examples/student-assignment-demo`。
2. 让 ByteMind 总结仓库结构、技术栈和主要业务对象。
3. 让 ByteMind 跑：

   ```powershell
   go test ./...
   ```

4. 让 ByteMind 根据失败测试定位 bug。
5. 让 ByteMind 修复 `internal/course/search.go`。
6. 重新跑测试。
7. 展示 `git diff`，说明修复点非常小，但走完了理解、定位、修改、验证的闭环。

## 建议现场提问

```text
这是客户同学的课程大作业仓库。请先理解这个项目在做什么，然后运行测试，定位搜索功能为什么失败。修复后重新跑测试，并用简短中文总结你的修改。
```

## 预期修复方向

搜索逻辑应该：

- 去掉 query 两侧空格。
- 大小写不敏感。
- 同时匹配作业标题和课程名。
