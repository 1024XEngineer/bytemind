//go:build windows

package mcp

import (
	"os"
	"os/exec"
	"strconv"
)

func terminateCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	if cmd.Process.Pid <= 0 {
		return cmd.Process.Kill()
	}
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
