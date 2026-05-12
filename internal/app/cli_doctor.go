package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
	toolspkg "github.com/1024XEngineer/bytemind/internal/tools"
)

func RunDoctor(args []string, stdout, stderr io.Writer) error {
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
		}
	}

	write := func(format string, v ...any) {
		fmt.Fprintf(stdout, format+"\n", v...)
	}
	pass := func(msg string) {
		write("  \u2705 %s", msg)
	}
	fail := func(msg string) {
		write("  \u274c %s", msg)
	}
	warn := func(msg string) {
		write("  \u26a0\ufe0f %s", msg)
	}

	write("ByteMind Doctor")
	write("")

	write("Environment:")
	pass(fmt.Sprintf("Go %s", runtime.Version()))
	pass(fmt.Sprintf("OS: %s / Arch: %s", runtime.GOOS, runtime.GOARCH))
	if _, err := exec.LookPath("go"); err == nil {
		pass("Go available")
	} else {
		warn("Go not in PATH")
	}
	if _, err := exec.LookPath("git"); err == nil {
		pass("Git available")
	} else {
		warn("Git not in PATH")
	}

	write("Workspace:")
	absWorkspace, _ := filepath.Abs(workspace)
	if info, err := os.Stat(absWorkspace); err == nil && info.IsDir() {
		pass(fmt.Sprintf("Valid workspace: %s", absWorkspace))
	} else {
		fail(fmt.Sprintf("Workspace not found: %s", absWorkspace))
	}
	if _, err := os.Stat(filepath.Join(absWorkspace, ".git")); err == nil {
		pass("Git repository detected")
	} else {
		warn("Not a git repository")
	}

	write("Configuration:")
	cfg, err := configpkg.Load(workspace, configFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			warn(fmt.Sprintf("No config file (optional): %v", err))
		} else {
			fail(fmt.Sprintf("Config error: %v", err))
		}
		cfg = configpkg.Default(workspace)
	} else {
		pass("Config file loaded")
	}

	write("Provider:")
	selected := configpkg.SelectedModelID(cfg.ProviderRuntime)
	if selected != "" {
		pass(fmt.Sprintf("Model configured: %s", selected))
	} else if cfg.Provider.Model != "" {
		pass(fmt.Sprintf("Model configured: %s", cfg.Provider.Model))
	} else {
		warn("No model configured")
	}
	key := cfg.Provider.APIKey
	keySource := ""
	if key != "" {
		keySource = "config file"
	}
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
		if key != "" {
			keySource = "ANTHROPIC_API_KEY"
		}
	}
	if key == "" {
		key = os.Getenv("OPENAI_API_KEY")
		if key != "" {
			keySource = "OPENAI_API_KEY"
		}
	}
	if key == "" {
		key = os.Getenv("DEEPSEEK_API_KEY")
		if key != "" {
			keySource = "DEEPSEEK_API_KEY"
		}
	}
	if key != "" {
		pass(fmt.Sprintf("API key found (%s)", keySource))
	} else {
		fail("No API key found (set ANTHROPIC_API_KEY, OPENAI_API_KEY, or DEEPSEEK_API_KEY)")
	}

	write("Security:")
	mode, _ := configpkg.NormalizeApprovalMode(cfg.ApprovalMode)
	pass(fmt.Sprintf("Approval mode: %s", mode))
	pass(fmt.Sprintf("Approval policy: %s", cfg.ApprovalPolicy))
	if cfg.SandboxEnabled {
		pass(fmt.Sprintf("Sandbox: enabled (%s)", cfg.SystemSandboxMode))
	} else {
		warn("Sandbox: disabled")
	}
	if len(cfg.WritableRoots) > 0 {
		pass(fmt.Sprintf("Writable roots: %s", strings.Join(cfg.WritableRoots, ", ")))
	} else {
		warn("No writable roots (defaults to workspace only)")
	}
	if len(cfg.ExecAllowlist) > 0 {
		pass(fmt.Sprintf("Exec allowlist: %d rule(s)", len(cfg.ExecAllowlist)))
	} else {
		warn("No exec allowlist (all shell commands require approval)")
	}

	write("Tools:")
	reg := toolspkg.DefaultRegistry()
	pass(fmt.Sprintf("%d tools registered", reg.Count()))

	write("")
	write("Doctor check complete.")
	if key == "" {
		write("Tip: Set your API key via environment variable or config.json before using ByteMind.")
	}
	return nil
}
