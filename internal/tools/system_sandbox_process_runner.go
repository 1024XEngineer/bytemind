package tools

import (
	"errors"
	"os/exec"
	"strings"
)

var runCommandWithWindowsJobObjectFn = runCommandWithWindowsJobObject

func runCommandWithSystemSandbox(cmd *exec.Cmd, backendName, mode string) error {
	if cmd == nil {
		return errors.New("command is required")
	}
	if strings.EqualFold(strings.TrimSpace(backendName), "windows_job_object") {
		normalized := normalizeSystemSandboxMode(&ExecutionContext{SystemSandboxMode: mode})
		if normalized != systemSandboxModeOff {
			return runCommandWithWindowsJobObjectFn(cmd)
		}
	}
	return cmd.Run()
}
