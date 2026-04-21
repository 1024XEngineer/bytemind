package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	sandboxpkg "bytemind/internal/sandbox"
)

const (
	sandboxWorkerSubcommand = "worker"
	sandboxWorkerStdioFlag  = "--sandbox-stdio"
	sandboxWorkerEnvKey     = "BYTEMIND_SANDBOX_WORKER"
	sandboxWorkerEnvValue   = "1"
	sandboxWorkerProtocolV1 = "v1"
)

type workerProcessInvoker interface {
	Invoke(context.Context, workerRPCRequest) (workerRPCResponse, string, error)
}

type subprocessWorker struct {
	fallback inProcessWorker
	invoker  workerProcessInvoker
}

type osExecWorkerInvoker struct {
	executablePath string
}

type workerRPCRequest struct {
	Version   string                    `json:"version"`
	ToolName  string                    `json:"tool_name"`
	RawArgs   json.RawMessage           `json:"raw_args"`
	Execution workerRPCExecutionContext `json:"execution"`
}

type workerRPCExecutionContext struct {
	Workspace                 string                   `json:"workspace"`
	WritableRoots             []string                 `json:"writable_roots"`
	ApprovalPolicy            string                   `json:"approval_policy"`
	ApprovalMode              string                   `json:"approval_mode"`
	AwayPolicy                string                   `json:"away_policy"`
	SandboxEnabled            bool                     `json:"sandbox_enabled"`
	SkipShellApproval         bool                     `json:"skip_shell_approval"`
	SandboxEscalationApproved bool                     `json:"sandbox_escalation_approved"`
	LeaseID                   string                   `json:"lease_id"`
	RunID                     string                   `json:"run_id"`
	FSRead                    []string                 `json:"fs_read"`
	FSWrite                   []string                 `json:"fs_write"`
	ExecAllowlist             []sandboxpkg.ExecRule    `json:"exec_allowlist"`
	NetworkAllowlist          []sandboxpkg.NetworkRule `json:"network_allowlist"`
	Lease                     *sandboxpkg.Lease        `json:"lease,omitempty"`
	LeaseKeyring              map[string][]byte        `json:"lease_keyring,omitempty"`
}

type workerRPCResponse struct {
	Output string          `json:"output,omitempty"`
	Error  *workerRPCError `json:"error,omitempty"`
}

type workerRPCError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func newDefaultExecutorWorker(registry *Registry, normalizer OutputNormalizer) executorWorker {
	fallback := inProcessWorker{registry: registry, normalizer: normalizer}
	if strings.EqualFold(strings.TrimSpace(os.Getenv(sandboxWorkerEnvKey)), sandboxWorkerEnvValue) {
		return fallback
	}
	executablePath, err := os.Executable()
	if err != nil || strings.TrimSpace(executablePath) == "" {
		return subprocessWorker{
			fallback: fallback,
			invoker:  nil,
		}
	}
	return subprocessWorker{
		fallback: fallback,
		invoker:  osExecWorkerInvoker{executablePath: executablePath},
	}
}

func (w subprocessWorker) Run(ctx context.Context, req workerRunRequest) (string, error) {
	if !shouldUseSubprocessWorker(req.Execution) {
		return w.fallback.Run(ctx, req)
	}
	if w.invoker == nil {
		return "", NewToolExecError(ToolErrorInternal, "sandbox worker subprocess is unavailable", true, nil)
	}
	normalizedReq := req
	if normalizedReq.Execution == nil {
		normalizedReq.Execution = &ExecutionContext{}
	} else {
		execCopy := *normalizedReq.Execution
		normalizedReq.Execution = &execCopy
	}
	if err := w.preApproveForSubprocess(ctx, &normalizedReq); err != nil {
		return "", err
	}

	response, stderrOutput, err := w.invoker.Invoke(ctx, buildWorkerRPCRequest(normalizedReq))
	if err != nil {
		details := strings.TrimSpace(stderrOutput)
		if details == "" {
			details = strings.TrimSpace(err.Error())
		}
		return "", NewToolExecError(ToolErrorInternal, "sandbox worker process failed: "+details, true, err)
	}
	if response.Error != nil {
		return "", response.Error.toToolExecError()
	}
	return response.Output, nil
}

func shouldUseSubprocessWorker(execCtx *ExecutionContext) bool {
	if execCtx == nil || !execCtx.SandboxEnabled {
		return false
	}
	return true
}

