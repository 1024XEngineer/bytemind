package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	sandboxpkg "bytemind/internal/sandbox"
)

type workerRunRequest struct {
	ToolName  string
	RawArgs   json.RawMessage
	Execution *ExecutionContext
}

type executorWorker interface {
	Run(context.Context, workerRunRequest) (string, error)
}

type inProcessWorker struct {
	registry   *Registry
	normalizer OutputNormalizer
}

func (w inProcessWorker) Run(ctx context.Context, req workerRunRequest) (string, error) {
	resolved, err := w.resolveTool(req.ToolName)
	if err != nil {
		return "", err
	}
	if err := w.enforcePolicy(ctx, resolved.Definition.Function.Name, req.RawArgs, req.Execution); err != nil {
		return "", err
	}
	normalizer := w.normalizer
	if normalizer == nil {
		normalizer = maxCharsOutputNormalizer{}
	}
	runCtx, cancel := context.WithTimeout(ctx, executionTimeout(req.RawArgs, resolved.Spec))
	defer cancel()

	output, err := resolved.Tool.Run(runCtx, req.RawArgs, req.Execution)
	if err != nil {
		return "", normalizeToolError(err)
	}
	return normalizer.Normalize(output, resolved), nil
}

func (w inProcessWorker) resolveTool(toolName string) (ResolvedTool, error) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return ResolvedTool{}, NewToolExecError(ToolErrorInvalidArgs, "tool name is required", false, nil)
	}
	if w.registry == nil {
		return ResolvedTool{}, NewToolExecError(ToolErrorInternal, "sandbox worker registry is unavailable", false, nil)
	}
	resolved, ok := w.registry.Get(toolName)
	if !ok {
		return ResolvedTool{}, NewToolExecError(ToolErrorInvalidArgs, fmt.Sprintf("unknown tool %q", toolName), false, nil)
	}
	return resolved, nil
}

func (w inProcessWorker) enforcePolicy(ctx context.Context, toolName string, raw json.RawMessage, execCtx *ExecutionContext) error {
	if execCtx == nil || !execCtx.SandboxEnabled {
		return nil
	}
	decision, err := workerSandboxDecision(ctx, toolName, raw, execCtx)
	if err != nil {
		return err
	}

	switch decision.Decision {
	case sandboxpkg.DecisionAllow:
		return nil
	case sandboxpkg.DecisionDeny:
		return NewToolExecError(ToolErrorPermissionDenied, formatBrokerDeniedMessage(toolName, decision), false, nil)
	case sandboxpkg.DecisionEscalate:
		if execCtx.SandboxEscalationApproved {
			return nil
		}
		return escalateWorkerApproval(toolName, decision, execCtx)
	default:
		return NewToolExecError(ToolErrorPermissionDenied, "sandbox policy returned unknown decision", false, nil)
	}
}

func workerSandboxDecision(ctx context.Context, toolName string, raw json.RawMessage, execCtx *ExecutionContext) (sandboxpkg.DecisionResult, error) {
	if execCtx == nil || !execCtx.SandboxEnabled {
		return sandboxpkg.DecisionResult{Decision: sandboxpkg.DecisionAllow}, nil
	}
	lease, keyring, err := resolvePolicyLease(execCtx)
	if err != nil {
		return sandboxpkg.DecisionResult{}, NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
	}

	runtimeReq, err := runtimeRequestForTool(toolName, raw, execCtx)
	if err != nil {
		code := ToolErrorInvalidArgs
		if isRuntimeRequestPermissionError(err) {
			code = ToolErrorPermissionDenied
		}
		return sandboxpkg.DecisionResult{}, NewToolExecError(code, err.Error(), false, err)
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
			ApprovalChannelAvailable: execCtx.Approval != nil || execCtx.Stdin != nil || execCtx.SandboxEscalationApproved,
		},
		Request: runtimeReq,
	})
	if err != nil {
		return sandboxpkg.DecisionResult{}, NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
	}
	return decision, nil
}

func isRuntimeRequestPermissionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "permission denied")
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

func runtimeRequestForTool(toolName string, raw json.RawMessage, execCtx *ExecutionContext) (sandboxpkg.RuntimeRequest, error) {
	switch strings.TrimSpace(toolName) {
	case "run_shell":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		parts := splitShellCommandFields(args.Command)
		if len(parts) == 0 {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("command cannot be empty")
		}
		command := strings.TrimSpace(parts[0])
		if command == "" {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("command cannot be empty")
		}
		return sandboxpkg.RuntimeRequest{
			ToolName: command,
			Command:  command,
			Args:     append([]string(nil), parts[1:]...),
			Network:  extractRunShellNetworkTarget(parts),
		}, nil
	case "web_fetch":
		var args struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		target, err := normalizeWebURL(args.URL)
		if err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		return sandboxpkg.RuntimeRequest{
			ToolName: "web_fetch",
			Network:  networkRuleFromURL(target),
		}, nil
	case "web_search":
		baseURL := strings.TrimSpace(defaultWebSearchBaseURL)
		network := networkRuleFromURL(baseURL)
		if network.Host == "" {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("web_search base url is invalid")
		}
		return sandboxpkg.RuntimeRequest{
			ToolName: "web_search",
			Network:  network,
		}, nil
	case "list_files":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		path, err := resolveSandboxPathForAccess(execCtx, args.Path, sandboxpkg.FileAccessRead)
		if err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		return sandboxpkg.RuntimeRequest{
			ToolName:   "list_files",
			FilePath:   path,
			FileAccess: sandboxpkg.FileAccessRead,
		}, nil
	case "search_text":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		path, err := resolveSandboxPathForAccess(execCtx, args.Path, sandboxpkg.FileAccessRead)
		if err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		return sandboxpkg.RuntimeRequest{
			ToolName:   "search_text",
			FilePath:   path,
			FileAccess: sandboxpkg.FileAccessRead,
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
		path, err := resolveSandboxPathForAccess(execCtx, args.Path, sandboxpkg.FileAccessRead)
		if err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		return sandboxpkg.RuntimeRequest{
			ToolName:   "read_file",
			FilePath:   path,
			FileAccess: sandboxpkg.FileAccessRead,
		}, nil
	case "replace_in_file":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		if strings.TrimSpace(args.Path) == "" {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("path cannot be empty")
		}
		path, err := resolveSandboxPathForAccess(execCtx, args.Path, sandboxpkg.FileAccessWrite)
		if err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		return sandboxpkg.RuntimeRequest{
			ToolName:   "replace_in_file",
			FilePath:   path,
			FileAccess: sandboxpkg.FileAccessWrite,
		}, nil
	case "apply_patch":
		var args struct {
			Patch string `json:"patch"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		if strings.TrimSpace(args.Patch) == "" {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("patch cannot be empty")
		}
		targets := collectApplyPatchPaths(args.Patch)
		if len(targets) == 0 {
			return sandboxpkg.RuntimeRequest{}, fmt.Errorf("patch has no file operations")
		}
		roots := sandboxRootsForAccess(execCtx, sandboxpkg.FileAccessWrite)
		firstPath := ""
		for _, target := range targets {
			path, err := resolveSandboxPathForAccess(execCtx, target, sandboxpkg.FileAccessWrite)
			if err != nil {
				return sandboxpkg.RuntimeRequest{}, err
			}
			if firstPath == "" {
				firstPath = path
			}
			if !pathWithinSandboxRoots(path, roots) {
				return sandboxpkg.RuntimeRequest{
					ToolName:   "apply_patch",
					FilePath:   path,
					FileAccess: sandboxpkg.FileAccessWrite,
				}, nil
			}
		}
		return sandboxpkg.RuntimeRequest{
			ToolName:   "apply_patch",
			FilePath:   firstPath,
			FileAccess: sandboxpkg.FileAccessWrite,
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
		path, err := resolveSandboxPathForAccess(execCtx, args.Path, sandboxpkg.FileAccessWrite)
		if err != nil {
			return sandboxpkg.RuntimeRequest{}, err
		}
		return sandboxpkg.RuntimeRequest{
			ToolName:   "write_file",
			FilePath:   path,
			FileAccess: sandboxpkg.FileAccessWrite,
		}, nil
	default:
		return sandboxpkg.RuntimeRequest{
			ToolName: strings.TrimSpace(toolName),
		}, nil
	}
}

func resolveSandboxPathForAccess(execCtx *ExecutionContext, input string, access sandboxpkg.FileAccess) (string, error) {
	path := strings.TrimSpace(input)
	if execCtx == nil {
		if path == "" {
			return ".", nil
		}
		return path, nil
	}
	roots := sandboxRootsForAccess(execCtx, access)
	resolved, err := resolvePath(execCtx.Workspace, path, roots...)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func sandboxRootsForAccess(execCtx *ExecutionContext, access sandboxpkg.FileAccess) []string {
	if execCtx == nil {
		return nil
	}
	if !execCtx.SandboxEnabled {
		return writableRootsFromExecContext(execCtx)
	}
	switch access {
	case sandboxpkg.FileAccessRead:
		return normalizeWorkerRoots(execCtx.FSRead, execCtx.Workspace, writableRootsFromExecContext(execCtx))
	case sandboxpkg.FileAccessWrite:
		return normalizeWorkerRoots(execCtx.FSWrite, execCtx.Workspace, writableRootsFromExecContext(execCtx))
	default:
		return writableRootsFromExecContext(execCtx)
	}
}

func pathWithinSandboxRoots(path string, roots []string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if len(roots) == 0 {
		return true
	}
	canonicalPath, err := canonicalPathForAccess(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		canonicalRoot, err := canonicalPathForAccess(root)
		if err != nil {
			continue
		}
		if isPathWithinRoot(canonicalRoot, canonicalPath) {
			return true
		}
	}
	return false
}

func collectApplyPatchPaths(patch string) []string {
	patch = normalizePatchText(patch)
	if patch == "" {
		return nil
	}
	lines := strings.Split(patch, "\n")
	paths := make([]string, 0, 8)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			if path != "" {
				paths = append(paths, path)
			}
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			if path != "" {
				paths = append(paths, path)
			}
		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			if path != "" {
				paths = append(paths, path)
			}
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(next, "*** Move to: ") {
					movePath := strings.TrimSpace(strings.TrimPrefix(next, "*** Move to: "))
					if movePath != "" {
						paths = append(paths, movePath)
					}
				}
			}
		}
	}
	return paths
}

func extractRunShellNetworkTarget(parts []string) sandboxpkg.NetworkRule {
	return extractRunShellNetworkTargetWithDepth(parts, 0)
}

func extractRunShellNetworkTargetWithDepth(parts []string, depth int) sandboxpkg.NetworkRule {
	if len(parts) == 0 {
		return sandboxpkg.NetworkRule{}
	}
	if depth >= 3 {
		return sandboxpkg.NetworkRule{}
	}
	command := normalizeShellCommandName(parts[0])
	args := parts[1:]
	if nested := extractNestedShellCommand(command, args); nested != "" {
		if nestedRule := extractRunShellNetworkTargetWithDepth(splitShellCommandFields(nested), depth+1); nestedRule.Host != "" {
			return nestedRule
		}
	}

	var candidate string
	switch command {
	case "curl", "wget":
		candidate = firstURLLikeToken(args)
	case "invoke-webrequest", "iwr", "invoke-restmethod", "irm":
		candidate = findPowerShellURLArgument(args)
	default:
		return sandboxpkg.NetworkRule{}
	}
	return networkRuleFromURL(candidate)
}

func extractNestedShellCommand(command string, args []string) string {
	switch command {
	case "sh", "bash", "zsh", "ksh", "dash":
		return shellCommandAfterFlag(args)
	case "cmd":
		return argumentAfterFlag(args, "/c", "/k")
	case "powershell", "pwsh":
		return argumentAfterFlag(args, "-command", "/command", "-c", "/c")
	default:
		return ""
	}
}

func shellCommandAfterFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		flag := strings.ToLower(strings.TrimSpace(args[i]))
		if flag == "-c" {
			if i+1 < len(args) {
				return strings.TrimSpace(args[i+1])
			}
			return ""
		}
		if strings.HasPrefix(flag, "-") && len(flag) > 2 {
			compact := strings.TrimPrefix(flag, "-")
			if isAlphaOnly(compact) && strings.Contains(compact, "c") {
				if i+1 < len(args) {
					return strings.TrimSpace(args[i+1])
				}
				return ""
			}
		}
	}
	return ""
}

func isAlphaOnly(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func argumentAfterFlag(args []string, flags ...string) string {
	for i := 0; i < len(args); i++ {
		flag := strings.ToLower(strings.TrimSpace(args[i]))
		for _, target := range flags {
			if flag == target {
				if i+1 < len(args) {
					return strings.TrimSpace(args[i+1])
				}
				return ""
			}
		}
	}
	return ""
}

func normalizeShellCommandName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	name = strings.ToLower(filepath.Base(name))
	return strings.TrimSuffix(name, ".exe")
}

func firstURLLikeToken(args []string) string {
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if rule := networkRuleFromURL(arg); rule.Host != "" {
			return arg
		}
	}
	return ""
}

func findPowerShellURLArgument(args []string) string {
	for i := 0; i < len(args); i++ {
		flag := strings.ToLower(strings.TrimSpace(args[i]))
		switch flag {
		case "-uri", "--uri", "-url", "--url":
			if i+1 < len(args) {
				return strings.TrimSpace(args[i+1])
			}
			return ""
		}
	}
	return firstURLLikeToken(args)
}

func networkRuleFromURL(raw string) sandboxpkg.NetworkRule {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sandboxpkg.NetworkRule{}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return sandboxpkg.NetworkRule{}
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return sandboxpkg.NetworkRule{}
	}
	port := 0
	switch scheme {
	case "https":
		port = 443
	case "http":
		port = 80
	default:
		return sandboxpkg.NetworkRule{}
	}
	if parsed.Port() != "" {
		if parsedPort := strings.TrimSpace(parsed.Port()); parsedPort != "" {
			switch parsedPort {
			case "80":
				port = 80
			case "443":
				port = 443
			default:
				return sandboxpkg.NetworkRule{}
			}
		}
	}
	return sandboxpkg.NetworkRule{
		Host:   host,
		Port:   port,
		Scheme: scheme,
	}
}

func splitShellCommandFields(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	fields := make([]string, 0, 8)
	var builder strings.Builder
	inSingle := false
	inDouble := false

	flush := func() {
		if builder.Len() == 0 {
			return
		}
		fields = append(fields, builder.String())
		builder.Reset()
	}

	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
				continue
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
				continue
			}
		}
		if !inSingle && !inDouble && unicode.IsSpace(rune(ch)) {
			flush()
			continue
		}
		builder.WriteByte(ch)
	}
	flush()
	return fields
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
		if execCtx.Stdin == nil {
			return approvalChannelUnavailableError("tool", toolName)
		}
		if execCtx.Stdout != nil {
			reason := strings.TrimSpace(decision.Message)
			if reason != "" {
				fmt.Fprintf(execCtx.Stdout, "Approve tool (%s) %q? [y/N]: ", reason, toolName)
			} else {
				fmt.Fprintf(execCtx.Stdout, "Approve tool %q? [y/N]: ", toolName)
			}
		}
		reader := bufio.NewReader(execCtx.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			return NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q was not run because approval was denied", toolName), false, nil)
		}
		return nil
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
	_ = execCtx
	switch toolName {
	case "run_shell", "web_fetch", "web_search", "list_files", "search_text", "read_file", "replace_in_file", "apply_patch", "write_file":
		return true
	default:
		return false
	}
}
