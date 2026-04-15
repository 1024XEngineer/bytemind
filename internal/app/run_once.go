package app

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"
)

type RunOneShotRequest struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func RunOneShotArgs(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return RunOneShot(RunOneShotRequest{
		Args:   args,
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
}

func RunOneShot(req RunOneShotRequest) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(req.Stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Reuse an existing session")
	prompt := fs.String("prompt", "", "Prompt to send")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")

	if err := fs.Parse(req.Args); err != nil {
		return err
	}

	if strings.TrimSpace(*prompt) == "" {
		*prompt = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(*prompt) == "" {
		return errors.New("run requires -prompt or trailing prompt text")
	}

	runtimeBundle, err := BootstrapEntrypoint(EntrypointRequest{
		WorkspaceOverride:     *workspaceOverride,
		ConfigPath:            *configPath,
		ModelOverride:         *model,
		SessionID:             *sessionID,
		StreamOverride:        *streamOverride,
		MaxIterationsOverride: *maxIterations,
		RequireAPIKey:         true,
		Stdin:                 req.Stdin,
		Stdout:                req.Stdout,
	})
	if err != nil {
		return err
	}

	_, err = runtimeBundle.Runner.RunPrompt(context.Background(), runtimeBundle.Session, *prompt, "build", req.Stdout)
	return err
}
