package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var configDocumentMu sync.Mutex

type configDocumentEdit struct {
	start       int
	end         int
	replacement []byte
}

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
	case "type", "family", "base_url", "model", "api_key", "api_key_env":
	default:
		return "", fmt.Errorf("unsupported provider field: %s", field)
	}
	return upsertProviderValues(configPath, map[string]string{
		field: value,
	})
}

func UpsertProviderRuntimeSelection(configPath string, runtimeCfg ProviderRuntimeConfig) (string, error) {
	configDocumentMu.Lock()
	defer configDocumentMu.Unlock()
	path, err := resolveWritableConfigPath(configPath)
	if err != nil {
		return "", err
	}

	selectedProvider := SelectedProviderID(runtimeCfg)
	selectedModel := strings.TrimSpace(runtimeCfg.DefaultModel)
	if selectedModel == "" {
		selectedModel = SelectedModelID(runtimeCfg)
	}
	runtimeCfg, _, err = SelectProviderRuntimeModel(runtimeCfg, selectedProvider, selectedModel)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err := writeConfigDocument(path, providerRuntimeSelectionDocument(runtimeCfg, selectedProvider, selectedModel)); err != nil {
			return "", err
		}
		return path, nil
	}
	if strings.TrimSpace(string(data)) == "" {
		if err := writeConfigDocument(path, providerRuntimeSelectionDocument(runtimeCfg, selectedProvider, selectedModel)); err != nil {
			return "", err
		}
		return path, nil
	}

	updated, err := patchProviderRuntimeSelectionDocument(data, selectedProvider, selectedModel)
	if err != nil {
		return "", err
	}
	if err := writeRawConfigDocument(path, updated); err != nil {
		return "", err
	}
	return path, nil
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
	syncProviderRuntimeDocument(raw, providerSection)
	ensureDefaultConfigDocumentFields(raw)

	if err := writeConfigDocument(path, raw); err != nil {
		return "", err
	}

	return path, nil
}

func syncProviderRuntimeDocument(raw map[string]any, providerSection map[string]any) {
	if raw == nil || providerSection == nil {
		return
	}
	providerCfg := providerConfigFromDocument(providerSection)
	runtimeCfg := ProviderRuntimeConfig{}
	if runtimeRaw, ok := raw["provider_runtime"]; ok {
		if data, err := json.Marshal(runtimeRaw); err == nil {
			_ = json.Unmarshal(data, &runtimeCfg)
		}
	}
	raw["provider_runtime"] = SyncProviderRuntimeWithProvider(runtimeCfg, providerCfg)
}

func providerConfigFromDocument(providerSection map[string]any) ProviderConfig {
	providerCfg := ProviderConfig{}
	data, err := json.Marshal(providerSection)
	if err != nil {
		return providerCfg
	}
	_ = json.Unmarshal(data, &providerCfg)
	return providerCfg
}

func providerConfigDocument(providerCfg ProviderConfig) map[string]any {
	raw := map[string]any{}
	data, err := json.Marshal(providerCfg)
	if err != nil {
		return raw
	}
	_ = json.Unmarshal(data, &raw)
	return raw
}

func providerRuntimeProviderDocument(providerCfg ProviderConfig) map[string]any {
	providerDoc := map[string]any{}
	setNonEmptyProviderString(providerDoc, "type", providerCfg.Type)
	setNonEmptyProviderString(providerDoc, "family", providerCfg.Family)
	setNonEmptyProviderString(providerDoc, "base_url", providerCfg.BaseURL)
	setNonEmptyProviderString(providerDoc, "api_path", providerCfg.APIPath)
	setNonEmptyProviderString(providerDoc, "model", providerCfg.Model)
	setNonEmptyProviderString(providerDoc, "api_key", providerCfg.APIKey)
	setNonEmptyProviderString(providerDoc, "api_key_env", providerCfg.APIKeyEnv)
	setNonEmptyProviderString(providerDoc, "auth_header", providerCfg.AuthHeader)
	setNonEmptyProviderString(providerDoc, "auth_scheme", providerCfg.AuthScheme)
	setNonEmptyProviderString(providerDoc, "anthropic_version", providerCfg.AnthropicVersion)
	if providerCfg.AutoDetectType {
		providerDoc["auto_detect_type"] = true
	}
	if len(providerCfg.ExtraHeaders) > 0 {
		providerDoc["extra_headers"] = providerCfg.ExtraHeaders
	}
	if models := normalizeStringList(providerCfg.Models); len(models) > 0 {
		providerDoc["models"] = models
	}
	return providerDoc
}

func providerRuntimeSelectionDocument(runtimeCfg ProviderRuntimeConfig, providerID, modelID string) map[string]any {
	providers := normalizedProviderRuntimeProviders(runtimeCfg)
	providerDocs := make(map[string]any, len(providers))
	for id, providerCfg := range providers {
		providerDoc := providerRuntimeProviderDocument(providerCfg)
		if id == providerID {
			providerDoc["model"] = modelID
		}
		providerDocs[id] = providerDoc
	}
	if _, ok := providerDocs[providerID]; !ok {
		providerDocs[providerID] = map[string]any{"model": modelID}
	}

	return map[string]any{
		"provider_runtime": map[string]any{
			"current_provider": providerID,
			"providers":        providerDocs,
		},
	}
}

