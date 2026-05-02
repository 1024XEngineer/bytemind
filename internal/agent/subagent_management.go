package agent

import (
	"context"
	"strings"

	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	subagentspkg "bytemind/internal/subagents"
	"bytemind/internal/tools"
)

func (r *Runner) ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic) {
	if r.subAgentManager == nil {
		return nil, nil
	}
	return r.subAgentManager.List()
}

func (r *Runner) FindSubAgent(name string) (subagentspkg.Agent, bool) {
	if r.subAgentManager == nil {
		return subagentspkg.Agent{}, false
	}
	return r.subAgentManager.Find(name)
}

func (r *Runner) FindBuiltinSubAgent(name string) (subagentspkg.Agent, bool) {
	if r.subAgentManager == nil {
		return subagentspkg.Agent{}, false
	}
	return r.subAgentManager.FindBuiltin(name)
}

func (r *Runner) DispatchSubAgent(
	ctx context.Context,
	sess *session.Session,
	mode string,
	request tools.DelegateSubAgentRequest,
) (tools.DelegateSubAgentResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workspace := ""
	if r != nil {
		workspace = strings.TrimSpace(r.workspace)
	}
	if sess != nil {
		if scoped := strings.TrimSpace(sess.Workspace); scoped != "" {
			workspace = scoped
		}
	}
	execCtx := &tools.ExecutionContext{
		Workspace: workspace,
		Session:   sess,
		Mode:      planpkg.NormalizeMode(mode),
	}
	return r.delegateSubAgent(ctx, request, execCtx)
}
