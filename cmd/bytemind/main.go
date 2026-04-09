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
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/provider"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiBlue  = "\x1b[94m"
	ansiGray  = "\x1b[90m"

	defaultBroadWorkspaceEntryThreshold = 300
)

var workspaceProjectMarkers = []string{
	".git",
	"go.mod",
	"package.json",
	"pnpm-workspace.yaml",
	"pyproject.toml",
	"Cargo.toml",
	"pom.xml",
	"build.gradle",
	"Makefile",
}

type slashCommand struct {
	Name        string
	Usage       string
	Description string
}

var slashCommands = []slashCommand{
	{Name: "/help", Usage: "/help", Description: "Show available commands"},
	{Name: "/session", Usage: "/session", Description: "Show the current session"},
	{Name: "/sessions", Usage: "/sessions [limit]", Description: "List recent sessions"},
	{Name: "/resume", Usage: "/resume <id>", Description: "Resume a recent session by id or prefix"},
	{Name: "/new", Usage: "/new", Description: "Start a new session in the current workspace"},
	{Name: "/quit", Usage: "/quit", Description: "Exit the CLI"},
}

func main() {
	configureConsoleEncoding()
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runChat(nil, stdin, stdout, stderr)
	}

	switch args[0] {
	case "chat":
		return runTUI(args[1:], stdin, stdout, stderr)
	case "tui":
		return runTUI(args[1:], stdin, stdout, stderr)
	case "run":
		return runOneShot(args[1:], stdin, stdout, stderr)
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return runTUI(args, stdin, stdout, stderr)
	}
}

func runChat(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return runTUI(args, stdin, stdout, stderr)
}

func runOneShot(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "Path to config file")
	model := fs.String("model", "", "Override model name")
	sessionID := fs.String("session", "", "Reuse an existing session")
	prompt := fs.String("prompt", "", "Prompt to send")
	streamOverride := fs.String("stream", "", "Override streaming: true or false")
	workspaceOverride := fs.String("workspace", "", "Workspace to operate on; defaults to current directory")
	maxIterations := fs.Int("max-iterations", 0, "Override execution budget for this run")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*prompt) == "" {
		*prompt = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(*prompt) == "" {
		return errors.New("run requires -prompt or trailing prompt text")
	}

	app, _, sess, err := bootstrap(*configPath, *model, *sessionID, *streamOverride, *workspaceOverride, *maxIterations, stdin, stdout)
	if err != nil {
		return err
	}

	_, err = app.RunPrompt(context.Background(), sess, *prompt, "build", stdout)
	return err
}

func promptPrefix() string {
	return "\n" + ansiBlue + "bytemind>" + ansiReset + " "
}

func bootstrap(configPath, modelOverride, sessionID, streamOverride, workspaceOverride string, maxIterationsOverride int, stdin io.Reader, stdout io.Writer) (*agent.Runner, *session.Store, *session.Session, error) {
	return bootstrapWithOptions(configPath, modelOverride, sessionID, streamOverride, workspaceOverride, maxIterationsOverride, stdin, stdout, true)
}

func bootstrapForTUI(configPath, modelOverride, sessionID, streamOverride, workspaceOverride string, maxIterationsOverride int, stdin io.Reader, stdout io.Writer) (*agent.Runner, *session.Store, *session.Session, error) {
	return bootstrapWithOptions(configPath, modelOverride, sessionID, streamOverride, workspaceOverride, maxIterationsOverride, stdin, stdout, false)
}

func bootstrapWithOptions(configPath, modelOverride, sessionID, streamOverride, workspaceOverride string, maxIterationsOverride int, stdin io.Reader, stdout io.Writer, requireAPIKey bool) (*agent.Runner, *session.Store, *session.Session, error) {
	workspace, err := resolveWorkspace(workspaceOverride)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg, err := config.Load(workspace, configPath)
	if err != nil {
		return nil, nil, nil, err
	}
	home, err := config.EnsureHomeLayout()
	if err != nil {
		return nil, nil, nil, err
	}
	sessionDir := filepath.Join(home, "sessions")
	if modelOverride != "" {
		cfg.Provider.Model = modelOverride
	}
	if streamOverride != "" {
		parsed, err := strconv.ParseBool(streamOverride)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid -stream value: %w", err)
		}
		cfg.Stream = parsed
	}
	if maxIterationsOverride < 0 {
		return nil, nil, nil, errors.New("-max-iterations must be greater than 0")
	}
	if maxIterationsOverride > 0 {
		cfg.MaxIterations = maxIterationsOverride
	}

	apiKey := cfg.Provider.ResolveAPIKey()
	if requireAPIKey && apiKey == "" {
		return nil, nil, nil, errors.New("missing API key; configure provider.api_key in the config file")
	}

	store, err := session.NewStore(sessionDir)
	if err != nil {
		return nil, nil, nil, err
	}

	var sess *session.Session
	if sessionID != "" {
		sess, err = store.Load(sessionID)
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		sess = session.New(workspace)
		if err := store.Save(sess); err != nil {
			return nil, nil, nil, err
		}
	}

	cfg.Provider.APIKey = apiKey
	client, err := provider.NewClient(cfg.Provider)
	if err != nil {
		return nil, nil, nil, err
	}

	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config:    cfg,
		Client:    client,
		Store:     store,
		Registry:  tools.DefaultRegistry(),
		Stdin:     stdin,
		Stdout:    stdout,
	})

	return runner, store, sess, nil
}

