# ByteMind User Stories

These stories show common combinations of ByteMind's currently implemented capabilities and call out the architecture chain behind each workflow. They are not a complete feature matrix, and they do not promise that every internal module is covered. For complete commands and configuration fields, use the reference pages.

---

## Story 1: Onboard a New Repository

> **Role**: A backend engineer has just inherited a Go project and wants to understand the structure, entry points, and test flow before changing code.

### 1. Configure a Model and Start

Create a global config at `~/.bytemind/config.json`. If you want to switch between multiple models, use the current `provider_runtime.providers` object format:

```json
{
  "provider_runtime": {
    "current_provider": "deepseek",
    "default_provider": "deepseek",
    "default_model": "deepseek-v4-flash",
    "providers": {
      "deepseek": {
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com",
        "api_key_env": "DEEPSEEK_API_KEY",
        "model": "deepseek-v4-flash",
        "models": ["deepseek-v4-flash", "deepseek-v4-pro"]
      },
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "api_key_env": "OPENAI_API_KEY",
        "model": "gpt-5.4-mini",
        "models": ["gpt-5.4-mini", "gpt-5.4"]
      }
    }
  }
}
```

Start ByteMind from the project directory:

```bash
bytemind
```

`bytemind chat` and `bytemind tui` are compatibility entry points that open the same interactive TUI.

### 2. Explore Read-Only First

Start with a bounded prompt:

```text
Please learn this repository first: map the entry points, main packages, test commands, and configuration loading flow. Do not modify files.
```

ByteMind can call read-only tools such as `list_files`, `read_file`, and `search_text`. If the task benefits from isolated context, you can explicitly mention the built-in explorer subagent:

```text
@explorer Locate the configuration loading path, CLI entry point, and test entry points. Return key file paths.
```

`@explorer` is a delegation hint to the main Agent. When appropriate, the main Agent calls `delegate_subagent` to run the read-only explorer, instead of requiring you to call low-level tools directly.

### 3. Use Sessions and Model Switching

To switch models, enter:

```text
/model
```

The TUI opens a picker for configured provider/model targets. You can also switch directly with a target such as `/model openai/gpt-5.4`.

The conversation is saved automatically. To resume later in the TUI, enter:

```text
/session
```

Then select a recent session with the arrow keys and press `Enter`. The TUI does not expose `/resume <id>`; that command is kept for CLI or scripted recovery paths.

### Architecture Chain

TUI input layer -> Session Store -> Agent Runner -> Tool Registry -> Subagent Gateway -> Provider Runtime -> TUI session and tool rendering

| Layer | Capabilities Involved |
| ---- | --------------------- |
| User entry | `bytemind`, `/model`, `/session` |
| Session layer | Session creation, autosave, recent session restore |
| Agent orchestration | Decide whether to explore directly or delegate to `explorer` |
| Tool layer | `list_files`, `read_file`, `search_text`, `delegate_subagent` |
| Provider layer | `provider_runtime` configuration and model switching |

---

## Story 2: Plan First, Then Execute a Multi-File Change

> **Role**: The engineer needs to extract token validation logic into a separate package. The change touches multiple call sites, so they want to review the plan before writing files.

### 1. Switch to Plan Mode

In the TUI, press `Tab` to switch between Build and Plan modes. After switching to Plan, enter:

```text
Extract token validation from the auth module into internal/tokenval. First give me a plan with files, risks, and verification commands. Do not write files until I confirm.
```

In Plan mode, the Agent explores the relevant code first and then maintains structured steps through `update_plan`. A typical plan includes:

- Read the existing auth middleware and tests
- Design the smallest useful `internal/tokenval` interface
- Update call sites
- Add or adjust tests
- Run `go test ./...`

### 2. Approve the Plan and Enter Build

After approving the plan, use the on-screen execution action to start implementation. As a compatibility fallback, `start execution` or `continue execution` can also move the current plan into execution.

During execution, ByteMind may call `read_file`, `search_text`, `write_file`, `replace_in_file`, `apply_patch`, and `run_shell`. In the default approval mode, file writes and shell commands ask for confirmation. Reads and searches usually run without interruption.

### 3. Control Risk with Approval and Rollback

If ByteMind wants to run:

```bash
go test ./...
```

The TUI shows the command and reason when approval is required. You can approve only this operation, or allow later requests from the same tool during the current TUI session.

If a file edit goes in the wrong direction, the TUI provides:

```text
/rollback
```

It lists file edit snapshots recorded by `write_file`, `replace_in_file`, or `apply_patch`. Use `/rollback last` or a specific operation id to undo a ByteMind-recorded file edit. This is not a Git rollback and does not replace reviewing the diff.

### Architecture Chain

TUI mode state -> Plan State -> `update_plan` -> Agent Runner -> Tool Registry -> Approval/Sandbox -> file edit snapshots -> TUI tool-call rendering

| Layer | Capabilities Involved |
| ---- | --------------------- |
| User entry | `Tab` Build / Plan toggle, plan approval, `/rollback` |
| Plan layer | Structured steps, risks, and verification plan |
| Tool layer | `read_file`, `search_text`, `write_file`, `replace_in_file`, `apply_patch`, `run_shell` |
| Safety layer | High-risk tool approval, `exec_allowlist`, sandbox boundaries |
| Recovery layer | File edit snapshots and rollback |

---

