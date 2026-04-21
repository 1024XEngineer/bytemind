package app

import (
	"context"
	"errors"
	"io"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	"bytemind/internal/skills"
	"bytemind/internal/tools"
	"bytemind/tui"
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
		UserMessage: input.UserMessage,
		Assets:      input.Assets,
		DisplayText: input.DisplayText,
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
	a.runner.SetApprovalHandler(func(req tools.ApprovalRequest) (bool, error) {
		if handler == nil {
			return false, nil
		}
		return handler(tui.ApprovalRequest{
			Command: req.Command,
			Reason:  req.Reason,
		})
	})
}

func (a *tuiRunnerAdapter) UpdateProvider(providerCfg config.ProviderConfig, client llm.Client) {
	if a == nil || a.runner == nil {
		return
	}
	a.runner.UpdateProvider(providerCfg, client)
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

func mapAgentEvent(event agent.Event) tui.Event {
	return tui.Event{
		Type:          mapAgentEventType(event.Type),
		SessionID:     string(event.SessionID),
		UserInput:     event.UserInput,
		Content:       event.Content,
		ToolName:      event.ToolName,
		ToolArguments: event.ToolArguments,
		ToolResult:    event.ToolResult,
		Error:         event.Error,
		Plan:          event.Plan,
		Usage:         event.Usage,
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
	default:
		return tui.EventType(value)
	}
}
