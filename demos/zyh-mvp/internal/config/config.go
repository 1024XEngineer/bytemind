package config

import (
	"bytes"
	"encoding/json"
	"os"
)

type Config struct {
	WorkspaceRoot         string      `json:"workspace_root"`
	DefaultVerifyCmd      string      `json:"default_verify_cmd"`
	LogLevel              string      `json:"log_level"`
	DangerousCmdDenylist  []string    `json:"dangerous_cmd_denylist"`
	SensitivePatterns     []string    `json:"sensitive_patterns"`
	SearchIgnore          []string    `json:"search_ignore"`
	CommandTimeoutSeconds int         `json:"command_timeout_seconds"`
	Model                 ModelConfig `json:"model"`
	Agent                 AgentConfig `json:"agent"`
}

type ModelConfig struct {
	Provider       string  `json:"provider"`
	BaseURL        string  `json:"base_url"`
	APIKey         string  `json:"api_key"`
	APIKeyEnv      string  `json:"api_key_env"`
	Model          string  `json:"model"`
	SystemPrompt   string  `json:"system_prompt"`
	Temperature    float64 `json:"temperature"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}

type AgentConfig struct {
	MaxToolRounds      int `json:"max_tool_rounds"`
	MaxHistoryMessages int `json:"max_history_messages"`
	ReadFileMaxBytes   int `json:"read_file_max_bytes"`
	ListFilesLimit     int `json:"list_files_limit"`
	SearchLimit        int `json:"search_limit"`
}

func Default() Config {
	return Config{
		LogLevel:              "info",
		DangerousCmdDenylist:  []string{"rm", "del", "rmdir", "rd", "format", "shutdown", "reboot", "mkfs", "powershell", "pwsh", "cmd", "bash", "sh", "curl", "wget", "invoke-webrequest", "iwr", "irm", "remove-item"},
		SensitivePatterns:     []string{".env", ".pem", ".key", ".crt", "id_rsa", "id_ed25519"},
		SearchIgnore:          []string{".git", "node_modules", "vendor", "bin", ".gocache", ".gotmp"},
		CommandTimeoutSeconds: 15,
		Model: ModelConfig{
			Provider:       "openai_compatible",
			BaseURL:        "https://api.openai.com/v1",
			APIKeyEnv:      "OPENAI_API_KEY",
			Model:          "gpt-4.1-mini",
			SystemPrompt:   defaultSystemPrompt(),
			Temperature:    0,
			TimeoutSeconds: 120,
		},
		Agent: AgentConfig{MaxToolRounds: 12, MaxHistoryMessages: 40, ReadFileMaxBytes: 20000, ListFilesLimit: 200, SearchLimit: 20},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var user Config
	if err := json.Unmarshal(data, &user); err != nil {
		return Config{}, err
	}
	return merge(cfg, user), nil
}

func merge(base, user Config) Config {
	cfg := base
	if user.WorkspaceRoot != "" {
		cfg.WorkspaceRoot = user.WorkspaceRoot
	}
	if user.DefaultVerifyCmd != "" {
		cfg.DefaultVerifyCmd = user.DefaultVerifyCmd
	}
	if user.LogLevel != "" {
		cfg.LogLevel = user.LogLevel
	}
	if len(user.DangerousCmdDenylist) > 0 {
		cfg.DangerousCmdDenylist = user.DangerousCmdDenylist
	}
	if len(user.SensitivePatterns) > 0 {
		cfg.SensitivePatterns = user.SensitivePatterns
	}
	if len(user.SearchIgnore) > 0 {
		cfg.SearchIgnore = user.SearchIgnore
	}
	if user.CommandTimeoutSeconds > 0 {
		cfg.CommandTimeoutSeconds = user.CommandTimeoutSeconds
	}
	if user.Model.Provider != "" {
		cfg.Model.Provider = user.Model.Provider
	}
	if user.Model.BaseURL != "" {
		cfg.Model.BaseURL = user.Model.BaseURL
	}
	if user.Model.APIKey != "" {
		cfg.Model.APIKey = user.Model.APIKey
	}
	if user.Model.APIKeyEnv != "" {
		cfg.Model.APIKeyEnv = user.Model.APIKeyEnv
	}
	if user.Model.Model != "" {
		cfg.Model.Model = user.Model.Model
	}
	if user.Model.SystemPrompt != "" {
		cfg.Model.SystemPrompt = user.Model.SystemPrompt
	}
	if user.Model.Temperature != 0 {
		cfg.Model.Temperature = user.Model.Temperature
	}
	if user.Model.TimeoutSeconds > 0 {
		cfg.Model.TimeoutSeconds = user.Model.TimeoutSeconds
	}
	if user.Agent.MaxToolRounds > 0 {
		cfg.Agent.MaxToolRounds = user.Agent.MaxToolRounds
	}
	if user.Agent.MaxHistoryMessages > 0 {
		cfg.Agent.MaxHistoryMessages = user.Agent.MaxHistoryMessages
	}
	if user.Agent.ReadFileMaxBytes > 0 {
		cfg.Agent.ReadFileMaxBytes = user.Agent.ReadFileMaxBytes
	}
	if user.Agent.ListFilesLimit > 0 {
		cfg.Agent.ListFilesLimit = user.Agent.ListFilesLimit
	}
	if user.Agent.SearchLimit > 0 {
		cfg.Agent.SearchLimit = user.Agent.SearchLimit
	}
	return cfg
}

func defaultSystemPrompt() string {
	return "You are ForgeCLI, a local coding assistant. Work inside the provided workspace, inspect files before editing, and prefer file tools over shell commands. Use list_files, read_file, and search for repository exploration. For standalone webpage requests, prefer a single self-contained HTML file with inline CSS and JavaScript instead of splitting into style.css or game.js. Do not use run_command for pwd, cd, dir, ls, or other directory inspection unless the user explicitly asks for shell behavior. Use run_command mainly for explicit verification tasks such as tests, builds, or git status. Keep final answers concise. When you write a file, send the full updated file content. If a tool reports a denied approval or an error, adapt and explain the next best step."
}
