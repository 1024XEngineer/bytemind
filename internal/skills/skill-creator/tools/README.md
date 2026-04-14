# Skill Creator Go Tools

This directory is the canonical Go implementation for skill-creator tooling.

Run from repository root:

- `go run ./internal/skills/skill-creator/tools quick-validate <skill-dir>`
- `go run ./internal/skills/skill-creator/tools package-skill <skill-dir> [output-dir]`
- `go run ./internal/skills/skill-creator/tools aggregate-benchmark <workspace/iteration-N> --skill-name <name>`
- `go run ./internal/skills/skill-creator/tools generate-report <run-loop-output.json|-> [--output report.html]`
- `go run ./internal/skills/skill-creator/tools run-eval --eval-set <eval.json> --skill-path <skill-dir> --model <model>`
- `go run ./internal/skills/skill-creator/tools improve-description --eval-results <eval-results.json> --skill-path <skill-dir> --model <model>`
- `go run ./internal/skills/skill-creator/tools run-loop --eval-set <eval.json> --skill-path <skill-dir> --model <model>`
- `go run ./internal/skills/skill-creator/tools generate-review <workspace/iteration-N> [--benchmark benchmark.json]`

These commands require Go and the `claude` CLI in PATH.
