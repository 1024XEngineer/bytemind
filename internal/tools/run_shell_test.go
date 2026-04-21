package tools

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAssessShellCommandAllowsReadOnlyCommands(t *testing.T) {
	assessment := assessShellCommand("git status")
	if assessment.Risk != shellRiskSafe {
		t.Fatalf("expected safe command, got %#v", assessment)
	}
}

func TestAssessShellCommandRequiresApprovalForWriteRedirection(t *testing.T) {
	assessment := assessShellCommand("echo hi > out.txt")
	if assessment.Risk != shellRiskApproval {
		t.Fatalf("expected approval risk, got %#v", assessment)
	}
	if !strings.Contains(assessment.Reason, "redirection") {
		t.Fatalf("expected redirection reason, got %#v", assessment)
	}
}

func TestAssessShellCommandBlocksDangerousGitReset(t *testing.T) {
	assessment := assessShellCommand("git reset --hard HEAD~1")
	if assessment.Risk != shellRiskBlocked {
		t.Fatalf("expected blocked command, got %#v", assessment)
	}
}

func TestAssessShellCommandSplitsSegments(t *testing.T) {
	assessment := assessShellCommand("git status && go test ./...")
	if assessment.Risk != shellRiskApproval {
		t.Fatalf("expected approval risk from second segment, got %#v", assessment)
	}
}

func TestAssessShellCommandIgnoresRedirectionInsideQuotes(t *testing.T) {
	assessment := assessShellCommand(`echo "hello > world"`)
	if assessment.Risk != shellRiskSafe {
		t.Fatalf("expected quoted redirection to stay safe, got %#v", assessment)
	}
}

func TestAssessShellCommandDoesNotSplitQuotedSegments(t *testing.T) {
	assessment := assessShellCommand(`echo "git status && go test ./..."`)
	if assessment.Risk != shellRiskSafe {
		t.Fatalf("expected quoted separators to stay inside one safe echo command, got %#v", assessment)
	}
}

func TestAssessShellCommandBlocksDangerousCommandInLaterSegment(t *testing.T) {
	assessment := assessShellCommand("git status && rm -rf .")
	if assessment.Risk != shellRiskBlocked {
		t.Fatalf("expected later dangerous segment to block command, got %#v", assessment)
	}
}

