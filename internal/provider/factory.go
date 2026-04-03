package provider

import (
	"fmt"

	"bytemind/internal/config"
	"bytemind/internal/llm"
)

func NewClient(cfg config.ProviderConfig) (llm.Client, error) {
	clientCfg := Config{
		Type:             cfg.Type,
		BaseURL:          cfg.BaseURL,
		APIKey:           cfg.ResolveAPIKey(),
		Model:            cfg.Model,
		AnthropicVersion: cfg.AnthropicVersion,
	}

	switch cfg.Type {
	case "openai-compatible", "openai":
		return NewOpenAICompatible(clientCfg), nil
	case "anthropic":
		return NewAnthropic(clientCfg), nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cfg.Type)
	}
}
