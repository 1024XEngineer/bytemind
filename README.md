<p align="right">
  <b>English</b> | <a href="./README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <img src="https://capsule-render.vercel.app/api?type=waving&height=240&color=0:020617,35:0ea5e9,70:2563eb,100:4f46e5&text=ByteMind&fontAlignY=38&desc=Terminal-native%20AI%20Coding%20Agent&descAlignY=58&fontColor=ffffff&fontSize=58&animation=fadeIn" alt="ByteMind Banner" />
</p>

<p align="center">
  <img src="https://readme-typing-svg.demolab.com?font=Fira+Code&pause=1200&center=true&vCenter=true&width=900&lines=Inspect+repositories+with+AI;Plan+before+you+build;Run+tools+under+human+control;Skills.+MCP.+SubAgents.;Built+for+real+engineering+workflows" alt="Typing SVG" />
</p>

<h1 align="center">ByteMind</h1>

<p align="center">
  <strong>A terminal-native AI coding agent for real repositories.</strong>
</p>

<p align="center">
  Let AI inspect code, search files, run commands, edit files, plan tasks, and operate under configurable human approval.
</p>

<p align="center">
  <a href="https://github.com/1024XEngineer/bytemind/stargazers"><img src="https://img.shields.io/github/stars/1024XEngineer/bytemind?style=for-the-badge&logo=github&color=f59e0b" alt="Stars" /></a>
  <a href="https://github.com/1024XEngineer/bytemind/network/members"><img src="https://img.shields.io/github/forks/1024XEngineer/bytemind?style=for-the-badge&logo=github&color=06b6d4" alt="Forks" /></a>
  <a href="https://github.com/1024XEngineer/bytemind/releases"><img src="https://img.shields.io/github/v/release/1024XEngineer/bytemind?style=for-the-badge&color=8b5cf6" alt="Release" /></a>
  <a href="https://github.com/1024XEngineer/bytemind/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-22c55e?style=for-the-badge" alt="License" /></a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-1f2937?style=flat-square" alt="Platform" />
  <img src="https://img.shields.io/badge/Provider-OpenAI--Compatible%20%7C%20Anthropic-334155?style=flat-square" alt="Provider" />
  <img src="https://img.shields.io/badge/Mode-Build%20%7C%20Plan-0f766e?style=flat-square" alt="Mode" />
  <img src="https://img.shields.io/badge/Runtime-Skills%20%7C%20MCP%20%7C%20SubAgents-6d28d9?style=flat-square" alt="Runtime" />
  <a href="https://github.com/1024XEngineer/bytemind/actions"><img src="https://img.shields.io/github/actions/workflow/status/1024XEngineer/bytemind/ci.yml?style=flat-square&logo=github&label=CI" alt="CI" /></a>
  <a href="./evals/README.md"><img src="https://img.shields.io/badge/evals-reproducible-8b5cf6?style=flat-square" alt="Evals" /></a>
  <a href="./DEMO.md"><img src="https://img.shields.io/badge/demo-5--minute-16a34a?style=flat-square" alt="Demo" /></a>
  <a href="https://codecov.io/gh/1024XEngineer/bytemind"><img src="https://img.shields.io/codecov/c/github/1024XEngineer/bytemind?style=flat-square&token=&label=coverage" alt="Coverage" /></a>
</p>

<p align="center">
  <a href="https://1024xengineer.github.io/bytemind/zh/"><b>Documentation</b></a>
  ·
  <a href="#why-bytemind"><b>Why ByteMind</b></a>
  ·
  <a href="#use-cases"><b>Use Cases</b></a>
  ·
  <a href="#quick-start"><b>Quick Start</b></a>
  ·
  <a href="#feature-matrix"><b>Feature Matrix</b></a>
  ·
  <a href="#architecture"><b>Architecture</b></a>
  ·
  <a href="#skills-mcp-and-subagents"><b>Skills / MCP / SubAgents</b></a>
</p>

---

## Why ByteMind

ByteMind is built for developers who want AI to work **inside the repository**, not outside it.

Instead of stopping at suggestions, ByteMind can participate in the actual engineering loop:

```text
Prompt → Plan → Tool Call → Observation → Code Change → Verification → Result
```

<p align="center">
  <img src="https://img.shields.io/badge/Terminal--native-Work%20where%20developers%20already%20operate-0ea5e9?style=for-the-badge" alt="Terminal-native" />
  <img src="https://img.shields.io/badge/Human--in--the--loop-Control%20high-risk%20execution-f59e0b?style=for-the-badge" alt="Human-in-the-loop" />
  <img src="https://img.shields.io/badge/Extensible-Encode%20workflows%20as%20runtime%20capabilities-8b5cf6?style=for-the-badge" alt="Extensible" />
