# Session Management

Every conversation in ByteMind exists within a **session**. Sessions persist automatically to disk — interrupt and resume anytime without losing context.

## How Sessions Work

- Each session has a unique ID (e.g., `abc123def`)
- Session data is stored under ByteMind's home directory, which defaults to `.bytemind/` in your user home directory
- When you start `bytemind`, it creates a new session or lets you resume an existing one
- Message history is preserved, giving the agent accumulated context for follow-up tasks; very long sessions automatically trigger context compaction to stay within model window limits

## Opening the Session Picker

```text
/session
```

This opens an interactive picker modal:

```
Recent Sessions
Page 1/3 · Total 22
Up/Down move, Left/Right page, Enter resume, Delete remove, Esc close

> abc123def  2026-05-11 14:22  raw:18
   /d/code/my-project
   Refactoring auth module

  def456ghi  2026-05-10 09:15  raw:5
   /d/code/my-project
   Fix login 500 error
```

Keyboard controls:

| Key | Action |
| --- | ------ |
| `↑` `↓` or `k` `j` | Move cursor up/down |
| `←` `→` | Previous/next page (8 per page, max 10 pages) |
| `Enter` | Switch to selected session (resume context) |
| `Delete` | Remove the selected session |
| `Esc` | Close picker, stay in current session |

There is no separate `/sessions` or `/resume` command — viewing, resuming, and deleting sessions are all done within the `/session` picker.

## Starting a New Session

```text
/new
```

Creates a new session in the current workspace. Previous sessions remain saved and can be resumed anytime via the `/session` picker.

## Practical Scenarios

**Multi-day refactoring**

Work on part of a large task each day, then resume where you left off:

```
/session → select yesterday's session → Enter
```

**Parallel workflows**

Use `/new` to create separate sessions for different feature branches, keeping context focused. Each session persists independently.

**Clean up old sessions**

```
/session → navigate to unused sessions → Delete
```

## Storage Location

Session files are stored under ByteMind's home directory, which defaults to `.bytemind/` in your user home. Override with the `BYTEMIND_HOME` environment variable.

## See Also

- [Interactive Mode (Build)](/usage/chat-mode) — session usage in conversations
- [Environment Variables](/reference/env-vars) — `BYTEMIND_HOME` override
- [CLI Commands](/reference/cli-commands) — full command reference
