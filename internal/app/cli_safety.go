package app

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
)

func RunSafety(args []string, stdout, stderr io.Writer) error {
	workspace := "."
	configFile := ""
	subcommand := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-workspace", "--workspace":
			if i+1 < len(args) {
				workspace = args[i+1]
				i++
			}
		case "-config", "--config":
			if i+1 < len(args) {
				configFile = args[i+1]
				i++
			}
		case "status", "explain":
			subcommand = args[i]
		}
	}
	switch subcommand {
	case "explain":
		return renderSafetyExplain(stdout)
	default:
		return renderSafetyStatus(workspace, configFile, stdout)
	}
}

func renderSafetyStatus(workspace, configFile string, w io.Writer) error {
	write := func(format string, v ...any) {
		fmt.Fprintf(w, format+"\n", v...)
	}

	cfg, err := configpkg.Load(workspace, configFile)
	if err != nil {
		write("  Config error: %v", err)
		cfg = configpkg.Default(workspace)
	}

	absWorkspace, _ := filepath.Abs(workspace)
	mode, _ := configpkg.NormalizeApprovalMode(cfg.ApprovalMode)
	write("ByteMind Safety Report")
	write("======================")
	write("")

	write("Workspace:")
	write("  %s", absWorkspace)

	write("")
	write("Policy:")
	if mode == "full_access" {
		write("  \u274c Approval mode: full_access \u2014 ALL tools approved without prompting")
	} else {
		write("  \u2705 Approval mode: %s", mode)
	}
	write("  \u2705 Approval policy: %s", cfg.ApprovalPolicy)

	write("")
	write("Boundary:")
	if cfg.SandboxEnabled {
		write("  \u2705 Sandbox: enabled (%s)", cfg.SystemSandboxMode)
	} else {
		write("  \u26a0\ufe0f Sandbox: disabled")
	}
	if len(cfg.WritableRoots) == 0 {
		write("  \u26a0\ufe0f Writable roots: none (defaults to workspace only)")
	} else {
		write("  \u2705 Writable roots: %s", strings.Join(cfg.WritableRoots, ", "))
	}
	dangerousCommands := []string{"rm -rf", "sudo", "curl | sh", "chmod -R 777", "dd", "mkfs", "shutdown", "reboot"}
	write("  \u26a0\ufe0f High-risk commands (require approval): %s", strings.Join(dangerousCommands, ", "))

	write("")
	write("Network policy:")
	if len(cfg.NetworkAllowlist) == 0 {
		write("  \u26a0\ufe0f Default: restricted (agent-initiated access to any host)")
	} else {
		targets := make([]string, len(cfg.NetworkAllowlist))
		for i, r := range cfg.NetworkAllowlist {
			targets[i] = fmt.Sprintf("%s://%s:%d", r.Scheme, r.Host, r.Port)
		}
		write("  \u2705 Allowlist: %s", strings.Join(targets, ", "))
	}

	write("")
	write("Shell allowlist:")
	if len(cfg.ExecAllowlist) == 0 {
		write("  \u26a0\ufe0f (none \u2014 all shell commands require approval)")
	} else {
		for _, rule := range cfg.ExecAllowlist {
			write("  \u2705 %s %s", rule.Command, strings.Join(rule.ArgsPattern, " "))
		}
	}

	write("")
	write("Access summary:")
	write("  \u2705 Read operations: always allowed")
	if cfg.ApprovalPolicy == "never" || mode == "full_access" {
		if cfg.ApprovalPolicy == "never" {
			write("  \u274c Write operations: blocked (approval_policy=never)")
			write("  \u274c Shell execution: blocked (approval_policy=never)")
		} else {
			write("  \u274c Write operations: auto-approved (no prompt)")
			write("  \u274c Shell execution: auto-approved (no prompt)")
		}
	} else {
		write("  \u2705 Write operations: require confirmation")
		write("  \u2705 Shell execution: require confirmation")
	}
	write("  \u26a0\ufe0f Network access: restricted")

	if mode == "full_access" {
		write("")
		write("  \u26a0\ufe0f full_access mode bypasses all approval prompts. Recommended for CI/demo only.")
	}

	write("")
	resolvedConfig := configFile
	if resolvedConfig == "" {
		resolvedConfig = "default (user/project config)"
	}
	write("Config file: %s", resolvedConfig)
	write("Max iterations: %d", cfg.MaxIterations)
	write("Stream: %v", cfg.Stream)
	return nil
}

