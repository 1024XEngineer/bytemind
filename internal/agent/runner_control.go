package agent

import (
	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/tools"
)

func (r *Runner) SetObserver(observer Observer) {
	r.observer = observer
}

func (r *Runner) SetApprovalHandler(handler tools.ApprovalHandler) {
	r.approval = handler
}

func (r *Runner) UpdateProvider(providerCfg config.ProviderConfig, client llm.Client) {
	r.config.Provider = providerCfg
	if client != nil {
		r.client = client
	}
}
