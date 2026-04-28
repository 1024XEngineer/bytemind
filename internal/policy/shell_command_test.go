package policy

import "testing"

func TestIsPlanSafeShellCommand(t *testing.T) {
	cases := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "allow ls", command: "ls -la", want: true},
		{name: "allow git status", command: "git status", want: true},
		{name: "allow go env", command: "go env GOPATH", want: true},
		{name: "allow find name filter", command: `find . -name "*.go"`, want: true},
		{name: "deny empty", command: "", want: false},
		{name: "deny write redirection", command: "echo hi > out.txt", want: false},
		{name: "deny multiple segments", command: "git status && pwd", want: false},
		{name: "deny script path", command: "./script.sh", want: false},
		{name: "deny shell interpreter", command: "bash -lc 'pwd'", want: false},
		{name: "deny command substitution", command: "ls $(pwd)", want: false},
		{name: "deny go env write flag", command: "go env -w GOPATH=/tmp/go", want: false},
		{name: "deny git output file flag", command: "git diff --output out.patch", want: false},
		{name: "deny find delete", command: "find . -delete", want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPlanSafeShellCommand(tc.command); got != tc.want {
				t.Fatalf("IsPlanSafeShellCommand(%q) = %v, want %v", tc.command, got, tc.want)
			}
		})
	}
}

func TestAssessCommandSegment(t *testing.T) {
	cases := []struct {
		name    string
		segment string
		risk    ShellRisk
	}{
		{name: "empty segment", segment: "   ", risk: ShellRiskSafe},
		{name: "blocked rm", segment: "rm -rf .", risk: ShellRiskBlocked},
		{name: "safe read only", segment: "git status", risk: ShellRiskSafe},
		{name: "approval go test", segment: "go test ./...", risk: ShellRiskApproval},
		{name: "approval by redirection", segment: "echo hi > out.txt", risk: ShellRiskApproval},
		{name: "approval by unknown command", segment: "custom-tool --run", risk: ShellRiskApproval},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := assessCommandSegment(tc.segment)
			if got.Risk != tc.risk {
				t.Fatalf("assessCommandSegment(%q) risk = %q, want %q (%#v)", tc.segment, got.Risk, tc.risk, got)
			}
		})
	}
}

func TestSplitCommandSegments(t *testing.T) {
	got := splitCommandSegments(`git status && echo "a;b|c" ; pwd|wc -l`)
	want := []string{"git status", `echo "a;b|c"`, "pwd", "wc -l"}
	if len(got) != len(want) {
		t.Fatalf("unexpected segments len: got=%d want=%d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment[%d] = %q, want %q (all=%#v)", i, got[i], want[i], got)
		}
	}
}

func TestSplitCommandFields(t *testing.T) {
	got := splitCommandFields(`go test "./internal/..." -run "Test Name"`)
	want := []string{"go", "test", "./internal/...", "-run", "Test Name"}
	if len(got) != len(want) {
		t.Fatalf("unexpected fields len: got=%d want=%d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field[%d] = %q, want %q (all=%#v)", i, got[i], want[i], got)
		}
	}
}

func TestHasWriteRedirection(t *testing.T) {
	if !hasWriteRedirection("echo hi > out.txt") {
		t.Fatal("expected write redirection to be detected")
	}
	if hasWriteRedirection(`echo "hello > world"`) {
		t.Fatal("expected quoted redirection symbol to be ignored")
	}
}

func TestBlockedGitVariants(t *testing.T) {
	if !isBlockedGit([]string{"git", "reset", "--hard", "HEAD~1"}) {
		t.Fatal("expected git reset --hard to be blocked")
	}
	if !isBlockedGit([]string{"git", "clean", "-fd"}) {
		t.Fatal("expected git clean -fd to be blocked")
	}
	if !isBlockedGit([]string{"git", "checkout", "--", "a.txt"}) {
		t.Fatal("expected git checkout -- file to be blocked")
	}
	if !isBlockedGit([]string{"git", "restore", "a.txt"}) {
		t.Fatal("expected git restore with file to be blocked")
	}
	if isBlockedGit([]string{"git", "reset", "--soft", "HEAD~1"}) {
		t.Fatal("expected git reset --soft not to be blocked")
	}
}

func TestReadOnlyAndApprovalCommandClassifiers(t *testing.T) {
	if !isReadOnlyGit([]string{"git", "status"}) {
		t.Fatal("expected read-only git status")
	}
	if isReadOnlyGit([]string{"git", "status", "-s"}) {
		t.Fatal("expected extra args to disallow read-only git classification")
	}
	if !isReadOnlyCommand("go", []string{"go", "version"}) {
		t.Fatal("expected go version to be read-only")
	}
	if !isReadOnlyCommand("npm", []string{"npm", "view", "react"}) {
		t.Fatal("expected npm view to be read-only")
	}
	if !isApprovalCommand("git", []string{"git", "commit", "-m", "x"}) {
		t.Fatal("expected git commit to require approval")
	}
	if !isApprovalCommand("python", []string{"python", "script.py"}) {
		t.Fatal("expected python command to require approval")
	}
	if isApprovalCommand("ls", []string{"ls"}) {
		t.Fatal("expected ls not to require approval")
	}
}

func TestLooksLikeScript(t *testing.T) {
	if !looksLikeScript("./script.sh") {
		t.Fatal("expected ./script.sh to be script-like")
	}
	if !looksLikeScript("setup.ps1") {
		t.Fatal("expected .ps1 extension to be script-like")
	}
	if looksLikeScript("git") {
		t.Fatal("expected git not to be script-like")
	}
}
