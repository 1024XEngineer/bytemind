package agent

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/provider"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/skills"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

const (
	maxActiveSkillDescriptionChars  = 320
	maxActiveSkillInstructionsChars = 3600
	emptyReplyFallback              = "Model returned an empty response (no text and no tool calls). Retry the request or switch model if this persists."
	skillAuthorEnglishFallback      = "Describe the skill goals, workflow, and expected output in concise English."
	skillAuthorTranslatePrompt      = "Translate the user's skill description into concise English for backend metadata. Return only plain English text with no markdown or quotes."
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiCyan    = "\x1b[36m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiRed     = "\x1b[31m"
	toolPreview = 3
)

type Options struct {
	Workspace       string
	Config          config.Config
	Client          llm.Client
	Store           SessionStore
	Registry        ToolRegistry
	Executor        ToolExecutor
	PolicyGateway   PolicyGateway
	Engine          Engine
	TaskManager     runtimepkg.TaskManager
	Runtime         RuntimeGateway
	Extensions      extensionspkg.Manager
	SkillManager    *skills.Manager
	SubAgentManager *subagentspkg.Manager
	AuditStore      storagepkg.AuditStore
	PromptStore     storagepkg.PromptHistoryWriter
	Observer        Observer
	Approval        tools.ApprovalHandler
	Stdin           io.Reader
	Stdout          io.Writer
}

type RunPromptInput struct {
	UserMessage                     llm.Message
	Assets                          map[llm.AssetID]llm.ImageAsset
	DisplayText                     string
	PersistDisplayTextAsUserMessage bool
	SubAgent                        *SubAgentPromptInput
}

type SubAgentPromptInput struct {
	Name           string
	Task           string
	ScopePaths     []string
	ScopeSymbols   []string
	AllowedTools   []string
	Isolation      string
	ResultPolicy   string
	DefinitionBody string
}

type Runner struct {
	workspace        string
	config           config.Config
	client           llm.Client
	store            SessionStore
	registry         ToolRegistry
	executor         ToolExecutor
	policyGateway    PolicyGateway
	engine           Engine
	taskManager      runtimepkg.TaskManager
	runtime          RuntimeGateway
	extensions       extensionspkg.Manager
	skillManager     *skills.Manager
	subAgentManager  *subagentspkg.Manager
	subAgentExecutor SubAgentExecutor
	subAgentNotifier SubAgentNotifier
	worktreeManager  *runtimepkg.WorktreeManager
	auditStore       storagepkg.AuditStore
	promptStore      storagepkg.PromptHistoryWriter
	observer         Observer
	approval         tools.ApprovalHandler
	stdin            io.Reader
	stdout           io.Writer

	bridgeMu            sync.Mutex
	bridgeSessions      map[string]bridgeSessionState
	bridgeSessionTurns  map[string]int
	bridgeToolRefCounts map[string]int

	extensionSyncMu    sync.Mutex
	extensionSyncTTL   time.Duration
	extensionSyncAt    time.Time
	extensionSyncDirty bool
	extensionSyncGen   uint64
	extensionToolKeys  map[string]map[string]struct{}

	modelsCacheMu       sync.RWMutex
	modelsCacheAt       time.Time
	modelsCache         []provider.ModelInfo
	modelsCacheWarnings []provider.Warning
}

func NewRunner(opts Options) *Runner {
	cfg := opts.Config
	if providerID, providerCfg, ok := config.SelectedProviderConfig(cfg.ProviderRuntime); ok {
		cfg.ProviderRuntime.CurrentProvider = providerID
		cfg.ProviderRuntime.DefaultProvider = providerID
		cfg.ProviderRuntime.DefaultModel = strings.TrimSpace(providerCfg.Model)
		cfg.Provider = providerCfg
	} else if model := strings.TrimSpace(config.SelectedModelID(cfg.ProviderRuntime)); model != "" {
		cfg.Provider.Model = model
	}

	manager := opts.SkillManager
	if manager == nil {
		manager = skills.NewManager(opts.Workspace)
	}
	subAgentManager := opts.SubAgentManager
	if subAgentManager == nil {
		subAgentManager = subagentspkg.NewManager(opts.Workspace)
	}
	registry := opts.Registry
	if registry == nil {
		registry = tools.DefaultRegistry()
	}
	executor := opts.Executor
	if executor == nil {
		if concrete, ok := registry.(*tools.Registry); ok {
			executor = tools.NewExecutor(concrete)
		}
	}
	policyGateway := opts.PolicyGateway
	if policyGateway == nil {
		policyGateway = NewDefaultPolicyGateway()
	}
	auditStore := opts.AuditStore
	if auditStore == nil {
		auditStore = storagepkg.NopAuditStore{}
	}
	promptStore := opts.PromptStore
	if promptStore == nil {
		promptStore = storagepkg.NopPromptHistoryStore{}
	}
	taskManager := opts.TaskManager
	if taskManager == nil {
		taskManager = runtimepkg.NewInMemoryTaskManager()
	}
	runtimeGateway := opts.Runtime
	if runtimeGateway == nil {
		runtimeGateway = newDefaultRuntimeGateway(taskManager)
	}
	extensions := opts.Extensions
	if extensions == nil {
		extensions = extensionspkg.NopManager{}
	}
	extensionSyncTTL := time.Duration(cfg.MCP.SyncTTLSeconds) * time.Second
	if extensionSyncTTL <= 0 {
		extensionSyncTTL = 30 * time.Second
	}
	client := opts.Client
	if client != nil {
		client = routeAwareClient{base: client}
	}
	runner := &Runner{
		workspace:       opts.Workspace,
		config:          cfg,
		client:          client,
		store:           opts.Store,
		registry:        registry,
		executor:        executor,
		policyGateway:   policyGateway,
		taskManager:     taskManager,
		runtime:         runtimeGateway,
		extensions:      extensions,
		skillManager:    manager,
		subAgentManager: subAgentManager,
		auditStore:      auditStore,
		promptStore:     promptStore,
		observer:        opts.Observer,
		approval:        opts.Approval,
		stdin:           opts.Stdin,
		stdout:          opts.Stdout,

		extensionSyncTTL:   extensionSyncTTL,
		extensionSyncDirty: true,
		extensionToolKeys:  map[string]map[string]struct{}{},
	}

	engine := opts.Engine
	if engine == nil {
		engine = NewDefaultEngine(runner)
	}
	runner.engine = engine

	runner.subAgentExecutor = NewSubAgentExecutor(runner)
	runner.subAgentNotifier = &defaultSubAgentNotifier{}

	if wm, wmErr := runtimepkg.NewWorktreeManager(opts.Workspace); wmErr == nil {
		runner.worktreeManager = wm
	}

	return runner
}

func (r *Runner) GetClient() llm.Client {
	if r == nil {
		return nil
	}
	if wrapped, ok := r.client.(routeAwareClient); ok {
		return wrapped.base
	}
	return r.client
}

func (r *Runner) GetConfig() config.Config {
	if r == nil {
		return config.Config{}
	}
	return r.config
}

func (r *Runner) ListModels(ctx context.Context) ([]provider.ModelInfo, []provider.Warning, error) {
	if r == nil || r.client == nil {
		return nil, nil, fmt.Errorf("client is unavailable")
	}
	if cachedModels, cachedWarnings, ok := r.listModelsFromCache(); ok {
		return cachedModels, cachedWarnings, nil
	}
	runtimeCfg := r.config.ProviderRuntime
	if len(runtimeCfg.Providers) == 0 {
		runtimeCfg = config.LegacyProviderRuntimeConfig(r.config.Provider)
	}
	registry, err := provider.NewRegistry(runtimeCfg)
	if err != nil {
		return nil, nil, err
	}
	models, warnings, err := provider.ListModels(ctx, registry)
	if err != nil {
		return nil, nil, err
	}
	registryView := provider.NewModelRegistry(runtimeCfg, models)
	resolved := registryView.Models()
	r.storeModelsCache(resolved, warnings)
	return resolved, warnings, nil
}

func (r *Runner) listModelsFromCache() ([]provider.ModelInfo, []provider.Warning, bool) {
	if r == nil {
		return nil, nil, false
	}
	r.modelsCacheMu.RLock()
	defer r.modelsCacheMu.RUnlock()
	if r.modelsCacheAt.IsZero() || time.Since(r.modelsCacheAt) > 30*time.Second {
		return nil, nil, false
	}
	return append([]provider.ModelInfo(nil), r.modelsCache...), append([]provider.Warning(nil), r.modelsCacheWarnings...), true
}

func (r *Runner) storeModelsCache(models []provider.ModelInfo, warnings []provider.Warning) {
	if r == nil {
		return
	}
	r.modelsCacheMu.Lock()
	defer r.modelsCacheMu.Unlock()
	r.modelsCacheAt = time.Now()
	r.modelsCache = append([]provider.ModelInfo(nil), models...)
	r.modelsCacheWarnings = append([]provider.Warning(nil), warnings...)
}

func (r *Runner) clearModelsCache() {
	if r == nil {
		return
	}
	r.modelsCacheMu.Lock()
	defer r.modelsCacheMu.Unlock()
	r.modelsCacheAt = time.Time{}
	r.modelsCache = nil
	r.modelsCacheWarnings = nil
}

func (r *Runner) modelID() string {
	if r == nil {
		return ""
	}
	if model := strings.TrimSpace(config.SelectedModelID(r.config.ProviderRuntime)); model != "" {
		return model
	}
	return strings.TrimSpace(r.config.Provider.Model)
}

type routeAwareClient struct {
	base llm.Client
}

func (c routeAwareClient) CreateMessage(ctx context.Context, request llm.ChatRequest) (llm.Message, error) {
	return c.base.CreateMessage(mergeAllowFallbackRouteContext(ctx), request)
}

func (c routeAwareClient) StreamMessage(ctx context.Context, request llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	return c.base.StreamMessage(mergeAllowFallbackRouteContext(ctx), request, onDelta)
}

func (r *Runner) Close() error { return nil }

func (r *Runner) emitUsageEvent(sess *session.Session, usage *llm.Usage) {
	if sess == nil || usage == nil {
		return
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.ContextTokens == 0 && usage.TotalTokens == 0 {
		return
	}
	r.emit(Event{
		Type:      EventUsageUpdated,
		SessionID: corepkg.SessionID(sess.ID),
		Usage:     *usage,
	})
}

func mergeAllowFallbackRouteContext(ctx context.Context) context.Context {
	rc := provider.RouteContextFromContext(ctx)
	rc.AllowFallback = true
	return provider.WithRouteContext(ctx, rc)
}

func (r *Runner) RunPrompt(ctx context.Context, sess *session.Session, userInput, mode string, out io.Writer) (string, error) {
	return r.RunPromptWithInput(ctx, sess, RunPromptInput{
		UserMessage: llm.NewUserTextMessage(userInput),
		DisplayText: userInput,
	}, mode, out)
}

func (r *Runner) RunPromptWithInput(ctx context.Context, sess *session.Session, input RunPromptInput, mode string, out io.Writer) (string, error) {
	if r.engine == nil {
		return "", fmt.Errorf("agent engine is unavailable")
	}

	events, err := r.engine.HandleTurn(ctx, TurnRequest{
		Session: sess,
		Input:   input,
		Mode:    mode,
		Out:     out,
	})
	if err != nil {
		return "", err
	}
	if events == nil {
		return "", fmt.Errorf("engine returned nil event stream")
	}

	handleEvent := func(event TurnEvent) (string, error, bool) {
		switch event.Type {
		case TurnEventCompleted:
			return event.Answer, nil, true
		case TurnEventFailed:
			if event.Error != nil {
				return "", event.Error, true
			}
			return "", fmt.Errorf("agent turn failed"), true
		default:
			return "", nil, false
		}
	}

	for {
		// Prefer already-ready engine events (especially terminal ones) over cancellation.
		select {
		case event, ok := <-events:
			if !ok {
				return "", fmt.Errorf("engine ended without terminal event")
			}
			answer, eventErr, done := handleEvent(event)
			if done {
				return answer, eventErr
			}
			continue
		default:
		}

		select {
		case event, ok := <-events:
			if !ok {
				return "", fmt.Errorf("engine ended without terminal event")
			}
			answer, eventErr, done := handleEvent(event)
			if done {
				return answer, eventErr
			}
		case <-ctx.Done():
			// If cancellation races with terminal events, prefer already-ready terminal events.
			for {
				select {
				case event, ok := <-events:
					if !ok {
						return "", ctx.Err()
					}
					answer, eventErr, done := handleEvent(event)
					if done {
						return answer, eventErr
					}
				default:
					return "", ctx.Err()
				}
			}
		}
	}
}
