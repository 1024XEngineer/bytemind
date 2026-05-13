# Issue #42: 搜索存在的课程作业时结果为空

## 现象

客户同学反馈：仓库里明明有 `Software Engineering - Design review report`，但搜索 `software` 或 `design` 时结果为空。

## 复现

```powershell
go test ./...
```

当前失败用例：

- `TestSearchAssignmentsMatchesTitleCaseInsensitive`
- `TestSearchAssignmentsMatchesCourseName`

## 期望

搜索逻辑应该：

- 去掉 query 两侧空格。
- 大小写不敏感。
- 同时匹配作业标题和课程名。

## 建议 review 重点

- 先解释 `SearchAssignments` 当前只匹配了什么。
- 再说明为什么两个测试分别失败。
- 最后做最小修复，不要重构其他模块。
