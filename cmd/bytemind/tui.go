package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bytemind/internal/agent"
	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/provider"
	"bytemind/internal/session"
	"bytemind/internal/tui"
)

var runTUIProgram = tui.Run

func runTUI(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Resume an existing session")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")

	if err := fs.Parse(args); err != nil {
		return err
	}

	workspace, err := resolveWorkspace(*workspaceOverride)
	if err != nil {
		return err
	}

	cfg, err := config.Load(workspace, *configPath)
	if err != nil {
		return err
	}
	if err := applyRuntimeOverrides(&cfg, *model, *streamOverride, *maxIterations); err != nil {
		return err
	}

	interactive := isInteractiveStdin(stdin)
	check := provider.Availability{Ready: true}
	if interactive {
		check = provider.CheckAvailability(context.Background(), cfg.Provider)
	}

	guide := tui.StartupGuide{}
	if !check.Ready {
		guide = buildStartupGuide(cfg, check, workspace, *configPath)
	}

	var app *agent.Runner
	var store *session.Store
	var sess *session.Session
	if guide.Active && interactive {
		app, store, sess, err = bootstrapForTUI(*configPath, *model, *sessionID, *streamOverride, *workspaceOverride, *maxIterations, stdin, stdout)
	} else {
		app, store, sess, err = bootstrap(*configPath, *model, *sessionID, *streamOverride, *workspaceOverride, *maxIterations, stdin, stdout)
	}
	if err != nil {
		return err
	}
	if app == nil || store == nil || sess == nil {
		return errors.New("internal error: bootstrap returned nil runtime")
	}
	defer func() { _ = app.Close() }()
	home, err := config.EnsureHomeLayout()
	if err != nil {
		return err
	}
	imageStore, err := assets.NewFileAssetStore(home)
	if err != nil {
		return err
	}

	return runTUIProgram(tui.Options{
		Runner:       app,
		Store:        store,
		Session:      sess,
		ImageStore:   imageStore,
		Config:       cfg,
		Workspace:    sess.Workspace,
		StartupGuide: guide,
	})
}

func applyRuntimeOverrides(cfg *config.Config, modelOverride, streamOverride string, maxIterations int) error {
	if modelOverride != "" {
		cfg.Provider.Model = modelOverride
	}
	if streamOverride != "" {
		parsed, err := strconv.ParseBool(streamOverride)
		if err != nil {
			return fmt.Errorf("invalid -stream value: %w", err)
		}
		cfg.Stream = parsed
	}
	if maxIterations > 0 {
		cfg.MaxIterations = maxIterations
	}
	return nil
}

func isInteractiveStdin(stdin io.Reader) bool {
	file, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func buildStartupGuide(cfg config.Config, check provider.Availability, workspace, explicitConfigPath string) tui.StartupGuide {
	path := configPathHint(workspace, explicitConfigPath)
	envName := strings.TrimSpace(cfg.Provider.APIKeyEnv)
	if envName == "" {
		envName = "BYTEMIND_API_KEY"
	}
	lines := []string{
		"Paste your API key in the input box below and press Enter.",
		"Bytemind will verify it and save it automatically.",
		"Default OpenAI setup only needs API key.",
		"For other providers, set provider.base_url and provider.model too.",
		"Optional: model=<name>  base_url=<url>  provider=<openai-compatible|anthropic>",
		"You can still use /help and /quit commands.",
		"Env fallback: " + envName,
	}
	if path != "" {
		lines = append(lines, "Config file: "+path)
	}
	lines = append(lines, "Issue: "+startupIssueHint(check))

	return tui.StartupGuide{
		Active:       true,
		Title:        "Let's finish setup",
		Status:       "Bytemind will guide you through provider, base_url, model, and API key.",
		Lines:        lines,
		ConfigPath:   path,
		CurrentField: "type",
	}
}

func startupIssueHint(check provider.Availability) string {
	reason := strings.ToLower(strings.TrimSpace(check.Reason))
	switch {
	case strings.Contains(reason, "missing api key"):
		return "No API key is configured yet."
	case strings.Contains(reason, "unauthorized"):
		return "The API key was rejected by the provider."
	case strings.Contains(reason, "failed to reach"):
		return "Cannot reach provider endpoint. Check proxy or network."
	case strings.Contains(reason, "not found"):
		return "Provider endpoint path looks incorrect."
	default:
		if strings.TrimSpace(check.Reason) == "" {
			return "Provider check failed."
		}
		return compactLine(check.Reason, 120)
	}
}

func configPathHint(workspace, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		if abs, err := filepath.Abs(explicit); err == nil {
			return abs
		}
		return explicit
	}

	candidates := []string{
		filepath.Join(workspace, "config.json"),
		filepath.Join(workspace, ".bytemind", "config.json"),
		filepath.Join(workspace, "bytemind.config.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	home, err := config.ResolveHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "config.json")
}

func compactLine(raw string, limit int) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\n", " "))
	if len(raw) <= limit {
		return raw
	}
	if limit <= 3 {
		return raw[:limit]
	}
	return raw[:limit-3] + "..."
}
