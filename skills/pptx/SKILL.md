---
name: pptx
description: 按业务目标生成、改写和审阅 PPT 演示文稿。
---

# PPTX

## 何时使用

- 用户要创建、改写、总结、拆页或套模板处理 `.pptx`。
- 用户提到 deck、slides、presentation、汇报、路演、讲稿。

## 工作方式

- 先确认受众、场景、页数、风格、结论和截止时间。
- 优先输出逐页结构、标题、副标题、要点、备注和改稿建议。
- 需要结构级处理时，统一使用 Go helper：

```bash
go run ./cmd/skilltool office unpack -in deck.pptx -out work/pptx
go run ./cmd/skilltool office pack -in work/pptx -out output/deck.pptx
```

## 交付要求

- 明确每页目标、视觉重点和信息层级。
- 改稿时说明删改原因、风险和仍待确认的问题。
