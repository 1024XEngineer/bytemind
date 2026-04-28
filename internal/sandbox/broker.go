package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ReasonFSOutOfScope               = "fs_out_of_scope"
	ReasonExecNotAllowed             = "exec_not_allowed"
	ReasonNetworkNotAllowed          = "network_not_allowed"
	ReasonApprovalRequired           = "approval_required"
	ReasonApprovalChannelUnavailable = "approval_channel_unavailable"
	ReasonDeniedDependency           = "denied_dependency"
)

type Decision string

const (
	DecisionAllow    Decision = "allow"
	DecisionDeny     Decision = "deny"
	DecisionEscalate Decision = "escalate"
)

type FileAccess string

const (
	FileAccessRead  FileAccess = "read"
	FileAccessWrite FileAccess = "write"
)

type StaticPolicy struct {
	ApprovalPolicy string
}

type ModeContext struct {
	ApprovalMode             string
	AwayPolicy               string
	ApprovalChannelAvailable bool
}

type RuntimeRequest struct {
	ToolName         string
	FilePath         string
	FileAccess       FileAccess
	Command          string
	Args             []string
	Network          NetworkRule
	RequiresApproval bool
}

type DecisionInput struct {
	Lease   Lease
	Keyring map[string][]byte
	Now     time.Time
	Static  StaticPolicy
	Mode    ModeContext
	Request RuntimeRequest
}

type DecisionResult struct {
	Decision   Decision
	ReasonCode string
	Message    string
}

type PolicyBroker interface {
	Decide(context.Context, DecisionInput) (DecisionResult, error)
}

type defaultPolicyBroker struct{}

func NewPolicyBroker() PolicyBroker {
	return defaultPolicyBroker{}
}

func (defaultPolicyBroker) Decide(_ context.Context, input DecisionInput) (DecisionResult, error) {
	if err := VerifySignedLease(input.Lease, input.Keyring, input.Now); err != nil {
		return leaseErrorDecision(err), nil
	}

	request := normalizeRuntimeRequest(input.Request)

	if request.FileAccess != "" {
		roots := input.Lease.FSRead
		if request.FileAccess == FileAccessWrite {
			roots = input.Lease.FSWrite
		}
		if !pathWithinAnyRoot(request.FilePath, roots) {
			return boundaryDecision(
				input,
				ReasonFSOutOfScope,
				"requested file path is outside lease scope",
				true,
			), nil
		}
	}

	if request.Command != "" && !commandAllowedByLease(request.Command, request.Args, input.Lease.ExecAllowlist) {
		return boundaryDecision(
			input,
			ReasonExecNotAllowed,
			"command is not allowed by lease",
			true,
		), nil
	}

	if hasNetworkTarget(request.Network) && !networkAllowedByLease(request.Network, input.Lease.NetworkAllowlist) {
		return boundaryDecision(
			input,
			ReasonNetworkNotAllowed,
			"network target is not allowed by lease",
			true,
		), nil
	}

	approvalPolicy := normalizeApprovalPolicy(input.Static.ApprovalPolicy)
	requiresApproval := request.RequiresApproval || approvalPolicy == "always"
	if requiresApproval {
		mode := normalizeApprovalMode(input.Mode.ApprovalMode)
		if mode == "away" {
			return DecisionResult{
				Decision:   DecisionDeny,
				ReasonCode: ReasonApprovalRequired,
				Message:    "approval is unavailable in away mode",
			}, nil
		}
		if approvalPolicy == "never" {
			return DecisionResult{
				Decision:   DecisionDeny,
				ReasonCode: ReasonApprovalRequired,
				Message:    "approval policy prevents escalation",
			}, nil
		}
		if !input.Mode.ApprovalChannelAvailable {
			return DecisionResult{
				Decision:   DecisionDeny,
				ReasonCode: ReasonApprovalChannelUnavailable,
				Message:    "approval channel is unavailable",
			}, nil
		}
		return DecisionResult{
			Decision:   DecisionEscalate,
			ReasonCode: ReasonApprovalRequired,
			Message:    "operation requires explicit approval",
		}, nil
	}

	return DecisionResult{
		Decision:   DecisionAllow,
		ReasonCode: "",
		Message:    "operation allowed by lease",
	}, nil
}

func boundaryDecision(input DecisionInput, boundaryReasonCode, boundaryMessage string, canEscalate bool) DecisionResult {
	if strings.TrimSpace(input.Mode.ApprovalMode) == "" {
		return DecisionResult{
			Decision:   DecisionDeny,
			ReasonCode: boundaryReasonCode,
			Message:    boundaryMessage,
		}
	}
	approvalPolicy := normalizeApprovalPolicy(input.Static.ApprovalPolicy)
	approvalMode := normalizeApprovalMode(input.Mode.ApprovalMode)
	if canEscalate && approvalPolicy != "never" && approvalMode == "interactive" {
		if input.Mode.ApprovalChannelAvailable {
			return DecisionResult{
				Decision:   DecisionEscalate,
				ReasonCode: ReasonApprovalRequired,
				Message:    "operation is outside lease scope and requires explicit approval",
			}
		}
		return DecisionResult{
			Decision:   DecisionDeny,
			ReasonCode: ReasonApprovalChannelUnavailable,
			Message:    "approval channel is unavailable",
		}
	}
	return DecisionResult{
		Decision:   DecisionDeny,
		ReasonCode: boundaryReasonCode,
		Message:    boundaryMessage,
	}
}

