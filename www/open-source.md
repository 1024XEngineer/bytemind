# Open Source

ByteMind is open source software released under the **MIT License**.

## Repository

- **GitHub**: https://github.com/1024XEngineer/bytemind
- **Issues**: https://github.com/1024XEngineer/bytemind/issues
- **Releases**: https://github.com/1024XEngineer/bytemind/releases

## License

ByteMind is MIT licensed. You can use it freely in personal, commercial, and internal projects. See [`LICENSE`](https://github.com/1024XEngineer/bytemind/blob/main/LICENSE) for the full text.

## Contributing

Contributions are welcome. Before opening a pull request, please follow these guidelines:

### Before You Start

- **For bugs**: open an issue first describing the behavior, expected behavior, and steps to reproduce. This avoids duplicate work.
- **For features**: open a discussion or issue to align on scope before investing in implementation.
- **For documentation**: feel free to open a PR directly for typos, broken links, or factual corrections.

### Development Setup

Requires Go 1.24+.

```bash
git clone https://github.com/1024XEngineer/bytemind.git
cd bytemind
go build ./...
go test ./...
```

### Pull Request Guidelines

1. **Keep scope focused** — one logical change per PR
2. **Add or update tests** when behavior changes, especially in `internal/agent/` or runner flow
3. **Run existing tests** before submitting: `go test ./...`
4. **Include a clear description** of what changed and why, with reproduction or validation steps
5. **Reference the issue** your PR resolves (e.g. `Fixes #123`)

### Architecture Overview

The codebase is organized around a few key packages:

| Package            | Purpose                                      |
| ------------------ | -------------------------------------------- |
| `cmd/bytemind`     | CLI entry point                              |
| `internal/agent`   | Agent runner, prompt assembly, tool dispatch |
| `internal/app`     | TUI app, slash command handling              |
| `internal/config`  | Config loading and defaults                  |
| `internal/skills`  | Skill catalog and loader                     |
| `internal/session` | Session persistence                          |
| `internal/tools`   | Tool implementations                         |

For changes affecting prompt assembly or the agent runner flow, review `AGENTS.md` in the repo root for architecture expectations.

## Feedback and Support

- **Bug reports**: [GitHub Issues](https://github.com/1024XEngineer/bytemind/issues)
- **Feature requests**: [GitHub Discussions](https://github.com/1024XEngineer/bytemind/discussions)
- **Security issues**: please use private disclosure via the GitHub Security tab rather than opening a public issue