func setNonEmptyProviderString(raw map[string]any, field, value string) {
	if value = strings.TrimSpace(value); value != "" {
		raw[field] = value
	}
}

func patchProviderRuntimeSelectionDocument(data []byte, providerID, modelID string) ([]byte, error) {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	modelID = strings.TrimSpace(modelID)
	if providerID == "" {
		return nil, errors.New("provider id is required")
	}
	if modelID == "" {
		return nil, errors.New("model id is required")
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	runtimeRaw, ok := root["provider_runtime"]
	if !ok {
		return nil, errors.New("provider_runtime is missing")
	}
	var runtimeDoc struct {
		Providers map[string]json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(runtimeRaw, &runtimeDoc); err != nil {
		return nil, err
	}
	if _, ok := runtimeDoc.Providers[providerID]; !ok {
		return nil, fmt.Errorf("provider %q is not configured in provider_runtime", providerID)
	}

	rootStart, err := findJSONObjectStart(data)
	if err != nil {
		return nil, err
	}
	runtimeStart, _, ok, err := findJSONObjectField(data, rootStart, "provider_runtime")
	if err != nil {
		return nil, err
	}
	if !ok || runtimeStart >= len(data) || data[runtimeStart] != '{' {
		return nil, errors.New("provider_runtime must be an object")
	}
	providersStart, _, ok, err := findJSONObjectField(data, runtimeStart, "providers")
	if err != nil {
		return nil, err
	}
	if !ok || providersStart >= len(data) || data[providersStart] != '{' {
		return nil, errors.New("provider_runtime.providers must be an object")
	}
	providerStart, _, ok, err := findJSONObjectField(data, providersStart, providerID)
	if err != nil {
		return nil, err
	}
	if !ok || providerStart >= len(data) || data[providerStart] != '{' {
		return nil, fmt.Errorf("provider %q must be an object", providerID)
	}

	edits := make([]configDocumentEdit, 0, 2)
	currentStart, currentEnd, ok, err := findJSONObjectField(data, runtimeStart, "current_provider")
	if err != nil {
		return nil, err
	}
	if ok {
		edits = append(edits, configDocumentEdit{start: currentStart, end: currentEnd, replacement: jsonStringLiteral(providerID)})
	} else {
		edit, err := insertJSONStringFieldEdit(data, runtimeStart, "current_provider", providerID)
		if err != nil {
			return nil, err
		}
		edits = append(edits, edit)
	}

	modelStart, modelEnd, ok, err := findJSONObjectField(data, providerStart, "model")
	if err != nil {
		return nil, err
	}
	if ok {
		edits = append(edits, configDocumentEdit{start: modelStart, end: modelEnd, replacement: jsonStringLiteral(modelID)})
	} else {
		edit, err := insertJSONStringFieldEdit(data, providerStart, "model", modelID)
		if err != nil {
			return nil, err
		}
		edits = append(edits, edit)
	}

	return applyConfigDocumentEdits(data, edits), nil
}

func applyConfigDocumentEdits(data []byte, edits []configDocumentEdit) []byte {
	out := append([]byte(nil), data...)
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].start > edits[j].start
	})
	for _, edit := range edits {
		out = append(out[:edit.start], append(edit.replacement, out[edit.end:]...)...)
	}
	return out
}

func insertJSONStringFieldEdit(data []byte, objectStart int, key, value string) (configDocumentEdit, error) {
	if objectStart < 0 || objectStart >= len(data) || data[objectStart] != '{' {
		return configDocumentEdit{}, errors.New("json object start is invalid")
	}

	keyLiteral := jsonStringLiteral(key)
	valueLiteral := jsonStringLiteral(value)
	field := make([]byte, 0, len(keyLiteral)+len(valueLiteral)+4)
	field = append(field, keyLiteral...)
	field = append(field, []byte(": ")...)
	field = append(field, valueLiteral...)

	next := skipJSONWhitespace(data, objectStart+1)
	if next >= len(data) {
		return configDocumentEdit{}, errors.New("unterminated json object")
	}
	if data[next] == '}' {
		return configDocumentEdit{start: objectStart + 1, end: objectStart + 1, replacement: field}, nil
	}
	if hasLineBreakBetween(data, objectStart+1, next) {
		replacement := make([]byte, 0, len(field)+32)
		replacement = append(replacement, objectLineEnding(data, objectStart+1)...)
		replacement = append(replacement, lineIndentBefore(data, next)...)
		replacement = append(replacement, field...)
		replacement = append(replacement, ',')
		return configDocumentEdit{start: objectStart + 1, end: objectStart + 1, replacement: replacement}, nil
	}

	replacement := make([]byte, 0, len(field)+2)
	replacement = append(replacement, field...)
	replacement = append(replacement, []byte(", ")...)
	return configDocumentEdit{start: objectStart + 1, end: objectStart + 1, replacement: replacement}, nil
}

