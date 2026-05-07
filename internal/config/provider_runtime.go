package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

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

func SelectProviderRuntimeModel(runtimeCfg ProviderRuntimeConfig, providerID, modelID string) (ProviderRuntimeConfig, ProviderConfig, error) {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	modelID = strings.TrimSpace(modelID)
	if providerID == "" {
		return runtimeCfg, ProviderConfig{}, errors.New("provider id is required")
	}
	if modelID == "" {
		return runtimeCfg, ProviderConfig{}, errors.New("model id is required")
	}

	providers := make(map[string]ProviderConfig, len(runtimeCfg.Providers))
	for id, cfg := range runtimeCfg.Providers {
		normalizedID := strings.ToLower(strings.TrimSpace(id))
		if normalizedID == "" {
			continue
		}
		providers[normalizedID] = cfg
	}
	providerCfg, ok := providers[providerID]
	if !ok {
		return runtimeCfg, ProviderConfig{}, fmt.Errorf("provider %q is not configured", providerID)
	}
	providerCfg.Model = modelID
	providers[providerID] = providerCfg

	runtimeCfg.DefaultProvider = providerID
	runtimeCfg.DefaultModel = modelID
	runtimeCfg.Providers = providers
	return runtimeCfg, providerCfg, nil
}

func DeleteProviderRuntimeProvider(runtimeCfg ProviderRuntimeConfig, providerID string) (ProviderRuntimeConfig, ProviderConfig, error) {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if providerID == "" {
		return runtimeCfg, ProviderConfig{}, errors.New("provider id is required")
	}

	providers := make(map[string]ProviderConfig, len(runtimeCfg.Providers))
	for id, cfg := range runtimeCfg.Providers {
		normalizedID := strings.ToLower(strings.TrimSpace(id))
		if normalizedID == "" {
			continue
		}
		providers[normalizedID] = cfg
	}
	if _, ok := providers[providerID]; !ok {
		return runtimeCfg, ProviderConfig{}, fmt.Errorf("provider %q is not configured", providerID)
	}
	if len(providers) <= 1 {
		return runtimeCfg, ProviderConfig{}, errors.New("cannot delete the last configured model")
	}

	delete(providers, providerID)

	selectedProvider := strings.ToLower(strings.TrimSpace(runtimeCfg.DefaultProvider))
	if selectedProvider == "" || selectedProvider == providerID {
		selectedProvider = ""
	}
	selectedCfg, ok := providers[selectedProvider]
	if !ok {
		ids := make([]string, 0, len(providers))
		for id := range providers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		selectedProvider = ids[0]
		selectedCfg = providers[selectedProvider]
	}

	runtimeCfg.DefaultProvider = selectedProvider
	runtimeCfg.DefaultModel = strings.TrimSpace(selectedCfg.Model)
	runtimeCfg.Providers = providers
	return runtimeCfg, selectedCfg, nil
}
