package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"forgecli/internal/agent"
	"forgecli/internal/app"
	"forgecli/internal/config"
	"forgecli/internal/generator"
	"forgecli/internal/model"
	"forgecli/internal/ui"
)

var outputPathPattern = regexp.MustCompile(`(?i)([A-Za-z0-9._\\/-]+\.[A-Za-z0-9]+)`)

func Execute(args []string, version string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		printHelp(out)
		return 0
	}

	switch args[0] {
	case "version":
		fmt.Fprintln(out, version)
		return 0
	case "run":
		return run(args[1:], in, out, errOut)
	case "chat":
		return chat(args[1:], in, out, errOut)
	case "generate":
		return generate(args[1:], out, errOut)
	case "help", "-h", "--help":
		printHelp(out)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown command: %s\n\n", args[0])
		printHelp(errOut)
		return 2
	}
}

func run(args []string, in io.Reader, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("forgecli run", flag.ContinueOnError)
	flags.SetOutput(errOut)
	var repo, task, verify, configPath string
	flags.StringVar(&repo, "repo", "", "path to the target repository")
	flags.StringVar(&task, "task", "", "task description")
	flags.StringVar(&verify, "verify", "", "verification command to run after write")
	flags.StringVar(&configPath, "config", "", "optional path to forgecli.json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(errOut, "failed to load config: %v\n", err)
		return 1
	}
	if strings.TrimSpace(repo) == "" {
		repo = cfg.WorkspaceRoot
	}
	if strings.TrimSpace(verify) == "" {
		verify = cfg.DefaultVerifyCmd
	}
	if strings.TrimSpace(task) == "" {
		fmt.Fprintln(errOut, "task is required")
		flags.Usage()
		return 2
	}
	if strings.TrimSpace(repo) == "" {
		fmt.Fprintln(errOut, "repo is required")
		flags.Usage()
		return 2
	}
	logger := newLogger(errOut, cfg.LogLevel)
	terminal := ui.NewConsole(in, out)
	application := app.New(cfg, terminal, logger)
	params := app.Params{RepoPath: repo, Task: task, VerifyCommand: verify}
	if err := application.Run(context.Background(), params); err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	return 0
}

func chat(args []string, in io.Reader, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("forgecli chat", flag.ContinueOnError)
	flags.SetOutput(errOut)
	var repo, configPath, prompt, mode string
	flags.StringVar(&repo, "repo", "", "path to the target repository")
	flags.StringVar(&configPath, "config", "", "optional path to forgecli.json")
	flags.StringVar(&prompt, "prompt", "", "optional single prompt to run before exit")
	flags.StringVar(&mode, "mode", "analyze", "chat mode: analyze or full")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(errOut, "failed to load config: %v\n", err)
		return 1
	}
	if strings.TrimSpace(repo) == "" {
		repo = cfg.WorkspaceRoot
	}
	if strings.TrimSpace(repo) == "" {
		repo = "."
	}

	logger := newLogger(errOut, cfg.LogLevel)
	console := ui.NewConsole(in, out)
	chatAgent, err := agent.New(cfg, console, logger, nil)
	if err != nil {
		fmt.Fprintf(errOut, "failed to create agent: %v\n", err)
		return 1
	}
	if err := chatAgent.SetMode(mode); err != nil {
		fmt.Fprintf(errOut, "invalid mode: %v\n", err)
		return 2
	}
	if err := chatAgent.StartSession(repo); err != nil {
		fmt.Fprintf(errOut, "failed to start session: %v\n", err)
		return 1
	}

	var genClient model.ChatClient
	if shouldUseModel(cfg.Model) {
		genClient, err = model.NewChatClient(cfg.Model)
		if err != nil {
			console.Warn("Model init failed for file generation. Falling back to minimal local scaffolds.")
			genClient = nil
		}
	}
	genService := generator.New(cfg, genClient)

	printChatIntro(console, chatAgent)
	ctx := context.Background()

	if strings.TrimSpace(prompt) != "" {
		return runChatInput(ctx, console, chatAgent, genService, prompt)
	}

	for {
		line, err := console.PromptLine(console.Style("> ", "\x1b[36m"))
		if err != nil {
			fmt.Fprintf(errOut, "error: %v\n", err)
			return 1
		}
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if strings.HasPrefix(input, "/") {
			exit, err := handleSlashCommandConsole(console, chatAgent, input)
			if err != nil {
				console.Error(err.Error())
				continue
			}
			if exit {
				return 0
			}
			continue
		}
		if code := runChatInput(ctx, console, chatAgent, genService, input); code != 0 {
			return code
		}
	}
}