func leaseErrorDecision(err error) DecisionResult {
	leaseErr, ok := err.(*LeaseError)
	if !ok || leaseErr == nil {
		return DecisionResult{
			Decision:   DecisionDeny,
			ReasonCode: ReasonLeaseInvalid,
			Message:    strings.TrimSpace(err.Error()),
		}
	}
	return DecisionResult{
		Decision:   DecisionDeny,
		ReasonCode: leaseErr.Code,
		Message:    strings.TrimSpace(leaseErr.Error()),
	}
}

func normalizeRuntimeRequest(request RuntimeRequest) RuntimeRequest {
	request.ToolName = strings.TrimSpace(request.ToolName)
	request.FilePath = strings.TrimSpace(request.FilePath)
	request.Command = strings.TrimSpace(request.Command)
	request.Network.Host = strings.ToLower(strings.TrimSpace(request.Network.Host))
	request.Network.Scheme = strings.ToLower(strings.TrimSpace(request.Network.Scheme))
	request.Args = normalizeStringList(request.Args)
	switch request.FileAccess {
	case FileAccessRead, FileAccessWrite:
	default:
		request.FileAccess = ""
	}
	return request
}

func pathWithinAnyRoot(path string, roots []string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	candidate, ok := normalizeCandidatePath(path)
	if !ok {
		return false
	}
	for _, root := range roots {
		rootCandidate, ok := normalizeCandidatePath(root)
		if !ok {
			continue
		}
		rel, err := filepath.Rel(rootCandidate, candidate)
		if err != nil {
			continue
		}
		if rel == "." {
			return true
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return true
	}
	return false
}

func normalizeCandidatePath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	abs = filepath.Clean(abs)
	canonical, err := canonicalPathForBoundary(abs)
	if err != nil {
		return "", false
	}
	return canonical, true
}

func canonicalPathForBoundary(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	existingAncestor, missingSegments, err := splitMissingPathSegments(path)
	if err != nil {
		return "", err
	}
	resolvedAncestor, err := filepath.EvalSymlinks(existingAncestor)
	if err != nil {
		return "", err
	}
	canonical := filepath.Clean(resolvedAncestor)
	for _, segment := range missingSegments {
		canonical = filepath.Join(canonical, segment)
	}
	return canonical, nil
}

func splitMissingPathSegments(path string) (existingAncestor string, missingSegments []string, err error) {
	current := filepath.Clean(path)
	missing := make([]string, 0, 4)
	for {
		_, statErr := os.Lstat(current)
		if statErr == nil {
			for i, j := 0, len(missing)-1; i < j; i, j = i+1, j-1 {
				missing[i], missing[j] = missing[j], missing[i]
			}
			return current, missing, nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", nil, statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil, statErr
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func commandAllowedByLease(command string, args []string, rules []ExecRule) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	args = normalizeStringList(args)
	for _, rule := range rules {
		if !strings.EqualFold(strings.TrimSpace(rule.Command), command) {
			continue
		}
		pattern := normalizeStringList(rule.ArgsPattern)
		if len(pattern) == 0 {
			return true
		}
		if len(pattern) != len(args) {
			continue
		}
		matched := true
		for i := range pattern {
			if pattern[i] != args[i] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func hasNetworkTarget(rule NetworkRule) bool {
	return strings.TrimSpace(rule.Host) != "" || strings.TrimSpace(rule.Scheme) != "" || rule.Port != 0
}

func networkAllowedByLease(target NetworkRule, rules []NetworkRule) bool {
	target.Host = strings.ToLower(strings.TrimSpace(target.Host))
	target.Scheme = strings.ToLower(strings.TrimSpace(target.Scheme))
	if target.Host == "" || target.Scheme == "" || target.Port < 1 || target.Port > 65535 {
		return false
	}
	for _, rule := range rules {
		if target.Host == strings.ToLower(strings.TrimSpace(rule.Host)) &&
			target.Port == rule.Port &&
			target.Scheme == strings.ToLower(strings.TrimSpace(rule.Scheme)) {
			return true
		}
	}
	return false
}

func normalizeApprovalPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "on-request":
		return "on-request"
	case "always":
		return "always"
	case "never":
		return "never"
	default:
		return "on-request"
	}
}

func normalizeApprovalMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "interactive":
		return "interactive"
	case "away":
		return "away"
	default:
		return "interactive"
	}
}
