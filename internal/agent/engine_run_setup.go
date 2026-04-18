package agent

import (
	"fmt"
	"time"

	contextpkg "bytemind/internal/context"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	policypkg "bytemind/internal/policy"
	"bytemind/internal/session"
)

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
	allowedTools, deniedTools := resolveSkillToolSets(activeSkill)
	promptHint := policypkg.EvaluatePromptHint(userInput)
	availableTools := []string(nil)
	if runner.registry != nil {
		availableTools = toolNames(runner.registry.DefinitionsForMode(runMode))
	}
	return runPromptSetup{
		Input:                input,
		UserInput:            userInput,
		RunMode:              runMode,
		Mode:                 mode,
		ActiveSkill:          activeSkill,
		AllowedTools:         allowedTools,
		DeniedTools:          deniedTools,
		AllowedToolNames:     policypkg.SortedToolNames(allowedTools),
		DeniedToolNames:      policypkg.SortedToolNames(deniedTools),
		AvailableSkills:      runner.promptSkills(),
		AvailableTools:       availableTools,
		InstructionText:      loadAGENTSInstruction(runner.workspace),
		WebLookupInstruction: promptHint.Instruction,
		PromptTokens:         contextpkg.EstimateRequestTokens([]llm.Message{input.UserMessage}),
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
			Workspace:      runner.workspace,
			ApprovalPolicy: runner.config.ApprovalPolicy,
			Model:          runner.modelID(),
			Mode:           setup.Mode,
			Skills:         setup.AvailableSkills,
			Tools:          setup.AvailableTools,
			ActiveSkill:    promptActiveSkill(setup.ActiveSkill),
			Instruction:    setup.InstructionText,
		}),
		WebLookupInstruction: setup.WebLookupInstruction,
		ConversationMessages: sess.Messages,
	})
}
