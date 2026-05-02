package notify

import (
	"strings"
	"testing"
)

func TestBuildWindowsNotificationScriptPrefersBalloonAndHasToastFallback(t *testing.T) {
	script := buildWindowsNotificationScript("title", "body")

	required := []string{
		"function Show-ByteMindBalloon(){",
		"$balloonSent=Show-ByteMindBalloon;",
		"if(-not $balloonSent){",
		"function Show-ByteMindToast([string]$appId){",
		"CreateToastNotifier($appId)",
		"'Windows PowerShell'",
		"'PowerShell'",
		"'Microsoft.WindowsTerminal_8wekyb3d8bbwe!App'",
	}
	for _, token := range required {
		if !strings.Contains(script, token) {
			t.Fatalf("script missing token %q", token)
		}
	}
}

func TestBuildWindowsNotificationScriptEscapesSingleQuote(t *testing.T) {
	script := buildWindowsNotificationScript("o'hara", "don't leak")
	if strings.Count(script, "o''hara") == 0 {
		t.Fatalf("expected escaped title in script")
	}
	if strings.Count(script, "don''t leak") == 0 {
		t.Fatalf("expected escaped body in script")
	}
}
