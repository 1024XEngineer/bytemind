package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

const (
	defaultWebFetchMaxChars  = 6000
	maxWebFetchMaxChars      = 20000
	maxWebFetchTimeoutSecond = 60
	maxWebFetchBodyBytes     = 2 * 1024 * 1024
)

var (
	htmlScriptPattern = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	htmlStylePattern  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlTitlePattern  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlTagPattern    = regexp.MustCompile(`(?s)<[^>]*>`)
)

type WebFetchTool struct {
	HTTPClient *http.Client
}

func NewWebFetchTool() WebFetchTool {
	return WebFetchTool{}
}

func (WebFetchTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "web_fetch",
			Description: "Fetch a web page by URL and return lightweight content for citation.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "Target URL to fetch.",
					},
					"format": map[string]any{
						"type":        "string",
						"description": "Output format: text or html. Defaults to text.",
					},
					"max_chars": map[string]any{
						"type":        "integer",
						"description": "Maximum output characters (default 6000, max 20000).",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Request timeout in seconds (default 15, max 60).",
					},
				},
				"required": []string{"url"},
			},
		},
	}
}

func (t WebFetchTool) Run(ctx context.Context, raw json.RawMessage, _ *ExecutionContext) (string, error) {
	var args struct {
		URL            string `json:"url"`
		Format         string `json:"format"`
		MaxChars       int    `json:"max_chars"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}

	target, err := normalizeWebURL(args.URL)
	if err != nil {
		return "", err
	}

	format := strings.ToLower(strings.TrimSpace(args.Format))
	if format == "" {
		format = "text"
	}
	if format != "text" && format != "html" {
		return "", errors.New("format must be either text or html")
	}

	maxChars := args.MaxChars
	if maxChars <= 0 {
		maxChars = defaultWebFetchMaxChars
	}
	if maxChars > maxWebFetchMaxChars {
		maxChars = maxWebFetchMaxChars
	}

	timeout := defaultWebToolTimeout
	if args.TimeoutSeconds > 0 {
		if args.TimeoutSeconds > maxWebFetchTimeoutSecond {
			args.TimeoutSeconds = maxWebFetchTimeoutSecond
		}
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/html, text/plain, application/json;q=0.9, */*;q=0.5")
	request.Header.Set("User-Agent", defaultWebToolUA)

	client := webClientWithTimeout(t.HTTPClient, timeout)
	response, err := client.Do(request)
	if err != nil {
		return webFailureResult(webNetworkErrorCode(err), fmt.Sprintf("web fetch request failed: %v", err), map[string]any{
			"url": target,
		})
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		return webFailureResult(webHTTPStatusErrorCode(response.StatusCode), fmt.Sprintf("web fetch returned HTTP %d", response.StatusCode), map[string]any{
			"url":         target,
			"status_code": response.StatusCode,
		})
	}

	rawBody, err := io.ReadAll(io.LimitReader(response.Body, maxWebFetchBodyBytes+1))
	if err != nil {
		return webFailureResult("upstream_error", fmt.Sprintf("failed to read web response: %v", err), map[string]any{
			"url": target,
		})
	}

	bodyTruncated := false
	if len(rawBody) > maxWebFetchBodyBytes {
		rawBody = rawBody[:maxWebFetchBodyBytes]
		bodyTruncated = true
	}

	contentType := normalizeContentType(response.Header.Get("Content-Type"))
	if contentType != "" && !isTextualContentType(contentType) {
		return webFailureResult("unsupported_content_type", fmt.Sprintf("unsupported content type: %s", contentType), map[string]any{
			"url":          target,
			"content_type": contentType,
		})
	}

	rawText := string(rawBody)
	title := extractHTMLTitle(rawText)

	content := rawText
	if format == "text" {
		if looksLikeHTML(contentType, rawText) {
			content = htmlToText(rawText)
		} else {
			content = collapseWhitespace(rawText)
		}
	}
	content, truncatedByChars := truncateRunes(content, maxChars)
	if format == "html" {
		content = strings.TrimSpace(content)
	}

	finalURL := target
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String()
	}

	return toJSON(map[string]any{
		"ok":            true,
		"requested_url": target,
		"url":           finalURL,
		"status_code":   response.StatusCode,
		"content_type":  contentType,
		"title":         title,
		"content":       content,
		"truncated":     bodyTruncated || truncatedByChars,
		"format":        format,
	})
}

func normalizeWebURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("url is required")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("url must be absolute")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", errors.New("url scheme must be http or https")
	}
	return parsed.String(), nil
}

func normalizeContentType(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if index := strings.Index(raw, ";"); index >= 0 {
		raw = raw[:index]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

func isTextualContentType(contentType string) bool {
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	return strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "javascript") ||
		strings.Contains(contentType, "xhtml")
}

func looksLikeHTML(contentType, body string) bool {
	if strings.Contains(contentType, "html") || strings.Contains(contentType, "xhtml") {
		return true
	}
	head := strings.ToLower(strings.TrimSpace(body))
	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}

func extractHTMLTitle(body string) string {
	match := htmlTitlePattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return collapseWhitespace(html.UnescapeString(match[1]))
}

func htmlToText(body string) string {
	body = htmlScriptPattern.ReplaceAllString(body, " ")
	body = htmlStylePattern.ReplaceAllString(body, " ")
	body = htmlTagPattern.ReplaceAllString(body, " ")
	body = html.UnescapeString(body)
	return collapseWhitespace(body)
}
