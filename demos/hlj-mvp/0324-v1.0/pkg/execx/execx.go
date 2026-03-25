package execx

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Executor struct {
	timeout time.Duration
	dir     string
}

func New(timeout int, dir string) *Executor {
	return &Executor{
		timeout: time.Duration(timeout) * time.Second,
		dir:     dir,
	}
}

type ExecResult struct {
	Command  string
	ExitCode int
	Output   string
	Duration time.Duration
	TimedOut bool
}

var dangerousPatterns = []string{
	"rm -rf",
	"del /s /q",
	"format",
	"rmdir",
	"shutdown",
	"curl -X DELETE",
	"wget --delete",
}

func (e *Executor) IsDangerous(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func (e *Executor) Run(cmdStr string) (*ExecResult, error) {
	start := time.Now()

	cmd := exec.Command("cmd", "/c", cmdStr)
	cmd.Dir = e.dir

	done := make(chan error, 1)
	var output []byte
	var err error

	go func() {
		output, err = cmd.CombinedOutput()
		done <- err
	}()

	select {
	case <-done:
		break
	case <-time.After(e.timeout):
		cmd.Process.Kill()
		return &ExecResult{
			Command:  cmdStr,
			TimedOut: true,
			Duration: time.Since(start),
		}, nil
	}

	duration := time.Since(start)
	exitCode := 0
	if err != nil {
		exitCode = 1
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
