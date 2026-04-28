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
	"runtime"
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
	lookPath       func(string) (string, error)
	goos           string
}

type workerLaunchError struct {
	cause error
}

func (e workerLaunchError) Error() string {
	if e.cause == nil {
		return "worker launch failed"
	}
	return e.cause.Error()
}

func (e workerLaunchError) Unwrap() error {
	return e.cause
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
	SystemSandboxMode         string                   `json:"system_sandbox_mode"`
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
		var launchErr workerLaunchError
		if errors.As(err, &launchErr) &&
			normalizeSystemSandboxMode(normalizedReq.Execution) == systemSandboxModeRequired {
			return "", NewToolExecError(ToolErrorPermissionDenied, "sandbox worker process failed: "+details, false, err)
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
	launch, err := i.resolveLaunch(req)
	if err != nil {
		return workerRPCResponse{}, "", workerLaunchError{cause: err}
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return workerRPCResponse{}, "", err
	}

	cmd := exec.CommandContext(ctx, launch.Path, launch.Args[1:]...)
	if strings.TrimSpace(launch.Dir) != "" {
		cmd.Dir = launch.Dir
	}
	cmd.Env = launch.Env
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))

	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	cmd.Stderr = &stderrBuffer

	if err := runCommandWithSystemSandbox(cmd, launch.SystemSandboxBackendName, launch.SystemSandboxMode); err != nil {
		return workerRPCResponse{}, stderrBuffer.String(), err
	}

	var response workerRPCResponse
	if err := json.NewDecoder(bytes.NewReader(stdoutBuffer.Bytes())).Decode(&response); err != nil {
		return workerRPCResponse{}, stderrBuffer.String(), fmt.Errorf("decode worker response: %w", err)
	}
	return response, stderrBuffer.String(), nil
}

type workerProcessLaunch struct {
	Path                     string
	Args                     []string
	Dir                      string
	Env                      []string
	SystemSandboxBackendName string
	SystemSandboxMode        string
}

func (i osExecWorkerInvoker) resolveLaunch(req workerRPCRequest) (workerProcessLaunch, error) {
	executablePath := strings.TrimSpace(i.executablePath)
	if executablePath == "" {
		return workerProcessLaunch{}, errors.New("worker executable path is empty")
	}

	mode := normalizeSystemSandboxMode(&ExecutionContext{SystemSandboxMode: req.Execution.SystemSandboxMode})
	goos := strings.TrimSpace(i.goos)
	if goos == "" {
		goos = runtime.GOOS
	}
	lookPath := i.lookPath
	if lookPath == nil {
		lookPath = runShellLookPath
	}
	backend, err := resolveSystemSandboxRuntimeBackend(mode, goos, lookPath)
	if err != nil {
		return workerProcessLaunch{}, err
	}

	path := executablePath
	args := []string{executablePath, sandboxWorkerSubcommand, sandboxWorkerStdioFlag}
	backendName := ""
	if backend.Enabled {
		backendName = strings.TrimSpace(backend.Name)
		switch strings.TrimSpace(backend.Name) {
		case "windows_job_object":
			// Keep direct executable path. Windows process isolation is applied
			// by runCommandWithSystemSandbox when launching this process.
		case "darwin_sandbox_exec":
			path = backend.Runner
			allowNetwork := !strings.EqualFold(mode, systemSandboxModeRequired)
			profile, profileErr := buildDarwinSandboxProfile(&ExecutionContext{
				Workspace:     req.Execution.Workspace,
				WritableRoots: append([]string(nil), req.Execution.WritableRoots...),
			}, allowNetwork)
			if profileErr != nil {
				return workerProcessLaunch{}, profileErr
			}
			args = append([]string{backend.Runner}, buildDarwinSandboxWorkerArgs(profile, executablePath)...)
		default:
			path = backend.Runner
			backendArgs := append([]string(nil), backend.Worker.ArgPrefix...)
			if strings.EqualFold(mode, systemSandboxModeRequired) && strings.EqualFold(goos, "linux") {
				wrapped, wrapErr := buildRequiredLinuxWorkerCommand(executablePath, req.Execution)
				if wrapErr != nil {
					return workerProcessLaunch{}, wrapErr
				}
				backendArgs = append(backendArgs, "sh", "-lc", wrapped)
			} else {
				backendArgs = append(backendArgs, executablePath, sandboxWorkerSubcommand, sandboxWorkerStdioFlag)
			}
			args = append([]string{backend.Runner}, backendArgs...)
		}
	}

	return workerProcessLaunch{
		Path:                     path,
		Args:                     args,
		Dir:                      strings.TrimSpace(req.Execution.Workspace),
		Env:                      buildWorkerProcessEnv(os.Environ(), mode, goos),
		SystemSandboxBackendName: backendName,
		SystemSandboxMode:        mode,
	}, nil
}

func buildRequiredLinuxWorkerCommand(executablePath string, execution workerRPCExecutionContext) (string, error) {
	executablePath = strings.TrimSpace(executablePath)
	if executablePath == "" {
		return "", errors.New("worker executable path is empty")
	}
	command := strings.Join([]string{
		"exec",
		shellSingleQuote(executablePath),
		shellSingleQuote(sandboxWorkerSubcommand),
		shellSingleQuote(sandboxWorkerStdioFlag),
	}, " ")
	return buildRequiredLinuxShellCommand(command, &ExecutionContext{
		Workspace:     execution.Workspace,
		WritableRoots: append([]string(nil), execution.WritableRoots...),
	})
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
		SystemSandboxMode:         execCtx.SystemSandboxMode,
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
		SystemSandboxMode:         payload.SystemSandboxMode,
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

func buildWorkerProcessEnv(base []string, mode, goos string) []string {
	return buildSandboxEnv(base, sandboxEnvOptions{
		GOOS:          goos,
		RequiredMode:  strings.EqualFold(strings.TrimSpace(mode), systemSandboxModeRequired),
		DropSensitive: strings.EqualFold(strings.TrimSpace(mode), systemSandboxModeRequired),
		AlwaysDrop: map[string]struct{}{
			"BYTEMIND_API_KEY":              {},
			"BYTEMIND_PROVIDER_API_KEY":     {},
			"BYTEMIND_PROVIDER_API_KEY_ENV": {},
			sandboxWorkerEnvKey:             {},
		},
		ForceSet: map[string]string{
			sandboxWorkerEnvKey: sandboxWorkerEnvValue,
		},
	})
}
