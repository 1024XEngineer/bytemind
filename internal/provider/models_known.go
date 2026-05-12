package provider

import (
	"strconv"
	"strings"
)

type modelEntry struct {
	prefix       string
	exact        string
	contextWindow int
}

// knownModelContextWindows is ordered: exact matches first (checked in a separate pass),
// then prefix matches from most specific to least specific.
var knownModelContextWindows = []modelEntry{
	// ── Exact matches (checked first) ──────────────────────────────
	{exact: "gpt-4-32k", contextWindow: 32768},
	{exact: "gpt-4-turbo", contextWindow: 128000},

	// ── Prefix matches (most specific first) ───────────────────────

	// ── OpenAI ──────────────────────────────────────────────────────
	{prefix: "gpt-5.5", contextWindow: 1000000},
	{prefix: "gpt-5.4-mini", contextWindow: 400000},
	{prefix: "gpt-5.4", contextWindow: 1000000},
	{prefix: "gpt-4.1", contextWindow: 1047576},
	{prefix: "gpt-4o", contextWindow: 128000},
	{prefix: "gpt-4-turbo", contextWindow: 128000},
	{prefix: "gpt-4-32k", contextWindow: 32768},
	{prefix: "gpt-4", contextWindow: 8192},
	{prefix: "gpt-3.5-turbo-16k", contextWindow: 16384},
	{prefix: "gpt-3.5-turbo", contextWindow: 16384},
	{prefix: "gpt-3.5", contextWindow: 4096},
	{prefix: "o1", contextWindow: 200000},
	{prefix: "o3", contextWindow: 200000},
	{prefix: "o4", contextWindow: 200000},

	// ── Anthropic ───────────────────────────────────────────────────
	{prefix: "claude-opus", contextWindow: 1000000},
	{prefix: "claude-sonnet", contextWindow: 1000000},
	{prefix: "claude-haiku", contextWindow: 200000},
	{prefix: "claude-5", contextWindow: 200000},
	{prefix: "claude-4", contextWindow: 200000},
	{prefix: "claude-3.5", contextWindow: 200000},
	{prefix: "claude-3", contextWindow: 200000},
	{prefix: "claude", contextWindow: 100000},

	// ── Google Gemini ──────────────────────────────────────────────
	{prefix: "gemini-3.1-pro", contextWindow: 1048576},
	{prefix: "gemini-3-flash", contextWindow: 1048576},
	{prefix: "gemini-3", contextWindow: 1048576},
	{prefix: "gemini-2.0-flash-lite", contextWindow: 1000000},
	{prefix: "gemini-2.0-flash", contextWindow: 1000000},
	{prefix: "gemini-2.5-pro", contextWindow: 1048576},
	{prefix: "gemini-2.5-flash", contextWindow: 1048576},
	{prefix: "gemini-2.5", contextWindow: 1048576},
	{prefix: "gemini-1.5-pro", contextWindow: 2000000},
	{prefix: "gemini-1.5-flash", contextWindow: 1000000},
	{prefix: "gemini-1.5", contextWindow: 1000000},
	{prefix: "gemini-2.0", contextWindow: 1000000},
	{prefix: "gemini-exp", contextWindow: 1000000},
	{prefix: "learnlm", contextWindow: 1000000},
	{prefix: "gemini", contextWindow: 1000000},

	// ── DeepSeek ────────────────────────────────────────────────────
	{prefix: "deepseek-v4-flash", contextWindow: 1000000},
	{prefix: "deepseek-v4-pro", contextWindow: 1000000},
	{prefix: "deepseek-v4", contextWindow: 1000000},
	{prefix: "deepseek-chat", contextWindow: 1000000},
	{prefix: "deepseek-reasoner", contextWindow: 1000000},
	{prefix: "deepseek-v3", contextWindow: 65536},
	{prefix: "deepseek-r1", contextWindow: 65536},
	{prefix: "deepseek", contextWindow: 65536},

	// ── Alibaba Qwen ────────────────────────────────────────────────
	{prefix: "qwen-long", contextWindow: 10000000},
	{prefix: "qwen3-max", contextWindow: 262144},
	{prefix: "qwen3", contextWindow: 262144},
	{prefix: "qwen-plus", contextWindow: 1000000},
	{prefix: "qwen-turbo", contextWindow: 1000000},
	{prefix: "qwen-2.5", contextWindow: 131072},
	{prefix: "qwen-2", contextWindow: 131072},
	{prefix: "qwen", contextWindow: 32768},

	// ── Mistral ─────────────────────────────────────────────────────
	{prefix: "mistral-large", contextWindow: 256000},
	{prefix: "mistral-medium", contextWindow: 256000},
	{prefix: "mistral-small", contextWindow: 128000},
	{prefix: "mistral-tiny", contextWindow: 32768},
	{prefix: "ministral", contextWindow: 128000},
	{prefix: "mistral", contextWindow: 32768},
	{prefix: "mixtral", contextWindow: 32768},
	{prefix: "open-mistral", contextWindow: 32768},
	{prefix: "open-mixtral", contextWindow: 32768},

	// ── xAI Grok ────────────────────────────────────────────────────
	{prefix: "grok-4", contextWindow: 1000000},
	{prefix: "grok-3", contextWindow: 1000000},
	{prefix: "grok-2", contextWindow: 131072},
	{prefix: "grok", contextWindow: 131072},

	// ── Meta Llama ──────────────────────────────────────────────────
	{prefix: "llama-4-scout", contextWindow: 10000000},
	{prefix: "llama-4-maverick", contextWindow: 1000000},
	{prefix: "llama-4", contextWindow: 1000000},
	{prefix: "llama-3.2", contextWindow: 131072},
	{prefix: "llama-3.1", contextWindow: 131072},
	{prefix: "llama-3", contextWindow: 8192},
	{prefix: "llama-2", contextWindow: 4096},
	{prefix: "llama", contextWindow: 8192},

	// ── Cohere ──────────────────────────────────────────────────────
	{prefix: "command-a", contextWindow: 256000},
	{prefix: "command-r-plus", contextWindow: 128000},
	{prefix: "command-r", contextWindow: 128000},
	{prefix: "command", contextWindow: 4096},

	// ── Z.AI / GLM ──────────────────────────────────────────────────
	{prefix: "glm-4.5", contextWindow: 128000},
	{prefix: "glm-4", contextWindow: 128000},
	{prefix: "glm-3", contextWindow: 128000},
	{prefix: "glm", contextWindow: 128000},

	// ── Moonshot / Kimi ─────────────────────────────────────────────
	{prefix: "kimi-k2.6", contextWindow: 256000},
	{prefix: "kimi-k2.5", contextWindow: 256000},
	{prefix: "kimi-k2", contextWindow: 256000},
	{prefix: "kimi", contextWindow: 128000},

	// ── Others ──────────────────────────────────────────────────────
	{prefix: "phi-4", contextWindow: 131072},
	{prefix: "phi-3", contextWindow: 131072},
	{prefix: "phi", contextWindow: 4096},
	{prefix: "gemma-2", contextWindow: 8192},
	{prefix: "gemma", contextWindow: 8192},
	{prefix: "dbrx", contextWindow: 32768},
	{prefix: "aya", contextWindow: 8192},
	{prefix: "solar", contextWindow: 32768},
}

