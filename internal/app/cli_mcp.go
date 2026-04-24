package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"bytemind/internal/mcpctl"
)

func RunMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	_ = stdin
	if len(args) == 0 {
		renderMCPUsage(stdout)
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "help", "-h", "--help":
		renderMCPUsage(stdout)
		return nil
	case "list":
		return runMCPList(args[1:], stdout, stderr)
	case "show":
		return runMCPShow(args[1:], stdout, stderr)
	case "add":
		return runMCPAdd(args[1:], stdout, stderr)
	case "remove":
		return runMCPRemove(args[1:], stdout, stderr)
	case "enable":
		return runMCPEnable(args[1:], stdout, stderr, true)
	case "disable":
		return runMCPEnable(args[1:], stdout, stderr, false)
	case "test":
		return runMCPTest(args[1:], stdout, stderr)
	case "reload":
		return runMCPReload(args[1:], stdout, stderr)
	case "auth":
		return runMCPAuth(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown mcp subcommand %q", args[0])
	}
}

func runMCPList(args []string, stdout, stderr io.Writer) error {
	workspace, configPath, err := parseMCPCommonFlags("mcp list", args, stderr)
	if err != nil {
		return err
	}
	service := mcpctl.NewService(workspace, configPath, nil)
	items, err := service.List(context.Background())
	if err != nil {
		return err
	}
	renderMCPStatuses(stdout, items)
	return nil
}

func runMCPShow(args []string, stdout, stderr io.Writer) error {
	workspace, configPath, serverID, err := parseMCPActionTarget("mcp show", args, stderr)
	if err != nil {
		return err
	}
	service := mcpctl.NewService(workspace, configPath, nil)
	detail, err := service.Show(context.Background(), serverID)
	if err != nil {
		return err
	}
	renderMCPDetail(stdout, detail)
	return nil
}

func runMCPAdd(args []string, stdout, stderr io.Writer) error {
	serverID := ""
	if len(args) > 0 && !strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		serverID = strings.TrimSpace(args[0])
		args = args[1:]
	}
	fs := flag.NewFlagSet("mcp add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceFlag := fs.String("workspace", "", "Workspace directory; defaults to current directory")
	configPath := fs.String("config", "", "Path to config file")
	name := fs.String("name", "", "Display name for this MCP server")
	command := fs.String("cmd", "", "stdio command to launch MCP server")
	argsCSV := fs.String("args", "", "Comma-separated command arguments")
	cwd := fs.String("cwd", "", "Working directory for MCP server")
	envItems := mcpEnvFlag{}
	fs.Var(&envItems, "env", "Environment variable KEY=VALUE (repeatable)")
	autoStart := fs.Bool("auto-start", true, "Automatically start and discover this server")
	startupTimeout := fs.Int("startup-timeout-s", 0, "Startup timeout in seconds")
	callTimeout := fs.Int("call-timeout-s", 0, "Tool call timeout in seconds")
	maxConcurrency := fs.Int("max-concurrency", 0, "Maximum concurrent tool calls")
	protocolVersion := fs.String("protocol-version", "", "Preferred MCP protocol version")
	protocolVersionsCSV := fs.String("protocol-versions", "", "Comma-separated MCP protocol fallback versions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if serverID == "" && fs.NArg() < 1 {
		return fmt.Errorf("usage: bytemind mcp add <id> --cmd <command> [--args a,b]")
	}
	workspace, err := ResolveWorkspace(*workspaceFlag)
	if err != nil {
		return err
	}
	if serverID == "" {
		serverID = strings.TrimSpace(fs.Arg(0))
	}
	if serverID == "" {
		return fmt.Errorf("server id is required")
	}
	service := mcpctl.NewService(workspace, *configPath, nil)
	status, err := service.Add(context.Background(), mcpctl.AddRequest{
		ID:               serverID,
		Name:             strings.TrimSpace(*name),
		Command:          strings.TrimSpace(*command),
		Args:             splitCSV(*argsCSV),
		Env:              envItems.Clone(),
		CWD:              strings.TrimSpace(*cwd),
		AutoStart:        autoStart,
		StartupTimeoutS:  *startupTimeout,
		CallTimeoutS:     *callTimeout,
		MaxConcurrency:   *maxConcurrency,
		ProtocolVersion:  strings.TrimSpace(*protocolVersion),
		ProtocolVersions: splitCSV(*protocolVersionsCSV),
	})
	if err != nil {
		return err
	}
	renderMCPStatuses(stdout, []mcpctl.ServerStatus{status})
	return nil
}

