package tools

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

const (
	defaultWebSearchBaseURL = "https://www.bing.com/search"
	defaultWebSearchLimit   = 5
	maxWebSearchLimit       = 10
	maxWebSearchBodyBytes   = 2 * 1024 * 1024
)

var (
	searchHTMLTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)
)

type WebSearchTool struct {
	BaseURL    string
	HTTPClient *http.Client
}

type webSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
	Rank    int    `json:"rank"`
}

type rssResultFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

func NewWebSearchTool() WebSearchTool {
	return WebSearchTool{
		BaseURL: defaultWebSearchBaseURL,
	}
}

func (WebSearchTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "web_search",
			Description: "Search the web without API keys and return candidate sources.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query.",
					},
					"max_results": map[string]any{
						"type":        "integer",
						"description": "Maximum number of results to return (default 5, max 10).",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (t WebSearchTool) Run(ctx context.Context, raw json.RawMessage, _ *ExecutionContext) (string, error) {
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}

	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", errors.New("query is required")
	}

	limit := args.MaxResults
	if limit <= 0 {
		limit = defaultWebSearchLimit
	}
	if limit > maxWebSearchLimit {
		limit = maxWebSearchLimit
	}

	baseURL := strings.TrimSpace(t.BaseURL)
	if baseURL == "" {
		baseURL = defaultWebSearchBaseURL
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return webFailureResult("invalid_configuration", "web search URL is invalid", map[string]any{
			"query": query,
		})
	}

	params := endpoint.Query()
	params.Set("q", query)
	if strings.TrimSpace(params.Get("format")) == "" {
		params.Set("format", "rss")
	}
	endpoint.RawQuery = params.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, text/html;q=0.8")
	request.Header.Set("User-Agent", defaultWebToolUA)

	client := webClientWithTimeout(t.HTTPClient, defaultWebToolTimeout)
	response, err := client.Do(request)
	if err != nil {
		return webFailureResult(webNetworkErrorCode(err), fmt.Sprintf("web search request failed: %v", err), map[string]any{
			"query": query,
		})
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		return webFailureResult(webHTTPStatusErrorCode(response.StatusCode), fmt.Sprintf("web search returned HTTP %d", response.StatusCode), map[string]any{
			"query":       query,
			"status_code": response.StatusCode,
		})
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxWebSearchBodyBytes))
	if err != nil {
		return webFailureResult("upstream_error", fmt.Sprintf("failed to read web search response: %v", err), map[string]any{
			"query": query,
		})
	}

	results := parseRSSResults(body, limit)
	if len(results) == 0 {
		results = parseHTMLAnchorResults(body, endpoint, limit)
	}

	return toJSON(map[string]any{
		"ok":      true,
		"query":   query,
		"source":  endpoint.Host,
		"results": results,
	})
}

func parseRSSResults(body []byte, limit int) []webSearchResult {
	var feed rssResultFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(feed.XMLName.Local)) != "rss" {
		return nil
	}
	results := make([]webSearchResult, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, item := range feed.Channel.Items {
		link := strings.TrimSpace(item.Link)
		if link == "" {
			continue
		}
		if _, ok := seen[link]; ok {
			continue
		}
		seen[link] = struct{}{}
		result := webSearchResult{
			Title:   fallbackSearchTitle(item.Title, link, item.Description),
			URL:     link,
			Snippet: cleanSearchSnippet(item.Description),
			Rank:    len(results) + 1,
		}
		results = append(results, result)
		if len(results) >= limit {
			break
		}
	}
	return results
}

func parseHTMLAnchorResults(body []byte, endpoint *url.URL, limit int) []webSearchResult {
	content := string(body)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`),
		regexp.MustCompile(`(?is)<a[^>]*href="([^"]+)"[^>]*>(.*?)</a>`),
	}

	results := make([]webSearchResult, 0, limit)
	seen := make(map[string]struct{}, limit)
	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(content, limit*6)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}
			link := normalizeSearchLink(strings.TrimSpace(match[1]), endpoint)
			if link == "" {
				continue
			}
			if _, ok := seen[link]; ok {
				continue
			}
			seen[link] = struct{}{}
			title := cleanSearchSnippet(match[2])
			if !looksSearchResultTitle(title) {
				continue
			}
			results = append(results, webSearchResult{
				Title: fallbackSearchTitle(title, link, ""),
				URL:   link,
				Rank:  len(results) + 1,
			})
			if len(results) >= limit {
				return results
			}
		}
		if len(results) > 0 {
			return results
		}
	}
	return results
}

func normalizeSearchLink(raw string, endpoint *url.URL) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" {
		return ""
	}

	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if !parsed.IsAbs() && endpoint != nil {
		parsed = endpoint.ResolveReference(parsed)
	}
	if parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if strings.Contains(parsed.Path, "/l/") || strings.Contains(parsed.Path, "/redirect") {
		query := parsed.Query()
		for _, key := range []string{"uddg", "url", "u"} {
			if target := strings.TrimSpace(query.Get(key)); target != "" {
				if decoded, err := url.QueryUnescape(target); err == nil {
					target = decoded
				}
				if normalized, ok := normalizeAbsoluteHTTPURL(target); ok {
					return normalized
				}
			}
		}
	}
	if normalized, ok := normalizeAbsoluteHTTPURL(parsed.String()); ok {
		return normalized
	}
	return ""
}

func normalizeAbsoluteHTTPURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", false
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", false
	}
	return parsed.String(), true
}

func fallbackSearchTitle(title, rawURL, snippet string) string {
	title = cleanSearchSnippet(title)
	if title != "" {
		return title
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err == nil && strings.TrimSpace(parsed.Host) != "" {
		return parsed.Host
	}
	snippet = cleanSearchSnippet(snippet)
	if snippet == "" {
		return rawURL
	}
	runes := []rune(snippet)
	if len(runes) <= 72 {
		return snippet
	}
	return string(runes[:72]) + "..."
}

func cleanSearchSnippet(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = html.UnescapeString(raw)
	raw = searchHTMLTagPattern.ReplaceAllString(raw, " ")
	return collapseWhitespace(raw)
}

func looksSearchResultTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	lower := strings.ToLower(title)
	if strings.HasPrefix(lower, "cached") || strings.HasPrefix(lower, "similar") {
		return false
	}
	return true
}
