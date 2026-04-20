package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sandboxpkg "bytemind/internal/sandbox"
)

type workerRunRequest struct {
	Resolved  ResolvedTool
	RawArgs   json.RawMessage
	Execution *ExecutionContext
}

type executorWorker interface {
	Run(context.Context, workerRunRequest) (string, error)
}

type inProcessWorker struct {
	normalizer OutputNormalizer
}

func (w inProcessWorker) Run(ctx context.Context, req workerRunRequest) (string, error) {
	if err := w.enforcePolicy(ctx, req); err != nil {
		return "", err
	}
	normalizer := w.normalizer
	if normalizer == nil {
		normalizer = maxCharsOutputNormalizer{}
	}
	runCtx, cancel := context.WithTimeout(ctx, executionTimeout(req.RawArgs, req.Resolved.Spec))
	defer cancel()

	output, err := req.Resolved.Tool.Run(runCtx, req.RawArgs, req.Execution)
	if err != nil {
		return "", normalizeToolError(err)
	}
	return normalizer.Normalize(output, req.Resolved), nil
}

func (w inProcessWorker) enforcePolicy(ctx context.Context, req workerRunRequest) error {
	execCtx := req.Execution
	if execCtx == nil || !execCtx.SandboxEnabled {
		return nil
	}
	lease, keyring, err := resolvePolicyLease(execCtx)
	if err != nil {
		return NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
	}

	runtimeReq, err := runtimeRequestForTool(req.Resolved.Definition.Function.Name, req.RawArgs)
	if err != nil {
		return NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
	}

	broker := sandboxpkg.NewPolicyBroker()
	decision, err := broker.Decide(ctx, sandboxpkg.DecisionInput{
		Lease:   lease,
		Keyring: keyring,
		Now:     time.Now().UTC(),
		Static: sandboxpkg.StaticPolicy{
			ApprovalPolicy: execCtx.ApprovalPolicy,
		},
		Mode: sandboxpkg.ModeContext{
			ApprovalMode:             execCtx.approvalMode(),
			AwayPolicy:               execCtx.awayPolicy(),
			ApprovalChannelAvailable: execCtx.Approval != nil || execCtx.Stdin != nil,
		},
		Request: runtimeReq,
	})
	if err != nil {
		return NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
	}

	switch decision.Decision {
	case sandboxpkg.DecisionAllow:
		return nil
	case sandboxpkg.DecisionDeny:
		return NewToolExecError(ToolErrorPermissionDenied, formatBrokerDeniedMessage(req.Resolved.Definition.Function.Name, decision), false, nil)
	case sandboxpkg.DecisionEscalate:
		return escalateWorkerApproval(req.Resolved.Definition.Function.Name, decision, execCtx)
	default:
		return NewToolExecError(ToolErrorPermissionDenied, "sandbox policy returned unknown decision", false, nil)
	}
}

func resolvePolicyLease(execCtx *ExecutionContext) (sandboxpkg.Lease, map[string][]byte, error) {
	if execCtx == nil {
		return sandboxpkg.Lease{}, nil, fmt.Errorf("sandbox execution context is missing")
	}
	if execCtx.Lease != nil {
		lease := *execCtx.Lease
		keyring := cloneKeyring(execCtx.LeaseKeyring)
		if len(keyring) == 0 {
			return sandboxpkg.Lease{}, nil, fmt.Errorf("sandbox lease keyring is unavailable")
		}
		return lease, keyring, nil
	}

	now := time.Now().UTC()
	leaseID := strings.TrimSpace(execCtx.LeaseID)
	if leaseID == "" {
		leaseID = fmt.Sprintf("inline-lease-%d", now.UnixNano())
	}
	runID := strings.TrimSpace(execCtx.RunID)
	if runID == "" {
		runID = "inline-run"
	}
	readRoots := normalizeWorkerRoots(execCtx.FSRead, execCtx.Workspace, execCtx.WritableRoots)
	writeRoots := normalizeWorkerRoots(execCtx.FSWrite, execCtx.Workspace, execCtx.WritableRoots)
	lease := sandboxpkg.Lease{
		Version:          sandboxpkg.LeaseVersionV1,
		LeaseID:          leaseID,
		RunID:            runID,
		Scope:            sandboxpkg.LeaseScopeRun,
		IssuedAt:         now.Add(-1 * time.Minute),
		ExpiresAt:        now.Add(1 * time.Hour),
		KID:              "inline",
		ApprovalMode:     execCtx.approvalMode(),
		AwayPolicy:       execCtx.awayPolicy(),
		FSRead:           append([]string(nil), readRoots...),
		FSWrite:          append([]string(nil), writeRoots...),
		ExecAllowlist:    append([]sandboxpkg.ExecRule(nil), execCtx.ExecAllowlist...),
		NetworkAllowlist: append([]sandboxpkg.NetworkRule(nil), execCtx.NetworkAllowlist...),
	}
	keyring := map[string][]byte{"inline": []byte("inline-sandbox-key")}
	signedLease, err := sandboxpkg.SignLease(lease, keyring["inline"])
	if err != nil {
		return sandboxpkg.Lease{}, nil, err
	}
	return signedLease, keyring, nil
}

