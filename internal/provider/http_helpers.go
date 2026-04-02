package provider

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type providerHTTPError struct {
	StatusCode int
	Body       string
}

func (e *providerHTTPError) Error() string {
	return fmt.Sprintf("provider error %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
}

func resolveEndpointCandidates(baseURL, apiPath string, defaultPaths, endpointHints []string) []string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil
	}
	apiPath = strings.TrimSpace(apiPath)
	if apiPath != "" {
		return []string{buildEndpointURL(baseURL, apiPath)}
	}
	if looksLikeEndpoint(baseURL, endpointHints) {
		return []string{baseURL}
	}
	candidates := make([]string, 0, len(defaultPaths))
	for _, path := range defaultPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		candidates = append(candidates, buildEndpointURL(baseURL, path))
	}
	if len(candidates) == 0 {
		candidates = append(candidates, baseURL)
	}
	return dedupeStrings(candidates)
}

func buildEndpointURL(baseURL, endpointPath string) string {
	baseURL = strings.TrimSpace(baseURL)
	endpointPath = strings.TrimSpace(endpointPath)
	if endpointPath == "" {
		return baseURL
	}
	if parsed, err := url.Parse(endpointPath); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return endpointPath
	}
	parsedBase, err := url.Parse(baseURL)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpointPath, "/")
	}
	relative, err := url.Parse(endpointPath)
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpointPath, "/")
	}

	relativePath := strings.TrimLeft(relative.Path, "/")
	basePath := strings.TrimSuffix(parsedBase.Path, "/")
	switch {
	case relativePath == "":
	case basePath == "":
		parsedBase.Path = "/" + relativePath
	default:
		parsedBase.Path = basePath + "/" + relativePath
	}

	if relative.RawQuery != "" {
		parsedBase.RawQuery = relative.RawQuery
	}
	if relative.Fragment != "" {
		parsedBase.Fragment = relative.Fragment
	}
	return parsedBase.String()
}

func looksLikeEndpoint(value string, hints []string) bool {
	path := strings.ToLower(extractPath(value))
	for _, hint := range hints {
		hint = strings.ToLower(strings.TrimSpace(hint))
		if hint == "" {
			continue
		}
		if strings.Contains(path, hint) {
			return true
		}
	}
	return false
}

func extractPath(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err == nil && parsed.Path != "" {
		return parsed.Path
	}
	return value
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func applyAuthAndExtraHeaders(req *http.Request, authHeader, authScheme, apiKey string, extraHeaders map[string]string) {
	headerName := strings.TrimSpace(authHeader)
	if headerName == "" {
		headerName = "Authorization"
	}
	key := strings.TrimSpace(apiKey)
	if key != "" {
		scheme := strings.TrimSpace(authScheme)
		if scheme != "" {
			req.Header.Set(headerName, scheme+" "+key)
		} else {
			req.Header.Set(headerName, key)
		}
	}
	for name, value := range extraHeaders {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		req.Header.Set(name, value)
	}
}

func isEndpointNotFoundError(err error) bool {
	var httpErr *providerHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	switch httpErr.StatusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusGone:
		return true
	default:
		return false
	}
}

func isCompatibilityPayloadError(err error) bool {
	var httpErr *providerHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	switch httpErr.StatusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusNotImplemented:
	default:
		return false
	}
	body := strings.ToLower(httpErr.Body)
	if body == "" {
		return false
	}
	signals := []string{
		"unknown field",
		"unsupported",
		"not supported",
		"tool_choice",
		"tools",
		"temperature",
		"extra inputs are not permitted",
	}
	for _, signal := range signals {
		if strings.Contains(body, signal) {
			return true
		}
	}
	return false
}
