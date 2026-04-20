package sandbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSignLeaseCanonicalizationIsStable(t *testing.T) {
	base := t.TempDir()
	issued := time.Date(2026, 4, 20, 13, 5, 0, 0, time.FixedZone("CST", 8*3600))
	expires := issued.Add(2 * time.Hour)
	leaseA := Lease{
		Version:      LeaseVersionV1,
		LeaseID:      "lease-1",
		RunID:        "run-1",
		Scope:        LeaseScopeRun,
		IssuedAt:     issued,
		ExpiresAt:    expires,
		KID:          "k1",
		ApprovalMode: "interactive",
		AwayPolicy:   "auto_deny_continue",
		FSRead: []string{
			filepath.Join(base, "r-b"),
			filepath.Join(base, "r-a"),
			filepath.Join(base, ".", "r-a"),
		},
		FSWrite: []string{
			filepath.Join(base, "w-b"),
			filepath.Join(base, "w-a"),
		},
		ExecAllowlist: []ExecRule{
			{Command: "python", ArgsPattern: []string{"pytest", "-m"}},
			{Command: "go test", ArgsPattern: []string{"./...", "./..."}},
		},
		NetworkAllowlist: []NetworkRule{
			{Host: "Example.com", Port: 443, Scheme: "HTTPS"},
			{Host: "api.openai.com", Port: 443, Scheme: "https"},
			{Host: "example.com", Port: 443, Scheme: "https"},
		},
	}
	leaseB := Lease{
		Version:      LeaseVersionV1,
		LeaseID:      "lease-1",
		RunID:        "run-1",
		Scope:        LeaseScopeRun,
		IssuedAt:     issued.UTC(),
		ExpiresAt:    expires.UTC(),
		KID:          "k1",
		ApprovalMode: "interactive",
		AwayPolicy:   "auto_deny_continue",
		FSRead: []string{
			filepath.Join(base, "r-a"),
			filepath.Join(base, "r-b"),
		},
		FSWrite: []string{
			filepath.Join(base, ".", "w-a"),
			filepath.Join(base, "w-b"),
		},
		ExecAllowlist: []ExecRule{
			{Command: "go test", ArgsPattern: []string{"./...", "./..."}},
			{Command: "python", ArgsPattern: []string{"pytest", "-m"}},
		},
		NetworkAllowlist: []NetworkRule{
			{Host: "api.openai.com", Port: 443, Scheme: "https"},
			{Host: "example.com", Port: 443, Scheme: "https"},
		},
	}
	key := []byte("lease-secret")
	signedA, err := SignLease(leaseA, key)
	if err != nil {
		t.Fatalf("sign lease A: %v", err)
	}
	signedB, err := SignLease(leaseB, key)
	if err != nil {
		t.Fatalf("sign lease B: %v", err)
	}

	if signedA.Signature != signedB.Signature {
		t.Fatalf("expected stable signature, got A=%q B=%q", signedA.Signature, signedB.Signature)
	}

	payload, err := CanonicalPayload(signedA)
	if err != nil {
		t.Fatalf("canonical payload: %v", err)
	}
	if bytes.Contains(payload, []byte(`"signature"`)) {
		t.Fatalf("canonical payload must exclude signature, got %s", string(payload))
	}

	if err := VerifySignedLease(signedA, map[string][]byte{"k1": key}, issued.Add(1*time.Minute)); err != nil {
		t.Fatalf("verify signed lease: %v", err)
	}
}

func TestSignLeaseDistinguishesExecArgumentOrder(t *testing.T) {
	base := t.TempDir()
	issued := time.Date(2026, 4, 20, 13, 5, 0, 0, time.UTC)
	expires := issued.Add(2 * time.Hour)
	lease := Lease{
		Version:      LeaseVersionV1,
		LeaseID:      "lease-order",
		RunID:        "run-order",
		Scope:        LeaseScopeRun,
		IssuedAt:     issued,
		ExpiresAt:    expires,
		KID:          "k1",
		ApprovalMode: "interactive",
		AwayPolicy:   "auto_deny_continue",
		FSRead:       []string{filepath.Join(base, "r")},
		FSWrite:      []string{filepath.Join(base, "w")},
	}

	leaseA := lease
	leaseA.ExecAllowlist = []ExecRule{
		{Command: "deploy", ArgsPattern: []string{"--to", "prod"}},
	}
	leaseB := lease
	leaseB.ExecAllowlist = []ExecRule{
		{Command: "deploy", ArgsPattern: []string{"prod", "--to"}},
	}

	key := []byte("lease-secret")
	signedA, err := SignLease(leaseA, key)
	if err != nil {
		t.Fatalf("sign lease A: %v", err)
	}
	signedB, err := SignLease(leaseB, key)
	if err != nil {
		t.Fatalf("sign lease B: %v", err)
	}
	if signedA.Signature == signedB.Signature {
		t.Fatalf("expected different signatures when exec args order differs")
	}
}

