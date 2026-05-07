package agent

import (
	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

func (r *Runner) SetObserver(observer Observer) {
	r.observer = observer
}

func (r *Runner) SetApprovalHandler(handler tools.ApprovalHandler) {
	r.approval = handler
}

func (r *Runner) UpdateProvider(providerCfg config.ProviderConfig, client llm.Client) {
	if r == nil {
		return
	}
	r.config.Provider = providerCfg
	r.config.ProviderRuntime = config.SyncProviderRuntimeWithProvider(r.config.ProviderRuntime, providerCfg)
	if client != nil {
		if _, ok := client.(routeAwareClient); ok {
			r.client = client
		} else {
			r.client = routeAwareClient{base: client}
		}
	}
	r.clearModelsCache()
}

func (r *Runner) UpdateApprovalMode(mode string) {
	if r == nil {
		return
	}
	normalizedMode, err := config.NormalizeApprovalMode(mode)
	if err != nil || normalizedMode == "" {
		normalizedMode = "interactive"
	}
	r.config.ApprovalMode = normalizedMode
}
