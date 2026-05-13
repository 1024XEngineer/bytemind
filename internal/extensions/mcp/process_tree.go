package mcp

import (
	"os/exec"
	"time"
)

const commandWaitDelay = 500 * time.Millisecond

func configureCommandCancellation(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.Cancel = func() error {
		return terminateCommand(cmd)
	}
	cmd.WaitDelay = commandWaitDelay
}
