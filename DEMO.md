# ByteMind 5-Minute Demo

## Goal

Demonstrate ByteMind's ability to autonomously debug and fix a real code defect using its built-in tool chain, in under 5 minutes.

The agent reads a broken Go project, runs the failing tests, diagnoses the divide-by-zero bug in `CalculateAverage`, applies the fix, and re-verifies all tests pass — then presents a structured engineering summary.

## Command

Requires a configured LLM provider:

```bash
go run ./cmd/bytemind run \
  -prompt "Fix the failing test and verify it passes" \
  -workspace examples/bugfix-demo/broken-project \
  -approval-mode full_access
```

**Offline verification** (no API key):

```bash
go run ./evals/runner.go -smoke -run bugfix_go_001
```

## Initial Failure Point

```bash
cd examples/bugfix-demo/broken-project
go test ./...
# --- FAIL: TestCalculateAverageEmpty (0.00s)
#     calculator_test.go:15: expected 0 for empty slice, got NaN
```

**Root cause**: `CalculateAverage` computes `total / float64(len(nums))`. When `nums` is empty, `len(nums) == 0`, producing `0.0 / 0.0 = NaN`.

## Expected Agent Steps

| Step | Tool | Arguments | Expected Observation |
|------|------|-----------|---------------------|
| 1 | `list_files` | `{}` | Project structure: `calculator.go`, `calculator_test.go`, `go.mod` |
| 2 | `read_file` | `{"path":"calculator.go"}` | Source with `CalculateAverage` and `FindMax` functions |
| 3 | `read_file` | `{"path":"calculator_test.go"}` | Tests including `TestCalculateAverageEmpty` |
| 4 | `run_tests` | `{}` | Failing test: `TestCalculateAverageEmpty` returns NaN |
| 5 | `replace_in_file` | `{"path":"calculator.go","oldString":"return total / float64(len(nums))","newString":"if len(nums) == 0 {\n\t\treturn 0\n\t}\n\treturn total / float64(len(nums))"}` | File updated |
| 6 | `run_tests` | `{}` | All tests pass (`ok`) |
| 7 | `git_diff` | `{}` | Unified diff showing the guard clause |

## Expected Agent Output Summary

```
**Summary**
- Fixed `CalculateAverage` divide-by-zero bug in empty-slice case.

**Changed Files**
- calculator.go

**Verification**
- go test ./...: passed

**Risks**
- No known remaining risk.

**Next Steps**
- None.
```

## What This Proves

- **Multi-step tool orchestration**: agent chains 7+ tool calls autonomously
- **Real code modification**: file is actually edited on disk
- **Test-driven verification**: agent runs tests before and after the fix
- **Git-aware output**: agent shows the exact diff produced
- **Reproducible**: same project, same prompt, same expected output

## Related Files

- `examples/bugfix-demo/broken-project/calculator.go` — source with bug
- `examples/bugfix-demo/broken-project/calculator_test.go` — test with empty-slice case
- `examples/bugfix-demo/expected-output.md` — expected agent trace
- `evals/tasks/bugfix_go_001.yaml` — eval task definition
