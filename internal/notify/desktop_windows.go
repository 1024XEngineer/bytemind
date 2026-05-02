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
	script := buildWindowsNotificationScript(title, body)
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

func buildWindowsNotificationScript(title, body string) string {
	titleEscaped := escapePowerShellSingleQuoted(title)
	bodyEscaped := escapePowerShellSingleQuoted(body)
	parts := []string{
		"$ErrorActionPreference='Stop';",
		"$t=[System.Security.SecurityElement]::Escape('" + titleEscaped + "');",
		"$b=[System.Security.SecurityElement]::Escape('" + bodyEscaped + "');",
		"function Show-ByteMindBalloon(){",
		"try{",
		"Add-Type -AssemblyName System.Windows.Forms >$null;",
		"Add-Type -AssemblyName System.Drawing >$null;",
		"$notifyIcon=New-Object System.Windows.Forms.NotifyIcon;",
		"$notifyIcon.Icon=[System.Drawing.SystemIcons]::Information;",
		"$notifyIcon.Visible=$true;",
		"$notifyIcon.BalloonTipTitle=$t;",
		"$notifyIcon.BalloonTipText=$b;",
		"$notifyIcon.ShowBalloonTip(5000);",
		"Start-Sleep -Milliseconds 1500;",
		"$notifyIcon.Dispose();",
		"return $true;",
		"}catch{",
		"return $false;",
		"}",
		"}",
		"function Show-ByteMindToast([string]$appId){",
		"try{",
		"[Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]>$null;",
		"[Windows.Data.Xml.Dom.XmlDocument,Windows.Data.Xml.Dom.XmlDocument,ContentType=WindowsRuntime]>$null;",
		"$xml=New-Object Windows.Data.Xml.Dom.XmlDocument;",
		"$xml.LoadXml(\"<toast><visual><binding template='ToastGeneric'><text>$t</text><text>$b</text></binding></visual></toast>\");",
		"$toast=[Windows.UI.Notifications.ToastNotification]::new($xml);",
		"$notifier=[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($appId);",
		"$notifier.Show($toast);",
		"return $true;",
		"}catch{",
		"return $false;",
		"}",
		"}",
		"$balloonSent=Show-ByteMindBalloon;",
		"if(-not $balloonSent){",
		"$toastSent=$false;",
		"foreach($appId in @('Windows PowerShell','PowerShell','Microsoft.WindowsTerminal_8wekyb3d8bbwe!App')){",
		"if(Show-ByteMindToast $appId){$toastSent=$true;break;}",
		"}",
		"if(-not $toastSent){",
		"exit 0;",
		"}",
		"}",
	}
	return strings.Join(parts, "")
}

func escapePowerShellSingleQuoted(input string) string {
	return strings.ReplaceAll(input, "'", "''")
}