func (i osExecWorkerInvoker) Invoke(ctx context.Context, req workerRPCRequest) (workerRPCResponse, string, error) {
	executablePath := strings.TrimSpace(i.executablePath)
	if executablePath == "" {
		return workerRPCResponse{}, "", errors.New("worker executable path is empty")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return workerRPCResponse{}, "", err
	}

	cmd := exec.CommandContext(ctx, executablePath, sandboxWorkerSubcommand, sandboxWorkerStdioFlag)
	workspace := strings.TrimSpace(req.Execution.Workspace)
	if workspace != "" {
		cmd.Dir = workspace
	}
	cmd.Env = buildWorkerProcessEnv(os.Environ())
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))

	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	cmd.Stderr = &stderrBuffer

	if err := cmd.Run(); err != nil {
		return workerRPCResponse{}, stderrBuffer.String(), err
	}

	var response workerRPCResponse
	if err := json.NewDecoder(bytes.NewReader(stdoutBuffer.Bytes())).Decode(&response); err != nil {
		return workerRPCResponse{}, stderrBuffer.String(), fmt.Errorf("decode worker response: %w", err)
	}
	return response, stderrBuffer.String(), nil
}

func RunWorkerProcess(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	return runWorkerProcessWithRegistry(ctx, stdin, stdout, DefaultRegistry())
}

func runWorkerProcessWithRegistry(ctx context.Context, stdin io.Reader, stdout io.Writer, registry *Registry) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if stdin == nil {
		return errors.New("worker stdin is required")
	}
	if stdout == nil {
		return errors.New("worker stdout is required")
	}
	if registry == nil {
		return errors.New("worker registry is required")
	}

	var request workerRPCRequest
	if err := json.NewDecoder(stdin).Decode(&request); err != nil {
		return err
	}
	if strings.TrimSpace(request.Version) != sandboxWorkerProtocolV1 {
		return fmt.Errorf("unsupported worker protocol version %q", strings.TrimSpace(request.Version))
	}
	if len(bytes.TrimSpace(request.RawArgs)) == 0 {
		request.RawArgs = json.RawMessage(`{}`)
	}

	worker := inProcessWorker{
		registry:   registry,
		normalizer: maxCharsOutputNormalizer{},
	}
	output, runErr := worker.Run(ctx, workerRunRequest{
		ToolName:  request.ToolName,
		RawArgs:   request.RawArgs,
		Execution: decodeWorkerExecutionContext(request.Execution),
	})
	response := workerRPCResponse{Output: output}
	if runErr != nil {
		response.Error = toWorkerRPCError(runErr)
		response.Output = ""
	}
	return json.NewEncoder(stdout).Encode(response)
}

func buildWorkerRPCRequest(req workerRunRequest) workerRPCRequest {
	return workerRPCRequest{
		Version:   sandboxWorkerProtocolV1,
		ToolName:  strings.TrimSpace(req.ToolName),
		RawArgs:   append(json.RawMessage(nil), req.RawArgs...),
		Execution: encodeWorkerExecutionContext(req.Execution),
	}
}

func encodeWorkerExecutionContext(execCtx *ExecutionContext) workerRPCExecutionContext {
	if execCtx == nil {
		return workerRPCExecutionContext{}
	}
	return workerRPCExecutionContext{
		Workspace:                 execCtx.Workspace,
		WritableRoots:             append([]string(nil), execCtx.WritableRoots...),
		ApprovalPolicy:            execCtx.ApprovalPolicy,
		ApprovalMode:              execCtx.ApprovalMode,
		AwayPolicy:                execCtx.AwayPolicy,
		SandboxEnabled:            execCtx.SandboxEnabled,
		SkipShellApproval:         execCtx.SkipShellApproval,
		SandboxEscalationApproved: execCtx.SandboxEscalationApproved,
		LeaseID:                   execCtx.LeaseID,
		RunID:                     execCtx.RunID,
		FSRead:                    append([]string(nil), execCtx.FSRead...),
		FSWrite:                   append([]string(nil), execCtx.FSWrite...),
		ExecAllowlist:             append([]sandboxpkg.ExecRule(nil), execCtx.ExecAllowlist...),
		NetworkAllowlist:          append([]sandboxpkg.NetworkRule(nil), execCtx.NetworkAllowlist...),
		Lease:                     cloneLease(execCtx.Lease),
		LeaseKeyring:              cloneKeyring(execCtx.LeaseKeyring),
	}
}

