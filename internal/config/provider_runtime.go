package config

import "strings"

type ProviderHealthRuntimeConfig struct {
	FailThreshold           int `json:"fail_threshold"`
	RecoverProbeSec         int `json:"recover_probe_sec"`
	RecoverSuccessThreshold int `json:"recover_success_threshold"`
	WindowSize              int `json:"window_size"`
}

type ProviderRuntimeConfig struct {
	DefaultProvider string                      `json:"default_provider"`
	DefaultModel    string                      `json:"default_model"`
	AllowFallback   bool                        `json:"allow_fallback"`
	Providers       map[string]ProviderConfig   `json:"providers"`
	Health          ProviderHealthRuntimeConfig `json:"health"`
}

func LegacyProviderRuntimeConfig(cfg ProviderConfig) ProviderRuntimeConfig {
	providerID := strings.ToLower(strings.TrimSpace(cfg.Type))
	switch providerID {
	case "", "openai", "openai-compatible":
		providerID = "openai"
	case "anthropic":
		providerID = "anthropic"
	case "gemini":
		providerID = "gemini"
	}
	cfg.Type = providerID
	return ProviderRuntimeConfig{
		DefaultProvider: providerID,
		DefaultModel:    cfg.Model,
		AllowFallback:   false,
		Providers: map[string]ProviderConfig{
			providerID: cfg,
		},
	}
}

func SyncProviderRuntimeWithProvider(runtimeCfg ProviderRuntimeConfig, providerCfg ProviderConfig) ProviderRuntimeConfig {
	legacy := LegacyProviderRuntimeConfig(providerCfg)
	providerID := strings.ToLower(strings.TrimSpace(legacy.DefaultProvider))
	if providerID == "" {
		return runtimeCfg
	}
	providerEntry := legacy.Providers[providerID]

	providers := make(map[string]ProviderConfig, len(runtimeCfg.Providers)+1)
	for id, cfg := range runtimeCfg.Providers {
		normalizedID := strings.ToLower(strings.TrimSpace(id))
		if normalizedID == "" {
			continue
		}
		providers[normalizedID] = cfg
	}
	providers[providerID] = providerEntry

	runtimeCfg.DefaultProvider = providerID
	runtimeCfg.DefaultModel = strings.TrimSpace(providerEntry.Model)
	runtimeCfg.Providers = providers
	return runtimeCfg
}
