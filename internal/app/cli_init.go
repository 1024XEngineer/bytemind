package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	configpkg "github.com/1024XEngineer/bytemind/internal/config"
)

func RunInit(args []string, stdout, stderr io.Writer) error {
	workspace := "."
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-workspace", "--workspace":
			if i+1 < len(args) {
				workspace = args[i+1]
				i++
			}
		}
	}

	reader := bufio.NewReader(os.Stdin)
	write := func(format string, v ...any) {
		fmt.Fprintf(stdout, format+"\n", v...)
	}
	prompt := func(msg string) string {
		fmt.Fprint(stdout, msg+" ")
		line, _ := reader.ReadString('\n')
		return strings.TrimSpace(line)
	}

	write("ByteMind Initialization")
	write("======================")
	write("")
	write("This will guide you through setting up ByteMind for first use.")
	write("")

	// Step 1: Set up home directory
	write("Step 1: Setting up ByteMind home directory...")
	home, err := configpkg.EnsureHomeLayout()
	if err != nil {
		return fmt.Errorf("setup home directory: %w", err)
	}
	write("  Home directory: %s", home)

	// Step 2: Select provider
	write("")
	write("Step 2: Choose your LLM provider:")
	write("  1) DeepSeek (default)")
	write("  2) OpenAI")
	write("  3) Anthropic")
	write("  4) Custom (OpenAI-compatible)")
	providerChoice := prompt("Enter choice [1]:")
	if providerChoice == "" {
		providerChoice = "1"
	}

	var providerType, baseURL, apiKeyEnv, model string
	switch strings.TrimSpace(providerChoice) {
	case "2":
		providerType = "openai"
		baseURL = "https://api.openai.com/v1"
		apiKeyEnv = "OPENAI_API_KEY"
		model = prompt("Enter model [gpt-5.4-mini]:")
		if model == "" {
			model = "gpt-5.4-mini"
		}
	case "3":
		providerType = "anthropic"
		baseURL = "https://api.anthropic.com"
		apiKeyEnv = "ANTHROPIC_API_KEY"
		model = prompt("Enter model [claude-sonnet-4-20250514]:")
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
	case "4":
		providerType = "openai-compatible"
		baseURL = prompt("Enter API base URL:")
		apiKeyEnv = prompt("Enter API key env var name [CUSTOM_API_KEY]:")
		if apiKeyEnv == "" {
			apiKeyEnv = "CUSTOM_API_KEY"
		}
		model = prompt("Enter model name:")
	default:
		providerType = "openai-compatible"
		baseURL = "https://api.deepseek.com"
		apiKeyEnv = "DEEPSEEK_API_KEY"
		model = "deepseek-v4-flash"
	}

	apiKey := prompt(fmt.Sprintf("Enter API key (or press Enter to use %s env var):", apiKeyEnv))

	// Step 3: Set approval policy
	write("")
	write("Step 3: Choose approval policy:")
	write("  1) on-request (default) - High-risk tools prompt for approval")
	write("  2) always - All tools require approval")
	write("  3) never - High-risk tools are blocked")
	approvalChoice := prompt("Enter choice [1]:")
	approvalPolicy := "on-request"
	switch strings.TrimSpace(approvalChoice) {
	case "2":
		approvalPolicy = "always"
	case "3":
		approvalPolicy = "never"
	}

	// Write config
	write("")
	write("Step 4: Writing configuration...")

	cfg := configpkg.Default(workspace)
	cfg.Provider.Type = providerType
	cfg.Provider.BaseURL = baseURL
	cfg.Provider.Model = model
	cfg.Provider.APIKeyEnv = apiKeyEnv
	if apiKey != "" {
		cfg.Provider.APIKey = apiKey
	}
	cfg.ProviderRuntime = configpkg.ProviderRuntimeConfig{
		CurrentProvider: "default",
		DefaultProvider: "default",
		DefaultModel:    model,
		Providers: map[string]configpkg.ProviderConfig{
			"default": cfg.Provider,
		},
	}
	cfg.ApprovalPolicy = approvalPolicy

	configPath := filepath.Join(home, "config.json")
	if err := configpkg.WriteConfig(configPath, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	write("  Configuration written to: %s", configPath)
	write("")
	write("Initialization complete! You can now use ByteMind.")
	write("")
	write("Next steps:")
	write("  - Run 'bytemind doctor' to verify your setup")
	write("  - Run 'bytemind demo bugfix' to see ByteMind in action")
	write("  - Run 'bytemind chat' to start an interactive session")
	write("  - Run 'bytemind run -prompt \"your task\"' for one-shot tasks")

	return nil
}
