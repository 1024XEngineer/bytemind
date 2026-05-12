# CLI Commands

ByteMind starts the interactive UI when you run it without a subcommand. `chat` is a compatibility alias, and `run` executes a single non-interactive task.

## `bytemind`

Start an interactive, multi-turn session.

```bash
bytemind [flags]
```

| Flag                  | Description                   | Default     |
| --------------------- | ----------------------------- | ----------- |
| `-config <path>`      | Path to config file           | auto-detect |
| `-max-iterations <n>` | Max tool-call rounds per task | 32          |
| `-workspace <path>`   | Workspace directory to open   | current dir |
| `-v`                  | Enable verbose/debug output   | false       |

**Examples:**

```bash
bytemind
bytemind -max-iterations 64
bytemind -config ~/.bytemind/work.json
bytemind -workspace D:\code\my-project
bytemind -v
```

`bytemind chat` and `bytemind tui` still work and behave like the default interactive UI.

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

## `bytemind doctor`

Check the environment, configuration, API key, workspace, and security settings.

```bash
bytemind doctor [-workspace path]
```

## `bytemind safety`

View or explain the ByteMind safety model.

```bash
bytemind safety status         # Show current safety configuration
bytemind safety explain        # Explain the layered safety model
```

## `bytemind --version`

Print the installed version, then exit.

```bash
bytemind --version
# vX.Y.Z
```

## Session Slash Commands

These are typed inside an active `bytemind` interactive session, not on the shell:

| Command                                       | Description                                  |
| --------------------------------------------- | -------------------------------------------- |
| `/help`                                       | List all available commands                  |
| `/session`                                    | Show current session ID and summary          |
| `/sessions [n]`                               | List most recent n sessions (default 10)     |
| `/resume <id>`                                | Resume a session by ID or prefix             |
| `/new`                                        | Start a new session in the current workspace |
| `/plan`                                       | Switch to Plan mode                          |
| `/build`                                      | Switch to Build mode                         |
| `/commit <message>`                           | Stage all current changes and create a local Git commit |
| `/undo-commit`                                | Undo the last local commit created by `/commit` in this session |
| `/skills`                                     | List available skills and diagnostics        |
| `/skill clear`                                | Clear the active skill for this session      |
| `/skill delete <name>`                        | Delete a project skill                       |
| `/quit`                                       | Exit safely                                  |

### `/commit <message>`

Use `/commit` inside a `bytemind` session when you want ByteMind to save the current workspace changes as a local Git commit.

```text
/commit fix(/commit): improve commit feedback
```

When you choose `/commit` from the slash command palette, ByteMind fills the input with `/commit ` and waits for you to type the message. After you press Enter, ByteMind runs `git add -A`, creates the commit, and reports the commit hash, message, and number of files included.

### `/undo-commit`

Use `/undo-commit` to undo the last commit that ByteMind created with `/commit` in the current session.

```text
/undo-commit
```

ByteMind only runs this when the current `HEAD` is still that session commit, the working tree has no newer changes, and the upstream branch does not already contain the commit. It uses `git reset --soft HEAD~1`, so the file changes remain available locally.

## Config Load Order

When no `-config` flag is given, ByteMind loads global config first, then project overrides:

1. `~/.bytemind/config.json` in the home directory
2. `.bytemind/config.json` in the current workspace (optional project overrides)

## See Also

- [Config Reference](/reference/config-reference)
- [Environment Variables](/reference/env-vars)
- [Sessions](/usage/session-management)
