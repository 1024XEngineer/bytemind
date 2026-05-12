package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchModelContextWindow queries the provider API for a model's context window.
// Returns 0 if the provider doesn't support querying this info.
func FetchModelContextWindow(ctx context.Context, providerType, baseURL, apiKey, modelID string) int {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "gemini":
		return fetchGeminiContextWindow(ctx, baseURL, apiKey, modelID)
	default:
		return 0
	}
}

func fetchGeminiContextWindow(ctx context.Context, baseURL, apiKey, modelID string) int {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return 0
	}
	if !strings.HasPrefix(modelID, "models/") && !strings.HasPrefix(modelID, "tunedModels/") {
		modelID = "models/" + modelID
	}

	apiURL := baseURL + "/" + modelID
	if apiKey != "" {
		sep := "?"
		if strings.Contains(apiURL, "?") {
			sep = "&"
		}
		apiURL += sep + "key=" + apiKey
	}

	cli := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return 0
	}

	resp, err := cli.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	var info struct {
		InputTokenLimit int `json:"inputTokenLimit"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return 0
	}
	if info.InputTokenLimit > 0 {
		return info.InputTokenLimit
	}
	return 0
}

// contextWindowFetchFunc allows overriding for tests.
var contextWindowFetchFunc = FetchModelContextWindow

// LookupModelContextWindow checks the known table first, then tries the provider API.
func LookupModelContextWindow(ctx context.Context, providerType, baseURL, apiKey, modelID string) int {
	if cw := knownContextWindow(modelID); cw > 0 {
		return cw
	}
	if ctx == nil {
		return 0
	}
	return contextWindowFetchFunc(ctx, providerType, baseURL, apiKey, modelID)
}
