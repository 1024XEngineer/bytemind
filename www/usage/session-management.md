# Session Management

Every ByteMind conversation lives inside a **session**. Sessions are automatically persisted to disk so you can stop and resume work at any time without losing context.

## How Sessions Work

- Each session has a unique ID (e.g. `abc123def`)
- Session data is stored in `.bytemind/` in your working directory
- When you start `bytemind chat`, it creates a new session or lets you resume an existing one
- The full message history is preserved, so the agent has context for follow-up tasks

## Listing Sessions

```text
/sessions
```

Shows a table of recent sessions with IDs, start times, and message counts. Pass a number to limit results:

```text
/sessions 5
```

## Inspecting the Current Session

```text
/session
```

Displays the current session's ID, workspace path, message count, and a brief summary.

## Resuming a Session

```text
/resume abc123
```

You can use the full ID or a unique prefix. The agent loads the full conversation history and you can continue where you left off.

## Starting a New Session

```text
/new
```

Creates a fresh session in the current workspace. The previous session remains saved and can be resumed later.

## Practical Patterns

**Long refactors across multiple days**

Start a refactor session, do a chunk of work, `/quit`. Come back the next day:

```bash
bytemind chat
/sessions
/resume <id from yesterday>
```

**Parallel workstreams**

Use `/new` to create a separate session for a different feature branch so contexts don't mix.

**Reviewing what was done**

```text
/session
```

Shows a summary of tools called and changes made in the current session.

## Storage Location

Session files are stored at `.bytemind/sessions/` relative to the working directory where you ran `bytemind chat`. Use `BYTEMIND_HOME` to override the `.bytemind/` base directory.

## See Also

- [Chat Mode](/usage/chat-mode) — where sessions are used
- [Environment Variables](/reference/env-vars) — `BYTEMIND_HOME` override
- [CLI Commands](/reference/cli-commands) — full command reference
