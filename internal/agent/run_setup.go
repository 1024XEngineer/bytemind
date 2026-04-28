package agent

import (
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
)

type runPromptSetup struct {
	Input                        RunPromptInput
	UserInput                    string
	RunMode                      planpkg.AgentMode
	Mode                         string
	SystemSandboxBackend         string
	SystemSandboxRequiredCapable bool
	SystemSandboxCapabilityLevel string
	SystemSandboxShellNetwork    bool
	SystemSandboxWorkerNetwork   bool
	SystemSandboxFallback        bool
	SystemSandboxStatus          string
	ActiveSkill                  *activeSkillRuntime
	AllowedTools                 map[string]struct{}
	DeniedTools                  map[string]struct{}
	AllowedToolNames             []string
	DeniedToolNames              []string
	AvailableSkills              []PromptSkill
	AvailableTools               []string
	InstructionText              string
	WebLookupInstruction         string
	PromptTokens                 int
}

func (r *Runner) prepareRunPrompt(sess *session.Session, input RunPromptInput, mode string) (runPromptSetup, error) {
	engine := &defaultEngine{runner: r}
	return engine.prepareRunPrompt(sess, input, mode)
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
	engine := &defaultEngine{runner: r}
	return engine.beginRunSession(sess, userMessage, userInput)
}

func (r *Runner) buildTurnMessages(sess *session.Session, setup runPromptSetup) ([]llm.Message, error) {
	engine := &defaultEngine{runner: r}
	return engine.buildTurnMessages(sess, setup)
}
