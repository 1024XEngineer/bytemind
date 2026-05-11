package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
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
		write("  [PASS] %s", msg)
	}
	fail := func(msg string) {
		write("  [FAIL] %s", msg)
	}
	warn := func(msg string) {
		write("  [WARN] %s", msg)
	}

	write("ByteMind Doctor")
	write("")

	write("Configuration:")
	cfg, err := configpkg.Load(workspace, configFile)
	configFound := err == nil
	if configFound {
		pass("config file loaded")
	} else {
		warn(fmt.Sprintf("no config file found (optional): %v", err))
		cfg = configpkg.Default(workspace)
	}

	write("Provider:")
	selected := configpkg.SelectedModelID(cfg.ProviderRuntime)
	if selected != "" {
		pass(fmt.Sprintf("model configured: %s", selected))
	} else if cfg.Provider.Model != "" {
		pass(fmt.Sprintf("model configured: %s", cfg.Provider.Model))
	} else {
		warn("no model configured")
	}
	key := cfg.Provider.APIKey
	if key == "" {
		key = os.Getenv("ANTHROPIC_API_KEY")
	}
	if key == "" {
		key = os.Getenv("OPENAI_API_KEY")
	}
	if key == "" {
		key = os.Getenv("DEEPSEEK_API_KEY")
	}
	if key != "" {
		pass("API key detected")
	} else {
		fail("no API key found (set ANTHROPIC_API_KEY, OPENAI_API_KEY, DEEPSEEK_API_KEY)")
	}

	write("Workspace:")
	absWorkspace, _ := filepath.Abs(workspace)
	if info, err := os.Stat(absWorkspace); err == nil && info.IsDir() {
		pass(fmt.Sprintf("workspace valid: %s", absWorkspace))
	} else {
		fail(fmt.Sprintf("workspace not found: %s", absWorkspace))
	}
	if _, err := exec.LookPath("git"); err == nil {
		pass("git found in PATH")
	} else {
		warn("git not found in PATH")
	}
	if _, err := os.Stat(filepath.Join(absWorkspace, ".git")); err == nil {
		pass("git repository detected")
	}
	if _, err := os.Stat(filepath.Join(absWorkspace, "go.mod")); err == nil {
		pass("Go project detected (go.mod)")
	}
	if _, err := os.Stat(filepath.Join(absWorkspace, "package.json")); err == nil {
		pass("Node project detected (package.json)")
	}

	write("Security:")
	mode, _ := configpkg.NormalizeApprovalMode(cfg.ApprovalMode)
	pass(fmt.Sprintf("approval mode: %s", mode))
	if cfg.SandboxEnabled {
		pass(fmt.Sprintf("sandbox: enabled (%s)", cfg.SystemSandboxMode))
	} else {
		warn("sandbox: disabled")
	}
	if len(cfg.WritableRoots) > 0 {
		pass(fmt.Sprintf("writable roots: %s", strings.Join(cfg.WritableRoots, ", ")))
	} else {
		warn("no writable roots configured (defaults to workspace only)")
	}
	if len(cfg.ExecAllowlist) > 0 {
		pass(fmt.Sprintf("exec allowlist: %d rule(s)", len(cfg.ExecAllowlist)))
	} else {
		warn("no exec allowlist configured")
	}

	write("Environment:")
	pass(fmt.Sprintf("OS: %s / Arch: %s", runtime.GOOS, runtime.GOARCH))
	if _, err := exec.LookPath("go"); err == nil {
		pass("Go is installed")
	} else {
		warn("Go not found in PATH")
	}

	write("")
	write("Doctor check complete.")
	if key == "" {
		write("Set your API key via environment variable or config.json before using ByteMind.")
	}
	return nil
}
