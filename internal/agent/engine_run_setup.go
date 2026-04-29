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

func (e *defaultEngine) prepareRunPrompt(sess *session.Session, input RunPromptInput, mode string) (runPromptSetup, error) {
	if e == nil || e.runner == nil {
		return runPromptSetup{}, fmt.Errorf("agent engine is unavailable")
	}
	runner := e.runner

	input = normalizeRunPromptInput(input)
	userInput := input.DisplayText
	runMode := resolveRunMode(sess, mode)
	mode = string(runMode)
	if sess.Mode != runMode {
		sess.Mode = runMode
	}
	planpkg.SeedForRun(&sess.Plan, runMode, userInput, input.UserMessage.Text())

	if err := e.beginRunSession(sess, input.UserMessage, userInput); err != nil {
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
		AvailableTools:               availableTools,
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

	if err := llm.ValidateMessage(userMessage); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, userMessage)
	if runner.store != nil {
		if err := runner.store.Save(sess); err != nil {
			return err
		}
	}
	runner.appendPromptHistory(corepkg.SessionID(sess.ID), userInput, time.Now().UTC())
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
			Tools:                        setup.AvailableTools,
			Plan:                         sess.Plan,
			ActiveSkill:                  promptActiveSkill(setup.ActiveSkill),
			Instruction:                  setup.InstructionText,
		}),
		WebLookupInstruction: setup.WebLookupInstruction,
		ConversationMessages: sess.Messages,
	})
}
