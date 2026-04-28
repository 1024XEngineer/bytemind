# CLI Commands

ByteMind exposes two top-level commands: `chat` and `run`.

## `bytemind chat`

Start an interactive, multi-turn session.

```bash
bytemind chat [flags]
```

| Flag                  | Description                   | Default     |
| --------------------- | ----------------------------- | ----------- |
| `-config <path>`      | Path to config file           | auto-detect |
| `-max-iterations <n>` | Max tool-call rounds per task | 32          |
| `-v`                  | Enable verbose/debug output   | false       |

**Examples:**

```bash
bytemind chat
bytemind chat -max-iterations 64
bytemind chat -config ~/.bytemind/work.json
bytemind chat -v
```

## `bytemind run`

Execute a single task non-interactively and exit.

```bash
bytemind run -prompt "<task>" [flags]
```

| Flag                  | Description                     | Default     |
| --------------------- | ------------------------------- | ----------- |
| `-prompt <text>`      | Task description **(required)** | —           |
| `-config <path>`      | Path to config file             | auto-detect |
| `-max-iterations <n>` | Max tool-call rounds per task   | 32          |
| `-v`                  | Enable verbose/debug output     | false       |

**Examples:**

```bash
bytemind run -prompt "Update the README installation section"
bytemind run -prompt "Rename Foo to Bar across all Go files" -max-iterations 64
```

## `bytemind --version`

Print the installed version and build info, then exit.

```bash
bytemind --version
# ByteMind v0.4.0 (go1.24.0 darwin/arm64)
```

## Session Slash Commands

These are typed inside an active `bytemind chat` session, not on the shell:

| Command                                       | Description                                  |
| --------------------------------------------- | -------------------------------------------- |
| `/help`                                       | List all available commands                  |
| `/session`                                    | Show current session ID and summary          |
| `/sessions [n]`                               | List most recent n sessions (default 10)     |
| `/resume <id>`                                | Resume a session by ID or prefix             |
| `/new`                                        | Start a new session in the current workspace |
| `/plan`                                       | Switch to Plan mode                          |
| `/build`                                      | Switch to Build mode                         |
| `/quit`                                       | Exit safely                                  |
| `/bug-investigation [symptom="..."]`          | Activate bug investigation skill             |
| `/review [base_ref=<ref>]`                    | Activate code review skill                   |
| `/github-pr [pr_number=<n>] [base_ref=<ref>]` | Activate GitHub PR skill                     |
| `/repo-onboarding`                            | Activate repo onboarding skill               |
| `/write-rfc [path=<file>]`                    | Activate RFC writing skill                   |

## Config Load Order

When no `-config` flag is given, ByteMind looks for config in this order:

1. `.bytemind/config.json` in the current working directory
2. `config.json` in the current working directory
3. `~/.bytemind/config.json` in the home directory

## See Also

- [Config Reference](/reference/config-reference)
- [Environment Variables](/reference/env-vars)
- [Sessions](/usage/session-management)
