package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestPolicyBrokerAllowsOperationWithinLease(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(2 * time.Minute),
		Static:  StaticPolicy{ApprovalPolicy: "on-request"},
		Mode:    ModeContext{ApprovalMode: "interactive", ApprovalChannelAvailable: true},
		Request: RuntimeRequest{
			ToolName:   "write_file",
			FilePath:   filepath.Join(roots.Write, "note.txt"),
			FileAccess: FileAccessWrite,
			Command:    "go",
			Args:       []string{"test", "./..."},
			Network:    NetworkRule{Host: "api.openai.com", Port: 443, Scheme: "https"},
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionAllow {
		t.Fatalf("expected allow decision, got %#v", result)
	}
}

func TestPolicyBrokerDeniesPathOutsideLease(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Request: RuntimeRequest{
			FilePath:   filepath.Join(t.TempDir(), "escape.txt"),
			FileAccess: FileAccessWrite,
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonFSOutOfScope {
		t.Fatalf("expected fs_out_of_scope deny, got %#v", result)
	}
}

func TestPolicyBrokerDeniesCommandOutsideAllowlist(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Request: RuntimeRequest{
			Command: "rm",
			Args:    []string{"-rf", "."},
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonExecNotAllowed {
		t.Fatalf("expected exec_not_allowed deny, got %#v", result)
	}
}

func TestPolicyBrokerRequiresOrderedExecArguments(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Request: RuntimeRequest{
			Command: "go",
			Args:    []string{"./...", "test"},
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonExecNotAllowed {
		t.Fatalf("expected ordered-args mismatch to deny, got %#v", result)
	}

	allowResult, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Request: RuntimeRequest{
			Command: "go",
			Args:    []string{"test", "./..."},
		},
	})
	if err != nil {
		t.Fatalf("broker decide allow case: %v", err)
	}
	if allowResult.Decision != DecisionAllow {
		t.Fatalf("expected ordered-args exact match to allow, got %#v", allowResult)
	}
}

func TestPolicyBrokerEscalatesCommandOutsideAllowlistWhenInteractiveApprovalAvailable(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Static:  StaticPolicy{ApprovalPolicy: "on-request"},
		Mode:    ModeContext{ApprovalMode: "interactive", ApprovalChannelAvailable: true},
		Request: RuntimeRequest{
			Command: "rm",
			Args:    []string{"-rf", "."},
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionEscalate || result.ReasonCode != ReasonApprovalRequired {
		t.Fatalf("expected approval_required escalate, got %#v", result)
	}
}

func TestPolicyBrokerDeniesCommandOutsideAllowlistWhenApprovalChannelUnavailable(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Static:  StaticPolicy{ApprovalPolicy: "on-request"},
		Mode:    ModeContext{ApprovalMode: "interactive", ApprovalChannelAvailable: false},
		Request: RuntimeRequest{
			Command: "rm",
			Args:    []string{"-rf", "."},
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonApprovalChannelUnavailable {
		t.Fatalf("expected approval_channel_unavailable deny, got %#v", result)
	}
}

func TestPolicyBrokerDeniesNetworkOutsideAllowlist(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Request: RuntimeRequest{
			Network: NetworkRule{Host: "example.org", Port: 443, Scheme: "https"},
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonNetworkNotAllowed {
		t.Fatalf("expected network_not_allowed deny, got %#v", result)
	}
}

func TestPolicyBrokerEscalatesInteractiveApproval(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Mode:    ModeContext{ApprovalMode: "interactive", ApprovalChannelAvailable: true},
		Static:  StaticPolicy{ApprovalPolicy: "on-request"},
		Request: RuntimeRequest{
			RequiresApproval: true,
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionEscalate || result.ReasonCode != ReasonApprovalRequired {
		t.Fatalf("expected approval_required escalate, got %#v", result)
	}
}

func TestPolicyBrokerDeniesWhenApprovalChannelUnavailable(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Mode:    ModeContext{ApprovalMode: "interactive", ApprovalChannelAvailable: false},
		Request: RuntimeRequest{
			RequiresApproval: true,
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonApprovalChannelUnavailable {
		t.Fatalf("expected approval_channel_unavailable deny, got %#v", result)
	}
}

func TestPolicyBrokerDeniesAwayApprovalRequirement(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Mode:    ModeContext{ApprovalMode: "away", AwayPolicy: "auto_deny_continue"},
		Request: RuntimeRequest{
			RequiresApproval: true,
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonApprovalRequired {
		t.Fatalf("expected approval_required deny in away mode, got %#v", result)
	}
}

func TestPolicyBrokerDeniesExpiredLease(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonLeaseExpired {
		t.Fatalf("expected lease_expired deny, got %#v", result)
	}
}

func TestPolicyBrokerDeniesInvalidSignature(t *testing.T) {
	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	roots := sandboxRoots(t)
	lease, keyring := mustSignedLease(t, now, roots)
	lease.Signature = "abc123"

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionDeny || result.ReasonCode != ReasonLeaseSignatureInvalid {
		t.Fatalf("expected lease_signature_invalid deny, got %#v", result)
	}
}

func TestPolicyBrokerAllowsMissingPathWithinSymlinkedLeaseRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies by Windows permissions")
	}

	now := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	base := t.TempDir()
	realRoot := filepath.Join(base, "real-root")
	linkRoot := filepath.Join(base, "link-root")

	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unavailable in current environment: %v", err)
	}

	lease, keyring := mustSignedLease(t, now, testSandboxRoots{
		Read:  linkRoot,
		Write: linkRoot,
	})

	broker := NewPolicyBroker()
	result, err := broker.Decide(context.Background(), DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     now.Add(1 * time.Minute),
		Request: RuntimeRequest{
			FilePath:   filepath.Join(linkRoot, "nested", "new-file.txt"),
			FileAccess: FileAccessWrite,
		},
	})
	if err != nil {
		t.Fatalf("broker decide: %v", err)
	}
	if result.Decision != DecisionAllow {
		t.Fatalf("expected allow decision for symlinked lease root path, got %#v", result)
	}
}

type testSandboxRoots struct {
	Read  string
	Write string
}

func sandboxRoots(t *testing.T) testSandboxRoots {
	t.Helper()
	base := t.TempDir()
	readRoot := filepath.Join(base, "read-root")
	writeRoot := filepath.Join(base, "write-root")
	if err := os.MkdirAll(readRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(writeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return testSandboxRoots{Read: readRoot, Write: writeRoot}
}

func mustSignedLease(t *testing.T, issuedAt time.Time, roots testSandboxRoots) (Lease, map[string][]byte) {
	t.Helper()
	lease := Lease{
		Version:      LeaseVersionV1,
		LeaseID:      "lease-broker",
		RunID:        "run-broker",
		Scope:        LeaseScopeRun,
		IssuedAt:     issuedAt,
		ExpiresAt:    issuedAt.Add(1 * time.Hour),
		KID:          "k1",
		ApprovalMode: "interactive",
		AwayPolicy:   "auto_deny_continue",
		FSRead:       []string{roots.Read},
		FSWrite:      []string{roots.Write},
		ExecAllowlist: []ExecRule{
			{Command: "go", ArgsPattern: []string{"test", "./..."}},
		},
		NetworkAllowlist: []NetworkRule{
			{Host: "api.openai.com", Port: 443, Scheme: "https"},
		},
	}
	keyring := map[string][]byte{"k1": []byte("broker-secret")}
	signed, err := SignLease(lease, keyring["k1"])
	if err != nil {
		t.Fatalf("sign lease: %v", err)
	}
	return signed, keyring
}
