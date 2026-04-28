package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var configDocumentMu sync.Mutex

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
	configDocumentMu.Lock()
	defer configDocumentMu.Unlock()
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
	providerType := asString(providerSection["type"])
	baseURL := asString(providerSection["base_url"])
	if strings.TrimSpace(baseURL) == "" || usesOpenAIDefaultBaseURLForNativeProvider(providerType, baseURL) {
		providerSection["base_url"] = defaultBaseURL(providerType)
	}
	model := asString(providerSection["model"])
	if strings.TrimSpace(model) == "" || usesOpenAIDefaultModelForNativeProvider(providerType, model) {
		providerSection["model"] = defaultModel(providerType, asString(providerSection["base_url"]))
	}
	raw["provider"] = providerSection

	if _, ok := raw["approval_policy"]; !ok {
		raw["approval_policy"] = "on-request"
	}
	if _, ok := raw["approval_mode"]; !ok {
		raw["approval_mode"] = "interactive"
	}
	if _, ok := raw["away_policy"]; !ok {
		raw["away_policy"] = "auto_deny_continue"
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

func MutateMCPConfig(workspace, explicitPath string, mutator func(*MCPConfig) error) (Config, string, error) {
	path, err := ResolveWritableMCPConfigPathForWorkspace(workspace, explicitPath)
	if err != nil {
		return Config{}, "", err
	}
	configDocumentMu.Lock()
	defer configDocumentMu.Unlock()

	mcp := Default(workspace).MCP
	if _, statErr := os.Stat(path); statErr == nil {
		if err := mergeMCPConfigFromFile(path, &mcp); err != nil {
			return Config{}, "", err
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Config{}, "", statErr
	}

	if err := normalizeMCPConfig(&mcp); err != nil {
		return Config{}, "", err
	}
	if mutator != nil {
		if err := mutator(&mcp); err != nil {
			return Config{}, "", err
		}
	}
	if err := normalizeMCPConfig(&mcp); err != nil {
		return Config{}, "", err
	}
	if err := writeConfigDocument(path, mcp); err != nil {
		return Config{}, "", err
	}
	loaded, err := LoadWithMCPConfigPath(workspace, "", path)
	if err != nil {
		return Config{}, "", err
	}
	return loaded, path, nil
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

func writeConfigDocument(path string, raw any) error {
	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	removeTmp = false
	_ = os.Chmod(path, 0o644)
	syncDirectory(dir)
	return nil
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

func ResolveWritableConfigPathForWorkspace(workspace, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}
	workspace = strings.TrimSpace(workspace)
	if workspace != "" {
		return filepath.Join(workspace, ".bytemind", "config.json"), nil
	}
	return resolveWritableConfigPath("")
}

func ResolveWritableMCPConfigPathForWorkspace(workspace, explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Abs(explicit)
	}
	workspace = strings.TrimSpace(workspace)
	if workspace != "" {
		return filepath.Join(workspace, ".bytemind", "mcp.json"), nil
	}
	home, err := EnsureHomeLayout()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "mcp.json"), nil
}

func syncDirectory(path string) {
	dir, err := os.Open(path)
	if err != nil {
		return
	}
	defer dir.Close()
	_ = dir.Sync()
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
