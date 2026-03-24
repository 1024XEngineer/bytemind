# ForgeCLI Agent MVP

A small Go coding-agent MVP that imitates the core loop of tools-first products such as OpenCode and Claude Code.

- configure an OpenAI-compatible model endpoint
- keep multi-turn conversation context in memory
- expose local coding tools to the model
- ask for approval before writing files or running commands
- support a simple HTML generation command that writes a runnable page to disk

## What this MVP includes

- `forgecli chat`: interactive REPL session with context memory
- chat modes:
  - `analyze`: read-only analysis with `list_files`, `read_file`, `search`
  - `full`: adds `write_file` and a whitelisted `run_command`
- `forgecli generate`: generate one self-contained HTML page and save it locally
  - works with a configured model when available
  - falls back to a built-in local template for requests like a pomodoro page
- `forgecli run`: single-task mode for one closed-loop code change
- OpenAI-compatible `/chat/completions` client
- workspace boundary checks and a basic dangerous-command denylist

## Quick start

Generate a pomodoro web page directly to a local file:

```powershell
go run ./cmd/forgecli generate --prompt '我想做一个番茄钟网页' --output pomodoro.html
```

Start in read-only analyze mode:

```powershell
go run ./cmd/forgecli chat --repo . --config forgecli.json --mode analyze
```

If you need file edits or verification commands, switch to full mode:

```powershell
go run ./cmd/forgecli chat --repo . --config forgecli.json --mode full
```

## Chat commands

- `/help`
- `/tools`
- `/reset`
- `/exit`

## Notes

- conversation history is kept in memory for the current session only
- `write_file` expects full file content, not a patch
- `generate` writes a complete `.html` file you can open directly in a browser
- `.gocache` and `.gotmp` are ignored during repo scanning by default