// normalizeModelID prepares a user-supplied model ID for table matching.
// It handles case, common provider prefixes, and path-style IDs.
func normalizeModelID(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ""
	}
	// Strip common "provider/" prefix (e.g. "openai/gpt-4o", "azure/gpt-4o")
	if slash := strings.IndexByte(id, '/'); slash > 0 && slash < len(id)-1 {
		prefix := id[:slash]
		// Only strip if it looks like a provider/endpoint name, not a model family
		if isProviderPrefix(prefix) {
			id = id[slash+1:]
		}
	}
	// Trim again since some IDs have "organization/model-name" pattern
	id = strings.TrimSpace(id)
	return id
}

func isProviderPrefix(s string) bool {
	known := []string{
		"openai", "anthropic", "google", "gemini", "azure", "azure-openai",
		"aws", "aws-bedrock", "bedrock", "deepseek", "together", "together_ai",
		"openrouter", "xai", "meta", "mistral", "cohere", "qwen", "alibaba",
		"moonshot", "zai", "zhipu", "huggingface", "hf", "replicate",
		"groq", "fireworks", "perplexity", "anyscale", "togethercomputer",
	}
	for _, k := range known {
		if s == k {
			return true
		}
	}
	return false
}

func knownContextWindow(modelID string) int {
	id := normalizeModelID(modelID)
	if id == "" {
		return 0
	}
	for _, entry := range knownModelContextWindows {
		if entry.exact != "" && id == entry.exact {
			return entry.contextWindow
		}
	}
	for _, entry := range knownModelContextWindows {
		if entry.prefix != "" && strings.HasPrefix(id, entry.prefix) {
			return entry.contextWindow
		}
	}
	// If still not found, try matching after the last "/" for path-style IDs
	// like "meta-llama/Llama-4-Scout-17B" → "llama-4-scout-17b"
	if lastSlash := strings.LastIndexByte(id, '/'); lastSlash > 0 && lastSlash < len(id)-1 {
		tail := id[lastSlash+1:]
		for _, entry := range knownModelContextWindows {
			if entry.exact != "" && tail == entry.exact {
				return entry.contextWindow
			}
		}
		for _, entry := range knownModelContextWindows {
			if entry.prefix != "" && strings.HasPrefix(tail, entry.prefix) {
				return entry.contextWindow
			}
		}
	}
	return 0
}

func init() {
	m := map[string]struct{}{}
	for _, e := range knownModelContextWindows {
		var k string
		switch {
		case e.exact != "":
			k = "exact:" + e.exact
		case e.prefix != "":
			k = "prefix:" + e.prefix
		default:
			panic("known model entry has neither exact nor prefix")
		}
		if _, ok := m[k]; ok {
			panic("duplicate known model entry: " + k)
		}
		m[k] = struct{}{}
	}
}

func fillContextWindow(meta map[string]string, modelID string) {
	if meta == nil {
		return
	}
	if strings.TrimSpace(meta["context_window"]) != "" {
		return
	}
	cw := knownContextWindow(modelID)
	if cw > 0 {
		meta["context_window"] = strconv.Itoa(cw)
	}
}
