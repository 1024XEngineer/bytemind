# Environment Variables

ByteMind TUI supports the following runtime environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `BYTEMIND_ENABLE_MOUSE` | `true` | Enables Bubble Tea mouse capture (`WithMouseAllMotion`). Set to `0` / `false` / `off` to disable. |
| `BYTEMIND_WINDOWS_INPUT_TTY` | `false` | Windows-only opt-in for `WithInputTTY`. Can improve mouse reporting in some terminals, but may affect IME behavior. |
| `BYTEMIND_MOUSE_Y_OFFSET` | auto on some Windows terminals, otherwise `0` | Manual mouse Y-axis compensation. If unset, ByteMind may auto-set it to `2` in Windows Terminal / VSCode terminal when input TTY is disabled. |

## Notes

- `BYTEMIND_MOUSE_Y_OFFSET` is clamped to `[-10, 10]`.
- Explicitly setting `BYTEMIND_MOUSE_Y_OFFSET` disables auto-offset detection.
