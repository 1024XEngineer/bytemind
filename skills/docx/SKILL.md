---
name: docx
description: 生成、改写和整理 Word 报告、公文与模板文档。
---

# DOCX

## 何时使用

- 用户要创建、改写、总结或审阅 `.docx`。
- 用户要正式报告、方案、函件、纪要、模板、公文或带结构的正文。

## 工作方式

- 先确认文种、受众、语气、页数、章节结构和是否需要封面、目录、附录。
- 优先输出标题层级、正文草稿、表格说明、修订建议和定稿版本。
- 需要结构级处理时，统一使用 Go helper：

```bash
go run ./cmd/skilltool office unpack -in report.docx -out work/docx
go run ./cmd/skilltool office pack -in work/docx -out output/report.docx
```

## 交付要求

- 文档结构清楚，标题层级稳定，语气符合正式书面表达。
- 改稿时标注重点改动、删除内容和需要人工确认的部分。
