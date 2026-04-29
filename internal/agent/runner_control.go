package agent

import (
	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/tools"
	"strings"
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

func (r *Runner) UpdateApprovalMode(mode string) {
	if r == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case approvalModeAway:
		r.config.ApprovalMode = approvalModeAway
	default:
		r.config.ApprovalMode = approvalModeInteractive
	}
}