func handleSlashCommand(stdout io.Writer, store *session.Store, current *session.Session, input string) (*session.Session, bool, bool, error) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return current, false, false, nil
	}

	switch fields[0] {
	case "/quit":
		return current, true, true, nil
	case "/help":
		printHelp(stdout)
		return current, false, true, nil
	case "/session":
		printCurrentSession(stdout, current)
		return current, false, true, nil
	case "/sessions":
		limit, err := parseOptionalLimit(fields)
		if err != nil {
			return current, false, true, err
		}
		if err := printSessions(stdout, store, current.ID, limit); err != nil {
			return current, false, true, err
		}
		return current, false, true, nil
	case "/resume":
		if len(fields) < 2 {
			fmt.Fprintln(stdout, "usage: /resume <id>")
			return current, false, true, nil
		}
		id, err := resolveSessionID(store, fields[1])
		if err != nil {
			return current, false, true, err
		}
		next, err := store.Load(id)
		if err != nil {
			return current, false, true, err
		}
		if !sameWorkspace(current.Workspace, next.Workspace) {
			return current, false, true, fmt.Errorf("session %s belongs to workspace %s, current workspace is %s", next.ID, next.Workspace, current.Workspace)
		}
		fmt.Fprintf(stdout, "%sresumed%s %s\n", ansiDim, ansiReset, next.ID)
		printCurrentSession(stdout, next)
		return next, false, true, nil
	case "/new":
		next := session.New(current.Workspace)
		if err := store.Save(next); err != nil {
			return current, false, true, err
		}
		fmt.Fprintf(stdout, "%snew session%s %s\n", ansiDim, ansiReset, next.ID)
		printCurrentSession(stdout, next)
		return next, false, true, nil
	default:
		fmt.Fprintf(stdout, "unknown command: %s\n", fields[0])
		printCommandSuggestions(stdout, fields[0], commandNames())
		return current, false, true, nil
	}
}

func completeSlashCommand(input string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return input, nil
	}

	matches := matchSlashCommands(fields[0])
	if len(matches) == 0 {
		return input, nil
	}
	if len(matches) > 1 && !containsCommand(matches, fields[0]) {
		return input, matches
	}
	if containsCommand(matches, fields[0]) {
		return input, nil
	}

	completed := matches[0]
	if len(fields) == 1 {
		return completed, nil
	}
	return completed + " " + strings.Join(fields[1:], " "), nil
}

func matchSlashCommands(prefix string) []string {
	matches := make([]string, 0, len(slashCommands))
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd.Name)
		}
	}
	return matches
}

func containsCommand(commands []string, target string) bool {
	for _, cmd := range commands {
		if cmd == target {
			return true
		}
	}
	return false
}

func printHelp(w io.Writer) {
	for _, cmd := range slashCommands {
		fmt.Fprintf(w, "%-42s %s\n", cmd.Usage, cmd.Description)
	}
}

func printCurrentSession(w io.Writer, sess *session.Session) {
	fmt.Fprintf(w, "%ssession%s %s\n", ansiDim, ansiReset, sess.ID)
	fmt.Fprintf(w, "%sworkspace%s %s\n", ansiDim, ansiReset, sess.Workspace)
	fmt.Fprintf(w, "%supdated%s %s\n", ansiDim, ansiReset, sess.UpdatedAt.Local().Format(time.DateTime))
}

