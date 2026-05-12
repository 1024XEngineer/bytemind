# User Stories

Four end-to-end scenarios covering all of ByteMind's functionality.

---

## Story 1: Design — "Creating a Technical Plan for a New Module"

**Role**: A backend engineer who needs to produce a technical plan for a push notification module.

### 1. Install and Onboard

Install ByteMind on Windows:

 + '`' + powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
 + '`' + 

Copy the example config, fill in your API key, and add backup providers.

### 2. Launch Plan Mode and Explore

 + '`' + ash
bytemind chat
 + '`' + 

Use  + '/new' +  to create a session,  + '/models' +  to switch models. Press Tab to open the sub-agent panel:

 + '`' + 	ext
@explorer Map all code files and module dependencies related to push notifications.
 + '`' + 

The sub-agent traverses the project with list_files, search_text, and read_file, returning a dependency report.

### 3. Plan Mode: From Exploration to Proposal

Enter /plan:

> "Design a push notification module supporting both APNs and FCM channels."

ByteMind walks through the Plan phase pipeline: explore -> clarify -> draft -> converge_ready -> approved_to_build.

The Plan panel shows step status and risk levels. Context window usage triggers a warning at 85%.

### 4. Load a Skill

Activate the write-rfc skill: /skill write-rfc

### 5. Persistence

Use /sessions to view the session list. Sessions auto-persist as JSONL.

**Covered features**: Run modes (chat/tui, install), Providers (OpenAI-compatible, Anthropic, Gemini, routing, model switching), TUI (Bubble Tea, onboarding, panels, context visualization, autocomplete), Plan mode (phase pipeline, step tracking, risk levels), Tools (list_files, read_file, search_text, web_fetch, web_search), Sub-agents (explorer), Skills (write-rfc), Sessions (JSONL persistence, restore), Context (window budget, alerts), Notifications, Config

---

## Story 2: Development — "Building the Push Notification Module"

**Role**: After confirming the plan, switch to Build mode to start coding.

### 1. Switch to Build Mode

Resume with `/resume <id>`, switch to Build mode:

> "Implement the push notification module: APNs and FCM providers, message queue, and retry logic."

Build mode streams thinking and tool calls in different colors.

### 2. High-Intensity Tool Calling

ByteMind orchestrates: write_file, replace_in_file, apply_patch, run_shell. The TUI renders tool output with syntax-highlighted diffs.

### 3. Safety Approval and Sandbox

With approval_policy: "on-request", shell commands trigger an approval dialog. Sandbox enforces:
- File sandbox (writable_roots)
- Command whitelist (go build/test/mod/vet/fmt)
- Network sandbox (api.github.com only)

### 4. Provider Failover

When primary provider returns 503, health checks auto-switch to backup provider. Status bar shows the switch.

### 5. Parallel Sub-agent Acceleration

Dispatch general sub-agent in background: @general Write unit tests for push/. Check results with task_output.

### 6. Budget Control and Context Compression

After 50+ tool calls, stop summary triggers. Context compression auto-compresses earlier turns. Duplicate call detection catches repeats.

### 7. Token Usage Monitoring

Real-time token monitor shows input/output/total. Alert fires near threshold. Data persisted to SQLite.

**Covered features**: Build mode, yolo, Provider health/failover, Streaming, tool loop, max_iterations, stop summary, duplicate detection, context compression, write_file/replace_in_file/apply_patch/run_shell, Markdown rendering, diff highlighting, approval dialog, background tasks, token monitoring, file/network/command sandbox, worktree isolation

---

## Story 3: Debugging — "Investigating Push Failure in Production"

**Role**: Push module has intermittent failures. Find the root cause.

### 1. Quick Problem Location

Resume session: /resume push-module

> "Push notifications failing intermittently. Error: 'connection timeout after 30s'."

ByteMind searches for timeout configs, finds hardcoded 30s in push/apns.go.

### 2. Deep Investigation: Shell + Web

Runs go test -v -run TestAPNsRetry ./push/..., finds no timeout coverage. Web searches "APNs timeout best practice", fetches Apple docs.

### 3. Activate Bug Investigation Skill

/skill bug-investigation replaces prompt with bug investigation template. Systematically checks:
- Reproduction conditions
- Impact scope
- Code root cause
- Config/environment factors
- Fix plan and regression

Finds two root causes: hardcoded timeout + retry logic not catching context.DeadlineExceeded.

### 4. Fix and Verify

> "Make timeout configurable (default 60s), fix retry.go context.DeadlineExceeded handling."

ByteMind uses replace_in_file, then go vet and go test -race to verify.

**Covered features**: chat TUI, /resume, search_text, read_file, run_shell, replace_in_file, web_search, web_fetch, bug-investigation skill, diff syntax highlighting, mouse/clipboard, command whitelist

---

## Story 4: Code Review — "Reviewing the Push Module PR"

**Role**: A teammate reviews the push module changes with deep analysis.

### 1. Start the Review

 + '`' + ash
bytemind chat
 + '`' + 

> "Review this branch against main. Focus on concurrency safety, error handling, resource leaks."

### 2. Activate Review Skill + Review Sub-agent

/skill review then @review Review all files under push/ for concurrency safety issues.

Sub-agent finds improperly closed channel in push/queue.go causing goroutine leak.

### 3. Per-File Review with MCP Integration

 + '`' + ash
bytemind mcp add my-linter -- node ./linter-mcp-server.js
bytemind mcp list
bytemind mcp health my-linter
 + '`' + 

MCP tools auto-register. Reviews push/apns.go, push/fcm.go, push/queue.go, push/retry.go.

### 4. Diff Preview and Summary

diff_preview generates change summary. TUI diff renderer highlights additions/deletions/modifications.

Final report: Critical (goroutine leak, high), Suggestion (timeout config validation, medium), Test coverage (timeout scenarios covered, pass).

Session auto-persisted. Use /session for message stats and token consumption.

**Covered features**: MCP management (add/list/health), MCP panel, diff renderer, review skill, review sub-agent, diff_preview, JSONL persistence, token display

---

## Feature Coverage Overview

| Module | Story 1 | Story 2 | Story 3 | Story 4 |
|--------|:---:|:---:|:---:|:---:|
| Run Modes (chat/tui, run, install, mcp, yolo) | ✅ | ✅ | ✅ | ✅ |
| Providers (OpenAI, Anthropic, Gemini, routing, failover) | ✅ | ✅ | | |
| Engine (conversation, streaming, Build/Plan mode, compression) | ✅ | ✅ | ✅ | |
| Tools (files, search, shell, web, diff) | ✅ | ✅ | ✅ | ✅ |
| TUI (Bubble Tea, panels, rendering, notifications) | ✅ | ✅ | ✅ | ✅ |
| Security (approval, sandbox, whitelist, worktree) | | ✅ | ✅ | |
| Extensions (MCP, Skills, Sub-agents) | ✅ | ✅ | ✅ | ✅ |
| Plan Mode (pipeline, tracking, risks) | ✅ | | | |
| Sessions (persistence, restore) | ✅ | ✅ | ✅ | ✅ |
| Context & Token (budget, compression, monitoring) | ✅ | ✅ | | ✅ |
| Background Tasks (parallel, timeout) | | ✅ | | |
| Notifications (desktop, approval) | ✅ | ✅ | | |
| Config (JSON, env vars, provider_runtime) | ✅ | | | |
