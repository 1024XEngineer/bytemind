# Environment Variables

ByteMind TUI supports the following runtime environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `BYTEMIND_ENABLE_MOUSE` | `true` | Enables Bubble Tea mouse capture (`WithMouseAllMotion`). Set to `0` / `false` / `off` to disable. |
| `BYTEMIND_WINDOWS_INPUT_TTY` | `false` | Windows-only opt-in for `WithInputTTY`. Can improve mouse reporting in some terminals, but may affect IME behavior. |
| `BYTEMIND_MOUSE_Y_OFFSET` | auto on some Windows terminals, otherwise `0` | Manual mouse Y-axis compensation. If unset, ByteMind may auto-set it to `2` in Windows Terminal / VSCode terminal when input TTY is disabled. |
| `BYTEMIND_APPROVAL_MODE` | `interactive` | Runtime approval mode override. Supported values: `interactive`, `away`. |
| `BYTEMIND_AWAY_POLICY` | `auto_deny_continue` | Away-mode behavior for denied approval requests. Supported values: `auto_deny_continue`, `fail_fast`. |
| `BYTEMIND_SANDBOX_ENABLED` | `false` | Enables lease-based sandbox policy checks and worker-path enforcement for shell/file tools. |
| `BYTEMIND_SYSTEM_SANDBOX_MODE` | `off` | System sandbox execution mode for shell tools. Supported values: `off`, `best_effort`, `required`. `required` fail-closes when backend is unavailable; `best_effort` falls back without system sandbox and logs a startup warning. |
| `BYTEMIND_WRITABLE_ROOTS` | empty | Additional writable roots. Use the OS path-list separator (`;` on Windows, `:` on Linux/macOS). |
| `BYTEMIND_SANDBOX_WORKER` | internal | Internal process marker used to avoid recursive worker spawning. Do not set manually. |

## Notes

- `BYTEMIND_MOUSE_Y_OFFSET` is clamped to `[-10, 10]`.
- Explicitly setting `BYTEMIND_MOUSE_Y_OFFSET` disables auto-offset detection.
- `BYTEMIND_SANDBOX_WORKER` is reserved for the worker subprocess bootstrap path.
- Current backend support:
  - Linux: `unshare`
  - macOS: `sandbox-exec` (when available in `PATH`)
  - Windows: Job Object process isolation; `required` mode is enabled with strict single-segment read-only allowlist guard (`run_shell` rejects commands outside plan-safe allowlist)
  - In `required` mode, network-targeted operations are fail-closed when the active backend does not provide the required network isolation for the relevant tool path (`run_shell` vs `web_*`).
  - In `required` mode, when backend worker network isolation is unavailable, `web_fetch` / `web_search` are denied at policy stage (`reason_code=sandbox_guard`) before tool execution.
  - In `required` mode, when backend shell network isolation is unavailable, network-targeted `run_shell` is denied at policy stage (`reason_code=sandbox_guard`) before tool execution.
- Runtime visibility:
  - when sandbox mode is enabled (`best_effort` or `required`), run output prints a startup status line (`mode/backend/state/required_capable/capability_level/shell_network_isolation/worker_network_isolation`).
  - denied/error tool results now carry a `system_sandbox` envelope (when sandbox is enabled), so CLI feedback can show sandbox state/reason on failure paths, not only successful `run_shell` outputs.
  - audit events include startup and per-tool sandbox metadata (`system_sandbox_startup`, `permission_decision`, `tool_execute_start`, `tool_execute_result`, `task_state_changed`) including `sandbox_capability_level`, `sandbox_shell_network_isolation`, and `sandbox_worker_network_isolation`.
- See [Sandbox Acceptance Checklist](./sandbox-acceptance.md) for validation matrix and test commands.
