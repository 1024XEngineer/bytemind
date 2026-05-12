# ByteMind Evaluation System

This directory contains reproducible evaluation tasks for ByteMind.

## Structure

```
evals/
  tasks/        YAML task definitions
  runner.go     Evaluation runner
  README.md     This file
```

## Usage

```bash
# List available tasks
go run ./evals/runner.go -list

# Run a single task
go run ./evals/runner.go -run bugfix_go_001

# Run all tasks
go run ./evals/runner.go -run all
```

## Adding a Task

Create a YAML file in `evals/tasks/`:

```yaml
id: my_task_001
name: Descriptive task name
description: What the task tests
workspace: path/to/project
prompt: "Instructions for the agent"

success:
  - command: "go test ./..."
    exit_code: 0
  - file_contains:
      path: some_file.go
      pattern: "expected code pattern"
  - output_contains:
      - "expected output text"
```

## Success Checks

| Check | Description |
|---|---|
| `command` + `exit_code` | Run a command and verify exit code |
| `output_contains` | Agent output must contain all strings |
| `file_contains` | File must match a regex pattern |
| `files_modified` | Listed files must exist and be non-empty |