func runChatInput(ctx context.Context, console *ui.Console, chatAgent *agent.Agent, genService *generator.Service, input string) int {
	if handled, err := maybeHandleFileGeneration(ctx, console, chatAgent, genService, input); handled {
		if err != nil {
			console.Error(humanizeModelError(err, chatAgent.ModelName(), os.Getenv("DEEPSEEK_API_KEY") != "", "DEEPSEEK_API_KEY"))
		}
		return 0
	}
	response, err := chatAgent.RunTurn(ctx, input)
	if err != nil {
		console.Error(humanizeModelError(err, chatAgent.ModelName(), os.Getenv("DEEPSEEK_API_KEY") != "", "DEEPSEEK_API_KEY"))
		return 0
	}
	console.Assistant(response)
	return 0
}

func generate(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("forgecli generate", flag.ContinueOnError)
	flags.SetOutput(errOut)
	var prompt, outputPath, configPath string
	flags.StringVar(&prompt, "prompt", "", "what code or page to generate")
	flags.StringVar(&outputPath, "output", "generated.html", "output file path")
	flags.StringVar(&configPath, "config", "", "optional path to forgecli.json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(prompt) == "" {
		fmt.Fprintln(errOut, "prompt is required")
		flags.Usage()
		return 2
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(errOut, "failed to load config: %v\n", err)
		return 1
	}
	var client model.ChatClient
	if shouldUseModel(cfg.Model) {
		client, err = model.NewChatClient(cfg.Model)
		if err != nil {
			fmt.Fprintf(errOut, "warning: model init failed, using local file scaffold fallback: %v\n", err)
			client = nil
		}
	}
	service := generator.New(cfg, client)
	path, err := service.GenerateFile(context.Background(), prompt, outputPath)
	if err != nil {
		fmt.Fprintf(errOut, "%s\n", humanizeModelError(err, cfg.Model.Model, strings.TrimSpace(os.Getenv(cfg.Model.APIKeyEnv)) != "", cfg.Model.APIKeyEnv))
		return 1
	}
	fmt.Fprintf(out, "File generated: %s\n", path)
	return 0
}

func handleSlashCommandConsole(console *ui.Console, chatAgent *agent.Agent, input string) (bool, error) {
	trimmed := strings.TrimSpace(input)
	switch {
	case strings.EqualFold(trimmed, "/help"):
		console.System("Commands: /help /tools /mode analyze /mode full /reset /exit")
	case strings.EqualFold(trimmed, "/tools"):
		console.System("Mode: " + chatAgent.ModeName() + " | Tools: " + strings.Join(chatAgent.ToolNames(), ", "))
	case strings.EqualFold(trimmed, "/reset"):
		chatAgent.Reset()
		console.System("Conversation history cleared.")
	case strings.EqualFold(trimmed, "/exit") || strings.EqualFold(trimmed, "/quit"):
		console.System("Bye.")
		return true, nil
	case strings.EqualFold(trimmed, "/mode analyze"):
		if err := chatAgent.SetMode("analyze"); err != nil {
			return false, err
		}
		console.System("Switched to analyze mode.")
	case strings.EqualFold(trimmed, "/mode full"):
		if err := chatAgent.SetMode("full"); err != nil {
			return false, err
		}
		console.System("Switched to full mode.")
	default:
		console.Warn("Unknown command. Use /help.")
	}
	return false, nil
}

