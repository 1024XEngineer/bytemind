# Project Skills

Put project-local skills under `skills/<name>/SKILL.md`.

Minimal example:

```text
skills/
  review/
    SKILL.md
```

Example `SKILL.md`:

```md
---
name: review
description: Review code for correctness, regressions, and missing tests.
---

# Review

Prioritize concrete bugs, risks, and test gaps.
Keep summaries short.
```

Usage:

- `go run ./cmd/bytemind chat -skill review`
- `go run ./cmd/bytemind run -skill review -prompt "inspect this branch"`
- In TUI: `/skills`, `/<skill>`, `/clear-skill`
