package agent

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

var destructiveApprovalTools = map[string]struct{}{
	"apply_patch":     {},
	"replace_in_file": {},
	"write_file":      {},
}

const destructiveApprovalReasonPrefix = "destructive tool may modify workspace files:"

type runApprovalGrants struct {
	Shell       bool
	Destructive bool
}

type preapprovalIntent struct {
	Shell       bool
	Destructive bool
}

func (g runApprovalGrants) hasAny() bool {
	return g.Shell || g.Destructive
}

func (r *Runner) prepareRunApprovalHandler(setup runPromptSetup, out io.Writer) tools.ApprovalHandler {
	base, approvalChannelAvailable := r.resolveRunApprovalBaseHandler()
	r.renderApprovalPrecheck(out, setup)
	if !approvalChannelAvailable {
		writeApprovalChannelUnavailableNotice(out, r.config.ApprovalPolicy, r.config.ApprovalMode)
		return base
	}
	if !shouldAttemptPreapproval(r, setup, base) {
		return base
	}

	toolNames := filteredToolNamesForMode(r.registry, setup.RunMode, setup.AllowedToolNames, setup.DeniedToolNames)
	hasShell, destructive := classifyPreapprovalToolGroups(toolNames)
	if !hasShell && len(destructive) == 0 {
		return base
	}
	intent := inferPreapprovalIntent(setup.UserInput)
	policy := strings.ToLower(strings.TrimSpace(r.config.ApprovalPolicy))
	if policy == "" {
		policy = "on-request"
	}
	requestShell := hasShell && (policy == "always" || intent.Shell)
	requestDestructive := len(destructive) > 0 && (policy == "always" || intent.Destructive)

	grants := runApprovalGrants{}
	if requestShell {
		decision, err := base(tools.ApprovalRequest{
			ToolName: "run_shell",
			Command:  "run_shell (session pre-approval)",
			Reason:   "pre-approve approval-required run_shell commands for this run",
		})
		approved := tools.NormalizeApprovalDecision(decision).Approved()
		writePreapprovalResult(out, "run_shell", approved, err)
		if err == nil && approved {
			grants.Shell = true
		}
	}
	if requestDestructive {
		decision, err := base(tools.ApprovalRequest{
			ToolName: "destructive_tools",
			Command:  "workspace-modifying tools (session pre-approval)",
			Reason:   fmt.Sprintf("pre-approve destructive tool calls for this run: %s", strings.Join(destructive, ", ")),
		})
		approved := tools.NormalizeApprovalDecision(decision).Approved()
		writePreapprovalResult(out, "workspace-modifying tools", approved, err)
		if err == nil && approved {
			grants.Destructive = true
		}
	}
	return func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
		if grants.Destructive && isDestructiveToolApprovalRequest(req) {
			return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
		}
		if grants.Shell && isRunShellApprovalRequest(req) {
			return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
		}
		decision, err := base(req)
		normalized := tools.NormalizeApprovalDecision(decision)
		if err != nil || !normalized.Approved() {
			return normalized, err
		}
		if normalized.ReusableForAllTools() {
			grants.Destructive = true
			grants.Shell = true
		} else if normalized.ReusableForSameTool() && isDestructiveToolApprovalRequest(req) {
			grants.Destructive = true
		} else if normalized.ReusableForSameTool() && isRunShellApprovalRequest(req) {
			grants.Shell = true
		}
		return normalized, nil
	}
}

func (r *Runner) resolveRunApprovalBaseHandler() (tools.ApprovalHandler, bool) {
	if r == nil {
		return nil, false
	}
	if r.approval != nil {
		return r.approval, true
	}
	if r.stdin == nil {
		return unavailableRunApprovalHandler(), false
	}
	reader := bufio.NewReader(r.stdin)
	return func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
		command := strings.TrimSpace(req.Command)
		if command == "" {
			command = "unknown action"
		}
		reason := strings.TrimSpace(req.Reason)
		if r.stdout != nil {
			if reason != "" {
				fmt.Fprintf(r.stdout, "Approve action (%s) %q? [y/N]: ", reason, command)
			} else {
				fmt.Fprintf(r.stdout, "Approve action %q? [y/N]: ", command)
			}
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return tools.ApprovalDecision{Disposition: tools.ApprovalDeny}, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer == "y" || answer == "yes" {
			return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
		}
		return tools.ApprovalDecision{Disposition: tools.ApprovalDeny}, nil
	}, true
}

