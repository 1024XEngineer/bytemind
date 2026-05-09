package agent

import (
	"fmt"
	"strings"
	"time"

	contextpkg "github.com/1024XEngineer/bytemind/internal/context"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	policypkg "github.com/1024XEngineer/bytemind/internal/policy"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

var resolveAgentSystemSandboxRuntimeStatus = tools.ResolveSystemSandboxRuntimeStatus

const userMessageModelMetaKey = "user_model"

func (e *defaultEngine) prepareRunPrompt(sess *session.Session, input RunPromptInput, mode string) (runPromptSetup, error) {
	if e == nil || e.runner == nil {
		return runPromptSetup{}, fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	input = normalizeRunPromptInput(input)
	userInput := input.DisplayText

	// Enhance user message with agent mentions
	if input.SubAgent == nil && runner.subAgentManager != nil {
		agents, _ := runner.subAgentManager.List()
		if len(agents) > 0 {
			knownAgents := make(map[string]struct{}, len(agents))
			agentDescs := make(map[string]string, len(agents))
			for _, a := range agents {
				name := strings.TrimSpace(a.Name)
				if name != "" {
					knownAgents[name] = struct{}{}
					agentDescs[name] = strings.TrimSpace(a.Description)
				}
			}
			enhanced := enhanceUserMessageWithAgentMentions(userInput, knownAgents, agentDescs)
			if enhanced != userInput {
				input.UserMessage = llm.NewUserTextMessage(enhanced)
				input.PersistDisplayTextAsUserMessage = true
			}
		}
	}

	persistedUserMessage := input.UserMessage
	if input.PersistDisplayTextAsUserMessage && strings.TrimSpace(userInput) != "" {
		persistedUserMessage = llm.NewUserTextMessage(userInput)
	}
	runMode := resolveRunMode(sess, mode)
	mode = string(runMode)
	if sess.Mode != runMode {
		sess.Mode = runMode
	}
	planpkg.SeedForRun(&sess.Plan, runMode, userInput, input.UserMessage.Text())

	persistedUserMessageIndex := len(sess.Messages)
	if err := e.beginRunSession(sess, persistedUserMessage, userInput); err != nil {
		return runPromptSetup{}, err
	}

	activeSkill := runner.resolveActiveSkill(sess)
	if err := runner.syncActiveSkillBridges(sess, activeSkill); err != nil {
		return runPromptSetup{}, err
	}
	allowedTools, deniedTools, err := resolveSkillToolSets(activeSkill, runner.registry)
	if err != nil {
		return runPromptSetup{}, err
	}
	promptHint := policypkg.EvaluatePromptHint(userInput)
	availableTools := []string(nil)
	if runner.registry != nil {
		availableTools = toolNames(runner.registry.DefinitionsForMode(runMode))
	}

	systemSandboxBackend := "none"
	systemSandboxRequiredCapable := false
	systemSandboxCapabilityLevel := "none"
	systemSandboxShellNetwork := false
	systemSandboxWorkerNetwork := false
	systemSandboxFallback := false
	systemSandboxStatus := ""
	if runtimeStatus, statusErr := resolveAgentSystemSandboxRuntimeStatus(runner.config.SandboxEnabled, runner.config.SystemSandboxMode); statusErr != nil {
		if runner.config.SandboxEnabled && strings.EqualFold(strings.TrimSpace(runner.config.SystemSandboxMode), "required") {
			return runPromptSetup{}, fmt.Errorf("system sandbox mode %q is unavailable: %w", "required", statusErr)
		}
		systemSandboxStatus = strings.TrimSpace(statusErr.Error())
	} else {
		if backend := strings.TrimSpace(runtimeStatus.BackendName); backend != "" {
			systemSandboxBackend = backend
		}
		systemSandboxRequiredCapable = runtimeStatus.RequiredCapable
		if level := strings.TrimSpace(runtimeStatus.CapabilityLevel); level != "" {
			systemSandboxCapabilityLevel = level
		}
		systemSandboxShellNetwork = runtimeStatus.ShellNetworkIsolation
		systemSandboxWorkerNetwork = runtimeStatus.WorkerNetworkIsolation
		systemSandboxFallback = runtimeStatus.Fallback
		systemSandboxStatus = strings.TrimSpace(runtimeStatus.Message)
	}

	return runPromptSetup{
		Input:                        input,
		UserInput:                    userInput,
		PersistedUserMessageIndex:    persistedUserMessageIndex,
		RunMode:                      runMode,
		Mode:                         mode,
		SystemSandboxBackend:         systemSandboxBackend,
		SystemSandboxRequiredCapable: systemSandboxRequiredCapable,
		SystemSandboxCapabilityLevel: systemSandboxCapabilityLevel,
		SystemSandboxShellNetwork:    systemSandboxShellNetwork,
		SystemSandboxWorkerNetwork:   systemSandboxWorkerNetwork,
		SystemSandboxFallback:        systemSandboxFallback,
		SystemSandboxStatus:          systemSandboxStatus,
		ActiveSkill:                  activeSkill,
		AllowedTools:                 allowedTools,
		DeniedTools:                  deniedTools,
		AllowedToolNames:             policypkg.SortedToolNames(allowedTools),
		DeniedToolNames:              policypkg.SortedToolNames(deniedTools),
		AvailableSkills:              runner.promptSkills(),
		AvailableSubAgents:           runner.promptSubAgents(),
		AvailableTools:               availableTools,
		SubAgentRuntime:              promptSubAgentRuntime(input.SubAgent),
		SubAgentDefinition:           promptSubAgentDefinition(input.SubAgent),
		InstructionText:              loadAGENTSInstruction(runner.workspace),
		WebLookupInstruction:         promptHint.Instruction,
		PromptTokens:                 contextpkg.EstimateRequestTokens([]llm.Message{input.UserMessage}),
	}, nil
}

func (e *defaultEngine) beginRunSession(sess *session.Session, userMessage llm.Message, userInput string) error {
	if e == nil || e.runner == nil {
		return fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner
	now := time.Now().UTC()

	// Persist minimal user-turn metadata so TUI can rebuild a consistent header
	// after session reload/switch.
	if strings.TrimSpace(userMessage.CreatedAt) == "" {
		userMessage.CreatedAt = now.Format(time.RFC3339Nano)
	}
	if userMessage.Role == llm.RoleUser {
		if model := strings.TrimSpace(runner.modelID()); model != "" {
			if userMessage.Meta == nil {
				userMessage.Meta = llm.MessageMeta{}
			}
			userMessage.Meta[userMessageModelMetaKey] = model
		}
	}

	if err := llm.ValidateMessage(userMessage); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, userMessage)
	if runner.store != nil {
		if err := runner.store.Save(sess); err != nil {
			return err
		}
	}
	runner.appendPromptHistory(corepkg.SessionID(sess.ID), userInput, now)
	runner.emit(Event{
		Type:      EventRunStarted,
		SessionID: corepkg.SessionID(sess.ID),
		UserInput: userInput,
	})
	return nil
}

func (e *defaultEngine) buildTurnMessages(sess *session.Session, setup runPromptSetup) ([]llm.Message, error) {
	if e == nil || e.runner == nil {
		return nil, fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	conversationMessages := conversationMessagesForTurn(sess.Messages, setup.Input, setup.PersistedUserMessageIndex)

	return contextpkg.BuildTurnMessages(contextpkg.TurnMessagesRequest{
		SystemPrompt: systemPrompt(PromptInput{
			Workspace:                    runner.workspace,
			ApprovalPolicy:               runner.config.ApprovalPolicy,
			ApprovalMode:                 runner.config.ApprovalMode,
			AwayPolicy:                   runner.config.AwayPolicy,
			SandboxEnabled:               runner.config.SandboxEnabled,
			SystemSandbox:                runner.config.SystemSandboxMode,
			SystemSandboxBackend:         setup.SystemSandboxBackend,
			SystemSandboxRequiredCapable: setup.SystemSandboxRequiredCapable,
			SystemSandboxCapabilityLevel: setup.SystemSandboxCapabilityLevel,
			SystemSandboxShellNetwork:    setup.SystemSandboxShellNetwork,
			SystemSandboxWorkerNetwork:   setup.SystemSandboxWorkerNetwork,
			SystemSandboxFallback:        setup.SystemSandboxFallback,
			SystemSandboxStatus:          setup.SystemSandboxStatus,
			Model:                        runner.config.Provider.Model,
			Mode:                         setup.Mode,
			Skills:                       setup.AvailableSkills,
			SubAgents:                    setup.AvailableSubAgents,
			Tools:                        setup.AvailableTools,
			SubAgentRuntime:              setup.SubAgentRuntime,
			SubAgentDefinition:           setup.SubAgentDefinition,
			Plan:                         sess.Plan,
			ActiveSkill:                  promptActiveSkill(setup.ActiveSkill),
			Instruction:                  setup.InstructionText,
		}),
		WebLookupInstruction: setup.WebLookupInstruction,
		ConversationMessages: conversationMessages,
	})
}

func conversationMessagesForTurn(messages []llm.Message, input RunPromptInput, persistedUserMessageIndex int) []llm.Message {
	if !input.PersistDisplayTextAsUserMessage {
		return messages
	}
	if persistedUserMessageIndex < 0 || persistedUserMessageIndex >= len(messages) {
		return messages
	}
	override := input.UserMessage
	override.Normalize()
	if override.Role != llm.RoleUser {
		return messages
	}
	overridden := append([]llm.Message(nil), messages...)
	overridden[persistedUserMessageIndex] = override
	return overridden
}

func promptSubAgentRuntime(input *SubAgentPromptInput) *PromptSubAgentRuntime {
	if input == nil {
		return nil
	}
	runtime := &PromptSubAgentRuntime{
		Name:         strings.TrimSpace(input.Name),
		Task:         strings.TrimSpace(input.Task),
		ScopePaths:   append([]string(nil), input.ScopePaths...),
		ScopeSymbols: append([]string(nil), input.ScopeSymbols...),
		AllowedTools: append([]string(nil), input.AllowedTools...),
		Isolation:    strings.TrimSpace(input.Isolation),
		ResultPolicy: strings.TrimSpace(input.ResultPolicy),
	}
	if runtime.Name == "" &&
		runtime.Task == "" &&
		len(runtime.ScopePaths) == 0 &&
		len(runtime.ScopeSymbols) == 0 &&
		len(runtime.AllowedTools) == 0 &&
		runtime.Isolation == "" &&
		runtime.ResultPolicy == "" {
		return nil
	}
	return runtime
}

func promptSubAgentDefinition(input *SubAgentPromptInput) string {
	if input == nil {
		return ""
	}
	return strings.TrimSpace(input.DefinitionBody)
}