func printChatIntro(console *ui.Console, chatAgent *agent.Agent) {
	console.Banner("ForgeCLI Chat")
	console.Info("workspace: " + chatAgent.WorkspaceRoot())
	console.Info("model: " + chatAgent.ModelName())
	console.Info("mode: " + chatAgent.ModeName())
	console.Info("tools: " + strings.Join(chatAgent.ToolNames(), ", "))
	console.Info("commands: /help /tools /mode analyze /mode full /reset /exit")
	console.Println()
}

func maybeHandleFileGeneration(ctx context.Context, console *ui.Console, chatAgent *agent.Agent, genService *generator.Service, input string) (bool, error) {
	output := extractOutputPath(input)
	if output == "" {
		if !looksLikeHTMLRequest(input) {
			return false, nil
		}
		output = "generated.html"
	}
	if chatAgent.ModeName() != "full" {
		console.Warn("File generation writes to disk. Switch to full mode first with /mode full.")
		return true, nil
	}
	outputPath := output
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(chatAgent.WorkspaceRoot(), filepath.FromSlash(output))
	}
	console.Tool("Generating file: " + filepath.Base(outputPath))
	path, err := genService.GenerateFile(ctx, input, outputPath)
	if err != nil {
		return true, err
	}
	console.Success("File generated: " + path)
	console.Assistant("Created file " + path + "\nReview it and run it locally if needed.")
	return true, nil
}

func looksLikeHTMLRequest(input string) bool {
	lower := strings.ToLower(input)
	for _, word := range []string{"html", "web page", "website"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	for _, word := range []string{"网页", "页面", "用html", "生成html", "生成一个html"} {
		if strings.Contains(input, word) {
			return true
		}
	}
	return false
}

func extractOutputPath(input string) string {
	match := outputPathPattern.FindStringSubmatch(input)
	if len(match) < 2 {
		return ""
	}
	return strings.Trim(match[1], "\"' ")
}

func humanizeModelError(err error, modelName string, hasEnv bool, envName string) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") {
		if strings.TrimSpace(envName) == "" {
			return "Model authentication failed (401 Unauthorized). Check your API key in forgecli.json or your provider settings."
		}
		if hasEnv {
			return "Model authentication failed (401 Unauthorized). Check whether your API key is valid for model " + modelName + "."
		}
		return "Model authentication failed (401 Unauthorized). Set the environment variable " + envName + " and try again."
	}
	if strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timeout") {
		return "Model request timed out. You can try again, switch to a faster model, or increase model.timeout_seconds in forgecli.json."
	}
	return err.Error()
}

func newLogger(writer io.Writer, level string) *slog.Logger {
	logLevel := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: logLevel})
	return slog.New(handler)
}

func printHelp(writer io.Writer) {
	fmt.Fprintln(writer, "ForgeCLI MVP Demo")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  forgecli chat [--repo <path>] [--config forgecli.json] [--mode analyze|full] [--prompt \"<task>\"]")
	fmt.Fprintln(writer, "  forgecli run --repo <path> --task \"<task>\" [--verify \"<command>\"] [--config forgecli.json]")
	fmt.Fprintln(writer, "  forgecli generate --prompt \"<idea>\" [--output file.ext] [--config forgecli.json]")
	fmt.Fprintln(writer, "  forgecli version")
}

func shouldUseModel(cfg config.ModelConfig) bool {
	if strings.TrimSpace(cfg.Model) == "" {
		return false
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		return true
	}
	if envName := strings.TrimSpace(cfg.APIKeyEnv); envName != "" {
		return strings.TrimSpace(os.Getenv(envName)) != ""
	}
	return strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.BaseURL) != "https://api.openai.com/v1"
}
