---
name: xlsx
description: 设计和整理 Excel 表格、公式、字段与图表方案。
---

# XLSX

## 何时使用

- 用户要创建、改写、检查或交付 `.xlsx`。
- 用户提到表格、工作簿、公式、透视、图表、数据校验、字段映射。

## 工作方式

- 先确认表结构、字段定义、计算规则、图表类型和最终使用者。
- 优先给出工作表拆分、字段清单、公式写法、图表建议和校验规则。
- 需要结构级处理时，统一使用 Go helper：

```bash
go run ./cmd/skilltool office unpack -in workbook.xlsx -out work/xlsx
go run ./cmd/skilltool office pack -in work/xlsx -out output/workbook.xlsx
```

- 当前项目不内置公式重算引擎，涉及真实公式结果时要提示用户二次校验。

## 交付要求

- 明确每个工作表用途、字段含义、公式范围和图表用途。
- 对高风险公式、链接和数据透视，说明验证方法。