</p>

<table>
  <tr>
    <td width="33%" align="center">
      <h3>🧠 Plan</h3>
      <p>Use <b>Plan mode</b> for higher-risk tasks. Review the approach before making changes.</p>
    </td>
    <td width="33%" align="center">
      <h3>🛠 Execute</h3>
      <p>Inspect files, search code, apply patches, run commands, and fetch external context when needed.</p>
    </td>
    <td width="33%" align="center">
      <h3>🧭 Control</h3>
      <p>Keep sensitive actions behind approval policies and runtime boundaries.</p>
    </td>
  </tr>
</table>

---

## Use Cases

| Scenario | What ByteMind can do |
| --- | --- |
| Understand a new repository | Inspect structure, find entrypoints, and map key modules and call paths |
| Debug failing tests | Read failures, locate related code, patch the issue, and verify again |
| Review or refine changes | Check correctness, regression risk, and missing test coverage |
| Generate technical plans and RFCs | Turn repository context into an actionable implementation proposal |
| Automate repeated engineering tasks | Encode common workflows through Skills, MCP, or SubAgents |
| Collaborate under approval | Read and write files, run commands, and advance tasks while preserving approval boundaries |

---

## Quick Start

### Install

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

**Windows PowerShell**

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

**Install a specific version**

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=vX.Y.Z bash
```

```powershell
$env:BYTEMIND_VERSION='vX.Y.Z'; iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

### Configure

```bash
mkdir -p .bytemind
cp config.example.json .bytemind/config.json
```

### Run

```bash
bytemind chat
```

```bash
bytemind run -prompt "Analyze this repository and summarize the architecture"
```

```bash
bytemind run -prompt "Refactor this module and update tests" -max-iterations 64
```

---

## 5-Minute Demo

A reproducible bug-fix cycle that demonstrates ByteMind's full engineering loop:

```bash
go run ./cmd/bytemind run \
  -prompt "Fix the failing test and verify it passes" \
  -workspace examples/bugfix-demo/broken-project \
  -approval-mode full_access
```

| Step | Tool | What happens |
|------|------|-------------|
| 1 | `list_files` | Reads project structure |
| 2 | `read_file` | Reads source code and test file |
| 3 | `run_tests` | Discovers the failing test |
| 4 | `replace_in_file` | Fixes the divide-by-zero bug |
| 5 | `run_tests` | Verifies all tests pass |
| 6 | `git_diff` | Shows the exact change made |

**The bug**: `CalculateAverage` returns `NaN` on empty slice (divide by zero).  
**The fix**: Add a guard clause for `len(nums) == 0`.  

**Offline verification** (no API key needed):
```bash
go run ./evals/runner.go -smoke -run bugfix_go_001
```

See [examples/bugfix-demo/](examples/bugfix-demo/README.md) for details, [DEMO.md](DEMO.md) for a judge-facing walkthrough, and [ENGINEERING.md](ENGINEERING.md) for engineering evidence.

---

## Engineering Evidence

ByteMind is built for evaluators who need reproducible, verifiable engineering output.

### Real Agent Loop
Multi-step tool use with observation feedback, context compaction, rate-limit retry, and execution budgets (`internal/agent/engine_run_loop.go`).

### Coding-native Tools
14 built-in tools with JSON-structured output — `git_status`, `git_diff`, `run_tests`, file read/search/write/patch, shell execution, and web access. Each tool has unit tests and a safety classification.

### Reproducible Demo
`examples/bugfix-demo/broken-project` is a self-contained Go project that fails `go test ./...` initially and passes after agent fix. Complete with expected output and offline verification.

### Evaluation System
YAML-defined eval tasks run via `evals/runner.go` with flexible success criteria: command exit codes, output patterns, file content regex, and file modification detection. CI-integrated with `-validate` and `-smoke` flags.

### Safety Boundary
Three-layer safety model: approval policy (`on-request`/`always`/`never`), sandbox (`off`/`best_effort`/`required`), and runtime boundaries (writable roots, exec allowlist, network allowlist). See `bytemind safety explain`.

### CI and Testing
PR-gated CI: `go build ./...`, unit tests with coverage, sandbox acceptance on Linux/macOS/Windows, and eval smoke checks. See [`.github/workflows/ci.yml`](.github/workflows/ci.yml).

### Extensibility
Skills, MCP servers, and SubAgents for encoding reusable workflows and delegating focused work.

