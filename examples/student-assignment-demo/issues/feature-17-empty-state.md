# Feature #17: 搜索空结果页需要更友好

## 背景

客户同学展示 CourseHub 时发现：搜索没有结果时，界面只显示 `No assignments found.`。这句话太冷，也没有告诉用户当前搜索词是什么。

截图反馈见：

```text
assets/search-empty-state-feedback.png
```

## 期望

当搜索词不为空，但没有找到作业时，文案需要：

- 使用中文。
- 包含用户输入的搜索词。
- 提醒用户可以试试课程名或作业标题关键词。

建议文案：

```text
没有找到与「software」相关的作业，试试课程名或作业标题关键词。
```

## 验收

- 修改范围尽量小。
- 补一个针对 `EmptySearchMessage` 的测试。
- 不要改搜索匹配逻辑；搜索 bug 留给 issue #42。