func unavailableRunApprovalHandler() tools.ApprovalHandler {
	return func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
		command := strings.TrimSpace(req.Command)
		if command == "" {
			command = "unknown action"
		}
		return tools.ApprovalDecision{Disposition: tools.ApprovalDeny}, fmt.Errorf("approval channel unavailable for %q: missing TUI approval bridge and stdin fallback", command)
	}
}

func writeApprovalChannelUnavailableNotice(out io.Writer, approvalPolicy, approvalMode string) {
	if out == nil {
		return
	}
	policy := strings.ToLower(strings.TrimSpace(approvalPolicy))
	if policy == "" {
		policy = "on-request"
	}
	mode := strings.ToLower(strings.TrimSpace(approvalMode))
	if mode == "" {
		mode = "interactive"
	}
	if policy == "never" || mode == approvalModeAway {
		return
	}
	_, _ = io.WriteString(out, fmt.Sprintf("%sapproval channel unavailable%s interactive approvals cannot be prompted in this run; approval-required actions will be denied\n", ansiYellow, ansiReset))
}

func (r *Runner) renderApprovalPrecheck(out io.Writer, setup runPromptSetup) {
	if out == nil || r == nil || r.registry == nil {
		return
	}
	if setup.RunMode != planpkg.ModeBuild {
		return
	}
	toolNames := filteredToolNamesForMode(r.registry, setup.RunMode, setup.AllowedToolNames, setup.DeniedToolNames)
	summary := buildApprovalPrecheckSummary(approvalPrecheckSummaryInput{
		ToolNames:      toolNames,
		ApprovalPolicy: r.config.ApprovalPolicy,
		ApprovalMode:   r.config.ApprovalMode,
		AwayPolicy:     r.config.AwayPolicy,
	})
	if strings.TrimSpace(summary) == "" {
		return
	}
	_, _ = io.WriteString(out, summary)
}

type approvalPrecheckSummaryInput struct {
	ToolNames      []string
	ApprovalPolicy string
	ApprovalMode   string
	AwayPolicy     string
}

func buildApprovalPrecheckSummary(input approvalPrecheckSummaryInput) string {
	policy := strings.ToLower(strings.TrimSpace(input.ApprovalPolicy))
	if policy == "" {
		policy = "on-request"
	}
	if policy == "never" {
		return ""
	}

	toolSet := make(map[string]struct{}, len(input.ToolNames))
	for _, name := range input.ToolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		toolSet[name] = struct{}{}
	}

	hasShell := false
	if _, ok := toolSet["run_shell"]; ok {
		hasShell = true
	}

	destructive := make([]string, 0, len(destructiveApprovalTools))
	for name := range destructiveApprovalTools {
		if _, ok := toolSet[name]; ok {
			destructive = append(destructive, name)
		}
	}
	sort.Strings(destructive)

	if !hasShell && len(destructive) == 0 {
		return ""
	}

	lines := []string{
		fmt.Sprintf("%sapproval precheck%s potential approval-required actions:", ansiDim, ansiReset),
	}

	if hasShell {
		if policy == "always" {
			lines = append(lines, "  - run_shell commands (approval_policy=always)")
		} else {
			lines = append(lines, "  - run_shell commands that are not read-only")
		}
	}

	if len(destructive) > 0 {
		lines = append(lines, fmt.Sprintf("  - workspace-modifying tools: %s", strings.Join(destructive, ", ")))
	}

	approvalMode := strings.ToLower(strings.TrimSpace(input.ApprovalMode))
	if approvalMode == "" {
		approvalMode = "interactive"
	}
	if approvalMode == "away" {
		awayPolicy := strings.ToLower(strings.TrimSpace(input.AwayPolicy))
		if awayPolicy == "" {
			awayPolicy = "auto_deny_continue"
		}
		lines = append(lines, fmt.Sprintf("  away mode: approvals are unavailable; matched actions will be denied (away_policy=%s)", awayPolicy))
		if awayPolicy == "fail_fast" {
			lines = append(lines, "  fail_fast: run stops after the first denied approval-required action")
		}
	} else {
		lines = append(lines, "  interactive mode: approvals are requested only when an action is actually attempted")
	}

	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func shouldAttemptPreapproval(r *Runner, setup runPromptSetup, base tools.ApprovalHandler) bool {
	if r == nil || r.registry == nil || base == nil {
		return false
	}
	if setup.RunMode != planpkg.ModeBuild {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(r.config.ApprovalMode))
	if mode == "" {
		mode = "interactive"
	}
	if mode != "interactive" {
		return false
	}
	policy := strings.ToLower(strings.TrimSpace(r.config.ApprovalPolicy))
	return policy != "never"
}

