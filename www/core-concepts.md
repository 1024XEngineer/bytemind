# Core Concepts

Understanding these concepts helps you get the most out of ByteMind.

## Agent Modes

ByteMind has two working modes you can switch between at any time during a session.

### Build Mode (Default)

In Build mode the agent reads files, searches code, writes changes, and runs verification commands directly after receiving your task. Best for most everyday coding work:

- Fixing bugs
- Adding new features
- Refactoring code
- Updating documentation

```bash
bytemind chat          # starts in Build mode by default
```

### Plan Mode

In Plan mode the agent first produces a step-by-step plan that you review before any execution begins. Best for complex, multi-step tasks:

- Large-scale refactors spanning many modules
- Feature implementations with sequential dependencies
- Phased migrations requiring stage-by-stage sign-off

Switch modes with slash commands inside a session:

```text
/plan    switch to Plan mode
/build   switch back to Build mode
```

## Sessions

Every `bytemind chat` invocation creates or resumes a **session**. Sessions automatically persist the full conversation context.

- Stored in the `.bytemind/` directory
- Survive interruptions — restart and continue where you left off
- Multiple sessions can coexist; switch by ID

Common session commands:

| Command         | Description                                  |
| --------------- | -------------------------------------------- |
| `/session`      | Show current session ID and summary          |
| `/sessions [n]` | List the most recent n sessions (default 10) |
| `/resume <id>`  | Resume a session by ID or prefix             |
| `/new`          | Start a new session in the current workspace |

## Tools

Tools are the capability units the agent uses to take action. ByteMind ships with:

| Tool              | What it does                       |
| ----------------- | ---------------------------------- |
| `list_files`      | List directory structure           |
| `read_file`       | Read file contents                 |
| `search_text`     | Full-text search (regex supported) |
| `write_file`      | Write or create files              |
| `replace_in_file` | Replace specific content in a file |
| `apply_patch`     | Apply a unified diff patch         |
| `run_shell`       | Execute shell commands             |
| `update_plan`     | Update the task plan (Plan mode)   |
| `web_fetch`       | Fetch a web page                   |
| `web_search`      | Search the web                     |

High-risk tools (`write_file`, `replace_in_file`, `apply_patch`, `run_shell`) trigger the approval flow before executing.

## Approval Policy

The approval policy controls how the agent handles high-risk operations:

- **`on-request` (default)**: waits for your explicit confirmation before each high-risk tool call
- **Away mode**: in unattended scenarios, automatically denies or aborts based on `away_policy`

See [Tools and Approval](/usage/tools-and-approval) for details.

## Iteration Budget

`max_iterations` caps the number of **tool-call rounds** per task, preventing runaway loops from consuming tokens:

- Default: `32`
- When the limit is reached, the agent produces a progress summary and stops gracefully
- Raise it for complex tasks via config or CLI flag:

```bash
bytemind chat -max-iterations 64
```

## Skills

A skill is an **activatable workflow guide** — it injects additional system-level instructions that steer the agent through a domain-specific process.

ByteMind's built-in skills:

| Skill             | Command              | When to use                          |
| ----------------- | -------------------- | ------------------------------------ |
| Bug Investigation | `/bug-investigation` | Structured bug diagnosis             |
| Code Review       | `/review`            | Focus on correctness and risk        |
| GitHub PR         | `/github-pr`         | Analyze PR diffs and merge risk      |
| Repo Onboarding   | `/repo-onboarding`   | Get up to speed on a new repo        |
| Write RFC         | `/write-rfc`         | Draft structured technical proposals |

Skills also support project-level and user-level customization. See [Skills](/usage/skills).

## Context Budget

ByteMind tracks token consumption per session and warns before approaching the model's context window limit:

- `warning_ratio` (default 0.85): emits a warning at 85% usage
- `critical_ratio` (default 0.95): triggers compaction or stops at 95% usage

Adjust these thresholds in the `context_budget` section of your config file.
