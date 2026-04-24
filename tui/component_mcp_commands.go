package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"bytemind/internal/mcpctl"
)

const (
	mcpUsage = "usage: /mcp <list|show <id>|setup <id>|help>"
)

func (m *model) runMCPCommand(input string, fields []string) error {
	response, status, err := executeMCPServiceCommand(m.mcpService, fields)
	if err != nil {
		return err
	}
	m.appendCommandExchange(input, response)
	m.statusNote = status
	return nil
}

func (m *model) runMCPCommandDispatch(input string, fields []string) error {
	if m == nil {
		return fmt.Errorf("model is unavailable")
	}
	if len(fields) >= 2 && strings.EqualFold(strings.TrimSpace(fields[1]), "setup") {
		return m.runMCPSetupSlashCommand(input, fields)
	}
	if m.async == nil {
		return m.runMCPCommand(input, fields)
	}
	return m.runMCPCommandAsync(input, fields)
}

func (m *model) runMCPCommandAsync(input string, fields []string) error {
	if m == nil {
		return fmt.Errorf("model is unavailable")
	}
	if m.mcpCommandPending {
		return fmt.Errorf("an MCP command is already running")
	}
	service := m.mcpService
	if service == nil {
		return fmt.Errorf("mcp service is unavailable")
	}
	if m.async == nil {
		return m.runMCPCommand(input, fields)
	}
	m.mcpCommandPending = true
	m.statusNote = "MCP command running..."
	asyncCh := m.async
	commandInput := strings.TrimSpace(input)
	commandFields := append([]string(nil), fields...)

	go func() {
		response, status, err := executeMCPServiceCommand(service, commandFields)
		asyncCh <- mcpCommandResultMsg{
			Input:    commandInput,
			Response: response,
			Status:   status,
			Err:      err,
		}
	}()
	return nil
}

func executeMCPServiceCommand(service MCPService, fields []string) (response string, status string, err error) {
	if service == nil {
		return "", "", fmt.Errorf("mcp service is unavailable")
	}
	if len(fields) < 2 {
		return "", "", fmt.Errorf(mcpUsage)
	}
	sub := strings.ToLower(strings.TrimSpace(fields[1]))

	switch sub {
	case "help":
		return formatMCPHelpText(), "MCP help shown.", nil
	case "list":
		items, listErr := service.List(context.Background())
		if listErr != nil {
			return "", "", listErr
		}
		return formatMCPStatusText(items), fmt.Sprintf("Listed %d MCP server(s).", len(items)), nil
	case "show":
		if len(fields) < 3 {
			return "", "", fmt.Errorf("usage: /mcp show <id>")
		}
		detail, showErr := service.Show(context.Background(), fields[2])
		if showErr != nil {
			return "", "", showErr
		}
		return formatMCPDetailText(detail), "MCP server details shown.", nil
	default:
		return "", "", fmt.Errorf(mcpUsage)
	}
}

func formatMCPHelpText() string {
	return strings.Join([]string{
		mcpUsage,
		"- /mcp list",
		"- /mcp show <id>",
		"- /mcp setup <id> [--cmd <command>] [--args a,b] [--env K=V[,K2=V2]]",
		"  runs Add -> Test -> Enable -> Reload in one command.",
		"  `github` id uses built-in preset when --cmd is omitted.",
		"- /mcp help",
	}, "\n")
}

func formatMCPStatusText(items []mcpctl.ServerStatus) string {
	if len(items) == 0 {
		return "No MCP servers configured."
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	lines := []string{"MCP servers:"}
	for _, item := range items {
		lines = append(
			lines,
			fmt.Sprintf(
				"- %s | enabled=%t | status=%s | tools=%d | %s",
				item.ID,
				item.Enabled,
				item.Status,
				item.Tools,
				firstNonEmptyStatus(item.Message, "-"),
			),
		)
	}
	return strings.Join(lines, "\n")
}

func formatMCPDetailText(detail mcpctl.ServerDetail) string {
	status := detail.Status
	lines := []string{
		fmt.Sprintf("id: %s", status.ID),
		fmt.Sprintf("name: %s", firstNonEmptyStatus(status.Name, status.ID)),
		fmt.Sprintf("enabled: %t", status.Enabled),
		fmt.Sprintf("auto_start: %t", status.AutoStart),
		fmt.Sprintf("status: %s", status.Status),
		fmt.Sprintf("tools: %d", status.Tools),
		fmt.Sprintf("message: %s", firstNonEmptyStatus(status.Message, "-")),
		fmt.Sprintf("transport: %s", firstNonEmptyStatus(detail.TransportType, "-")),
		fmt.Sprintf("command: %s", firstNonEmptyStatus(detail.Command, "-")),
		fmt.Sprintf("args: %s", firstNonEmptyStatus(strings.Join(detail.Args, " "), "-")),
		fmt.Sprintf("cwd: %s", firstNonEmptyStatus(detail.CWD, "-")),
		fmt.Sprintf("env_keys: %s", firstNonEmptyStatus(strings.Join(detail.EnvKeys, ","), "-")),
		fmt.Sprintf("startup_timeout_s: %d", detail.StartupTimeoutS),
		fmt.Sprintf("call_timeout_s: %d", detail.CallTimeoutS),
		fmt.Sprintf("max_concurrency: %d", detail.MaxConcurrency),
		fmt.Sprintf("protocol_versions: %s", firstNonEmptyStatus(strings.Join(detail.ProtocolVersions, ","), "-")),
	}
	return strings.Join(lines, "\n")
}

func firstNonEmptyStatus(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
