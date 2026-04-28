# Sandbox Acceptance Checklist

This document defines the minimum acceptance checks for the current sandbox implementation.

## Scope

- Lease-based policy checks for core shell/file/network tools:
  - `run_shell`
  - `list_files`
  - `search_text`
  - `read_file`
  - `replace_in_file`
  - `apply_patch`
  - `write_file`
  - `web_fetch`
  - `web_search`
- Subprocess worker path for sandbox-enabled executions.
- Approval behavior alignment across `interactive` and `away` modes.
- Fail-closed behavior when subprocess worker is unavailable.
- Linux `required` mode execution hardening:
  - root filesystem remounted read-only inside sandboxed shell namespace
  - writable bind remount only for `workspace`, `writable_roots`, and `/tmp`
  - minimal runtime environment with sensitive key stripping

## Matrix

| Scenario | Expected Result |
| --- | --- |
| `sandbox_enabled=false` | Existing behavior remains unchanged. |
| `sandbox_enabled=true` + lease allows operation | Tool executes successfully. |
| `sandbox_enabled=true` + lease denies file/exec/network | Tool result is denied with a clear reason code. |
| `interactive` + escalation needed + approval available | Parent process prompts once, then executes. |
| `interactive` + escalation needed + no approval channel | Operation is denied with `approval_channel_unavailable`. |
| `away` + operation needs approval | Operation is denied immediately, no approval prompt. |
| Subprocess worker unavailable while sandbox enabled | Fail closed with internal sandbox worker error. |
| `system_sandbox_mode=required` + OS backend unavailable | Fail closed before worker execution. |
| `system_sandbox_mode=best_effort` + OS backend unavailable | Fallback to normal worker launch with explicit fallback reason in startup status/log. |
| `system_sandbox_mode=required` + runtime backend unavailable at agent startup | Run fails closed before first model/tool turn is executed. |
| `system_sandbox_mode=required` + network-targeted operation + backend lacks required network isolation for that tool path | Operation is denied fail-closed with permission_denied (no silent fallback). |
| Any run with `sandbox_enabled=true` and `system_sandbox_mode!=off` | Run output includes a startup status line with mode/backend/state/required_capable/capability_level/shell_network_isolation/worker_network_isolation. |
| Any run with sandbox context | Audit stream includes `system_sandbox_startup` event plus sandbox metadata on permission/start/result/task_state audit events (`sandbox_capability_level`, `sandbox_shell_network_isolation`, `sandbox_worker_network_isolation` included). |
| Any denied/error tool result while sandbox is enabled | Tool result payload includes `system_sandbox` context so CLI feedback can surface sandbox state/reason on failure paths. |
| Linux + `system_sandbox_mode=required` + shell command writes outside writable roots | Write fails from read-only filesystem enforcement. |
| macOS + `system_sandbox_mode=best_effort` + `sandbox-exec` available | Uses `sandbox-exec` profile-based launch with writable roots; worker profile allows network for web tools, with explicit fallback reason when probe fails. |
| macOS + `system_sandbox_mode=required` + `sandbox-exec` available | Uses `sandbox-exec` profile-based launch with writable roots and network denied in worker/shell profiles. |
| Windows + `system_sandbox_mode=best_effort` | Uses Job Object process isolation backend (no startup fallback). |
| Windows + `system_sandbox_mode=required` | Uses Job Object backend with startup active state; `run_shell` enforces strict single-segment read-only allowlist (commands outside plan-safe allowlist are denied). |
| Windows + `system_sandbox_mode=required` + network-targeted `run_shell`/`web_*` | Denied when backend cannot provide the required network isolation for the selected tool path. |
| Any OS + `system_sandbox_mode=required` + `web_fetch`/`web_search` + backend worker network isolation unavailable | Policy pre-check denies before execution (`reason_code=sandbox_guard`, no `tool_execute_start`). |
| Any OS + `system_sandbox_mode=required` + network-targeted `run_shell` + backend shell network isolation unavailable | Policy pre-check denies before execution (`reason_code=sandbox_guard`, no `tool_execute_start`). |

