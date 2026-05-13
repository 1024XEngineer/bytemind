package app

import (
	"context"
	"errors"
	"io"

	"github.com/1024XEngineer/bytemind/internal/agent"
	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/provider"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/skills"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"
	"github.com/1024XEngineer/bytemind/tui"
)

type tuiRunnerAdapter struct {
	runner *agent.Runner
}

func newTUIRunnerAdapter(r *agent.Runner) tui.Runner {
	if r == nil {
		return nil
	}
	return &tuiRunnerAdapter{runner: r}
}

func (a *tuiRunnerAdapter) RunPromptWithInput(ctx context.Context, sess *session.Session, input tui.RunPromptInput, mode string, out io.Writer) (string, error) {
	if a == nil || a.runner == nil {
		return "", errors.New("runner is unavailable")
	}
	return a.runner.RunPromptWithInput(ctx, sess, agent.RunPromptInput{
		UserMessage:                     input.UserMessage,
		Assets:                          input.Assets,
		DisplayText:                     input.DisplayText,
		PersistDisplayTextAsUserMessage: input.PersistDisplayTextAsUserMessage,
	}, mode, out)
}

func (a *tuiRunnerAdapter) SetObserver(observer tui.Observer) {
	if a == nil || a.runner == nil {
		return
	}
	a.runner.SetObserver(agent.ObserverFunc(func(event agent.Event) {
		if observer == nil {
			return
		}
		observer(mapAgentEvent(event))
	}))
}

func (a *tuiRunnerAdapter) SetApprovalHandler(handler tui.ApprovalHandler) {
	if a == nil || a.runner == nil {
		return
	}
	a.runner.SetApprovalHandler(func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
		if handler == nil {
			return tools.ApprovalDecision{Disposition: tools.ApprovalDeny}, nil
		}
		decision, err := handler(tui.ApprovalRequest{
			ToolName: req.ToolName,
			Command:  req.Command,
			Reason:   req.Reason,
		})
		if err != nil {
			return tools.ApprovalDecision{}, err
		}
		return tools.ApprovalDecision{Disposition: tools.ApprovalDisposition(decision.Disposition)}, nil
	})
}

func (a *tuiRunnerAdapter) UpdateProvider(providerCfg config.ProviderConfig, client llm.Client) {
	if a == nil || a.runner == nil {
		return
	}
	a.runner.UpdateProvider(providerCfg, client)
}

func (a *tuiRunnerAdapter) UpdateProviderRuntime(runtimeCfg config.ProviderRuntimeConfig, providerCfg config.ProviderConfig, client llm.Client) {
	if a == nil || a.runner == nil {
		return
	}
	a.runner.UpdateProviderRuntime(runtimeCfg, providerCfg, client)
}

func (a *tuiRunnerAdapter) UpdateApprovalMode(mode string) {
	if a == nil || a.runner == nil {
		return
	}
	a.runner.UpdateApprovalMode(mode)
}

func (a *tuiRunnerAdapter) ListSkills() ([]skills.Skill, []skills.Diagnostic) {
	if a == nil || a.runner == nil {
		return nil, nil
	}
	return a.runner.ListSkills()
}

func (a *tuiRunnerAdapter) GetActiveSkill(sess *session.Session) (skills.Skill, bool) {
	if a == nil || a.runner == nil {
		return skills.Skill{}, false
	}
	return a.runner.GetActiveSkill(sess)
}

func (a *tuiRunnerAdapter) ActivateSkill(sess *session.Session, name string, args map[string]string) (skills.Skill, error) {
	if a == nil || a.runner == nil {
		return skills.Skill{}, errors.New("runner is unavailable")
	}
	return a.runner.ActivateSkill(sess, name, args)
}

func (a *tuiRunnerAdapter) ClearActiveSkill(sess *session.Session) error {
	if a == nil || a.runner == nil {
		return nil
	}
	return a.runner.ClearActiveSkill(sess)
}

