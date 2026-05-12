# ByteMind 5-Minute Bugfix Demo

This demo shows ByteMind completing a real bug-fix cycle:
1. Read project structure and source code
2. Run tests to discover the failure
3. Analyze the root cause
4. Fix the bug
5. Re-run tests to verify
6. Show the diff

## Quick Start

```bash
# From the byte mind project root:
go run ./cmd/bytemind run -prompt "Fix the failing test in examples/bugfix-demo/broken-project and verify it" -workspace examples/bugfix-demo/broken-project

# Or with full tool access:
go run ./cmd/bytemind run -prompt "Fix the failing test in examples/bugfix-demo/broken-project and verify it" -workspace examples/bugfix-demo/broken-project -approval-mode full_access
```

## Expected Behaviours

- Agent reads the code, identifies the divide-by-zero bug
- Adds guard clause for empty slice
- Runs `go test ./...` to verify
- All tests pass
- Agent shows summary of changes

## Why This Demo

- **Reproducible**: always the same bug, same fix, same verification
- **Self-verifying**: "fix the failing test and verify it" has a binary success criterion
- **5-minute cap**: a simple guard-clause fix fits within time limit
- **Full engineering loop**: read → understand → fix → test → diff
