package agent

import (
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	"github.com/1024XEngineer/bytemind/internal/session"
)

type runPromptSetup struct {
	Input                        RunPromptInput
	UserInput                    string
	PersistedUserMessageIndex    int
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
	AvailableSubAgents           []PromptSubAgent
	AvailableTools               []string
	SubAgentRuntime              *PromptSubAgentRuntime
	SubAgentDefinition           string
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
	input.SubAgent = normalizeSubAgentPromptInput(input.SubAgent)
	return input
}

func normalizeSubAgentPromptInput(input *SubAgentPromptInput) *SubAgentPromptInput {
	if input == nil {
		return nil
	}
	normalized := &SubAgentPromptInput{
		Name:           strings.TrimSpace(input.Name),
		Task:           strings.TrimSpace(input.Task),
		Isolation:      strings.TrimSpace(input.Isolation),
		ResultPolicy:   strings.TrimSpace(input.ResultPolicy),
		DefinitionBody: strings.TrimSpace(input.DefinitionBody),
	}
	if len(input.ScopePaths) > 0 {
		normalized.ScopePaths = normalizeUniqueStrings(input.ScopePaths)
	}
	if len(input.ScopeSymbols) > 0 {
		normalized.ScopeSymbols = normalizeUniqueStrings(input.ScopeSymbols)
	}
	if len(input.AllowedTools) > 0 {
		normalized.AllowedTools = normalizeUniqueStrings(input.AllowedTools)
	}
	if normalized.Name == "" &&
		normalized.Task == "" &&
		len(normalized.ScopePaths) == 0 &&
		len(normalized.ScopeSymbols) == 0 &&
		len(normalized.AllowedTools) == 0 &&
		normalized.Isolation == "" &&
		normalized.ResultPolicy == "" &&
		normalized.DefinitionBody == "" {
		return nil
	}
	return normalized
}

func normalizeUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
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