func runMCPRemove(args []string, stdout, stderr io.Writer) error {
	workspace, configPath, serverID, err := parseMCPActionTarget("mcp remove", args, stderr)
	if err != nil {
		return err
	}
	service := mcpctl.NewService(workspace, configPath, nil)
	if err := service.Remove(context.Background(), serverID); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "removed mcp server %s\n", serverID)
	return nil
}

func runMCPEnable(args []string, stdout, stderr io.Writer, enabled bool) error {
	commandName := "mcp enable"
	if !enabled {
		commandName = "mcp disable"
	}
	workspace, configPath, serverID, err := parseMCPActionTarget(commandName, args, stderr)
	if err != nil {
		return err
	}
	service := mcpctl.NewService(workspace, configPath, nil)
	status, err := service.Enable(context.Background(), serverID, enabled)
	if err != nil {
		return err
	}
	renderMCPStatuses(stdout, []mcpctl.ServerStatus{status})
	return nil
}

func runMCPTest(args []string, stdout, stderr io.Writer) error {
	workspace, configPath, serverID, err := parseMCPActionTarget("mcp test", args, stderr)
	if err != nil {
		return err
	}
	service := mcpctl.NewService(workspace, configPath, nil)
	status, err := service.Test(context.Background(), serverID)
	if err != nil {
		return err
	}
	renderMCPStatuses(stdout, []mcpctl.ServerStatus{status})
	return nil
}

func runMCPReload(args []string, stdout, stderr io.Writer) error {
	workspace, configPath, err := parseMCPCommonFlags("mcp reload", args, stderr)
	if err != nil {
		return err
	}
	service := mcpctl.NewService(workspace, configPath, nil)
	if err := service.Reload(context.Background()); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "mcp reload completed")
	return nil
}

func runMCPAuth(args []string, stdout, stderr io.Writer) error {
	_, _, serverID, err := parseMCPActionTarget("mcp auth", args, stderr)
	if err != nil {
		return err
	}
	lines := []string{
		fmt.Sprintf("Auth guide for `%s`:", serverID),
		"- Configure secrets through environment variables and pass them with --env KEY=VALUE when adding/updating the server.",
		"- Do not paste plaintext tokens into chat history or shell history.",
		"- Run `bytemind mcp test " + serverID + "` after updating credentials.",
	}
	for _, line := range lines {
		fmt.Fprintln(stdout, line)
	}
	return nil
}

func parseMCPActionTarget(name string, args []string, stderr io.Writer) (workspace string, configPath string, target string, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceFlag := fs.String("workspace", "", "Workspace directory; defaults to current directory")
	configFlag := fs.String("config", "", "Path to config file")
	target = ""
	if len(args) > 0 && !strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		target = strings.TrimSpace(args[0])
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return "", "", "", err
	}
	if target == "" && fs.NArg() > 0 {
		target = strings.TrimSpace(fs.Arg(0))
	}
	if target == "" {
		return "", "", "", fmt.Errorf("usage: bytemind %s <server-id>", name)
	}
	workspace, err = ResolveWorkspace(*workspaceFlag)
	if err != nil {
		return "", "", "", err
	}
	return workspace, strings.TrimSpace(*configFlag), target, nil
}

func parseMCPCommonFlags(name string, args []string, stderr io.Writer) (workspace string, configPath string, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspaceFlag := fs.String("workspace", "", "Workspace directory; defaults to current directory")
	configFlag := fs.String("config", "", "Path to config file")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	workspace, err = ResolveWorkspace(*workspaceFlag)
	if err != nil {
		return "", "", err
	}
	return workspace, strings.TrimSpace(*configFlag), nil
}

