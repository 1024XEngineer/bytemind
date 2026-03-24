package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	Command  string
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
	Duration time.Duration
}

type Runner struct {
	Timeout time.Duration
}

func (r Runner) Run(ctx context.Context, workingDir, commandLine string) (Result, error) {
	args, err := ParseCommand(commandLine)
	if err != nil {
		return Result{}, err
	}
	if len(args) == 0 {
		return Result{}, errors.New("command is empty")
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, args[0], args[1:]...)
	cmd.Dir = workingDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startedAt := time.Now()
	runErr := cmd.Run()
	duration := time.Since(startedAt)

	result := Result{
		Command:  commandLine,
		ExitCode: exitCode(runErr, cmd),
		Stdout:   truncateOutput(stdout.String(), 4000),
		Stderr:   truncateOutput(stderr.String(), 4000),
		TimedOut: errors.Is(runCtx.Err(), context.DeadlineExceeded),
		Duration: duration,
	}

	if result.TimedOut {
		return result, fmt.Errorf("command timed out after %s", timeout)
	}
	if runErr != nil {
		return result, fmt.Errorf("command failed: %w", runErr)
	}

	return result, nil
}

func ParseCommand(commandLine string) ([]string, error) {
	var args []string
	var current strings.Builder
	var quote rune
	escaped := false

	for _, char := range commandLine {
		switch {
		case escaped:
			current.WriteRune(char)
			escaped = false
		case char == '\\':
			escaped = true
		case quote != 0:
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
		case char == '"' || char == '\'':
			quote = char
		case char == ' ' || char == '\t' || char == '\n':
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	if escaped {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote in command")
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args, nil
}

func truncateOutput(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max < 4 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func exitCode(runErr error, cmd *exec.Cmd) int {
	if runErr == nil && cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode()
	}

	return -1
}
