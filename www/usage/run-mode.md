# Run Mode

Run mode (`bytemind run`) executes a single task non-interactively and exits when done. There is no back-and-forth - you supply the full task in `-prompt` and the agent works to completion.

```bash
bytemind run -prompt "update the README installation section"
```

## When to Use

| Scenario                       | Example                                     |
| ------------------------------ | ------------------------------------------- |
| CI pipeline automation         | Generate changelogs, bump versions          |
| Scripted documentation updates | Regenerate API docs after code changes      |
| One-shot refactors             | Rename a symbol across the whole codebase   |
| Batch processing               | Apply the same transformation to many files |

:::tip Chat vs Run
Use **chat mode** when you need iterative feedback, approval at each step, or want to refine the task mid-way. Use **run mode** when the task is fully defined and you want to fire-and-forget.
:::

## CLI Options

```bash
bytemind run -prompt "<task>"                  # basic usage
bytemind run -prompt "<task>" -max-iterations 64  # raise iteration limit
bytemind run -prompt "<task>" -config ./my.json   # custom config
```

| Flag              | Description                 | Default     |
| ----------------- | --------------------------- | ----------- |
| `-prompt`         | Task description (required) | -           |
| `-max-iterations` | Max tool-call rounds        | 64          |
| `-config`         | Path to config file         | auto-detect |

## Approval in Run Mode

By default, run mode still uses `approval_mode: interactive`, which means it will **block waiting for your input** on high-risk operations. For fully automated pipelines, set `full_access` so approval-required operations are auto-approved:

```json
{
  "approval_mode": "full_access"
}
```

Or via environment variable:

```bash
BYTEMIND_APPROVAL_MODE=full_access bytemind run -prompt "regenerate all API docs"
```

:::warning
`away_policy` is a deprecated compatibility field. Keep it only if older tooling still expects that key; it no longer changes runtime behavior.
:::

## Practical Examples

**Update documentation**

```bash
bytemind run -prompt "Regenerate the API reference in docs/api.md based on current source code"
```

**Automated code cleanup in CI**

```bash
BYTEMIND_APPROVAL_MODE=full_access \
  bytemind run -prompt "Remove all TODO comments from the src/ directory and log what was removed"
```

**Version bump**

```bash
bytemind run -prompt "Update the version in go.mod, README.md, and cmd/version.go to v1.2.0"
```

## See Also

- [Chat Mode](/usage/chat-mode) - interactive, multi-turn mode
- [Configuration](/configuration) - approval mode and compatibility fields
- [CLI Commands](/reference/cli-commands) - full flag reference
