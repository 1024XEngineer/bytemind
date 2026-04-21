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

## Automated Checks

Use one of the scripts below from repository root:

- PowerShell: `./scripts/sandbox-e2e.ps1`
- Bash: `./scripts/sandbox-e2e.sh`

Both scripts run focused suites first, then run `go test ./...`.

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