## Story 3: Investigate a Reproducible Bug

> **Role**: A production endpoint intermittently returns 500. The engineer has an error keyword and a short log excerpt, and wants ByteMind to find the root cause and propose the smallest fix.

### 1. Choose the Bug Investigation Workflow

Use the skill picker to choose the built-in Bug Investigation skill:

```text
/skills-select
```

You can also list currently loaded skills:

```text
/skills
```

After activating the skill, enter:

```text
Symptom: the order creation endpoint intermittently returns 500, and logs contain "nil pointer in price calculator". Please gather the reproduction path and evidence first, then propose the smallest fix.
```

The skill steers the Agent toward evidence gathering before guessing at a patch.

### 2. Read Code and Run Verification

ByteMind first uses `search_text` to find the error keyword and related call chain, then uses `read_file` to inspect implementation and tests. When verification is needed, it can run a focused command through `run_shell`, for example:

```bash
go test ./internal/order -run TestCreateOrder
```

If the command is not covered by `exec_allowlist`, the default flow asks for approval before running it.

### 3. Patch and Re-Test

After confirming the root cause, ask for a narrow fix:

```text
Only fix the nil pointer issue in the price calculator, and add one unit test that reproduces it. Do not refactor unrelated code.
```

ByteMind patches only the relevant files and re-runs focused tests. If it reaches `max_iterations`, it outputs a stop summary with completed work, blockers, and recommended next steps instead of silently continuing.

### Architecture Chain

Skill Manager -> Agent Runner -> Tool Registry -> Approval/Sandbox -> Provider Runtime -> Session Store -> TUI result rendering

| Layer | Capabilities Involved |
| ---- | --------------------- |
| User entry | `/skills-select`, `/skills`, natural-language symptom |
| Skill layer | Bug Investigation skill injects the investigation workflow |
| Agent orchestration | Evidence first, minimal fix, verification loop |
| Tool layer | `search_text`, `read_file`, `run_shell`, write-capable tools |
| Budget layer | `max_iterations` limit and stop summary |

---

## Story 4: Review the Current Branch and Commit the Result

> **Role**: A teammate wants to review the current branch against `main`, focusing on regression risk and missing tests.

### 1. Start the Review Workflow

Start ByteMind and describe the review target:

```text
Review the current branch against main. Prioritize correctness issues, regression risks, and missing tests. Give findings first, and do not modify files.
```

ByteMind can use `run_shell` for necessary Git information, then `read_file` and `search_text` to inspect relevant code. Review output should lead with concrete issues, file locations, and risk, rather than a broad summary.

If you want read-only review in isolated context, mention the review subagent:

```text
@review Read-only review the current changes, focusing on concurrency safety and error handling.
```

The main Agent can call the built-in `review` subagent through `delegate_subagent` when appropriate. The review subagent is read-only and does not modify files.

### 2. Check MCP Status

If the project has MCP servers configured, manage them from the shell:

```bash
bytemind mcp list
bytemind mcp add github --cmd npx --args "-y,@modelcontextprotocol/server-github"
bytemind mcp test github
```

In the TUI, inspect current MCP configuration and runtime status:

```text
/mcp list
/mcp show github
```

MCP-provided tools register with stable keys such as `mcp:github:search_code`.

### 3. Create a Local Commit

After the change has been reviewed and tested, create a local commit from the TUI:

```text
/commit fix(order): guard nil price calculator
```

ByteMind runs `git add -A`, creates the commit, and reports the commit hash, message, and file count. If the just-created commit needs to be undone, use:

```text
/undo-commit
```

This only undoes the last local commit created by `/commit` in the current session, and keeps file changes in the working tree.

### Architecture Chain

TUI input layer -> Skill / subagent hint -> Agent Runner -> Tool Registry / MCP Runtime -> Commit Command -> Session Store

| Layer | Capabilities Involved |
| ---- | --------------------- |
| User entry | Review prompt, `@review`, `/mcp list`, `/commit` |
| Review layer | Review skill or read-only review subagent |
| Extension layer | MCP configuration, status inspection, external tool registration |
| Tool layer | `run_shell`, `read_file`, `search_text`, MCP tools |
| Git layer | Local commit and `/undo-commit` |

---

## Common Capabilities Reference

| Scenario | Recommended Entry |
| ---- | ----------------- |
| Start an interactive session | `bytemind`, `bytemind chat`, `bytemind tui` |
| Run a one-shot non-interactive task | `bytemind run -prompt "task"` |
| Switch Build / Plan | Press `Tab` in the TUI |
| View and restore sessions | `/session` in the TUI |
| Start a new session | `/new` in the TUI |
| Switch models | `/model` or `/model provider/model` in the TUI |
| View subagents | `/agents` in the TUI |
| Hint that a subagent should be used | Mention `@explorer` or `@review` in the task |
| View skills | `/skills` or `/skills-select` in the TUI |
| Clear the active skill | `/skill clear` in the TUI |
| Check MCP status | `/mcp list`, `/mcp show <id>` in the TUI |
| Manage MCP config | `bytemind mcp <list|add|remove|enable|disable|test|reload>` in the shell |
| Compact a long conversation | `/compact` in the TUI |
| Roll back ByteMind file edits | `/rollback` in the TUI |
| Create a local commit | `/commit <message>` in the TUI |
| Undo the commit created in this session | `/undo-commit` in the TUI |
