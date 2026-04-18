package app

import (
	"fmt"
	"strconv"
	"strings"

	"bytemind/internal/config"
)

// ConfigRequest defines workspace config load plus CLI runtime overrides.
type ConfigRequest struct {
	Workspace             string
	ConfigPath            string
	ModelOverride         string
	StreamOverride        string
	ApprovalModeOverride  string
	AwayPolicyOverride    string
	MaxIterationsOverride int
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
	if req.MaxIterationsOverride > 0 {
		cfg.MaxIterations = req.MaxIterationsOverride
	}
	return cfg, nil
}