func TestVerifySignedLeaseRejectsTampering(t *testing.T) {
	base := t.TempDir()
	issued := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	lease := Lease{
		Version:      LeaseVersionV1,
		LeaseID:      "lease-tamper",
		RunID:        "run-tamper",
		Scope:        LeaseScopeRun,
		IssuedAt:     issued,
		ExpiresAt:    issued.Add(30 * time.Minute),
		KID:          "k1",
		ApprovalMode: "interactive",
		AwayPolicy:   "auto_deny_continue",
		FSRead:       []string{filepath.Join(base, "r")},
		FSWrite:      []string{filepath.Join(base, "w")},
	}
	key := []byte("lease-secret")
	signed, err := SignLease(lease, key)
	if err != nil {
		t.Fatalf("sign lease: %v", err)
	}
	signed.FSWrite = append(signed.FSWrite, filepath.Join(base, "hijack"))
	err = VerifySignedLease(signed, map[string][]byte{"k1": key}, issued.Add(5*time.Minute))
	assertLeaseErrorCode(t, err, ReasonLeaseSignatureInvalid)
}

func TestVerifySignedLeaseRejectsExpiredLease(t *testing.T) {
	issued := time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC)
	lease := Lease{
		Version:      LeaseVersionV1,
		LeaseID:      "lease-expired",
		RunID:        "run-expired",
		Scope:        LeaseScopeRun,
		IssuedAt:     issued,
		ExpiresAt:    issued.Add(10 * time.Minute),
		KID:          "k1",
		ApprovalMode: "away",
		AwayPolicy:   "fail_fast",
	}
	key := []byte("lease-secret")
	signed, err := SignLease(lease, key)
	if err != nil {
		t.Fatalf("sign lease: %v", err)
	}
	err = VerifySignedLease(signed, map[string][]byte{"k1": key}, issued.Add(10*time.Minute))
	assertLeaseErrorCode(t, err, ReasonLeaseExpired)
}

func TestParseHMACKeyring(t *testing.T) {
	keyring, err := ParseHMACKeyring("k1:secret-one, k2:secret-two")
	if err != nil {
		t.Fatalf("parse keyring: %v", err)
	}
	if len(keyring) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keyring))
	}
	if string(keyring["k1"]) != "secret-one" {
		t.Fatalf("unexpected key material for k1")
	}

	_, err = ParseHMACKeyring("k1:a,k1:b")
	assertLeaseErrorCode(t, err, ReasonLeaseInvalid)
}

func TestLeaseSchemaV1JSONIsValidAndContainsRequiredFields(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(LeaseSchemaV1JSON), &doc); err != nil {
		t.Fatalf("schema json must be valid: %v", err)
	}
	requiredAny, ok := doc["required"].([]any)
	if !ok {
		t.Fatalf("schema.required must be an array, got %#v", doc["required"])
	}
	required := make([]string, 0, len(requiredAny))
	for _, item := range requiredAny {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("schema.required item must be string, got %#v", item)
		}
		required = append(required, value)
	}
	for _, field := range []string{"version", "lease_id", "run_id", "issued_at", "expires_at", "signature"} {
		if !slices.Contains(required, field) {
			t.Fatalf("schema.required missing %q: %#v", field, required)
		}
	}
}

func TestLeaseErrorMethods(t *testing.T) {
	var nilErr *LeaseError
	if got := nilErr.Error(); got != "" {
		t.Fatalf("expected nil lease error string to be empty, got %q", got)
	}
	if got := nilErr.Unwrap(); got != nil {
		t.Fatalf("expected nil lease error unwrap to be nil, got %v", got)
	}

	cause := errors.New("boom")
	err := &LeaseError{Cause: cause}
	if got := err.Error(); got != "boom" {
		t.Fatalf("expected cause-backed lease error string, got %q", got)
	}
	if got := err.Unwrap(); !errors.Is(got, cause) {
		t.Fatalf("expected unwrap to return original cause, got %v", got)
	}
}

func TestNormalizeNetworkRulesSortsByPortWithinSameHost(t *testing.T) {
	rules, err := normalizeNetworkRules([]NetworkRule{
		{Host: "example.com", Port: 8443, Scheme: "https"},
		{Host: "example.com", Port: 443, Scheme: "https"},
		{Host: "example.com", Port: 9443, Scheme: "https"},
	})
	if err != nil {
		t.Fatalf("normalize network rules: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected three normalized rules, got %#v", rules)
	}
	if rules[0].Port != 443 || rules[1].Port != 8443 || rules[2].Port != 9443 {
		t.Fatalf("expected port-sort order 443/8443/9443, got %#v", rules)
	}
}

func assertLeaseErrorCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected lease error code %q, got nil", wantCode)
	}
	var leaseErr *LeaseError
	if !errors.As(err, &leaseErr) {
		t.Fatalf("expected lease error type, got %T: %v", err, err)
	}
	if leaseErr.Code != wantCode {
		t.Fatalf("unexpected lease error code: got=%q want=%q err=%v", leaseErr.Code, wantCode, err)
	}
}
