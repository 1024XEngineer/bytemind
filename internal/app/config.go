package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/config"
)

// ConfigRequest defines workspace config load plus CLI runtime overrides.
type ConfigRequest struct {
	Workspace                 string
	ConfigPath                string
	ModelOverride             string
	StreamOverride            string
	SandboxEnabledOverride    string
	SystemSandboxModeOverride string
	ApprovalModeOverride      string
	AwayPolicyOverride        string
	MaxIterationsOverride     int
}

func LoadRuntimeConfig(req ConfigRequest) (config.Config, error) {
	cfg, err := config.Load(strings.TrimSpace(req.Workspace), strings.TrimSpace(req.ConfigPath))
	if err != nil {
		return cfg, err
	}
	if req.ModelOverride != "" {
		cfg.Provider.Model = req.ModelOverride
	}
	if req.StreamOverride != "" {
		parsed, err := strconv.ParseBool(req.StreamOverride)
		if err != nil {
			return cfg, fmt.Errorf("invalid -stream value: %w", err)
		}
		cfg.Stream = parsed
	}
	if req.SandboxEnabledOverride != "" {
		parsed, err := strconv.ParseBool(req.SandboxEnabledOverride)
		if err != nil {
			return cfg, fmt.Errorf("invalid -sandbox-enabled value: %w", err)
		}
		cfg.SandboxEnabled = parsed
	}
	if req.SystemSandboxModeOverride != "" {
		mode := strings.ToLower(strings.TrimSpace(req.SystemSandboxModeOverride))
		switch mode {
		case "off", "best_effort", "required":
			cfg.SystemSandboxMode = mode
		default:
			return cfg, fmt.Errorf("invalid -system-sandbox-mode value: %q (expected off, best_effort, or required)", mode)
		}
	}
	if req.ApprovalModeOverride != "" {
		mode := strings.TrimSpace(req.ApprovalModeOverride)
		switch mode {
		case "interactive", "away":
			cfg.ApprovalMode = mode
		default:
			return cfg, fmt.Errorf("invalid -approval-mode value: %q (expected interactive or away)", mode)
		}
	}
	if req.AwayPolicyOverride != "" {
		policy := strings.TrimSpace(req.AwayPolicyOverride)
		switch policy {
		case "auto_deny_continue", "fail_fast":
			cfg.AwayPolicy = policy
		default:
			return cfg, fmt.Errorf("invalid -away-policy value: %q (expected auto_deny_continue or fail_fast)", policy)
		}
	}
	if req.MaxIterationsOverride < 0 {
		return cfg, fmt.Errorf("-max-iterations must be greater than 0")
	}
	if req.MaxIterationsOverride > 0 {
		cfg.MaxIterations = req.MaxIterationsOverride
	}
	if cfg.SystemSandboxMode != "off" && !cfg.SandboxEnabled {
		return cfg, fmt.Errorf("system_sandbox_mode requires sandbox_enabled=true (set -sandbox-enabled true)")
	}
	return cfg, nil
}
