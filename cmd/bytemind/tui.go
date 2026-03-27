package main

import (
	"flag"
	"io"
	"strconv"

	"bytemind/internal/config"
	"bytemind/internal/tui"
)

func runTUI(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Resume an existing session")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")

	if err := fs.Parse(args); err != nil {
		return err
	}

	app, store, sess, err := bootstrap(*configPath, *model, *sessionID, *streamOverride, *workspaceOverride, *maxIterations, stdin, stdout)
	if err != nil {
		return err
	}

	workspace, err := resolveWorkspace(*workspaceOverride)
	if err != nil {
		return err
	}
	cfg, err := config.Load(workspace, *configPath)
	if err != nil {
		return err
	}
	if *model != "" {
		cfg.Provider.Model = *model
	}
	if *streamOverride != "" {
		parsed, err := strconv.ParseBool(*streamOverride)
		if err != nil {
			return err
		}
		cfg.Stream = parsed
	}
	if *maxIterations > 0 {
		cfg.MaxIterations = *maxIterations
	}

	return tui.Run(tui.Options{
		Runner:    app,
		Store:     store,
		Session:   sess,
		Config:    cfg,
		Workspace: sess.Workspace,
	})
}
