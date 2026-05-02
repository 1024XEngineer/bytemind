package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/provider"
)

type StartupFieldRequest struct {
	Field      string
	Value      string
	ConfigPath string
	Provider   config.ProviderConfig
}

type StartupFieldResult struct {
	ConfigPath string
	Provider   config.ProviderConfig
}

type StartupVerifyRequest struct {
	APIKey     string
	ConfigPath string
	Provider   config.ProviderConfig
}

type StartupVerifyResult struct {
	ConfigPath  string
	Provider    config.ProviderConfig
	Ready       bool
	Check       provider.Availability
	WrittenPath string
	SaveErr     error
}

func (s *Service) ApplyStartupField(req StartupFieldRequest) (StartupFieldResult, error) {
	field := strings.TrimSpace(req.Field)
	value := strings.TrimSpace(req.Value)
	if value == "" {
		return StartupFieldResult{}, fmt.Errorf("%s cannot be empty", field)
	}

	providerCfg := req.Provider
	persistValue := value
	switch field {
	case "model":
		providerCfg.Model = value
	case "base_url":
		providerCfg.BaseURL = value
	case "type":
		normalized, ok := NormalizeProviderType(value)
		if !ok {
			return StartupFieldResult{}, fmt.Errorf("provider must be openai-compatible or anthropic")
		}
		providerCfg.Type = normalized
		persistValue = normalized
	default:
		return StartupFieldResult{}, fmt.Errorf("unsupported setup field: %s", field)
	}

	writtenPath, err := config.UpsertProviderField(req.ConfigPath, field, persistValue)
	if err != nil {
		return StartupFieldResult{}, err
	}
	if strings.TrimSpace(writtenPath) == "" {
		writtenPath = req.ConfigPath
	}
	return StartupFieldResult{
		ConfigPath: writtenPath,
		Provider:   providerCfg,
	}, nil
}

func (s *Service) VerifyStartupAPIKey(req StartupVerifyRequest) (StartupVerifyResult, error) {
	providerCfg := req.Provider
	providerCfg.APIKey = strings.TrimSpace(req.APIKey)
	check := provider.CheckAvailability(context.Background(), providerCfg)
	if !check.Ready {
		return StartupVerifyResult{
			ConfigPath: req.ConfigPath,
			Provider:   providerCfg,
			Ready:      false,
			Check:      check,
		}, nil
	}

	writtenPath, saveErr := config.UpsertProviderAPIKey(req.ConfigPath, providerCfg.APIKey)
	if envName := strings.TrimSpace(providerCfg.APIKeyEnv); envName != "" {
		_ = os.Setenv(envName, providerCfg.APIKey)
	} else {
		_ = os.Setenv("BYTEMIND_API_KEY", providerCfg.APIKey)
	}

	client, err := provider.NewClient(providerCfg)
	if err != nil {
		return StartupVerifyResult{}, err
	}
	if s != nil && s.runner != nil {
		s.runner.UpdateProvider(providerCfg, client)
	}

	if strings.TrimSpace(writtenPath) == "" {
		writtenPath = req.ConfigPath
	}
	return StartupVerifyResult{
		ConfigPath:  writtenPath,
		Provider:    providerCfg,
		Ready:       true,
		Check:       check,
		WrittenPath: writtenPath,
		SaveErr:     saveErr,
	}, nil
}

func NormalizeProviderType(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai-compatible", "openai_compatible", "openai":
		return "openai-compatible", true
	case "anthropic":
		return "anthropic", true
	default:
		return "", false
	}
}
