package sandbox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	LeaseVersionV1 = "v1"
	LeaseScopeRun  = "run"
)

const (
	ReasonLeaseExpired          = "lease_expired"
	ReasonLeaseSignatureInvalid = "lease_signature_invalid"
	ReasonLeaseInvalid          = "lease_invalid"
)

type Lease struct {
	Version          string        `json:"version"`
	LeaseID          string        `json:"lease_id"`
	RunID            string        `json:"run_id"`
	Scope            string        `json:"scope"`
	IssuedAt         time.Time     `json:"issued_at"`
	ExpiresAt        time.Time     `json:"expires_at"`
	KID              string        `json:"kid"`
	ApprovalMode     string        `json:"approval_mode"`
	AwayPolicy       string        `json:"away_policy"`
	FSRead           []string      `json:"fs_read"`
	FSWrite          []string      `json:"fs_write"`
	ExecAllowlist    []ExecRule    `json:"exec_allowlist"`
	NetworkAllowlist []NetworkRule `json:"network_allowlist"`
	Signature        string        `json:"signature,omitempty"`
}

type ExecRule struct {
	Command     string   `json:"command"`
	ArgsPattern []string `json:"args_pattern"`
}

type NetworkRule struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Scheme string `json:"scheme"`
}

type LeaseError struct {
	Code    string
	Message string
	Cause   error
}

func (e *LeaseError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "lease validation failed"
}

func (e *LeaseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func ParseHMACKeyring(raw string) (map[string][]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: "lease keyring is empty"}
	}
	pairs := strings.Split(raw, ",")
	keyring := make(map[string][]byte, len(pairs))
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("invalid lease key pair %q", pair)}
		}
		kid := strings.TrimSpace(parts[0])
		secret := strings.TrimSpace(parts[1])
		if kid == "" || secret == "" {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("invalid lease key pair %q", pair)}
		}
		if _, exists := keyring[kid]; exists {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("duplicate lease key id %q", kid)}
		}
		keyring[kid] = []byte(secret)
	}
	if len(keyring) == 0 {
		return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: "lease keyring is empty"}
	}
	return keyring, nil
}

func CanonicalPayload(lease Lease) ([]byte, error) {
	normalized, err := normalizeLease(lease, false)
	if err != nil {
		return nil, err
	}
	normalized.Signature = ""
	return canonicalPayloadFromNormalized(normalized)
}

func SignLease(lease Lease, key []byte) (Lease, error) {
	if len(key) == 0 {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: "lease signing key is empty"}
	}
	normalized, err := normalizeLease(lease, false)
	if err != nil {
		return Lease{}, err
	}
	normalized.Signature = ""
	payload, err := canonicalPayloadFromNormalized(normalized)
	if err != nil {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: "failed to build canonical lease payload", Cause: err}
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	normalized.Signature = hex.EncodeToString(mac.Sum(nil))
	return normalized, nil
}

func ValidateLease(lease Lease, now time.Time) error {
	normalized, err := normalizeLease(lease, false)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	if !now.Before(normalized.ExpiresAt) {
		return &LeaseError{
			Code:    ReasonLeaseExpired,
			Message: fmt.Sprintf("lease %q expired at %s", normalized.LeaseID, normalized.ExpiresAt.Format(time.RFC3339)),
		}
	}
	return nil
}

func VerifySignedLease(lease Lease, keyring map[string][]byte, now time.Time) error {
	normalized, err := normalizeLease(lease, true)
	if err != nil {
		return err
	}
	if err := ValidateLease(normalized, now); err != nil {
		return err
	}
	if len(keyring) == 0 {
		return &LeaseError{Code: ReasonLeaseSignatureInvalid, Message: "lease keyring is empty"}
	}
	key, ok := keyring[normalized.KID]
	if !ok {
		return &LeaseError{
			Code:    ReasonLeaseSignatureInvalid,
			Message: fmt.Sprintf("lease signing key %q was not found", normalized.KID),
		}
	}
	expectedLease, err := SignLease(normalized, key)
	if err != nil {
		return err
	}
	given, err := hex.DecodeString(normalized.Signature)
	if err != nil {
		return &LeaseError{
			Code:    ReasonLeaseSignatureInvalid,
			Message: "lease signature is not valid hex",
			Cause:   err,
		}
	}
	expected, err := hex.DecodeString(expectedLease.Signature)
	if err != nil {
		return &LeaseError{
			Code:    ReasonLeaseSignatureInvalid,
			Message: "failed to decode computed lease signature",
			Cause:   err,
		}
	}
	if !hmac.Equal(given, expected) {
		return &LeaseError{
			Code:    ReasonLeaseSignatureInvalid,
			Message: "lease signature verification failed",
		}
	}
	return nil
}

