package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
)

type remoteTokenUsage struct {
	Used    int
	Input   int
	Output  int
	Context int
}

func fetchCurrentMonthUsage(cfg config.Config) (remoteTokenUsage, error) {
	baseURL := strings.TrimSpace(cfg.Provider.BaseURL)
	apiKey := strings.TrimSpace(cfg.Provider.ResolveAPIKey())
	if baseURL == "" || apiKey == "" {
		return remoteTokenUsage{}, fmt.Errorf("usage pull skipped: missing base url or api key")
	}
	if !strings.Contains(strings.ToLower(baseURL), "api.openai.com") {
		return remoteTokenUsage{}, fmt.Errorf("usage pull skipped: non-openai base url")
	}

	end := time.Now().UTC()
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	startUnix := start.Unix()
	endUnix := end.Unix()

	apiBase := normalizeOpenAIBaseURL(baseURL)
	values := url.Values{}
	values.Set("start_time", fmt.Sprintf("%d", startUnix))
	values.Set("end_time", fmt.Sprintf("%d", endUnix))
	values.Set("bucket_width", "1d")
	values.Set("limit", "31")

	endpoint := apiBase + "/organization/usage/completions?" + values.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return remoteTokenUsage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return remoteTokenUsage{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return remoteTokenUsage{}, err
	}
	if resp.StatusCode >= 300 {
		return remoteTokenUsage{}, fmt.Errorf("usage pull failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var page struct {
		Data []struct {
			Results []struct {
				InputTokens       int `json:"input_tokens"`
				OutputTokens      int `json:"output_tokens"`
				InputCachedTokens int `json:"input_cached_tokens"`
				InputAudioTokens  int `json:"input_audio_tokens"`
				OutputAudioTokens int `json:"output_audio_tokens"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return remoteTokenUsage{}, err
	}

	usage := remoteTokenUsage{}
	for _, bucket := range page.Data {
		for _, result := range bucket.Results {
			input := max(0, result.InputTokens) + max(0, result.InputAudioTokens)
			output := max(0, result.OutputTokens) + max(0, result.OutputAudioTokens)
			context := max(0, result.InputCachedTokens)
			usage.Input += input
			usage.Output += output
			usage.Context += context
			usage.Used += input + output + context
		}
	}

	return usage, nil
}

func normalizeOpenAIBaseURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		return base
	}
	return base + "/v1"
}
