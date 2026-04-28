# Skills

A **skill** is a focused workflow guide that you activate with a slash command. When active, it injects domain-specific instructions into the agent's system prompt, steering it toward a structured process for a particular type of task.

Use a skill when a generic prompt would produce mediocre results — skills make the agent follow a proven, structured approach.

## Built-in Skills

ByteMind ships with five built-in skills:

### `/bug-investigation`

Structured bug diagnosis with evidence gathering before any fix is proposed.

```text
/bug-investigation
/bug-investigation symptom="checkout page returns 403 for logged-in users"
```

**What it produces:**

- Symptom summary
- Reproduction steps
- Evidence (logs, call chain, code location)
- Root cause hypothesis with confidence level
- Minimal fix proposal and verification plan

### `/review`

Code review focused on correctness, regression risk, and test coverage gaps.

```text
/review
/review base_ref=main
```

**What it produces:**

- Correctness issues and logic errors
- Regression risk assessment
- Missing test coverage
- Suggestions ranked by severity

### `/github-pr`

Analyzes PR diffs, review feedback, and merge risk.

```text
/github-pr
/github-pr pr_number=42
/github-pr pr_number=42 base_ref=main
```

### `/repo-onboarding`

Builds a quick understanding of a repository — structure, entry points, and run flow.

```text
/repo-onboarding
```

Ideal when you're new to a codebase and want a guided orientation before starting work.

### `/write-rfc`

Produces a structured technical proposal with problem statement, alternatives, tradeoffs, and rollout plan.

```text
/write-rfc
/write-rfc path=docs/rfc/feature-x.md
```

## Skill Scopes

Skills can come from three sources, applied in precedence order:

| Scope     | Location              | Use case                                 |
| --------- | --------------------- | ---------------------------------------- |
| `builtin` | Shipped with ByteMind | General-purpose workflows                |
| `user`    | `~/.bytemind/skills/` | Personal workflows across all projects   |
| `project` | `.bytemind/skills/`   | Team workflows for a specific repository |

Project skills override user skills, which override builtins with the same name.

## Creating Custom Skills

Use the built-in skill creator to scaffold a new skill:

```text
/skill-creator
```

Or create one manually. A skill requires two files in a named directory:

**`skill.json`** — manifest:

```json
{
  "name": "my-skill",
  "version": "0.1.0",
  "title": "My Custom Skill",
  "description": "What this skill does.",
  "entry": { "slash": "/my-skill" },
  "tools": {
    "policy": "allowlist",
    "items": ["list_files", "read_file", "search_text"]
  }
}
```

**`SKILL.md`** — the instructions injected into the agent:

```markdown
---
name: my-skill
description: Brief description for the skill catalog.
---

# my-skill

## Workflow

1. Step one
2. Step two
3. Step three

## Output Contract

- Expected output A
- Expected output B
```

**Tool policy options:**

| Policy      | Behavior                                       |
| ----------- | ---------------------------------------------- |
| `inherit`   | Use the session's default tool set             |
| `allowlist` | Only the listed tools are available            |
| `denylist`  | All tools except the listed ones are available |

## Placing Custom Skills

**For a project** — put the skill directory in `.bytemind/skills/`:

```
.bytemind/
  skills/
    my-skill/
      skill.json
      SKILL.md
```

**For personal use across all projects** — put it in `~/.bytemind/skills/`:

```
~/.bytemind/
  skills/
    my-skill/
      skill.json
      SKILL.md
```

## See Also

- [Core Concepts](/core-concepts) — skills overview
- [Chat Mode](/usage/chat-mode) — how to use skills in a session
