package app

import (
	"io"
)

type EntrypointRequest struct {
	WorkspaceOverride         string
	ConfigPath                string
	ModelOverride             string
	SessionID                 string
	StreamOverride            string
	SandboxEnabledOverride    string
	SystemSandboxModeOverride string
	ApprovalModeOverride      string
	AwayPolicyOverride        string
	MaxIterationsOverride     int
	RequireAPIKey             bool
	Stdin                     io.Reader
	Stdout                    io.Writer
}

func BootstrapEntrypoint(req EntrypointRequest) (Runtime, error) {
	workspace, err := ResolveWorkspace(req.WorkspaceOverride)
	if err != nil {
		return Runtime{}, err
	}

	return Bootstrap(BootstrapRequest{
		Workspace:                 workspace,
		ConfigPath:                req.ConfigPath,
		ModelOverride:             req.ModelOverride,
		SessionID:                 req.SessionID,
		StreamOverride:            req.StreamOverride,
		SandboxEnabledOverride:    req.SandboxEnabledOverride,
		SystemSandboxModeOverride: req.SystemSandboxModeOverride,
		ApprovalModeOverride:      req.ApprovalModeOverride,
		AwayPolicyOverride:        req.AwayPolicyOverride,
		MaxIterationsOverride:     req.MaxIterationsOverride,
		RequireAPIKey:             req.RequireAPIKey,
		Stdin:                     req.Stdin,
		Stdout:                    req.Stdout,
	})
}
