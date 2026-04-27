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