func findJSONObjectStart(data []byte) (int, error) {
	start := skipJSONWhitespace(data, 0)
	if start >= len(data) || data[start] != '{' {
		return 0, errors.New("config document must be a json object")
	}
	return start, nil
}

func findJSONObjectField(data []byte, objectStart int, field string) (int, int, bool, error) {
	i := skipJSONWhitespace(data, objectStart)
	if i >= len(data) || data[i] != '{' {
		return 0, 0, false, errors.New("json object start is invalid")
	}
	i++
	for {
		i = skipJSONWhitespace(data, i)
		if i >= len(data) {
			return 0, 0, false, errors.New("unterminated json object")
		}
		if data[i] == '}' {
			return 0, 0, false, nil
		}
		if data[i] != '"' {
			return 0, 0, false, errors.New("json object key must be a string")
		}
		keyStart := i
		keyEnd, err := scanJSONStringEnd(data, keyStart)
		if err != nil {
			return 0, 0, false, err
		}
		var key string
		if err := json.Unmarshal(data[keyStart:keyEnd], &key); err != nil {
			return 0, 0, false, err
		}
		i = skipJSONWhitespace(data, keyEnd)
		if i >= len(data) || data[i] != ':' {
			return 0, 0, false, errors.New("json object key must be followed by colon")
		}
		valueStart := skipJSONWhitespace(data, i+1)
		valueEnd, err := scanJSONValueEnd(data, valueStart)
		if err != nil {
			return 0, 0, false, err
		}
		if key == field {
			return valueStart, valueEnd, true, nil
		}
		i = skipJSONWhitespace(data, valueEnd)
		if i >= len(data) {
			return 0, 0, false, errors.New("unterminated json object")
		}
		switch data[i] {
		case ',':
			i++
		case '}':
			return 0, 0, false, nil
		default:
			return 0, 0, false, errors.New("json object fields must be separated by comma")
		}
	}
}

func scanJSONValueEnd(data []byte, start int) (int, error) {
	if start >= len(data) {
		return 0, errors.New("missing json value")
	}
	switch data[start] {
	case '"':
		return scanJSONStringEnd(data, start)
	case '{', '[':
		return scanJSONCompositeEnd(data, start)
	default:
		end := start
		for end < len(data) && !isJSONValueTerminator(data[end]) {
			end++
		}
		if end == start {
			return 0, errors.New("missing json value")
		}
		var value any
		if err := json.Unmarshal(data[start:end], &value); err != nil {
			return 0, err
		}
		return end, nil
	}
}

func scanJSONCompositeEnd(data []byte, start int) (int, error) {
	stack := []byte{matchingJSONClose(data[start])}
	for i := start + 1; i < len(data); i++ {
		switch data[i] {
		case '"':
			end, err := scanJSONStringEnd(data, i)
			if err != nil {
				return 0, err
			}
			i = end - 1
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || data[i] != stack[len(stack)-1] {
				return 0, errors.New("mismatched json brackets")
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1, nil
			}
		}
	}
	return 0, errors.New("unterminated json value")
}

func matchingJSONClose(open byte) byte {
	if open == '[' {
		return ']'
	}
	return '}'
}

func scanJSONStringEnd(data []byte, start int) (int, error) {
	if start >= len(data) || data[start] != '"' {
		return 0, errors.New("json string must start with quote")
	}
	for i := start + 1; i < len(data); i++ {
		switch data[i] {
		case '\\':
			i++
			if i >= len(data) {
				return 0, errors.New("unterminated json string escape")
			}
		case '"':
			return i + 1, nil
		}
	}
	return 0, errors.New("unterminated json string")
}

func jsonStringLiteral(value string) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func skipJSONWhitespace(data []byte, start int) int {
	for start < len(data) {
		switch data[start] {
		case ' ', '\n', '\r', '\t':
			start++
		default:
			return start
		}
	}
	return start
}

func isJSONValueTerminator(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', ',', '}', ']':
		return true
	default:
		return false
	}
}

func hasLineBreakBetween(data []byte, start, end int) bool {
	for i := start; i < end && i < len(data); i++ {
		if data[i] == '\n' || data[i] == '\r' {
			return true
		}
	}
	return false
}

func objectLineEnding(data []byte, start int) []byte {
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '\r':
			if i+1 < len(data) && data[i+1] == '\n' {
				return []byte("\r\n")
			}
			return []byte("\r")
		case '\n':
			return []byte("\n")
		case ' ', '\t':
			continue
		default:
			return []byte("\n")
		}
	}
	return []byte("\n")
}

func lineIndentBefore(data []byte, index int) []byte {
	lineStart := index
	for lineStart > 0 && data[lineStart-1] != '\n' && data[lineStart-1] != '\r' {
		lineStart--
	}
	return append([]byte(nil), data[lineStart:index]...)
}

func ensureDefaultConfigDocumentFields(raw map[string]any) {
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
}

// WriteConfig writes a Config struct to a JSON file at the given path.
func WriteConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
	return writeRawConfigDocument(path, encoded)
}

func writeRawConfigDocument(path string, encoded []byte) error {
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
