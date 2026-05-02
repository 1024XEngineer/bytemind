package notify

import (
	"context"
	"os/exec"
	"strings"
)

type darwinSender struct{}

func newPlatformSender() (platformSender, bool) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return nil, false
	}
	return darwinSender{}, true
}

func (darwinSender) Send(ctx context.Context, title, body string) error {
	command := "display notification \"" + escapeAppleScriptString(body) + "\" with title \"" + escapeAppleScriptString(title) + "\""
	cmd := exec.CommandContext(ctx, "osascript", "-e", command)
	return cmd.Run()
}

func escapeAppleScriptString(input string) string {
	input = strings.ReplaceAll(input, "\\", "\\\\")
	return strings.ReplaceAll(input, "\"", "\\\"")
}
