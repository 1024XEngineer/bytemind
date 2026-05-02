package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
)

const (
	preflightTimeout = 8 * time.Second
	maxErrorBodySize = 4 * 1024
)

type Availability struct {
	Ready  bool
	Reason string
	Detail string
}

func CheckAvailability(ctx context.Context, cfg config.ProviderConfig) Availability {
	apiKey := strings.TrimSpace(cfg.ResolveAPIKey())
	if apiKey == "" {
		return Availability{
			Ready:  false,
			Reason: "missing API key",
			Detail: "No API key was found in provider.api_key or provider.api_key_env.",
		}
	}

	endpoint, headers := preflightRequest(cfg, apiKey)
	if strings.TrimSpace(endpoint) == "" {
		return Availability{
			Ready:  false,
			Reason: "provider configuration is incomplete",
			Detail: "Provider base_url is empty.",
		}
	}

	reqCtx, cancel := context.WithTimeout(ctx, preflightTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Availability{
			Ready:  false,
			Reason: "failed to build provider check request",
			Detail: err.Error(),
		}
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := (&http.Client{Timeout: preflightTimeout}).Do(req)
	if err != nil {
		return Availability{
			Ready:  false,
			Reason: "failed to reach provider endpoint",
			Detail: err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Availability{Ready: true}
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		detail = fmt.Sprintf("provider returned HTTP %d", resp.StatusCode)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return Availability{
			Ready:  false,
			Reason: "API key unauthorized",
			Detail: detail,
		}
	case http.StatusNotFound:
		return Availability{
			Ready:  false,
			Reason: "provider endpoint not found",
			Detail: detail,
		}
	default:
		return Availability{
			Ready:  false,
			Reason: fmt.Sprintf("provider check failed (HTTP %d)", resp.StatusCode),
			Detail: detail,
		}
	}
}

func preflightRequest(cfg config.ProviderConfig, apiKey string) (string, map[string]string) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	providerType := strings.ToLower(strings.TrimSpace(cfg.Type))

	switch providerType {
	case "anthropic":
		version := strings.TrimSpace(cfg.AnthropicVersion)
		if version == "" {
			version = "2023-06-01"
		}
		return baseURL + "/v1/models", map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": version,
		}
	case "gemini":
		return baseURL + "/models", map[string]string{
			"x-goog-api-key": apiKey,
		}
	default:
		return baseURL + "/models", map[string]string{
			"Authorization": "Bearer " + apiKey,
		}
	}
}