func decodeWorkerExecutionContext(payload workerRPCExecutionContext) *ExecutionContext {
	return &ExecutionContext{
		Workspace:                 payload.Workspace,
		WritableRoots:             append([]string(nil), payload.WritableRoots...),
		ApprovalPolicy:            payload.ApprovalPolicy,
		ApprovalMode:              payload.ApprovalMode,
		AwayPolicy:                payload.AwayPolicy,
		SandboxEnabled:            payload.SandboxEnabled,
		SkipShellApproval:         payload.SkipShellApproval,
		SandboxEscalationApproved: payload.SandboxEscalationApproved,
		LeaseID:                   payload.LeaseID,
		RunID:                     payload.RunID,
		FSRead:                    append([]string(nil), payload.FSRead...),
		FSWrite:                   append([]string(nil), payload.FSWrite...),
		ExecAllowlist:             append([]sandboxpkg.ExecRule(nil), payload.ExecAllowlist...),
		NetworkAllowlist:          append([]sandboxpkg.NetworkRule(nil), payload.NetworkAllowlist...),
		Lease:                     cloneLease(payload.Lease),
		LeaseKeyring:              cloneKeyring(payload.LeaseKeyring),
	}
}

func cloneLease(lease *sandboxpkg.Lease) *sandboxpkg.Lease {
	if lease == nil {
		return nil
	}
	cloned := *lease
	cloned.FSRead = append([]string(nil), lease.FSRead...)
	cloned.FSWrite = append([]string(nil), lease.FSWrite...)
	cloned.ExecAllowlist = append([]sandboxpkg.ExecRule(nil), lease.ExecAllowlist...)
	cloned.NetworkAllowlist = append([]sandboxpkg.NetworkRule(nil), lease.NetworkAllowlist...)
	return &cloned
}

func toWorkerRPCError(err error) *workerRPCError {
	if err == nil {
		return nil
	}
	if execErr, ok := AsToolExecError(err); ok {
		return &workerRPCError{
			Code:      string(execErr.Code),
			Message:   execErr.Message,
			Retryable: execErr.Retryable,
		}
	}
	return &workerRPCError{
		Code:      string(ToolErrorToolFailed),
		Message:   strings.TrimSpace(err.Error()),
		Retryable: false,
	}
}

func (e *workerRPCError) toToolExecError() error {
	if e == nil {
		return NewToolExecError(ToolErrorInternal, "sandbox worker returned an empty error payload", false, nil)
	}
	code := normalizeWorkerRPCErrorCode(e.Code)
	return NewToolExecError(code, strings.TrimSpace(e.Message), e.Retryable, nil)
}

func normalizeWorkerRPCErrorCode(raw string) ToolErrorCode {
	switch ToolErrorCode(strings.TrimSpace(raw)) {
	case ToolErrorInvalidArgs, ToolErrorPermissionDenied, ToolErrorTimeout, ToolErrorToolFailed, ToolErrorInternal:
		return ToolErrorCode(strings.TrimSpace(raw))
	default:
		return ToolErrorInternal
	}
}

func (w subprocessWorker) preApproveForSubprocess(ctx context.Context, req *workerRunRequest) error {
	if req == nil || req.Execution == nil || !req.Execution.SandboxEnabled {
		return nil
	}
	decision, err := workerSandboxDecision(ctx, req.ToolName, req.RawArgs, req.Execution)
	if err != nil {
		return err
	}
	switch decision.Decision {
	case sandboxpkg.DecisionAllow:
		// continue
	case sandboxpkg.DecisionDeny:
		return NewToolExecError(ToolErrorPermissionDenied, formatBrokerDeniedMessage(req.ToolName, decision), false, nil)
	case sandboxpkg.DecisionEscalate:
		if err := escalateWorkerApproval(req.ToolName, decision, req.Execution); err != nil {
			return err
		}
		req.Execution.SandboxEscalationApproved = true
	default:
		return NewToolExecError(ToolErrorPermissionDenied, "sandbox policy returned unknown decision", false, nil)
	}

	if strings.TrimSpace(req.ToolName) == "run_shell" {
		command, err := workerRunShellCommand(req.RawArgs)
		if err != nil {
			return NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
		}
		if err := requireApproval(command, req.Execution); err != nil {
			return normalizeToolError(err)
		}
		req.Execution.SkipShellApproval = true
	}

	return nil
}

func workerRunShellCommand(raw json.RawMessage) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	command := strings.TrimSpace(args.Command)
	if command == "" {
		return "", errors.New("command is required")
	}
	return command, nil
}

func buildWorkerProcessEnv(base []string) []string {
	if len(base) == 0 {
		return []string{sandboxWorkerEnvKey + "=" + sandboxWorkerEnvValue}
	}
	trimmed := make([]string, 0, len(base)+1)
	for _, kv := range base {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		upperName := strings.ToUpper(name)
		if upperName == "BYTEMIND_API_KEY" || upperName == "BYTEMIND_PROVIDER_API_KEY" {
			continue
		}
		if upperName == sandboxWorkerEnvKey {
			continue
		}
		trimmed = append(trimmed, kv)
	}
	trimmed = append(trimmed, sandboxWorkerEnvKey+"="+sandboxWorkerEnvValue)
	return trimmed
}
