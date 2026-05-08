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
</p>

<p align="center">
  <a href="https://1024xengineer.github.io/bytemind/zh/"><b>Documentation</b></a>
  ·
  <a href="#quick-start"><b>Quick Start</b></a>
  ·
  <a href="#why-bytemind"><b>Why ByteMind</b></a>
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

## Terminal Preview

```text
┌─ ByteMind ───────────────────────────────────────────────────────────────┐
│ Mode: Build | Provider: gpt-5.x | Session: active                       │
├──────────────────────────────────────────────────────────────────────────┤
│ Ask anything, or type / for commands...                                 │
│                                                                          │
│ > analyze the provider layer and suggest improvements                    │
│                                                                          │
│ Thinking…                                                                │
│ • reading files                                                          │
│ • searching symbol usage                                                 │
│ • drafting a plan                                                        │
│                                                                          │
│ Approval required                                                        │
│ Tool: write_file                                                         │
│ Command: update internal/provider/registry.go                            │
│                                                                          │
│ [Approve once] [Approve session] [Reject]                                │
└──────────────────────────────────────────────────────────────────────────┘
```

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

```text
list_files
read_file
search_text
write_file
replace_in_file
apply_patch
run_shell
web_search
web_fetch
```

<details>
  <summary><b>What these tools enable</b></summary>

- inspect a repository structure
- locate relevant files and symbols
- update files incrementally
- apply patches instead of rewriting blindly
- run commands and verify results
- search external sources when local context is insufficient

</details>

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
flowchart TD
    A[User Prompt] --> B[Build Runtime Context]
    B --> C[LLM decides: answer or tool call]
    C --> D{Tool Call?}
    D -- No --> E[Final Answer]
    D -- Yes --> F[Approval / Policy Check]
    F --> G[Execute Tool]
    G --> H[Observation appended to session]
    H --> I{Done?}
    I -- No --> C
    I -- Yes --> E
```

---

<a id="architecture"></a>

## Architecture

```mermaid
flowchart TD
    User[User] --> CLI[cmd/bytemind]
    CLI --> App[App Bootstrap]
    App --> Runner[Runner]

    Runner --> Engine[Agent Engine]
    Engine --> Provider[Provider Runtime]
    Provider --> Model[LLM Provider]

    Engine --> Tools[Tool Registry]
    Tools --> FileTools[File Tools]
    Tools --> PatchTools[Patch Tools]
    Tools --> Shell[Shell Tool]
    Tools --> Web[Web Search / Fetch]
    Tools --> TaskTools[Task Output / Stop]
    Tools --> Delegate[Delegate SubAgent]

    Runner --> Session[Session Store]
    Runner --> Config[Config]
    Runner --> Skills[Skills Manager]
    Runner --> SubAgents[SubAgent Gateway]
    Runner --> Safety[Approval / Sandbox / Allowlist]
```

---

## Configuration

Recommended project config location:

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

Reusable workflows activated through slash commands.

```text
/bug-investigation    Structured bug investigation
/review               Correctness, regression risk, and test coverage review
/repo-onboarding      Understand an unfamiliar repository
/write-rfc            Generate a structured technical proposal
/skill-creator        Create, refine, and evaluate skills
```

### MCP

Use MCP to connect ByteMind to external tools and context beyond local repository operations.

### SubAgents

SubAgents provide isolated execution contexts for focused work:

- broad repository discovery
- file targeting
- read-only exploration
- bug scope reduction
- review / analysis subtasks

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

---

## Project Structure

```text
cmd/bytemind            CLI entrypoint
internal/app            Application bootstrap
internal/agent          Agent loop, prompts, streaming, subagent execution
internal/config         Config loading, defaults, environment overrides
internal/llm            Common message and tool types
internal/provider       Provider adapters and provider runtime
internal/session        Session persistence
internal/tools          File / patch / shell / web tools
internal/skills         Skills discovery and loading
internal/subagents      SubAgent manager and preflight gateway
internal/sandbox        Runtime boundary and sandbox-related logic
```

---

## Use Cases

- understand a new codebase
- debug failing tests
- review or refine changes
- generate technical plans and RFCs
- automate repeated engineering tasks
- work with explicit human approval on sensitive actions

---

## Roadmap

- [ ] Expand built-in skills
- [ ] Improve MCP integration and examples
- [ ] Improve SubAgent workflows
- [ ] Strengthen TUI interaction
- [ ] Add richer audit and sandbox controls
- [ ] Support team-shared workflow assets

---

## Links

- Documentation: <https://1024xengineer.github.io/bytemind/zh/>
- GitHub: <https://github.com/1024XEngineer/bytemind>

---

## License

This project is licensed under the [MIT License](LICENSE).
