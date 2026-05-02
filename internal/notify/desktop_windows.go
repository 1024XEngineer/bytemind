package notify

import (
	"context"
	"os/exec"
	"strings"
)

type windowsSender struct {
	executable string
}

func newPlatformSender() (platformSender, bool) {
	executable := resolveWindowsPowerShellExecutable()
	if strings.TrimSpace(executable) == "" {
		return nil, false
	}
	return windowsSender{executable: executable}, true
}

func (s windowsSender) Send(ctx context.Context, title, body string) error {
	script := buildWindowsToastScript(title, body)
	cmd := exec.CommandContext(
		ctx,
		s.executable,
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		script,
	)
	return cmd.Run()
}

func resolveWindowsPowerShellExecutable() string {
	candidates := []string{
		"powershell.exe",
		"powershell",
		"pwsh.exe",
		"pwsh",
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil && strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}

func buildWindowsToastScript(title, body string) string {
	titleEscaped := escapePowerShellSingleQuoted(title)
	bodyEscaped := escapePowerShellSingleQuoted(body)
	return "$t=[System.Security.SecurityElement]::Escape('" + titleEscaped + "');" +
		"$b=[System.Security.SecurityElement]::Escape('" + bodyEscaped + "');" +
		"[Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]>$null;" +
		"[Windows.Data.Xml.Dom.XmlDocument,Windows.Data.Xml.Dom.XmlDocument,ContentType=WindowsRuntime]>$null;" +
		"$xml=New-Object Windows.Data.Xml.Dom.XmlDocument;" +
		"$xml.LoadXml(\"<toast><visual><binding template='ToastGeneric'><text>$t</text><text>$b</text></binding></visual></toast>\");" +
		"$toast=[Windows.UI.Notifications.ToastNotification]::new($xml);" +
		"$notifier=[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('ByteMind');" +
		"$notifier.Show($toast);"
}

func escapePowerShellSingleQuoted(input string) string {
	return strings.ReplaceAll(input, "'", "''")
}
