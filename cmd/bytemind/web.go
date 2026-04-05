package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"

	"bytemind/internal/config"
	"bytemind/internal/web"
)

func runWeb(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Resume an existing session")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")
	addr := fs.String("addr", "127.0.0.1:8080", "HTTP listen address for web UI")

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

	fmt.Fprintf(stdout, "ByteMind Web UI listening on http://%s\n", *addr)
	return web.Run(web.Options{
		Runner:    app,
		Store:     store,
		Session:   sess,
		Config:    cfg,
		Workspace: sess.Workspace,
		Addr:      *addr,
	})
}
