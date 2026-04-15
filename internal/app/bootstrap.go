package app

import (
	"errors"
	"io"
	"path/filepath"
	"strings"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	extensionspkg "bytemind/internal/extensions"
	"bytemind/internal/provider"
	runtimepkg "bytemind/internal/runtime"
	"bytemind/internal/session"
	storagepkg "bytemind/internal/storage"
	"bytemind/internal/tools"
)

// BootstrapRequest declares dependencies and runtime overrides needed to
// assemble the application runtime for CLI/TUI execution.
type BootstrapRequest struct {
	Workspace             string
	ConfigPath            string
	ModelOverride         string
	SessionID             string
	StreamOverride        string
	MaxIterationsOverride int
	RequireAPIKey         bool
	Stdin                 io.Reader
	Stdout                io.Writer
}

// Runtime is the assembled runtime bundle consumed by command entrypoints.
type Runtime struct {
	Runner      *agent.Runner
	Store       *session.Store
	Session     *session.Session
	TaskManager runtimepkg.TaskManager
	Extensions  extensionspkg.Manager
}

func Bootstrap(req BootstrapRequest) (Runtime, error) {
	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		return Runtime{}, errors.New("workspace is required")
	}

	cfg, err := LoadRuntimeConfig(ConfigRequest{
		Workspace:             workspace,
		ConfigPath:            req.ConfigPath,
		ModelOverride:         req.ModelOverride,
		StreamOverride:        req.StreamOverride,
		MaxIterationsOverride: req.MaxIterationsOverride,
	})
	if err != nil {
		return Runtime{}, err
	}
	if req.MaxIterationsOverride < 0 {
		return Runtime{}, errors.New("-max-iterations must be greater than 0")
	}

	apiKey := cfg.Provider.ResolveAPIKey()
	if req.RequireAPIKey && apiKey == "" {
		return Runtime{}, errors.New("missing API key; configure provider.api_key in the config file")
	}

	home, err := config.EnsureHomeLayout()
	if err != nil {
		return Runtime{}, err
	}
	store, err := session.NewStore(filepath.Join(home, "sessions"))
	if err != nil {
		return Runtime{}, err
	}

	var sess *session.Session
	if req.SessionID != "" {
		sess, err = store.Load(req.SessionID)
		if err != nil {
			return Runtime{}, err
		}
	} else {
		sess = session.New(workspace)
		if err := store.Save(sess); err != nil {
			return Runtime{}, err
		}
	}

	cfg.Provider.APIKey = apiKey
	client, err := provider.NewClient(cfg.Provider)
	if err != nil {
		return Runtime{}, err
	}
	auditStore, err := storagepkg.NewDefaultAuditStore()
	if err != nil {
		auditStore = storagepkg.NopAuditStore{}
	}
	var promptStore storagepkg.PromptHistoryWriter
	promptStore, err = storagepkg.NewDefaultPromptHistoryStore()
	if err != nil {
		promptStore = storagepkg.NopPromptHistoryStore{}
	}

	taskManager := runtimepkg.NewInMemoryTaskManager()
	extensions := extensionspkg.NopManager{}
	runner := agent.NewRunner(agent.Options{
		Workspace:   workspace,
		Config:      cfg,
		Client:      client,
		Store:       store,
		Registry:    tools.DefaultRegistry(),
		TaskManager: taskManager,
		Extensions:  extensions,
		AuditStore:  auditStore,
		PromptStore: promptStore,
		Stdin:       req.Stdin,
		Stdout:      req.Stdout,
	})

	return Runtime{
		Runner:      runner,
		Store:       store,
		Session:     sess,
		TaskManager: taskManager,
		Extensions:  extensions,
	}, nil
}