func normalizeLease(lease Lease, requireSignature bool) (Lease, error) {
	lease.Version = strings.TrimSpace(lease.Version)
	if lease.Version == "" {
		lease.Version = LeaseVersionV1
	}
	if lease.Version != LeaseVersionV1 {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("unsupported lease version %q", lease.Version)}
	}

	lease.Scope = strings.TrimSpace(lease.Scope)
	if lease.Scope == "" {
		lease.Scope = LeaseScopeRun
	}
	if lease.Scope != LeaseScopeRun {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("unsupported lease scope %q", lease.Scope)}
	}

	lease.LeaseID = strings.TrimSpace(lease.LeaseID)
	lease.RunID = strings.TrimSpace(lease.RunID)
	lease.KID = strings.TrimSpace(lease.KID)
	if lease.LeaseID == "" {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: "lease_id is required"}
	}
	if lease.RunID == "" {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: "run_id is required"}
	}
	if lease.KID == "" {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: "kid is required"}
	}

	if lease.IssuedAt.IsZero() {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: "issued_at is required"}
	}
	if lease.ExpiresAt.IsZero() {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: "expires_at is required"}
	}
	lease.IssuedAt = lease.IssuedAt.UTC().Truncate(time.Second)
	lease.ExpiresAt = lease.ExpiresAt.UTC().Truncate(time.Second)
	if !lease.ExpiresAt.After(lease.IssuedAt) {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: "expires_at must be later than issued_at"}
	}

	lease.ApprovalMode = strings.ToLower(strings.TrimSpace(lease.ApprovalMode))
	if lease.ApprovalMode == "" {
		lease.ApprovalMode = "interactive"
	}
	if lease.ApprovalMode != "interactive" && lease.ApprovalMode != "away" {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("invalid approval_mode %q", lease.ApprovalMode)}
	}
	lease.AwayPolicy = strings.ToLower(strings.TrimSpace(lease.AwayPolicy))
	if lease.AwayPolicy == "" {
		lease.AwayPolicy = "auto_deny_continue"
	}
	if lease.AwayPolicy != "auto_deny_continue" && lease.AwayPolicy != "fail_fast" {
		return Lease{}, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("invalid away_policy %q", lease.AwayPolicy)}
	}

	var err error
	lease.FSRead, err = normalizePaths(lease.FSRead)
	if err != nil {
		return Lease{}, err
	}
	lease.FSWrite, err = normalizePaths(lease.FSWrite)
	if err != nil {
		return Lease{}, err
	}
	lease.ExecAllowlist, err = normalizeExecRules(lease.ExecAllowlist)
	if err != nil {
		return Lease{}, err
	}
	lease.NetworkAllowlist, err = normalizeNetworkRules(lease.NetworkAllowlist)
	if err != nil {
		return Lease{}, err
	}
	lease.Signature = strings.ToLower(strings.TrimSpace(lease.Signature))
	if requireSignature && lease.Signature == "" {
		return Lease{}, &LeaseError{
			Code:    ReasonLeaseSignatureInvalid,
			Message: "lease signature is required",
		}
	}
	return lease, nil
}

func canonicalPayloadFromNormalized(lease Lease) ([]byte, error) {
	payload := map[string]any{
		"approval_mode":     lease.ApprovalMode,
		"away_policy":       lease.AwayPolicy,
		"exec_allowlist":    lease.ExecAllowlist,
		"expires_at":        lease.ExpiresAt.Format(time.RFC3339),
		"fs_read":           lease.FSRead,
		"fs_write":          lease.FSWrite,
		"issued_at":         lease.IssuedAt.Format(time.RFC3339),
		"kid":               lease.KID,
		"lease_id":          lease.LeaseID,
		"network_allowlist": lease.NetworkAllowlist,
		"run_id":            lease.RunID,
		"scope":             lease.Scope,
		"version":           lease.Version,
	}
	return json.Marshal(payload)
}

func normalizePaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("invalid lease path %q", path), Cause: err}
		}
		abs = filepath.Clean(abs)
		resolved, err := filepath.EvalSymlinks(abs)
		if err == nil {
			abs = filepath.Clean(resolved)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: fmt.Sprintf("failed to evaluate lease path %q", path), Cause: err}
		}
		key := pathSortKey(abs)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, abs)
	}
	sort.Slice(out, func(i, j int) bool {
		return pathSortKey(out[i]) < pathSortKey(out[j])
	})
	return out, nil
}

func pathSortKey(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func normalizeExecRules(rules []ExecRule) ([]ExecRule, error) {
	if len(rules) == 0 {
		return []ExecRule{}, nil
	}
	out := make([]ExecRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		commandTokens := strings.Fields(strings.TrimSpace(rule.Command))
		if len(commandTokens) == 0 {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: "exec_allowlist.command cannot be empty"}
		}
		command := commandTokens[0]
		patternInputs := make([]string, 0, len(commandTokens)-1+len(rule.ArgsPattern))
		if len(commandTokens) > 1 {
			patternInputs = append(patternInputs, commandTokens[1:]...)
		}
		patternInputs = append(patternInputs, rule.ArgsPattern...)
		patterns := normalizeStringList(patternInputs)
		key := strings.ToLower(command) + "\x00" + strings.Join(patterns, "\x00")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ExecRule{
			Command:     command,
			ArgsPattern: patterns,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i].Command)
		right := strings.ToLower(out[j].Command)
		if left != right {
			return left < right
		}
		return strings.Join(out[i].ArgsPattern, "\x00") < strings.Join(out[j].ArgsPattern, "\x00")
	})
	return out, nil
}

func normalizeNetworkRules(rules []NetworkRule) ([]NetworkRule, error) {
	if len(rules) == 0 {
		return []NetworkRule{}, nil
	}
	out := make([]NetworkRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		host := strings.ToLower(strings.TrimSpace(rule.Host))
		scheme := strings.ToLower(strings.TrimSpace(rule.Scheme))
		if host == "" {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: "network_allowlist.host cannot be empty"}
		}
		if scheme == "" {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: "network_allowlist.scheme cannot be empty"}
		}
		if rule.Port < 1 || rule.Port > 65535 {
			return nil, &LeaseError{Code: ReasonLeaseInvalid, Message: "network_allowlist.port must be between 1 and 65535"}
		}
		key := host + "\x00" + strconv.Itoa(rule.Port) + "\x00" + scheme
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, NetworkRule{
			Host:   host,
			Port:   rule.Port,
			Scheme: scheme,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Scheme < out[j].Scheme
	})
	return out, nil
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
