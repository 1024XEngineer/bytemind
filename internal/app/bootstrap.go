package app

import (
	"errors"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/agent"
	"github.com/1024XEngineer/bytemind/internal/config"
	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
	extensionsruntime "github.com/1024XEngineer/bytemind/internal/extensionsruntime"
	"github.com/1024XEngineer/bytemind/internal/provider"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
	"github.com/1024XEngineer/bytemind/internal/session"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

var validateSystemSandboxRuntime = tools.ValidateSystemSandboxRuntime
var resolveSystemSandboxRuntimeStatus = tools.ResolveSystemSandboxRuntimeStatus

// BootstrapRequest declares dependencies and runtime overrides needed to
// assemble the application runtime for CLI/TUI execution.
type BootstrapRequest struct {
	Workspace                 string
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

// Runtime is the assembled runtime bundle consumed by command entrypoints.
type Runtime struct {
	Config      config.Config
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
		Workspace:                 workspace,
		ConfigPath:                req.ConfigPath,
		ModelOverride:             req.ModelOverride,
		StreamOverride:            req.StreamOverride,
		SandboxEnabledOverride:    req.SandboxEnabledOverride,
		SystemSandboxModeOverride: req.SystemSandboxModeOverride,
		ApprovalModeOverride:      req.ApprovalModeOverride,
		AwayPolicyOverride:        req.AwayPolicyOverride,
		MaxIterationsOverride:     req.MaxIterationsOverride,
	})
	if err != nil {
		return Runtime{}, err
	}
	if req.StreamOverride == "" {
		req.StreamOverride = strings.TrimSpace(strconv.FormatBool(cfg.Stream))
	}
	if err := validateSystemSandboxRuntime(cfg.SandboxEnabled, cfg.SystemSandboxMode); err != nil {
		return Runtime{}, err
	}
	if status, statusErr := resolveSystemSandboxRuntimeStatus(cfg.SandboxEnabled, cfg.SystemSandboxMode); statusErr != nil {
		return Runtime{}, statusErr
	} else if status.Fallback && strings.TrimSpace(status.Message) != "" {
		log.Printf("bootstrap: %s", status.Message)
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
	runtimeCfg := cfg.ProviderRuntime
	if len(runtimeCfg.Providers) == 0 {
		runtimeCfg = config.LegacyProviderRuntimeConfig(cfg.Provider)
	}
	client, err := provider.NewClientFromRuntime(runtimeCfg, nil)
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

	taskEventStore := runtimepkg.TaskEventStore(runtimepkg.NopTaskEventStore{})
	taskStore, taskStoreErr := storagepkg.NewDefaultTaskStoreWithOptions(nil, storagepkg.TaskStoreOptions{
		SyncOnAppend: false,
	})
	if taskStoreErr == nil {
		taskEventStore = storagepkg.NewRuntimeTaskEventAdapter(taskStore)
	} else {
		log.Printf("bootstrap: failed to initialize unified task store: %v", taskStoreErr)
		legacyTaskStore, legacyErr := storagepkg.NewDefaultRuntimeTaskStore()
		if legacyErr == nil {
			log.Printf("bootstrap: falling back to legacy runtime task store")
			taskEventStore = legacyTaskStore
		} else {
			log.Printf("bootstrap: failed to initialize legacy runtime task store: %v", legacyErr)
			log.Printf("bootstrap: runtime task events disabled; using NopTaskEventStore")
		}
	}

	taskManager := runtimepkg.NewInMemoryTaskManager(
		runtimepkg.WithTaskEventStore(taskEventStore),
	)
	baseExtensions := extensionspkg.NewManager(workspace)
	extensions := extensionsruntime.NewManager(workspace, req.ConfigPath, baseExtensions, cfg)
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
		Config:      cfg,
		Runner:      runner,
		Store:       store,
		Session:     sess,
		TaskManager: taskManager,
		Extensions:  extensions,
	}, nil
}
