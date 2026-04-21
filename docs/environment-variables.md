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
| `BYTEMIND_WRITABLE_ROOTS` | empty | Additional writable roots. Use the OS path-list separator (`;` on Windows, `:` on Linux/macOS). |
| `BYTEMIND_SANDBOX_WORKER` | internal | Internal process marker used to avoid recursive worker spawning. Do not set manually. |

## Notes

- `BYTEMIND_MOUSE_Y_OFFSET` is clamped to `[-10, 10]`.
- Explicitly setting `BYTEMIND_MOUSE_Y_OFFSET` disables auto-offset detection.
- `BYTEMIND_SANDBOX_WORKER` is reserved for the worker subprocess bootstrap path.
- See [Sandbox Acceptance Checklist](./sandbox-acceptance.md) for validation matrix and test commands.
