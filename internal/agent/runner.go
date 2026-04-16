package agent

import (
	"context"
	"fmt"
	"io"

	"bytemind/internal/config"
	extensionspkg "bytemind/internal/extensions"
	"bytemind/internal/llm"
	runtimepkg "bytemind/internal/runtime"
	"bytemind/internal/session"
	"bytemind/internal/skills"
	storagepkg "bytemind/internal/storage"
	"bytemind/internal/tokenusage"
	"bytemind/internal/tools"
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
	Workspace     string
	Config        config.Config
	Client        llm.Client
	Store         SessionStore
	Registry      ToolRegistry
	Executor      ToolExecutor
	PolicyGateway PolicyGateway
	Engine        Engine
	TaskManager   runtimepkg.TaskManager
	Extensions    extensionspkg.Manager
	SkillManager  *skills.Manager
	TokenManager  *tokenusage.TokenUsageManager
	AuditStore    storagepkg.AuditStore
	PromptStore   storagepkg.PromptHistoryWriter
	Observer      Observer
	Approval      tools.ApprovalHandler
	Stdin         io.Reader
	Stdout        io.Writer
}

type RunPromptInput struct {
	UserMessage llm.Message
	Assets      map[llm.AssetID]llm.ImageAsset
	DisplayText string
}

type Runner struct {
	workspace     string
	config        config.Config
	client        llm.Client
	store         SessionStore
	registry      ToolRegistry
	executor      ToolExecutor
	policyGateway PolicyGateway
	engine        Engine
	taskManager   runtimepkg.TaskManager
	extensions    extensionspkg.Manager
	skillManager  *skills.Manager
	tokenManager  *tokenusage.TokenUsageManager
	auditStore    storagepkg.AuditStore
	promptStore   storagepkg.PromptHistoryWriter
	observer      Observer
	approval      tools.ApprovalHandler
	stdin         io.Reader
	stdout        io.Writer
}

func NewRunner(opts Options) *Runner {
	manager := opts.SkillManager
	if manager == nil {
		manager = skills.NewManager(opts.Workspace)
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
	extensions := opts.Extensions
	if extensions == nil {
		extensions = extensionspkg.NopManager{}
	}
	runner := &Runner{
		workspace:     opts.Workspace,
		config:        opts.Config,
		client:        opts.Client,
		store:         opts.Store,
		registry:      registry,
		executor:      executor,
		policyGateway: policyGateway,
		taskManager:   taskManager,
		extensions:    extensions,
		skillManager:  manager,
		tokenManager:  opts.TokenManager,
		auditStore:    auditStore,
		promptStore:   promptStore,
		observer:      opts.Observer,
		approval:      opts.Approval,
		stdin:         opts.Stdin,
		stdout:        opts.Stdout,
	}

	engine := opts.Engine
	if engine == nil {
		engine = NewDefaultEngine(runner)
	}
	runner.engine = engine

	return runner
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