func renderMCPUsage(w io.Writer) {
	lines := []string{
		"bytemind mcp list [--workspace path] [--config path]",
		"bytemind mcp show <id> [--workspace path] [--config path]",
		"bytemind mcp add <id> --cmd <command> [--args a,b] [--env K=V] [--cwd path]",
		"bytemind mcp remove <id> [--workspace path] [--config path]",
		"bytemind mcp enable <id> [--workspace path] [--config path]",
		"bytemind mcp disable <id> [--workspace path] [--config path]",
		"bytemind mcp test <id> [--workspace path] [--config path]",
		"bytemind mcp reload [--workspace path] [--config path]",
		"bytemind mcp auth <id> [--workspace path] [--config path]",
	}
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
}

func renderMCPDetail(w io.Writer, detail mcpctl.ServerDetail) {
	status := detail.Status
	fmt.Fprintf(w, "id: %s\n", status.ID)
	fmt.Fprintf(w, "name: %s\n", firstNonEmptyValue(strings.TrimSpace(status.Name), status.ID))
	fmt.Fprintf(w, "extension_id: %s\n", firstNonEmptyValue(strings.TrimSpace(status.ExtensionID), "-"))
	fmt.Fprintf(w, "enabled: %t\n", status.Enabled)
	fmt.Fprintf(w, "auto_start: %t\n", status.AutoStart)
	fmt.Fprintf(w, "status: %s\n", status.Status)
	fmt.Fprintf(w, "tools: %d\n", status.Tools)
	fmt.Fprintf(w, "message: %s\n", firstNonEmptyValue(strings.TrimSpace(status.Message), "-"))
	fmt.Fprintf(w, "checked_at: %s\n", firstNonEmptyValue(strings.TrimSpace(status.CheckedAt), "-"))
	fmt.Fprintf(w, "last_error: %s\n", firstNonEmptyValue(strings.TrimSpace(string(status.LastError)), "-"))
	fmt.Fprintf(w, "transport: %s\n", firstNonEmptyValue(strings.TrimSpace(detail.TransportType), "-"))
	fmt.Fprintf(w, "command: %s\n", firstNonEmptyValue(strings.TrimSpace(detail.Command), "-"))
	fmt.Fprintf(w, "args: %s\n", firstNonEmptyValue(strings.Join(detail.Args, " "), "-"))
	fmt.Fprintf(w, "cwd: %s\n", firstNonEmptyValue(strings.TrimSpace(detail.CWD), "-"))
	fmt.Fprintf(w, "env_keys: %s\n", firstNonEmptyValue(strings.Join(detail.EnvKeys, ","), "-"))
	fmt.Fprintf(w, "startup_timeout_s: %d\n", detail.StartupTimeoutS)
	fmt.Fprintf(w, "call_timeout_s: %d\n", detail.CallTimeoutS)
	fmt.Fprintf(w, "max_concurrency: %d\n", detail.MaxConcurrency)
	fmt.Fprintf(w, "protocol_versions: %s\n", firstNonEmptyValue(strings.Join(detail.ProtocolVersions, ","), "-"))
}

func renderMCPStatuses(w io.Writer, statuses []mcpctl.ServerStatus) {
	if len(statuses) == 0 {
		fmt.Fprintln(w, "no mcp servers configured")
		return
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].ID < statuses[j].ID })
	fmt.Fprintf(w, "%-18s %-7s %-9s %-5s %s\n", "ID", "ENABLED", "STATUS", "TOOLS", "MESSAGE")
	for _, item := range statuses {
		fmt.Fprintf(
			w,
			"%-18s %-7t %-9s %-5d %s\n",
			item.ID,
			item.Enabled,
			item.Status,
			item.Tools,
			firstNonEmptyValue(strings.TrimSpace(item.Message), "-"),
		)
	}
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		items = append(items, part)
	}
	return items
}

type mcpEnvFlag map[string]string

func (f *mcpEnvFlag) String() string {
	if f == nil || len(*f) == 0 {
		return ""
	}
	items := make([]string, 0, len(*f))
	for key, value := range *f {
		items = append(items, key+"="+value)
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func (f *mcpEnvFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("env must be KEY=VALUE")
	}
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("env must be KEY=VALUE")
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return fmt.Errorf("env key is required")
	}
	if *f == nil {
		*f = map[string]string{}
	}
	(*f)[key] = parts[1]
	return nil
}

func (f *mcpEnvFlag) Clone() map[string]string {
	if f == nil || len(*f) == 0 {
		return nil
	}
	out := make(map[string]string, len(*f))
	for key, value := range *f {
		out[key] = value
	}
	return out
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
