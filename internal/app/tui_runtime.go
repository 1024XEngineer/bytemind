package app

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"time"

	"github.com/1024XEngineer/bytemind/internal/assets"
	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/mcpctl"
	notifypkg "github.com/1024XEngineer/bytemind/internal/notify"
	"github.com/1024XEngineer/bytemind/tui"
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

const tuiRuntimeNotifierCloseTimeout = 2 * time.Second

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
	approvalMode := fs.String("approval-mode", "", "Override approval mode: interactive or full_access")
	awayPolicy := fs.String("away-policy", "", "Deprecated compatibility field: auto_deny_continue or fail_fast")
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
	notifier := notifypkg.NewDesktopNotifier(notifypkg.DesktopConfig{
		Enabled:         cfg.Notifications.Desktop.Enabled,
		CooldownSeconds: cfg.Notifications.Desktop.CooldownSeconds,
	})

	return TUIRuntime{
		Options: tui.Options{
			Runner:       newTUIRunnerAdapter(runner),
			Store:        runtimeBundle.Store,
			MCPService:   mcpctl.NewService(workspace, *configPath, runtimeBundle.Extensions),
			Session:      runtimeBundle.Session,
			ImageStore:   imageStore,
			Notifier:     notifier,
			Config:       cfg,
			Workspace:    runtimeBundle.Session.Workspace,
			Version:      CurrentVersion(),
			StartupGuide: guide,
		},
		close: chainTUIRuntimeClose(runner.Close, notifier),
	}, nil
}

func chainTUIRuntimeClose(runnerClose func() error, notifier notifypkg.Notifier) func() error {
	return func() error {
		runnerErr := error(nil)
		if runnerClose != nil {
			runnerErr = runnerClose()
		}
		notifierErr := error(nil)
		if notifier != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), tuiRuntimeNotifierCloseTimeout)
			defer cancel()
			notifierErr = notifier.Close(closeCtx)
		}
		if runnerErr != nil {
			return runnerErr
		}
		if notifierErr != nil {
			return notifierErr
		}
		return nil
	}
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
