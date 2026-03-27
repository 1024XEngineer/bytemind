# Project Skills

Project-local skills live under `skills/<name>/SKILL.md`.

Recommended structure:

```text
skills/
  review/
    SKILL.md
```

Keep each skill lean:

- Default to a single `SKILL.md`.
- Put only trigger rules, workflow, and output expectations in the file.
- Do not vendor Python, JavaScript, PowerShell, Swift, images, or sample packs into `skills/` unless there is a hard requirement.
- If local automation is truly needed, prefer Go code under `cmd/skilltool` or `internal/skilltool`.
- Put generated files under `output/`, `work/`, or `.bytemind/`; these paths are ignored by Git.

Minimal example:

```md
---
name: review
description: 审查代码正确性、回归风险和测试缺口。
---

# Review

- 用于代码审查、回归检查和测试补漏。
- 先列问题，再给简短总结。
```

Usage:

- `go run ./cmd/bytemind chat -skill review`
- `go run ./cmd/bytemind run -skill review -prompt "inspect this branch"`
- `go run ./cmd/skilltool office unpack -in file.docx -out work/docx`
- In TUI: `/skills`, `/<skill>`, `/clear-skill`