func TestRequireApprovalOnRequestAllowsReadOnlyWithoutPrompt(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("git status", &ExecutionContext{
		ApprovalPolicy: "on-request",
		Stdin:          strings.NewReader(""),
		Stdout:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no prompt for safe command, got %q", out.String())
	}
}

func TestRequireApprovalSkipsWhenPreApprovedByParentWorker(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("go test ./...", &ExecutionContext{
		ApprovalPolicy:    "on-request",
		SkipShellApproval: true,
		Stdin:             strings.NewReader(""),
		Stdout:            &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no prompt when shell approval is pre-approved, got %q", out.String())
	}
}

func TestRequireApprovalOnRequestPromptsForRiskyCommand(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("go test ./...", &ExecutionContext{
		ApprovalPolicy: "on-request",
		Stdin:          strings.NewReader("yes\n"),
		Stdout:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Approve shell command") {
		t.Fatalf("expected approval prompt, got %q", out.String())
	}
}

func TestRequireApprovalAlwaysPromptsEvenForSafeCommand(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("git status", &ExecutionContext{
		ApprovalPolicy: "always",
		Stdin:          strings.NewReader("y\n"),
		Stdout:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Approve shell command") {
		t.Fatalf("expected prompt for always policy, got %q", out.String())
	}
}

func TestRequireApprovalNeverSkipsPromptForRiskyCommand(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("go test ./...", &ExecutionContext{
		ApprovalPolicy: "never",
		Stdin:          strings.NewReader(""),
		Stdout:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no prompt for never policy, got %q", out.String())
	}
}

func TestRequireApprovalBlocksDangerousCommandRegardlessOfPolicy(t *testing.T) {
	err := requireApproval("rm -rf .", &ExecutionContext{ApprovalPolicy: "never"})
	if err == nil {
		t.Fatal("expected dangerous command to be blocked")
	}
	if !strings.Contains(err.Error(), "blocked dangerous shell command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireApprovalNeedsStdinWhenPrompting(t *testing.T) {
	err := requireApproval("go test ./...", &ExecutionContext{ApprovalPolicy: "always"})
	if err == nil {
		t.Fatal("expected missing stdin error")
	}
	if !strings.Contains(err.Error(), "approval channel is unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireApprovalReturnsClearDenialMessage(t *testing.T) {
	err := requireApproval("go test ./...", &ExecutionContext{
		ApprovalPolicy: "on-request",
		Stdin:          strings.NewReader("n\n"),
		Stdout:         &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected denial error")
	}
	if !strings.Contains(err.Error(), "was not run because approval was denied") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireApprovalAwayModeAutoDenyDoesNotPrompt(t *testing.T) {
	var out bytes.Buffer
	err := requireApproval("go test ./...", &ExecutionContext{
		ApprovalPolicy: "on-request",
		ApprovalMode:   "away",
		AwayPolicy:     "auto_deny_continue",
		Stdin:          strings.NewReader("yes\n"),
		Stdout:         &out,
	})
	if err == nil {
		t.Fatal("expected away mode to deny approval-required shell command")
	}
	if out.Len() != 0 {
		t.Fatalf("expected no prompt output in away mode, got %q", out.String())
	}
	if !strings.Contains(err.Error(), "away mode") {
		t.Fatalf("expected away mode message, got %v", err)
	}
}

func TestRequireApprovalAwayModeFailFastIncludesPolicyInError(t *testing.T) {
	err := requireApproval("go test ./...", &ExecutionContext{
		ApprovalPolicy: "on-request",
		ApprovalMode:   "away",
		AwayPolicy:     "fail_fast",
		Stdin:          strings.NewReader(""),
		Stdout:         &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected away mode fail_fast to deny approval-required shell command")
	}
	if !strings.Contains(err.Error(), "away_policy=fail_fast") {
		t.Fatalf("expected fail_fast policy in error, got %v", err)
	}
}

func TestResolveWindowsShellExecutablePrefersLookPathCandidate(t *testing.T) {
	got := resolveWindowsShellExecutable(
		func(file string) (string, error) {
			if file == "powershell.exe" {
				return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, nil
			}
			return "", errors.New("not found")
		},
		func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		func(key string) string { return "" },
	)
	if !strings.EqualFold(got, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`) {
		t.Fatalf("expected lookPath candidate, got %q", got)
	}
}

func TestResolveWindowsShellExecutableFallsBackToAbsoluteCandidate(t *testing.T) {
	windowsRoot := `C:\Windows`
	expected := filepath.Join(windowsRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	got := resolveWindowsShellExecutable(
		func(file string) (string, error) {
			return "", errors.New("not found")
		},
		func(name string) (os.FileInfo, error) {
			if strings.EqualFold(name, expected) {
				return stubFileInfo{}, nil
			}
			return nil, os.ErrNotExist
		},
		func(key string) string {
			if key == "SystemRoot" {
				return windowsRoot
			}
			return ""
		},
	)
	if !strings.EqualFold(got, expected) {
		t.Fatalf("expected absolute fallback %q, got %q", expected, got)
	}
}

func TestResolveWindowsShellExecutableFallbacksToPowerShellLiteral(t *testing.T) {
	got := resolveWindowsShellExecutable(
		func(file string) (string, error) { return "", errors.New("not found") },
		func(name string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		func(key string) string { return "" },
	)
	if got != "powershell" {
		t.Fatalf("expected final fallback powershell, got %q", got)
	}
}

func TestRunShellToolReturnsTimeoutError(t *testing.T) {
	tool := RunShellTool{}
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "Start-Sleep -Seconds 2"
	}
	_, err := tool.Run(context.Background(), []byte(`{"command":"`+command+`","timeout_seconds":1}`), &ExecutionContext{
		Workspace:      t.TempDir(),
		ApprovalPolicy: "never",
		Stdin:          strings.NewReader(""),
		Stdout:         &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type stubFileInfo struct{}

func (stubFileInfo) Name() string       { return "powershell.exe" }
func (stubFileInfo) Size() int64        { return 0 }
func (stubFileInfo) Mode() os.FileMode  { return 0o644 }
func (stubFileInfo) ModTime() time.Time { return time.Time{} }
func (stubFileInfo) IsDir() bool        { return false }
func (stubFileInfo) Sys() any           { return nil }
