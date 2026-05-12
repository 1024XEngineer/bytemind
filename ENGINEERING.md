# ByteMind Engineering Evidence

## 1. Real Agent Loop

ByteMind's agent loop (`internal/agent/engine_run_loop.go`) is a stateful turn engine:

- Multi-step: tool call → observation → next LLM call, configurable up to `max_iterations`
- Context compaction: reactive compaction when context exceeds model window (`internal/agent/prompts/prompt_too_long.go`)
- Rate-limit resilience: exponential backoff with jitter, up to 8 retries
- SubAgent delegation: isolated execution contexts for focused work
- Turn intent contract: `continue_work` / `ask_user` / `finalize` tags in every response

Key code: `internal/agent/engine_run_loop.go:37-74` — main loop with max_iterations gate.

## 2. Coding-native Tools

ByteMind provides 14 built-in tools (`internal/tools/`), each with:

- A JSON-based argument schema
- A safety class (`safe` / `moderate` / `sensitive` / `destructive`)
- Unit tests with coverage for edge cases
- A TUI renderer for structured display in terminal UI

### Core tools

| Tool | File | Purpose |
|------|------|---------|
| `git_status` | `internal/tools/git_status.go` | Shows working tree: branch, staged, unstaged, untracked |
| `git_diff` | `internal/tools/git_diff.go` | Unified diff with added/removed line counts, file list |
| `run_tests` | `internal/tools/run_tests.go` | Auto-detects test command (go / npm / cargo / pytest / make), captures stdout/stderr, parses pass/fail/skip counts |

**Why these tools matter**: Unlike generic "run command" approaches, ByteMind's tools parse and structure their output as JSON, making observations machine-readable for subsequent LLM turns. `run_tests` for example returns `{ok, passed, failed, skipped, exit_code, summary}` — the agent never needs to regex-parse test output.

## 3. Reproducible Demo

`examples/bugfix-demo/` contains a self-contained failing Go project:

- `broken-project/calculator.go` — `CalculateAverage` returns NaN on empty slice
- `broken-project/calculator_test.go` — `TestCalculateAverageEmpty` explicitly tests the edge case
- `broken-project/go.mod` — self-contained module

**Verification**:
```bash
# Initial state: FAIL
cd examples/bugfix-demo/broken-project && go test ./...
# --- FAIL: TestCalculateAverageEmpty

# After fix: PASS
# (agent adds: if len(nums) == 0 { return 0 })
cd examples/bugfix-demo/broken-project && go test ./...
# ok
```

## 4. Evaluation System

`evals/` provides a YAML-driven evaluation framework (`evals/runner.go`):

- **Task definition**: YAML files with workspace path, prompt, and success criteria
- **Success checks**: command exit codes, output contains, file content regex, file modification detection
- **CI integration**: `-smoke` flag for static checks without LLM dependency
- **Extensible**: add new tasks by creating a YAML file in `evals/tasks/`

Example task (`evals/tasks/bugfix_go_001.yaml`):
```yaml
id: bugfix_go_001
name: Fix failing Go test
workspace: examples/bugfix-demo/broken-project
prompt: "Fix the failing test and verify it passes"
success:
  - command: "go test ./..."
    exit_code: 0
  - file_contains:
      - path: calculator.go
        pattern: "len\\(nums\\) == 0"
  - files_modified:
      - calculator.go
```

## 5. Safety Boundary

ByteMind implements a layered safety architecture:

| Layer | Config | Code |
|-------|--------|------|
| Tool Safety Classes | per-tool classification | `internal/tools/spec.go` |
| Approval Policy | `on-request` / `always` / `never` | `internal/config/config.go` |
| Approval Mode | `interactive` / `full_access` | `internal/config/config.go` |
| Sandbox | `off` / `best_effort` / `required` | `internal/sandbox/` |
| Writable Roots | directory allowlist | `internal/config/config.go` |
| Exec Allowlist | command allow patterns | `internal/config/config.go` |
| Network Allowlist | host allow patterns | `internal/config/config.go` |

Diagnostics:
- `bytemind safety status` — shows current safety config
- `bytemind safety explain` — explains safety model
- `bytemind doctor` — environment and config health check

## 6. CI and Testing

PR-gated CI (`.github/workflows/ci.yml`):

- `go build ./...` — compilation gate
- Unit tests with coverage — Codecov upload
- Eval smoke checks — `-list` and `-smoke` validation
- Sandbox acceptance — Linux/macOS/Windows matrix

Additional workflows:
- `ci-main.yml` — full check on main: vet, race detector, full sandbox
- `ci-pr.yml` — quick PR check with vet and coverage
- `release.yml` — cross-platform binary release on version tags
- `provider-contracts.yml` — provider interface contract tests
- `deploy-docs.yml` — VitePress docs deployment to GitHub Pages

## 7. Extensibility: Skills / MCP / SubAgents

### Skills
Reusable workflow definitions loaded from three scopes:
- Builtin (embedded): bug investigation, code review, RFC writing
- User: `~/.bytemind/skills/`
- Project: `<workspace>/.bytemind/skills/`

Each skill has a slash command, tool policy, and instruction file.

### MCP
Model Context Protocol integration for external tools. Configured via `mcp.example.json`.

### SubAgents
Isolated execution contexts (`internal/agent/subagent_*.go`):
- `explorer`: read-only repository exploration
- `review`: code review and bug detection
- `general`: full tool access for multi-step tasks

SubAgents run with their own tool policies and approval boundaries, reporting back a structured summary to the main agent context.
