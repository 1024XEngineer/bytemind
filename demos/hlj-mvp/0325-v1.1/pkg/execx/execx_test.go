package execx

import (
	"runtime"
	"testing"
)

func TestRiskBlocksDangerousCommands(t *testing.T) {
	executor := New(1, ".")

	if executor.Risk("rm -rf /tmp/demo") != RiskBlock {
		t.Fatal("expected rm -rf to be blocked")
	}
	if executor.Risk("git reset --hard HEAD") != RiskBlock {
		t.Fatal("expected git reset --hard to be blocked")
	}
	if executor.Risk("go test ./...") != RiskApprove {
		t.Fatal("expected normal command to require approval")
	}
}

func TestRunReturnsExitCode(t *testing.T) {
	executor := New(5, ".")

	result, err := executor.Run("echo hello")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestRunTimeout(t *testing.T) {
	executor := New(1, ".")

	cmd := "sleep 2"
	if runtime.GOOS == "windows" {
		cmd = "powershell -Command Start-Sleep -Seconds 2"
	}

	result, err := executor.Run(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut {
		t.Fatal("expected command to time out")
	}
}