## Automated Checks

Use one of the scripts below from repository root:

- PowerShell: `./scripts/sandbox-e2e.ps1`
- Bash: `./scripts/sandbox-e2e.sh`

Both scripts run focused suites first, then run `go test ./...`.
They also isolate test cache/config directories under the repository (for example `.gocache`, `.appdata`, `.localappdata`, `.xdg-*`) to reduce host-environment permission noise.

CI runs the same focused acceptance suites in a cross-platform matrix:

- PR workflow: `CI / Sandbox Acceptance (ubuntu-latest|macos-latest|windows-latest)`
- Main workflow: `Main Checks / Main Sandbox Acceptance (ubuntu-latest|macos-latest|windows-latest)`

## Key Test Coverage

- `internal/tools/worker_process_test.go`
  - subprocess route eligibility
  - parent pre-approval behavior
  - away-mode no-prompt denial
  - fail-closed when invoker is missing
  - worker protocol version validation
  - worker env sanitization
- `internal/tools/worker_test.go`
  - lease enforcement for file/exec/network
  - web tool network allowlist enforcement
  - apply_patch multi-path boundary detection
  - escalation with approval handler/stdin fallback
  - denied dependency and pre-approved escalation path
  - runtime request path resolution for workspace-relative file paths
- `internal/tools/run_shell_test.go`
  - shell approval policy behavior
  - away-mode denial behavior
  - skip-shell-approval branch for parent-approved subprocess path
- `internal/agent/runner_test.go`
  - required system sandbox fail-closed at startup before first model call
  - startup fallback visibility in run output and task report summary
- `internal/agent/tool_execution_audit_test.go`
  - startup audit event (`system_sandbox_startup`) with mode/backend/fallback metadata
  - sandbox metadata propagation across `tool_execute_start`, `tool_execute_result`, `task_state_changed`
- `internal/agent/runner_policy_test.go`
  - sandbox metadata on `permission_decision` and denied/ask execution audit paths
  - policy-stage `sandbox_guard` denial coverage for required-mode network-targeted `run_shell`/`web_fetch`

## Manual Smoke Checks

1. `approval_mode=interactive`, `sandbox_enabled=true`, allowlist does not include a shell command:
   - Expect one approval prompt.
   - Approve -> command runs.
2. Same as above with `approval_mode=away`:
   - Expect no prompt.
   - Command denied and run continues or stops based on `away_policy`.
3. `write_file` with relative path under workspace:
   - Expect success when lease allows workspace root.
4. `write_file` to path outside workspace/writable roots:
   - Expect immediate denial.
5. `sandbox_enabled=true` + `system_sandbox_mode=best_effort` + backend unavailable:
   - Expect startup line `system sandbox startup ... state=fallback`.
   - Expect task report summary includes `System sandbox fallback: startup (...)`.
6. `sandbox_enabled=true` + `system_sandbox_mode=required` + backend unavailable:
   - Expect run fails before first tool/model turn.
   - Expect no `tool_execute_start` event in audit for that run.
7. `sandbox_enabled=true` + `system_sandbox_mode=required` + network-targeted command/tool on backend without required network isolation:
   - Expect immediate permission denial (fail-closed).
   - Expect no silent downgrade to non-isolated execution.
  - Expect tool feedback to include sandbox summary/reason (mode/backend/required_capable/capability_level/shell_network_isolation/worker_network_isolation + fallback reason when present).
8. `sandbox_enabled=true` + `system_sandbox_mode=required` + backend worker network isolation unavailable + `web_fetch`/`web_search`:
   - Expect policy-stage denial with `reason_code=sandbox_guard`.
   - Expect no `tool_execute_start` audit event for that denied tool call.
9. `sandbox_enabled=true` + `system_sandbox_mode=required` + backend shell network isolation unavailable + network-targeted `run_shell`:
   - Expect policy-stage denial with `reason_code=sandbox_guard`.
   - Expect no `tool_execute_start` audit event for that denied tool call.
