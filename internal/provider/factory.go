package provider

import (
	"fmt"
	"strings"

	"bytemind/internal/config"
	"bytemind/internal/llm"
)

func NewClient(cfg config.ProviderConfig) (llm.Client, error) {
	return newBaseClient(cfg)
}

func NewClientFromRuntime(cfg config.ProviderRuntimeConfig, health HealthChecker) (llm.Client, error) {
	return NewRouterClient(cfg, health)
}

func newBaseClient(cfg config.ProviderConfig) (llm.Client, error) {
	typ := strings.ToLower(strings.TrimSpace(cfg.Type))
	clientCfg := Config{
		Type:             typ,
		BaseURL:          cfg.BaseURL,
		APIPath:          cfg.APIPath,
		APIKey:           cfg.ResolveAPIKey(),
		AuthHeader:       cfg.AuthHeader,
		AuthScheme:       cfg.AuthScheme,
		ExtraHeaders:     cfg.ExtraHeaders,
		Model:            cfg.Model,
		AnthropicVersion: cfg.AnthropicVersion,
	}

	switch typ {
	case "openai-compatible", "openai":
		return NewOpenAICompatible(clientCfg), nil
	case "anthropic":
		return NewAnthropic(clientCfg), nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cfg.Type)
	}
}

func NewDomainClient(cfg config.ProviderConfig) (Client, error) {
	providerID := ProviderID(strings.ToLower(strings.TrimSpace(cfg.Type)))
	if providerID == "openai-compatible" || providerID == "openai" {
		providerID = ProviderOpenAI
	}
	if providerID == "anthropic" {
		providerID = ProviderAnthropic
	}
	if providerID == "" {
		providerID = ProviderID("unknown")
	}
	return NewDomainClientWithID(providerID, cfg)
}

func NewDomainClientWithID(providerID ProviderID, cfg config.ProviderConfig) (Client, error) {
	baseClient, err := newBaseClient(cfg)
	if err != nil {
		return nil, err
	}
	id := ProviderID(strings.ToLower(strings.TrimSpace(string(providerID))))
	if id == "" {
		id = ProviderID("unknown")
	}
	return WrapClient(id, ModelID(strings.TrimSpace(cfg.Model)), baseClient), nil
}

func NewRouterClient(cfg config.ProviderRuntimeConfig, health HealthChecker) (llm.Client, error) {
	reg, err := NewRegistry(cfg)
	if err != nil {
		return nil, err
	}
	if health == nil {
		health = NewHealthChecker(HealthConfigFromRuntime(cfg.Health), nil)
	}
	return NewRoutedClientWithPolicy(NewRouter(reg, health, RouterConfig{
		DefaultProvider: ProviderID(cfg.DefaultProvider),
		DefaultModel:    ModelID(cfg.DefaultModel),
	}), health, cfg.AllowFallback), nil
}
