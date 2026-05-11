package app

import (
	"fmt"
	"io"
	"strings"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
)

func RunSafety(args []string, stdout, stderr io.Writer) error {
	workspace := "."
	configFile := ""
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
		case "status":
			return renderSafetyStatus(workspace, configFile, stdout)
		case "explain":
			return renderSafetyExplain(stdout)
		}
	}
	// default: show status if no subcommand matched
	if len(args) > 0 && args[0] != "" {
		// If first arg is "explain", show explain
		first := strings.TrimSpace(strings.ToLower(args[0]))
		if first == "explain" {
			return renderSafetyExplain(stdout)
		}
	}
	return renderSafetyStatus(workspace, configFile, stdout)
}

func renderSafetyStatus(workspace, configFile string, w io.Writer) error {
	write := func(format string, v ...any) {
		fmt.Fprintf(w, format+"\n", v...)
	}

	cfg, err := configpkg.Load(workspace, configFile)
	if err != nil {
		cfg = configpkg.Default(workspace)
	}

	mode, _ := configpkg.NormalizeApprovalMode(cfg.ApprovalMode)
	write("ByteMind Safety Status")
	write("")
	write("  Approval policy: %s", cfg.ApprovalPolicy)
	write("  Approval mode:   %s", mode)
	if len(cfg.WritableRoots) == 0 {
		write("  Writable roots:  none (defaults to workspace only)")
	} else {
		write("  Writable roots:  %s", strings.Join(cfg.WritableRoots, ", "))
	}
	write("  Shell allowlist:")
	if len(cfg.ExecAllowlist) == 0 {
		write("    (none)")
	} else {
		for _, rule := range cfg.ExecAllowlist {
			write("    - %s %s", rule.Command, strings.Join(rule.ArgsPattern, " "))
		}
	}
	if cfg.SandboxEnabled {
		write("  Sandbox:         enabled (%s)", cfg.SystemSandboxMode)
	} else {
		write("  Sandbox:         disabled")
	}
	if len(cfg.NetworkAllowlist) == 0 {
		write("  Network access:  restricted (agent-initiated only)")
	} else {
		targets := make([]string, len(cfg.NetworkAllowlist))
		for i, r := range cfg.NetworkAllowlist {
			targets[i] = r.Host
		}
		write("  Network access:  restricted to %s", strings.Join(targets, ", "))
	}
	write("  Approval bypass: %s", awayPolicyLabel(cfg.AwayPolicy))
	write("")
	write("  Summary:")
	write("    - Write operations: %s", writeAccessSummary(mode))
	write("    - Shell execution:  %s", writeAccessSummary(mode))
	write("    - Read operations:  always allowed")
	write("    - Network access:   restricted")
	return nil
}

func renderSafetyExplain(w io.Writer) error {
	write := func(format string, v ...any) {
		fmt.Fprintf(w, format+"\n", v...)
	}

	write("ByteMind Safety Model")
	write("")
	write("ByteMind uses a layered safety architecture:")
	write("")
	write("1. Tool Safety Classes")
	write("   Each tool is classified into a safety class:")
	write("     safe        - read-only, always allowed")
	write("     moderate    - read-modify, requires policy check")
	write("     sensitive   - shell execution, requires approval")
	write("     destructive - file modification, requires approval")
	write("")
	write("2. Approval Policy")
	write("   on-request  - high-risk tools prompt for approval (default)")
	write("   always      - all tools require approval")
	write("   never       - all tools auto-approved")
	write("")
	write("3. Approval Mode")
	write("   interactive - user sees approval prompts in TUI")
	write("   full_access - all tools approved without prompting")
	write("")
	write("4. Sandbox")
	write("   off         - no sandbox enforcement")
	write("   best_effort - sandbox if available, fallback otherwise")
	write("   required    - sandbox mandatory, fail if unavailable")
	write("")
	write("5. Writable Roots")
	write("   Restricts file writes to specific directories.")
	write("   Default: only the workspace directory.")
	write("")
	write("6. Shell Allowlist")
	write("   Commands like 'go test', 'npm test' are low-risk.")
	write("   Commands like 'rm -rf', 'sudo' require approval.")
	write("   Unrecognized shell commands require approval.")
	write("")
	write("7. Network Allowlist")
	write("   Controls which external hosts the agent can reach.")
	write("   Default: restricted (agent-initiated access only).")
	write("")
	write("8. Mode Authorization")
	write("   Plan mode: only read-only shell commands permitted.")
	write("   Build mode: all approved tools available.")
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

func writeAccessSummary(mode string) string {
	if mode == "full_access" {
		return "auto-approved"
	}
	return "require confirmation"
}
