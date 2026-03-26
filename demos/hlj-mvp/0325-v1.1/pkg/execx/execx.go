package execx

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type RiskLevel string

const (
	RiskApprove RiskLevel = "approve"
	RiskBlock   RiskLevel = "block"
)

type Executor struct {
	timeout time.Duration
	dir     string
}

type ExecResult struct {
	Command  string
	ExitCode int
	Output   string
	Duration time.Duration
	TimedOut bool
}

func New(timeout int, dir string) *Executor {
	return &Executor{
		timeout: time.Duration(timeout) * time.Second,
		dir:     dir,
	}
}

func (e *Executor) Risk(cmd string) RiskLevel {
	lower := strings.ToLower(cmd)
	blockedPatterns := []string{
		"rm -rf",
		"del /s /q",
		"remove-item -recurse -force",
		"format ",
		"format.com",
		"rmdir /s",
		"shutdown",
		"reboot",
		"poweroff",
		"mkfs",
		"diskpart",
		"git reset --hard",
		"git checkout --",
	}

	for _, pattern := range blockedPatterns {
		if strings.Contains(lower, pattern) {
			return RiskBlock
		}
	}

	return RiskApprove
}

func (e *Executor) IsDangerous(cmd string) bool {
	return e.Risk(cmd) == RiskBlock
}

func (e *Executor) Run(cmdStr string) (*ExecResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := shellCommand(ctx, cmdStr)
	cmd.Dir = e.dir

	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		return &ExecResult{
			Command:  cmdStr,
			ExitCode: -1,
			Output:   string(output),
			Duration: duration,
			TimedOut: true,
		}, nil
	}

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, err
		}
	}

	return &ExecResult{
		Command:  cmdStr,
		ExitCode: exitCode,
		Output:   string(output),
		Duration: duration,
		TimedOut: false,
	}, nil
}

func (e *Executor) RunWithApproval(cmdStr string, approved bool) (*ExecResult, error) {
	if !approved {
		return nil, fmt.Errorf("command not approved")
	}

	if e.IsDangerous(cmdStr) {
		return nil, fmt.Errorf("dangerous command blocked: %s", cmdStr)
	}

	return e.Run(cmdStr)
}

func (e *Executor) SetTimeout(seconds int) {
	e.timeout = time.Duration(seconds) * time.Second
}

func (e *Executor) Timeout() time.Duration {
	return e.timeout
}

func shellCommand(ctx context.Context, cmdStr string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", cmdStr)
	}

	return exec.CommandContext(ctx, "sh", "-c", cmdStr)
}