func classifyPreapprovalToolGroups(toolNames []string) (bool, []string) {
	toolSet := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		toolSet[name] = struct{}{}
	}
	_, hasShell := toolSet["run_shell"]

	destructive := make([]string, 0, len(destructiveApprovalTools))
	for name := range destructiveApprovalTools {
		if _, ok := toolSet[name]; !ok {
			continue
		}
		destructive = append(destructive, name)
	}
	sort.Strings(destructive)
	return hasShell, destructive
}

func inferPreapprovalIntent(userInput string) preapprovalIntent {
	normalized := strings.ToLower(strings.TrimSpace(userInput))
	if normalized == "" {
		return preapprovalIntent{}
	}
	hasCodeContext := containsAnyToken(normalized,
		"file", "files", "code", "repo", "repository", "project", "workspace", "function",
		".go", ".ts", ".js", ".py", ".java", ".md",
		"文件", "代码", "仓库", "项目", "目录", "函数",
	)
	shellHint := containsAnyToken(normalized,
		"run", "shell", "command", "terminal", "test", "build", "compile", "install",
		"go test", "go run", "npm", "pnpm", "yarn", "pip", "cargo", "make", "cmake",
		"powershell", "bash", "cmd",
		"执行", "命令", "终端", "测试", "构建", "编译", "安装", "脚本",
	)
	destructiveVerb := containsAnyToken(normalized,
		"edit", "modify", "update", "write", "rewrite", "create", "implement", "fix",
		"refactor", "patch", "delete", "remove", "rename", "change",
		"修改", "更新", "写入", "重写", "创建", "实现", "修复", "重构", "补丁", "删除", "重命名", "改",
	)
	return preapprovalIntent{
		Shell:       hasCodeContext && shellHint,
		Destructive: hasCodeContext && destructiveVerb,
	}
}

func containsAnyToken(input string, tokens ...string) bool {
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		if strings.Contains(input, token) {
			return true
		}
	}
	return false
}

func writePreapprovalResult(out io.Writer, category string, approved bool, err error) {
	if out == nil {
		return
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = "approvals"
	}
	switch {
	case err != nil:
		fmt.Fprintf(out, "%sapproval precheck%s failed to pre-approve %s (%s); runtime approvals remain enabled\n", ansiDim, ansiReset, category, strings.TrimSpace(err.Error()))
	case approved:
		fmt.Fprintf(out, "%sapproval precheck%s pre-approved %s for this run\n", ansiDim, ansiReset, category)
	default:
		fmt.Fprintf(out, "%sapproval precheck%s %s pre-approval denied; runtime approvals remain enabled\n", ansiDim, ansiReset, category)
	}
}

func isDestructiveToolApprovalRequest(req tools.ApprovalRequest) bool {
	reason := strings.ToLower(strings.TrimSpace(req.Reason))
	if strings.HasPrefix(reason, destructiveApprovalReasonPrefix) {
		return true
	}
	_, ok := destructiveApprovalTools[strings.TrimSpace(req.Command)]
	return ok
}

func isRunShellApprovalRequest(req tools.ApprovalRequest) bool {
	return !isDestructiveToolApprovalRequest(req)
}

func filteredToolNamesForMode(registry ToolRegistry, mode planpkg.AgentMode, allowlist, denylist []string) []string {
	if registry == nil {
		return nil
	}
	defs := registry.DefinitionsForModeWithFilters(mode, allowlist, denylist)
	if len(defs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(defs))
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		name := strings.TrimSpace(def.Function.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