---

## Terminal Preview

<p align="center">
  <img src="./docs/assets/bytemind-terminal-preview.webp" alt="ByteMind terminal preview" width="960" />
</p>

---

<a id="feature-matrix"></a>

## Feature Matrix

| Category | Capability | Notes |
| --- | --- | --- |
| **Terminal UX** | Terminal-first interaction | Built for repository-centric workflows |
| **Streaming** | Real-time output | Useful for long-running tasks |
| **Agent Loop** | Multi-step tool use + observations | More than a one-shot reply |
| **Build / Plan** | Separate planning and execution modes | Better for high-risk changes |
| **Files** | Read, search, write, replace, patch | Core repository operations |
| **Git** | `git_status`, `git_diff` | Show working tree status and changes |
| **Testing** | `run_tests` | Auto-detect and run project tests |
| **Shell** | Run commands under approval | Keep execution visible and controlled |
| **Web** | Search and fetch external content | Useful when external context is needed |
| **Sessions** | Persist and resume tasks | Suitable for long-running work |
| **Skills** | Reusable workflows | Bug investigation, review, RFC, onboarding |
| **MCP** | External tool / context integration | Extend the runtime beyond local tools |
| **SubAgents** | Focused delegated work | Reduce noise in the main context |
| **Safety** | Approval, allowlists, writable roots | Human-in-the-loop execution |
| **Providers** | OpenAI-compatible / Anthropic | Configurable runtime support |

---

## Built-in Tools

| Tool | Purpose |
| --- | --- |
| `list_files` | Inspect repository structure and candidate file scopes |
| `read_file` | Read source code, docs, config, and test content |
| `search_text` | Locate symbols, error messages, or call sites by keyword |
| `git_status` | Show the working tree status (staged, unstaged, untracked) |
| `git_diff` | Output a unified diff of the current changes |
| `run_tests` | Auto-detect and run project tests, return results |
| `write_file` | Create or fully rewrite files |
| `replace_in_file` | Make small text replacements in existing files |
| `apply_patch` | Apply incremental file changes through patches |
| `run_shell` | Run commands inside the approval boundary and read results |
| `web_search` | Search external sources when local context is insufficient |
| `web_fetch` | Fetch a specific page as supplemental context |

---

## Core Experience

<table>
  <tr>
    <td width="50%">
      <h3>✅ What ByteMind is good at</h3>
      <ul>
        <li>Understanding unfamiliar repositories</li>
        <li>Debugging code and failing tests</li>
        <li>Planning and applying small refactors</li>
        <li>Reviewing correctness and regression risk</li>
        <li>Writing RFC-style implementation plans</li>
        <li>Automating repetitive coding workflows</li>
      </ul>
    </td>
    <td width="50%">
      <h3>⚙️ What makes it practical</h3>
      <ul>
        <li>Approval before sensitive actions</li>
        <li>Execution budget via <code>max_iterations</code></li>
        <li>Session persistence</li>
        <li>Provider-agnostic runtime</li>
        <li>Extensible skills and external tools</li>
        <li>SubAgent-based context isolation</li>
      </ul>
    </td>
  </tr>
</table>

---

## How It Works

```mermaid
graph TD
    A["User Prompt"] --> B["Build Runtime Context"]
    B --> C["LLM decides: answer or tool call"]
    C --> D{"Tool Call?"}
    D -- "No" --> E["Final Answer"]
    D -- "Yes" --> F["Approval / Policy Check"]
    F --> G["Execute Tool"]
    G --> H["Observation appended to session"]
    H --> I{"Done?"}
    I -- "No" --> C
    I -- "Yes" --> E
```

---

<a id="architecture"></a>

## Architecture

```mermaid
graph TD
    User["User"] --> CLI["cmd/bytemind"]
    CLI --> App["App Bootstrap"]
    App --> Runner["Runner"]

    Runner --> Engine["Agent Engine"]
    Engine --> Provider["Provider Runtime"]
    Provider --> Model["LLM Provider"]

    Engine --> Tools["Tool Registry"]
    Tools --> FileTools["File Tools"]
    Tools --> PatchTools["Patch Tools"]
    Tools --> Shell["Shell Tool"]
    Tools --> Web["Web Search / Fetch"]
    Tools --> TaskTools["Task Output / Stop"]
    Tools --> Delegate["Delegate SubAgent"]

    Runner --> Session["Session Store"]
    Runner --> Config["Config"]
    Runner --> Skills["Skills Manager"]
    Runner --> SubAgents["SubAgent Gateway"]
    Runner --> Safety["Approval / Sandbox / Allowlist"]
```

