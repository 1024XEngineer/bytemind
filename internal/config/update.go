package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func UpsertProviderAPIKey(configPath, apiKey string) (string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", errors.New("api key is empty")
	}
	return upsertProviderValues(configPath, map[string]string{
		"api_key": apiKey,
	})
}

func UpsertProviderField(configPath, field, value string) (string, error) {
	field = strings.ToLower(strings.TrimSpace(field))
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("provider field value is empty")
	}
	switch field {
	case "type", "base_url", "model", "api_key", "api_key_env":
	default:
		return "", fmt.Errorf("unsupported provider field: %s", field)
	}
	return upsertProviderValues(configPath, map[string]string{
		field: value,
	})
}

func upsertProviderValues(configPath string, values map[string]string) (string, error) {
	path, err := resolveWritableConfigPath(configPath)
	if err != nil {
		return "", err
	}

	raw, err := loadConfigDocument(path)
	if err != nil {
		return "", err
	}

	providerSection, ok := raw["provider"].(map[string]any)
	if !ok || providerSection == nil {
		providerSection = map[string]any{}
	}
	for field, value := range values {
		if strings.TrimSpace(field) == "" {
			continue
		}
		providerSection[field] = strings.TrimSpace(value)
	}
	if strings.TrimSpace(asString(providerSection["api_key_env"])) == "" {
		providerSection["api_key_env"] = "BYTEMIND_API_KEY"
	}
	if strings.TrimSpace(asString(providerSection["type"])) == "" {
		providerSection["type"] = "openai-compatible"
	}
	if strings.TrimSpace(asString(providerSection["base_url"])) == "" {
		providerSection["base_url"] = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(asString(providerSection["model"])) == "" {
		providerSection["model"] = defaultModel(
			asString(providerSection["type"]),
			asString(providerSection["base_url"]),
		)
	}
	raw["provider"] = providerSection

	if _, ok := raw["approval_policy"]; !ok {
		raw["approval_policy"] = "on-request"
	}
	if _, ok := raw["max_iterations"]; !ok {
		raw["max_iterations"] = 32
	}
	if _, ok := raw["stream"]; !ok {
		raw["stream"] = true
	}

	if err := writeConfigDocument(path, raw); err != nil {
		return "", err
	}

	return path, nil
}

func loadConfigDocument(path string) (map[string]any, error) {
	raw := map[string]any{}
	data, err := os.ReadFile(path)
	if err == nil {
		if strings.TrimSpace(string(data)) != "" {
			if err := json.Unmarshal(data, &raw); err != nil {
				return nil, err
			}
		}
		return raw, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return raw, nil
	}
	return nil, err
}

func writeConfigDocument(path string, raw map[string]any) error {
	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

func resolveWritableConfigPath(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}

	home, err := EnsureHomeLayout()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config.json"), nil
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
