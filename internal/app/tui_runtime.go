package app

import (
	"errors"
	"flag"
	"io"
	"os"

	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/mcpctl"
	"bytemind/tui"
)

type TUIRequest struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type TUIRuntime struct {
	Options tui.Options
	close   func() error
}

func (r TUIRuntime) Close() error {
	if r.close == nil {
		return nil
	}
	return r.close()
}

func BuildTUIRuntime(req TUIRequest) (TUIRuntime, error) {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(req.Stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Resume an existing session")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	approvalMode := fs.String("approval-mode", "", "Override approval mode: interactive or away")
	awayPolicy := fs.String("away-policy", "", "Override away mode policy: auto_deny_continue or fail_fast")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")

	if err := fs.Parse(req.Args); err != nil {
		return TUIRuntime{}, err
	}

	workspace, err := ResolveWorkspace(*workspaceOverride)
	if err != nil {
		return TUIRuntime{}, err
	}

	cfg, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:             workspace,
		ConfigPath:            *configPath,
		ModelOverride:         *model,
		StreamOverride:        *streamOverride,
		ApprovalModeOverride:  *approvalMode,
		AwayPolicyOverride:    *awayPolicy,
		MaxIterationsOverride: *maxIterations,
	})
	if err != nil {
		return TUIRuntime{}, err
	}

	interactive := isInteractiveStdin(req.Stdin)
	guide, requireAPIKey := resolveTUIStartupPolicy(interactive)
	runtimeBundle, err := BootstrapEntrypoint(EntrypointRequest{
		WorkspaceOverride:     *workspaceOverride,
		ConfigPath:            *configPath,
		ModelOverride:         *model,
		SessionID:             *sessionID,
		StreamOverride:        *streamOverride,
		ApprovalModeOverride:  *approvalMode,
		AwayPolicyOverride:    *awayPolicy,
		MaxIterationsOverride: *maxIterations,
		RequireAPIKey:         requireAPIKey,
		Stdin:                 req.Stdin,
		Stdout:                req.Stdout,
	})
	if err != nil {
		return TUIRuntime{}, err
	}

	maybePrintUpdateReminder(cfg, req.Stderr)

	runner := runtimeBundle.Runner
	if runner == nil || runtimeBundle.Store == nil || runtimeBundle.Session == nil {
		return TUIRuntime{}, errors.New("internal error: bootstrap returned nil runtime")
	}
	home, err := config.EnsureHomeLayout()
	if err != nil {
		return TUIRuntime{}, err
	}
	imageStore, err := assets.NewFileAssetStore(home)
	if err != nil {
		return TUIRuntime{}, err
	}

	return TUIRuntime{
		Options: tui.Options{
			Runner:       newTUIRunnerAdapter(runner),
			Store:        runtimeBundle.Store,
			MCPService:   mcpctl.NewService(workspace, *configPath, runtimeBundle.Extensions),
			Session:      runtimeBundle.Session,
			ImageStore:   imageStore,
			Config:       cfg,
			Workspace:    runtimeBundle.Session.Workspace,
			StartupGuide: guide,
		},
		close: runner.Close,
	}, nil
}

func resolveTUIStartupPolicy(interactive bool) (tui.StartupGuide, bool) {
	// Startup guide at UI entry is disabled by default.
	// Keep interactive TUI accessible even when API key is not configured yet.
	return tui.StartupGuide{}, !interactive
}

func isInteractiveStdin(stdin io.Reader) bool {
	file, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