func normalizeWorkerRoots(preferred []string, workspace string, writable []string) []string {
	if len(preferred) > 0 {
		roots := make([]string, 0, len(preferred))
		for _, root := range preferred {
			root = strings.TrimSpace(root)
			if root != "" {
				roots = append(roots, root)
			}
		}
		if len(roots) > 0 {
			return roots
		}
	}
	roots := make([]string, 0, len(writable)+1)
	if strings.TrimSpace(workspace) != "" {
		roots = append(roots, workspace)
	}
	for _, root := range writable {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		roots = append(roots, root)
	}
	return roots
}

func runtimeRequestForTool(toolName string, raw json.RawMessage) (sandboxpkg.RuntimeRequest, error) {
	switch strings.TrimSpace(toolName) {
	case "run_shell":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		parts := strings.Fields(strings.TrimSpace(args.Command))
		if len(parts) == 0 {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("command cannot be empty")
		}
		return sandboxpkg.RuntimeRequest{
			ToolName: parts[0],
			Command:  parts[0],
			Args:     parts[1:],
		}, nil
	case "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		if strings.TrimSpace(args.Path) == "" {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("path cannot be empty")
		}
		return sandboxpkg.RuntimeRequest{
			ToolName:   "read_file",
			FilePath:   args.Path,
			FileAccess: sandboxpkg.FileAccessRead,
		}, nil
	case "write_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		if strings.TrimSpace(args.Path) == "" {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("path cannot be empty")
		}
		return sandboxpkg.RuntimeRequest{
			ToolName:   "write_file",
			FilePath:   args.Path,
			FileAccess: sandboxpkg.FileAccessWrite,
		}, nil
	default:
		return sandboxpkg.RuntimeRequest{
			ToolName: strings.TrimSpace(toolName),
		}, nil
	}
}

func formatBrokerDeniedMessage(toolName string, decision sandboxpkg.DecisionResult) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "unknown_tool"
	}
	if strings.TrimSpace(decision.ReasonCode) == "" {
		return fmt.Sprintf("tool %q was blocked by sandbox policy", toolName)
	}
	message := strings.TrimSpace(decision.Message)
	if message == "" {
		return fmt.Sprintf("%s: tool %q was blocked by sandbox policy", decision.ReasonCode, toolName)
	}
	return fmt.Sprintf("%s: %s", decision.ReasonCode, message)
}

func escalateWorkerApproval(toolName string, decision sandboxpkg.DecisionResult, execCtx *ExecutionContext) error {
	if execCtx == nil {
		return NewToolExecError(ToolErrorPermissionDenied, formatBrokerDeniedMessage(toolName, decision), false, nil)
	}
	if execCtx.isAwayMode() {
		return NewToolExecError(ToolErrorPermissionDenied, formatBrokerDeniedMessage(toolName, decision), false, nil)
	}
	if execCtx.Approval == nil {
		return approvalChannelUnavailableError("tool", toolName)
	}
	approved, err := execCtx.Approval(ApprovalRequest{
		Command: toolName,
		Reason:  decision.Message,
	})
	if err != nil {
		return NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
	}
	if !approved {
		return NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q was not run because approval was denied", toolName), false, nil)
	}
	return nil
}

func cloneKeyring(source map[string][]byte) map[string][]byte {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string][]byte, len(source))
	for kid, key := range source {
		kid = strings.TrimSpace(kid)
		if kid == "" {
			continue
		}
		out[kid] = append([]byte(nil), key...)
	}
	return out
}

func shouldRouteToWorker(toolName string, execCtx *ExecutionContext) bool {
	if execCtx == nil || !execCtx.SandboxEnabled {
		return false
	}
	switch toolName {
	case "run_shell", "read_file", "write_file":
		return true
	default:
		return false
	}
}