---

## Configuration

ByteMind normally merges three configuration layers: built-in defaults, user-level `~/.bytemind/config.json` (or `BYTEMIND_HOME/config.json`), and project-level `<workspace>/.bytemind/config.json`.

The example below is a **project-level config** and only affects the current workspace. Provider credentials reused across repositories usually belong in user-level config or environment variables. Passing `-config` uses that explicit config file.

```text
.bytemind/config.json
```

### OpenAI-compatible example

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key_env": "BYTEMIND_API_KEY"
  },
  "approval_policy": "on-request",
  "approval_mode": "interactive",
  "max_iterations": 32,
  "stream": true
}
```

### Anthropic example

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key_env": "ANTHROPIC_API_KEY",
    "anthropic_version": "2023-06-01"
  },
  "approval_policy": "on-request",
  "approval_mode": "interactive"
}
```

<details>
  <summary><b>Runtime boundary example</b></summary>

```json
{
  "approval_policy": "on-request",
  "approval_mode": "interactive",
  "writable_roots": [],
  "exec_allowlist": [],
  "network_allowlist": [],
  "system_sandbox_mode": "off"
}
```

</details>

---

<a id="skills-mcp-and-subagents"></a>

## Skills, MCP and SubAgents

### Skills

Reusable workflow definitions loaded from three scopes (builtin > user > project). Each skill has a slash entry, tool policy, and instruction file. Use the `/skills` and `/skill` commands to list, activate, and manage skills.

```text
/help                 Show available commands
/session              Show the current session
/sessions [limit]     List recent sessions
/agents [name]        List available subagents or show one definition
/explorer             Show the builtin explorer subagent definition
/review               Show the builtin review subagent definition
/resume <id>          Resume a recent session by id or prefix
/new                  Start a new session in the current workspace
/quit                 Exit the CLI
```

Built-in skills include bug investigation, GitHub PR review, repository onboarding, code review, RFC writing, and skill creation.

### MCP

Use MCP to connect ByteMind to external tools and context beyond local repository operations.

### SubAgents

SubAgents provide isolated execution contexts for focused work:

| SubAgent | Tools | Purpose |
|----------|-------|---------|
| `explorer` | `list_files`, `read_file`, `search_text` | Read-only repository exploration |
| `review` | `list_files`, `read_file`, `search_text` | Code review and bug detection |
| `general` | File tools + edit tools | Multi-step coding tasks |

<p align="center">
  <img src="https://img.shields.io/badge/Skills-Reusable%20workflows-0284c7?style=for-the-badge" alt="Skills" />
  <img src="https://img.shields.io/badge/MCP-External%20tooling-7c3aed?style=for-the-badge" alt="MCP" />
  <img src="https://img.shields.io/badge/SubAgents-Focused%20delegation-16a34a?style=for-the-badge" alt="SubAgents" />
</p>

---

## Safety Model

| Action | Typical behavior |
| --- | --- |
| Read files | Usually allowed automatically |
| Search files | Usually allowed automatically |
| Write files | Requires approval |
| Run shell commands | Requires approval or allowlist |
| High-risk actions | Shown before execution |

> ByteMind is designed around a simple principle:<br>
> **AI can execute, but humans should keep the final control boundary.**

### Safety diagnostics

```bash
# View current safety configuration
bytemind safety status

# Understand the safety model
bytemind safety explain

# Check environment, config, and dependencies
bytemind doctor
```

---

## Project Structure

```text
cmd/bytemind            CLI entrypoint (chat / run / doctor / safety / mcp)
internal/app            Application bootstrap and CLI dispatch
internal/agent          Agent loop, prompts, streaming, subagent execution
internal/config         Config loading, defaults, environment overrides
internal/llm            Common message and tool types
internal/provider       Provider adapters and provider runtime
internal/session        Session persistence
internal/tools          Tool registry and 14 built-in tools
internal/skills         Skills discovery and loading
internal/subagents      SubAgent manager and preflight gateway
internal/sandbox        Runtime boundary and sandbox-related logic
tui/                    Terminal UI (BubbleTea framework)
examples/bugfix-demo    5-minute reproducible bug-fix demo
evals/                  Evaluation tasks and runner
docs/                   Architecture docs, RFCs, PRDs
scripts/                Cross-platform install scripts
```

---

## Links

- Documentation: <https://1024xengineer.github.io/bytemind/zh/>
- GitHub: <https://github.com/1024XEngineer/bytemind>

---

## License

This project is licensed under the [MIT License](LICENSE).
