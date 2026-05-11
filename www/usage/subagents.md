# Subagents

**Subagents** are specialized agents with their own tool sets and instructions, invoked by the main agent via the `delegate_subagent` tool to complete sub-tasks within a defined scope. Subagents are ideal for parallel work, context isolation, or restricting tool permissions in complex scenarios.

## How They Work

1. The main agent identifies decomposable sub-tasks
2. It calls `delegate_subagent` with an agent name and task description
3. The subagent runs in an isolated session context
4. After completion, results (summary, modified files, transcript) are returned to the main agent
5. The main agent integrates the result and continues

Subagents can be configured with tool allowlists/denylists, max turns, isolation modes, and more to keep execution scope under control.

## Built-in Subagents

ByteMind ships with three built-in subagents:

### explorer

Read-only repository exploration agent. Finds files, symbols, call chains, and configuration flows. No writes or shell commands.

| Property    | Value                                              |
| ----------- | -------------------------------------------------- |
| Tools       | `list_files` `read_file` `search_text`             |
| Max turns   | 6                                                  |
| When to use | Finding files, understanding code structure, locating patterns |

```text
/explorer
```

### review

Code review agent. Analyzes correctness, regression risk, security issues, and test coverage gaps. Findings only — no code modifications.

| Property    | Value                                              |
| ----------- | -------------------------------------------------- |
| Tools       | `list_files` `read_file` `search_text`             |
| Max turns   | 8                                                  |
| When to use | Reviewing code, checking for bugs, assessing quality |

```text
/review
```

### general

General-purpose coding agent with read/write access. Handles complex multi-step tasks requiring file modifications. Cannot further delegate to other subagents.

| Property    | Value                                                          |
| ----------- | -------------------------------------------------------------- |
| Tools       | `list_files` `read_file` `search_text` `replace_in_file` `write_file` |
| Max turns   | 12                                                             |
| Isolation   | none                                                           |
| When to use | Multi-file edits, refactoring, feature implementation          |

## Listing Subagents

List all available subagents:

```text
/agents
```

View details for a specific subagent:

```text
/agents explorer
```

Show built-in agent definitions directly:

```text
/explorer
/review
```

## Custom Subagents

Define project-level subagents by creating `.md` files under `.agents/agents/`. Each file uses YAML frontmatter for metadata, with the Markdown body serving as the subagent's execution instructions.

### Directory Structure

```
.agents/
  agents/
    frontend-developer.md
    api-tester.md
```

### Frontmatter Fields

| Field             | Type   | Description                                  | Default                  |
| ----------------- | ------ | -------------------------------------------- | ------------------------ |
| `name`            | string | Agent name used in `delegate_subagent` calls | Filename (no extension)  |
| `description`     | string | Short description shown in `/agents` list    | Auto-extracted from body |
| `tools`           | array  | Allowed tools whitelist                      | —                        |
| `disallowed_tools`| array  | Forbidden tools blacklist                    | —                        |
| `model`           | string | Specific model override (e.g. `sonnet`)       | Inherited from session   |
| `mode`            | string | Work mode: `build` or `plan`                 | `build`                  |
| `max_turns`       | int    | Maximum tool call turns                      | 0 (unlimited)            |
| `isolation`       | string | Isolation mode: `none` or `worktree`         | `none`                   |
| `when_to_use`     | string | Hint for when the main agent should delegate | —                        |
| `aliases`         | array  | Alias names                                  | Auto-generated           |

### Example

```markdown
---
name: api-tester
description: "Focused on writing and debugging integration tests for API endpoints"
tools: [list_files, read_file, search_text, write_file, replace_in_file, run_shell]
disallowed_tools: [delegate_subagent]
model: sonnet
max_turns: 10
isolation: none
when_to_use: "For writing API tests, debugging endpoint issues, adding HTTP test coverage"
---

You are an API testing specialist.

## Workflow

1. Read relevant handler and route definitions first
2. Analyze existing test patterns and coverage gaps
3. Write minimal but complete integration tests
4. Run tests and fix failures

## Standards

- Each test case is independent and reproducible
- Cover happy paths and common error paths
- Do not modify business logic code
```

### Scope Priority

Subagents load from three scopes. Agents with the same name are overridden by higher priority scopes:

| Scope     | Path                      | Priority |
| --------- | ------------------------- | -------- |
| `project` | `.agents/agents/`         | Highest  |
| `user`    | `~/.bytemind/agents/`     | Medium   |
| `builtin` | Built-in                  | Lowest   |

Project-level configuration takes the highest priority: `.agents/agents/frontend-developer.md` (project) overrides `~/.bytemind/agents/frontend-developer.md` (user).

## Subagent Isolation (Worktree)

Setting `isolation: worktree` causes the subagent to run in an isolated git worktree. File changes are separated from the main workspace. The worktree can be kept or discarded upon completion. Ideal for scenarios requiring extensive experimental changes.

## Delegating in Conversation

You generally don't need to call `delegate_subagent` manually — the main agent decides when delegation is appropriate based on task complexity. You can also ask directly:

```text
Use the explorer subagent to find all auth-related middleware
Use the review subagent to review the latest changes
```

## See Also

- [Skills](/usage/skills) — specialized workflows activated by slash commands
- [Chat Mode](/usage/chat-mode) — the interactive mode in which subagents run
- [Core Concepts](/core-concepts) — agent modes and tools
