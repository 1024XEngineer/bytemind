//go:build !windows

package tools

import "os/exec"

func runCommandWithWindowsJobObject(cmd *exec.Cmd) error {
	return cmd.Run()
}
