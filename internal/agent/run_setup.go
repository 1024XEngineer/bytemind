package agent

import (
	"strings"
	"time"

	contextpkg "bytemind/internal/context"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	policypkg "bytemind/internal/policy"
	"bytemind/internal/session"
)

type runPromptSetup struct {
	Input                RunPromptInput
	UserInput            string
	RunMode              planpkg.AgentMode
	Mode                 string
	ActiveSkill          *activeSkillRuntime
	AllowedTools         map[string]struct{}
	DeniedTools          map[string]struct{}
	AllowedToolNames     []string
	DeniedToolNames      []string
	AvailableSkills      []PromptSkill
	AvailableTools       []string
	InstructionText      string
	WebLookupInstruction string
	PromptTokens         int
}

func (r *Runner) prepareRunPrompt(sess *session.Session, input RunPromptInput, mode string) (runPromptSetup, error) {
	input = normalizeRunPromptInput(input)
	userInput := input.DisplayText
	runMode := resolveRunMode(sess, mode)
	mode = string(runMode)
	if sess.Mode != runMode {
		sess.Mode = runMode
	}
	planpkg.SeedForRun(&sess.Plan, runMode, userInput, input.UserMessage.Text())

	if err := r.beginRunSession(sess, input.UserMessage, userInput); err != nil {
		return runPromptSetup{}, err
	}

	activeSkill := r.resolveActiveSkill(sess)
	allowedTools, deniedTools := resolveSkillToolSets(activeSkill)
	promptHint := policypkg.EvaluatePromptHint(userInput)
	availableTools := []string(nil)
	if r.registry != nil {
		availableTools = toolNames(r.registry.DefinitionsForMode(runMode))
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
		AvailableSkills:      r.promptSkills(),
		AvailableTools:       availableTools,
		InstructionText:      loadAGENTSInstruction(r.workspace),
		WebLookupInstruction: promptHint.Instruction,
		PromptTokens:         contextpkg.EstimateRequestTokens([]llm.Message{input.UserMessage}),
	}, nil
}

func normalizeRunPromptInput(input RunPromptInput) RunPromptInput {
	input.UserMessage.Normalize()
	if input.UserMessage.Role == "" {
		input.UserMessage = llm.NewUserTextMessage(input.DisplayText)
	}
	if strings.TrimSpace(input.DisplayText) == "" {
		input.DisplayText = input.UserMessage.Text()
	}
	return input
}

func resolveRunMode(sess *session.Session, mode string) planpkg.AgentMode {
	runMode := planpkg.NormalizeMode(mode)
	if strings.TrimSpace(mode) == "" {
		runMode = planpkg.NormalizeMode(string(sess.Mode))
	}
	return runMode
}

func (r *Runner) beginRunSession(sess *session.Session, userMessage llm.Message, userInput string) error {
	if err := llm.ValidateMessage(userMessage); err != nil {
		return err
	}
	sess.Messages = append(sess.Messages, userMessage)
	if r.store != nil {
		if err := r.store.Save(sess); err != nil {
			return err
		}
	}
	r.appendPromptHistory(corepkg.SessionID(sess.ID), userInput, time.Now().UTC())
	r.emit(Event{
		Type:      EventRunStarted,
		SessionID: corepkg.SessionID(sess.ID),
		UserInput: userInput,
	})
	return nil
}

func (r *Runner) buildTurnMessages(sess *session.Session, setup runPromptSetup) ([]llm.Message, error) {
	return contextpkg.BuildTurnMessages(contextpkg.TurnMessagesRequest{
		SystemPrompt: systemPrompt(PromptInput{
			Workspace:      r.workspace,
			ApprovalPolicy: r.config.ApprovalPolicy,
			Model:          r.config.Provider.Model,
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