func printSessions(w io.Writer, store *session.Store, currentID string, limit int) error {
	summaries, warnings, err := store.List(limit)
	if err != nil {
		return err
	}
	if len(summaries) == 0 {
		fmt.Fprintln(w, "No saved sessions.")
	} else {
		fmt.Fprintf(w, "%srecent sessions%s\n", ansiBold, ansiReset)
		for _, item := range summaries {
			marker := " "
			if item.ID == currentID {
				marker = "*"
			}
			preview := item.LastUserMessage
			if preview == "" {
				preview = "(no user prompt yet)"
			}
			fmt.Fprintf(w, "%s %s  %s  %2d msgs  %s\n", marker, item.ID, item.UpdatedAt.Local().Format("2006-01-02 15:04"), item.MessageCount, preview)
			fmt.Fprintf(w, "%s    %s%s\n", ansiGray, item.Workspace, ansiReset)
		}
	}

	if len(warnings) > 0 {
		if len(summaries) > 0 {
			fmt.Fprintln(w)
		}
		for _, warning := range warnings {
			fmt.Fprintf(w, "%swarning%s %s\n", ansiDim, ansiReset, warning)
		}
	}
	return nil
}

func resolveSessionID(store *session.Store, prefix string) (string, error) {
	summaries, _, err := store.List(0)
	if err != nil {
		return "", err
	}

	matches := make([]string, 0, 4)
	for _, item := range summaries {
		if item.ID == prefix {
			return item.ID, nil
		}
		if strings.HasPrefix(item.ID, prefix) {
			matches = append(matches, item.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("session not found: %s", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("session prefix %q matched multiple sessions", prefix)
	}
}

func parseOptionalLimit(fields []string) (int, error) {
	if len(fields) < 2 {
		return 8, nil
	}
	limit, err := strconv.Atoi(fields[1])
	if err != nil || limit <= 0 {
		return 0, errors.New("/sessions limit must be a positive integer")
	}
	return limit, nil
}

func sameWorkspace(a, b string) bool {
	left, err := filepath.Abs(a)
	if err != nil {
		left = a
	}
	right, err := filepath.Abs(b)
	if err != nil {
		right = b
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "bytemind chat [-config path] [-model name] [-session id] [-stream true|false] [-workspace path] [-max-iterations n]")
	fmt.Fprintln(w, "bytemind tui [-config path] [-model name] [-session id] [-stream true|false] [-workspace path] [-max-iterations n]")
	fmt.Fprintln(w, "bytemind run -prompt \"task\" [-config path] [-model name] [-session id] [-stream true|false] [-max-iterations n]")
	fmt.Fprintln(w, "bytemind install [-to dir] [-name binary-name]")
	fmt.Fprintln(w, "tip: first time use `go run ./cmd/bytemind install`, then add install dir to PATH")
}

func printCommandSuggestions(w io.Writer, input string, suggestions []string) {
	fmt.Fprintf(w, "%smatches%s for %s:\n", ansiDim, ansiReset, input)
	for _, suggestion := range suggestions {
		fmt.Fprintf(w, "  %s\n", suggestion)
	}
}

func commandNames() []string {
	items := make([]string, 0, len(slashCommands))
	for _, cmd := range slashCommands {
		items = append(items, cmd.Name)
	}
	return items
}

func resolveWorkspace(workspaceOverride string) (string, error) {
	workspaceOverride = strings.TrimSpace(workspaceOverride)
	if workspaceOverride != "" {
		workspace, err := filepath.Abs(workspaceOverride)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(workspace)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("workspace must be a directory: %s", workspace)
		}
		return workspace, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return "", err
	}

	if projectRoot := detectProjectRoot(cwd); projectRoot != "" {
		return projectRoot, nil
	}
	if isBroadWorkspacePath(cwd) {
		return "", fmt.Errorf("current directory %s is too broad for default workspace; rerun with -workspace <project-dir> (or set BYTEMIND_ALLOW_BROAD_WORKSPACE=true)", cwd)
	}
	return cwd, nil
}

func detectProjectRoot(start string) string {
	current := strings.TrimSpace(start)
	if current == "" {
		return ""
	}
	current = filepath.Clean(current)
	for {
		if hasProjectMarker(current) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func hasProjectMarker(dir string) bool {
	for _, marker := range workspaceProjectMarkers {
		if marker == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func isBroadWorkspacePath(dir string) bool {
	if value := strings.TrimSpace(os.Getenv("BYTEMIND_ALLOW_BROAD_WORKSPACE")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil && parsed {
			return false
		}
	}
	home, _ := os.UserHomeDir()
	return isBroadWorkspacePathWithHome(dir, home)
}

func isBroadWorkspacePathWithHome(dir, home string) bool {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" {
		return false
	}

	if isFilesystemRoot(dir) {
		return true
	}

	home = strings.TrimSpace(home)
	if home != "" {
		home = filepath.Clean(home)
		if sameWorkspace(dir, home) {
			return true
		}
		for _, name := range []string{"Desktop", "Documents", "Downloads"} {
			if sameWorkspace(dir, filepath.Join(home, name)) {
				return true
			}
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) >= defaultBroadWorkspaceEntryThreshold
}

func isFilesystemRoot(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	parent := filepath.Dir(path)
	return parent == path
}
