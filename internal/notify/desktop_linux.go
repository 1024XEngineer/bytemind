package notify

import (
	"context"
	"os/exec"
	"strings"
)

type linuxSender struct {
	executable string
}

func newPlatformSender() (platformSender, bool) {
	executable, err := exec.LookPath("notify-send")
	if err != nil || strings.TrimSpace(executable) == "" {
		return nil, false
	}
	return linuxSender{executable: executable}, true
}

func (s linuxSender) Send(ctx context.Context, title, body string) error {
	cmd := exec.CommandContext(ctx, s.executable, title, body)
	return cmd.Run()
}