func (a *tuiRunnerAdapter) ClearSkill(name string) (skills.ClearResult, error) {
	if a == nil || a.runner == nil {
		return skills.ClearResult{}, errors.New("runner is unavailable")
	}
	return a.runner.ClearSkill(name)
}

func (a *tuiRunnerAdapter) CompactSession(ctx context.Context, sess *session.Session) (string, bool, error) {
	if a == nil || a.runner == nil {
		return "", false, errors.New("runner is unavailable")
	}
	return a.runner.CompactSession(ctx, sess)
}

func (a *tuiRunnerAdapter) SubAgentManager() *subagentspkg.Manager {
	if a == nil || a.runner == nil {
		return nil
	}
	return a.runner.SubAgentManager()
}

func (a *tuiRunnerAdapter) ListModels(ctx context.Context) ([]provider.ModelInfo, []provider.Warning, error) {
	if a == nil || a.runner == nil {
		return nil, nil, errors.New("runner is unavailable")
	}
	return a.runner.ListModels(ctx)
}

func (a *tuiRunnerAdapter) ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic) {
	if a == nil || a.runner == nil {
		return nil, nil
	}
	return a.runner.ListSubAgents()
}

func (a *tuiRunnerAdapter) FindSubAgent(name string) (subagentspkg.Agent, bool) {
	if a == nil || a.runner == nil {
		return subagentspkg.Agent{}, false
	}
	return a.runner.FindSubAgent(name)
}

func (a *tuiRunnerAdapter) FindBuiltinSubAgent(name string) (subagentspkg.Agent, bool) {
	if a == nil || a.runner == nil {
		return subagentspkg.Agent{}, false
	}
	return a.runner.FindBuiltinSubAgent(name)
}

func (a *tuiRunnerAdapter) DispatchSubAgent(
	ctx context.Context,
	sess *session.Session,
	mode string,
	request tools.DelegateSubAgentRequest,
	streamObserver tui.Observer,
) (tools.DelegateSubAgentResult, error) {
	if a == nil || a.runner == nil {
		return tools.DelegateSubAgentResult{}, errors.New("runner is unavailable")
	}
	var agentObserver agent.Observer
	if streamObserver != nil {
		agentObserver = agent.ObserverFunc(func(event agent.Event) {
			streamObserver(mapAgentEvent(event))
		})
	}
	return a.runner.DispatchSubAgent(ctx, sess, mode, request, agentObserver)
}

func mapAgentEvent(event agent.Event) tui.Event {
	return tui.Event{
		Type:               mapAgentEventType(event.Type),
		SessionID:          string(event.SessionID),
		UserInput:          event.UserInput,
		Content:            event.Content,
		ToolName:           event.ToolName,
		ToolCallID:         event.ToolCallID,
		ToolArguments:      event.ToolArguments,
		ToolResult:         event.ToolResult,
		Error:              event.Error,
		Plan:               event.Plan,
		Usage:              event.Usage,
		AgentID:            event.AgentID,
		InvocationID:       event.InvocationID,
		ReasoningCharCount: event.ReasoningCharCount,
		ReasoningActive:    event.ReasoningActive,
	}
}

func mapAgentEventType(value agent.EventType) tui.EventType {
	switch value {
	case agent.EventRunStarted:
		return tui.EventRunStarted
	case agent.EventAssistantDelta:
		return tui.EventAssistantDelta
	case agent.EventAssistantMessage:
		return tui.EventAssistantMessage
	case agent.EventToolCallStarted:
		return tui.EventToolCallStarted
	case agent.EventToolCallCompleted:
		return tui.EventToolCallCompleted
	case agent.EventPlanUpdated:
		return tui.EventPlanUpdated
	case agent.EventUsageUpdated:
		return tui.EventUsageUpdated
	case agent.EventRunFinished:
		return tui.EventRunFinished
	case agent.EventThinkingProgress:
		return tui.EventThinkingProgress
	case agent.EventStatusUpdated:
		return tui.EventStatusUpdated
	default:
		return tui.EventType(value)
	}
}