func renderSafetyExplain(w io.Writer) error {
	write := func(format string, v ...any) {
		fmt.Fprintf(w, format+"\n", v...)
	}

	write("ByteMind Safety Model \u2014 Layered Safety Architecture")
	write("")
	write("1. Tool Safety Classes")
	write("   Each tool is classified into a safety class in its spec (internal/tools/spec.go):")
	write("     \u2705 safe        \u2014 read-only, always allowed (e.g. read_file, git_status)")
	write("     \u26a0\ufe0f moderate   \u2014 read-modify, requires policy check (e.g. replace_in_file)")
	write("     \u26a0\ufe0f sensitive  \u2014 shell execution, requires approval (e.g. run_shell, run_tests)")
	write("     \u274c destructive \u2014 file modification, requires approval (e.g. write_file)")
	write("")
	write("2. Approval Policy (config: approval_policy)")
	write("   on-request  \u2014 high-risk tools prompt for approval (default)")
	write("   always      \u2014 all tools require approval")
	write("   never       \u2014 high-risk tools are blocked entirely (deny, not auto-approve)")
	write("")
	write("3. Approval Mode (config: approval_mode)")
	write("   interactive \u2014 user sees approval prompts in TUI")
	write("   full_access \u2014 ALL tools approved without prompting. Intended for CI/demo only.")
	write("")
	write("4. Sandbox (config: sandbox_enabled / system_sandbox_mode)")
	write("   off         \u2014 no sandbox enforcement")
	write("   best_effort \u2014 sandbox if available, fallback otherwise")
	write("   required    \u2014 sandbox mandatory, fail if unavailable")
	write("")
	write("5. Writable Roots (config: writable_roots)")
	write("   Restricts file writes to specific directories.")
	write("   Default: only the workspace directory.")
	write("   Empty list = workspace only. Add paths to allow writes elsewhere.")
	write("")
	write("6. Shell Allowlist (config: exec_allowlist)")
	write("   Known safe commands (e.g. 'go test', 'npm test') run without approval.")
	write("   Commands like 'rm -rf', 'sudo', or anything not in the allowlist require approval.")
	write("   Empty allowlist = ALL shell commands require approval.")
	write("")
	write("7. Network Allowlist (config: network_allowlist)")
	write("   Controls which external hosts the agent can reach via web tools.")
	write("   Default: restricted (agent-initiated access to any host).")
	write("   Add host patterns to restrict to specific services.")
	write("")
	write("8. Mode Authorization")
	write("   Plan mode \u2014 only read-only shell commands permitted. No file modifications.")
	write("   Build mode \u2014 all approved tools available based on policy.")
	write("")
	write("What is NOT protected:")
	write("  \u2022 The agent runs on your machine with your user privileges.")
	write("  \u2022 ByteMind does not virtualize or containerize execution (unless sandbox is enabled).")
	write("  \u2022 full_access mode disables all approval gates \u2014 only use in CI or trusted scenarios.")
	write("")
	write("For configuration, see config.example.json or use CLI flags.")
	return nil
}

func awayPolicyLabel(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "fail_fast":
		return "deny and fail"
	default:
		return "deny and continue"
	}
}

func writeAccessSummary(mode, policy string) string {
	if policy == "never" {
		return "auto-approved (approval_policy=never)"
	}
	if mode == "full_access" {
		return "auto-approved"
	}
	return "require confirmation"
}
