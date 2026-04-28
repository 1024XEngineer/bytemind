package tools

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestNormalizeSystemSandboxModeDefaultsOff(t *testing.T) {
	if got := normalizeSystemSandboxMode(nil); got != systemSandboxModeOff {
		t.Fatalf("expected nil exec context to normalize as off, got %q", got)
	}
	if got := normalizeSystemSandboxMode(&ExecutionContext{}); got != systemSandboxModeOff {
		t.Fatalf("expected empty mode to normalize as off, got %q", got)
	}
	if got := normalizeSystemSandboxMode(&ExecutionContext{SystemSandboxMode: "unknown"}); got != systemSandboxModeOff {
		t.Fatalf("expected unknown mode to normalize as off, got %q", got)
	}
}

func TestResolveSystemSandboxBackendRequiredFailsOnUnsupportedOS(t *testing.T) {
	_, err := resolveSystemSandboxBackend(systemSandboxModeRequired, "freebsd", func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err == nil {
		t.Fatal("expected required mode to fail on unsupported OS")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSystemSandboxBackendBestEffortFallsBackWhenUnavailable(t *testing.T) {
	backend, err := resolveSystemSandboxBackend(systemSandboxModeBestEffort, "linux", func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err != nil {
		t.Fatalf("expected best_effort to fallback without error, got %v", err)
	}
	if backend.Enabled {
		t.Fatalf("expected backend to be disabled when unshare is unavailable, got %#v", backend)
	}
}

func TestResolveSystemSandboxBackendRequiredFailsWhenUnavailableOnLinux(t *testing.T) {
	_, err := resolveSystemSandboxBackend(systemSandboxModeRequired, "linux", func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err == nil {
		t.Fatal("expected required mode to fail when unshare is unavailable")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unshare") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSystemSandboxBackendEnablesLinuxUnshareWhenAvailable(t *testing.T) {
	backend, err := resolveSystemSandboxBackend(systemSandboxModeRequired, "linux", func(string) (string, error) {
		return "/usr/bin/unshare", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !backend.Enabled {
		t.Fatalf("expected backend enabled, got %#v", backend)
	}
	if backend.Runner != "/usr/bin/unshare" {
		t.Fatalf("expected unshare runner, got %#v", backend)
	}
	if len(backend.ArgPrefix) == 0 {
		t.Fatalf("expected unshare arg prefix, got %#v", backend)
	}
}

func TestWithRequiredLinuxShellLimitsAddsGuardCommands(t *testing.T) {
	got := withRequiredLinuxShellLimits("go test ./...")
	wantParts := []string{
		"ulimit -t 120 >/dev/null 2>&1 || true",
		"ulimit -f 1048576 >/dev/null 2>&1 || true",
		"ulimit -v 2097152 >/dev/null 2>&1 || true",
		"go test ./...",
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Fatalf("expected wrapped command to contain %q, got %q", part, got)
		}
	}
}

func TestWithRequiredLinuxShellLimitsTrimsAndHandlesEmpty(t *testing.T) {
	if got := withRequiredLinuxShellLimits("   "); got != "" {
		t.Fatalf("expected empty command to stay empty, got %q", got)
	}
	got := withRequiredLinuxShellLimits("  git status  ")
	if !strings.HasSuffix(got, "git status") {
		t.Fatalf("expected command suffix to be trimmed original command, got %q", got)
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

func TestBuildSystemSandboxExecutionMetadataDefaults(t *testing.T) {
	meta := buildSystemSandboxExecutionMetadata("", systemSandboxRuntimeBackend{})
	if got := meta["mode"]; got != systemSandboxModeOff {
		t.Fatalf("expected default mode off, got %#v", meta)
	}
	if got := meta["backend"]; got != "none" {
		t.Fatalf("expected backend none, got %#v", meta)
	}
	if got := meta["active"]; got != false {
		t.Fatalf("expected active=false, got %#v", meta)
	}
	if got := meta["required_capable"]; got != false {
		t.Fatalf("expected required_capable=false, got %#v", meta)
	}
	if got := meta["capability_level"]; got != "none" {
		t.Fatalf("expected capability_level=none, got %#v", meta)
	}
	if got := meta["shell_network_isolation"]; got != false {
		t.Fatalf("expected shell_network_isolation=false, got %#v", meta)
	}
	if got := meta["worker_network_isolation"]; got != false {
		t.Fatalf("expected worker_network_isolation=false, got %#v", meta)
	}
	if got := meta["fallback"]; got != false {
		t.Fatalf("expected fallback=false, got %#v", meta)
	}
	if got := meta["status"]; got != "inactive" {
		t.Fatalf("expected status inactive, got %#v", meta)
	}
}

func TestBuildSystemSandboxExecutionMetadataFallback(t *testing.T) {
	meta := buildSystemSandboxExecutionMetadata(systemSandboxModeBestEffort, systemSandboxRuntimeBackend{
		Name:              "darwin_sandbox_exec",
		UnavailableReason: "darwin backend \"sandbox-exec\" is unavailable",
	})
	if got := meta["fallback"]; got != true {
		t.Fatalf("expected fallback=true for best_effort without backend, got %#v", meta)
	}
	if got := meta["active"]; got != false {
		t.Fatalf("expected active=false without backend, got %#v", meta)
	}
	if got := meta["required_capable"]; got != false {
		t.Fatalf("expected required_capable=false in fallback metadata, got %#v", meta)
	}
	if got := meta["capability_level"]; got != "none" {
		t.Fatalf("expected capability_level=none in fallback metadata, got %#v", meta)
	}
	if got := meta["shell_network_isolation"]; got != false {
		t.Fatalf("expected shell_network_isolation=false in fallback metadata, got %#v", meta)
	}
	if got := meta["worker_network_isolation"]; got != false {
		t.Fatalf("expected worker_network_isolation=false in fallback metadata, got %#v", meta)
	}
	if got := meta["status"]; got != "fallback" {
		t.Fatalf("expected status fallback, got %#v", meta)
	}
	if got := metadataString(meta["fallback_reason"]); !strings.Contains(strings.ToLower(got), "sandbox-exec") {
		t.Fatalf("expected fallback_reason to include sandbox-exec, got %#v", meta)
	}
}

func TestRunShellToolResultIncludesSandboxMetadata(t *testing.T) {
	tool := RunShellTool{}
	command := "echo ok"
	if runtime.GOOS == "windows" {
		command = "Write-Output ok"
	}
	raw, err := tool.Run(context.Background(), []byte(`{"command":"`+command+`"}`), &ExecutionContext{
		Workspace:         t.TempDir(),
		ApprovalPolicy:    "never",
		SystemSandboxMode: "off",
		Stdin:             strings.NewReader(""),
		Stdout:            &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("run shell: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal run_shell result: %v", err)
	}
	metadataRaw, ok := payload["system_sandbox"]
	if !ok {
		t.Fatalf("expected system_sandbox metadata, got %#v", payload)
	}
	metadata, ok := metadataRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected system_sandbox object, got %#v", metadataRaw)
	}
	if got := metadata["mode"]; got != systemSandboxModeOff {
		t.Fatalf("expected mode off, got %#v", metadata)
	}
	if got := metadata["backend"]; got != "none" {
		t.Fatalf("expected backend none, got %#v", metadata)
	}
	if got := metadata["active"]; got != false {
		t.Fatalf("expected active=false, got %#v", metadata)
	}
	if got := metadata["required_capable"]; got != false {
		t.Fatalf("expected required_capable=false, got %#v", metadata)
	}
	if got := metadata["capability_level"]; got != "none" {
		t.Fatalf("expected capability_level=none, got %#v", metadata)
	}
	if got := metadata["shell_network_isolation"]; got != false {
		t.Fatalf("expected shell_network_isolation=false, got %#v", metadata)
	}
	if got := metadata["worker_network_isolation"]; got != false {
		t.Fatalf("expected worker_network_isolation=false, got %#v", metadata)
	}
	if got := metadata["fallback"]; got != false {
		t.Fatalf("expected fallback=false, got %#v", metadata)
	}
	if got := metadata["status"]; got != "inactive" {
		t.Fatalf("expected status inactive, got %#v", metadata)
	}
}

func TestBuildSystemSandboxExecutionMetadataMarksRequiredCapabilityWhenBackendSupportsIt(t *testing.T) {
	meta := buildSystemSandboxExecutionMetadata(systemSandboxModeRequired, systemSandboxRuntimeBackend{
		Enabled: true,
		Name:    "linux_unshare",
		Shell: systemSandboxLaunchSpec{
			Policy: systemSandboxPolicy{
				FileIsolation:    true,
				ProcessIsolation: true,
				NetworkIsolation: true,
			},
		},
		Worker: systemSandboxLaunchSpec{
			Policy: systemSandboxPolicy{
				FileIsolation:    true,
				ProcessIsolation: true,
				NetworkIsolation: false,
			},
		},
	})
	if got := meta["required_capable"]; got != true {
		t.Fatalf("expected required_capable=true for capable backend, got %#v", meta)
	}
	if got := meta["capability_level"]; got != "full" {
		t.Fatalf("expected capability_level=full for capable backend, got %#v", meta)
	}
	if got := meta["shell_network_isolation"]; got != true {
		t.Fatalf("expected shell_network_isolation=true for linux backend, got %#v", meta)
	}
	if got := meta["worker_network_isolation"]; got != false {
		t.Fatalf("expected worker_network_isolation=false for linux backend, got %#v", meta)
	}
}

func TestBuildSystemSandboxExecutionMetadataWindowsRequiredMarksCapability(t *testing.T) {
	meta := buildSystemSandboxExecutionMetadata(systemSandboxModeRequired, systemSandboxRuntimeBackend{
		Enabled: true,
		Name:    "windows_job_object",
		Shell: systemSandboxLaunchSpec{
			Policy: systemSandboxPolicy{
				FileIsolation:    false,
				ProcessIsolation: true,
			},
		},
		Worker: systemSandboxLaunchSpec{
			Policy: systemSandboxPolicy{
				FileIsolation:    false,
				ProcessIsolation: true,
			},
		},
	})
	if got := meta["required_capable"]; got != true {
		t.Fatalf("expected windows required mode to mark required_capable=true, got %#v", meta)
	}
	if got := meta["capability_level"]; got != "guarded" {
		t.Fatalf("expected capability_level=guarded for windows required mode, got %#v", meta)
	}
}

func TestValidateRequiredWindowsShellCommandAllowsReadOnly(t *testing.T) {
	if err := validateRequiredWindowsShellCommand("git status"); err != nil {
		t.Fatalf("expected read-only command to be allowed, got %v", err)
	}
}

func TestValidateRequiredWindowsShellCommandAllowsPlanSafeGoEnv(t *testing.T) {
	if err := validateRequiredWindowsShellCommand("go env"); err != nil {
		t.Fatalf("expected go env to be allowed in strict read-only mode, got %v", err)
	}
}

func TestValidateRequiredWindowsShellCommandRejectsMultiSegmentReadOnly(t *testing.T) {
	err := validateRequiredWindowsShellCommand("git status && pwd")
	if err == nil {
		t.Fatal("expected multi-segment command to be rejected in strict mode")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "strict read-only") {
		t.Fatalf("expected strict read-only guard message, got %v", err)
	}
}

func TestValidateRequiredWindowsShellCommandRejectsEchoEvenWithoutRedirection(t *testing.T) {
	err := validateRequiredWindowsShellCommand("echo ok")
	if err == nil {
		t.Fatal("expected echo command to be rejected in strict mode")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "strict read-only") {
		t.Fatalf("expected strict read-only guard message, got %v", err)
	}
}

func TestValidateRequiredWindowsShellCommandRejectsNonReadOnly(t *testing.T) {
	err := validateRequiredWindowsShellCommand("go test ./...")
	if err == nil {
		t.Fatal("expected non-read-only command to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "strict read-only") {
		t.Fatalf("expected required windows guard message, got %v", err)
	}
}

func TestValidateRequiredWindowsShellCommandRejectsGoEnvWriteFlag(t *testing.T) {
	err := validateRequiredWindowsShellCommand("go env -w GOPATH=C:\\tmp\\go")
	if err == nil {
		t.Fatal("expected go env -w to be rejected in strict mode")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "strict read-only") {
		t.Fatalf("expected strict read-only guard message, got %v", err)
	}
}

func TestValidateRequiredWindowsShellCommandRejectsGitOutputFlag(t *testing.T) {
	err := validateRequiredWindowsShellCommand("git diff --output out.patch")
	if err == nil {
		t.Fatal("expected git diff --output to be rejected in strict mode")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "strict read-only") {
		t.Fatalf("expected strict read-only guard message, got %v", err)
	}
}

func TestValidateRequiredWindowsShellCommandRejectsFindDelete(t *testing.T) {
	err := validateRequiredWindowsShellCommand("find . -delete")
	if err == nil {
		t.Fatal("expected find -delete to be rejected in strict mode")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "strict read-only") {
		t.Fatalf("expected strict read-only guard message, got %v", err)
	}
}

func TestRunShellToolRequiredModeBackendUnavailableReturnsPermissionDenied(t *testing.T) {
	originalLookPath := runShellLookPath
	runShellLookPath = func(string) (string, error) {
		return "", errors.New("backend unavailable")
	}
	defer func() {
		runShellLookPath = originalLookPath
	}()

	tool := RunShellTool{}
	command := "echo ok"
	if runtime.GOOS == "windows" {
		command = "Write-Output ok"
	}
	_, err := tool.Run(context.Background(), []byte(`{"command":"`+command+`"}`), &ExecutionContext{
		Workspace:         t.TempDir(),
		ApprovalPolicy:    "never",
		SystemSandboxMode: "required",
		Stdin:             strings.NewReader(""),
		Stdout:            &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected required mode to fail when sandbox backend is unavailable")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T (%v)", err, err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission_denied code, got %s (%v)", execErr.Code, err)
	}
	if !strings.Contains(strings.ToLower(execErr.Message), "required") {
		t.Fatalf("expected required-mode message, got %q", execErr.Message)
	}
}

func metadataString(v any) string {
	if value, ok := v.(string); ok {
		return value
	}
	return ""
}

type stubFileInfo struct{}

func (stubFileInfo) Name() string       { return "powershell.exe" }
func (stubFileInfo) Size() int64        { return 0 }
func (stubFileInfo) Mode() os.FileMode  { return 0o644 }
func (stubFileInfo) ModTime() time.Time { return time.Time{} }
func (stubFileInfo) IsDir() bool        { return false }
func (stubFileInfo) Sys() any           { return nil }
