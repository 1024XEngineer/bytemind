package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"bytemind/internal/mcpctl"
)

const (
	mcpAddUsage = "usage: /mcp-add <id> --cmd <command> [options]"
	mcpUsage    = "usage: /mcp <list|show|remove|enable|disable|test|reload|auth|help> ... or " + mcpAddUsage
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
		return mcpUsage, "MCP help shown.", nil
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
	case "reload":
		if reloadErr := service.Reload(context.Background()); reloadErr != nil {
			return "", "", reloadErr
		}
		return "MCP runtime reloaded.", "MCP reloaded.", nil
	case "test":
		if len(fields) < 3 {
			return "", "", fmt.Errorf("usage: /mcp test <id>")
		}
		item, testErr := service.Test(context.Background(), fields[2])
		if testErr != nil {
			return "", "", testErr
		}
		return formatMCPStatusText([]mcpctl.ServerStatus{item}), "MCP test completed.", nil
	case "remove":
		if len(fields) < 3 {
			return "", "", fmt.Errorf("usage: /mcp remove <id>")
		}
		serverID := strings.TrimSpace(fields[2])
		if removeErr := service.Remove(context.Background(), serverID); removeErr != nil {
			return "", "", removeErr
		}
		return fmt.Sprintf("Removed MCP server `%s`.", serverID), "MCP server removed.", nil
	case "enable":
		if len(fields) < 3 {
			return "", "", fmt.Errorf("usage: /mcp enable <id>")
		}
		item, enableErr := service.Enable(context.Background(), fields[2], true)
		if enableErr != nil {
			return "", "", enableErr
		}
		return formatMCPStatusText([]mcpctl.ServerStatus{item}), "MCP server enabled.", nil
	case "disable":
		if len(fields) < 3 {
			return "", "", fmt.Errorf("usage: /mcp disable <id>")
		}
		item, disableErr := service.Enable(context.Background(), fields[2], false)
		if disableErr != nil {
			return "", "", disableErr
		}
		return formatMCPStatusText([]mcpctl.ServerStatus{item}), "MCP server disabled.", nil
	case "add":
		request, parseErr := parseMCPAddFields(fields)
		if parseErr != nil {
			return "", "", parseErr
		}
		item, addErr := service.Add(context.Background(), request)
		if addErr != nil {
			return "", "", addErr
		}
		return formatMCPStatusText([]mcpctl.ServerStatus{item}), "MCP server added.", nil
	case "auth":
		if len(fields) < 3 {
			return "", "", fmt.Errorf("usage: /mcp auth <id>")
		}
		serverID := strings.TrimSpace(fields[2])
		return strings.Join([]string{
			fmt.Sprintf("Auth guide for `%s`:", serverID),
			"- Configure secrets through environment variables and pass them with `--env KEY=VALUE` when adding/updating the server.",
			"- Do not paste plaintext tokens into chat history.",
			"- Run `/mcp test " + serverID + "` after updating credentials.",
		}, "\n"), "MCP auth guidance shown.", nil
	default:
		return "", "", fmt.Errorf(mcpUsage)
	}
}

func parseMCPAddFields(fields []string) (mcpctl.AddRequest, error) {
	if len(fields) < 4 {
		return mcpctl.AddRequest{}, fmt.Errorf(mcpAddUsage)
	}
	request := mcpctl.AddRequest{
		ID: strings.TrimSpace(fields[2]),
	}

	for index := 3; index < len(fields); index++ {
		flagName := strings.ToLower(strings.TrimSpace(fields[index]))
		switch flagName {
		case "--cmd":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf(mcpAddUsage)
			}
			request.Command = strings.TrimSpace(fields[index])
		case "--name":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --name <display_name>")
			}
			request.Name = strings.TrimSpace(fields[index])
		case "--args":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --args a,b,c")
			}
			request.Args = splitCSVFields(fields[index])
		case "--cwd":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --cwd <path>")
			}
			request.CWD = strings.TrimSpace(fields[index])
		case "--env":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --env KEY=VALUE")
			}
			key, value, ok := parseEnvPair(fields[index])
			if !ok {
				return mcpctl.AddRequest{}, fmt.Errorf("invalid env pair %q, expected KEY=VALUE", fields[index])
			}
			if request.Env == nil {
				request.Env = map[string]string{}
			}
			request.Env[key] = value
		case "--auto-start":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --auto-start <true|false>")
			}
			value, err := strconv.ParseBool(strings.TrimSpace(fields[index]))
			if err != nil {
				return mcpctl.AddRequest{}, fmt.Errorf("invalid --auto-start value %q", fields[index])
			}
			request.AutoStart = &value
		case "--startup-timeout-s":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --startup-timeout-s <seconds>")
			}
			value, convErr := strconv.Atoi(strings.TrimSpace(fields[index]))
			if convErr != nil || value < 0 {
				return mcpctl.AddRequest{}, fmt.Errorf("invalid --startup-timeout-s value %q", fields[index])
			}
			request.StartupTimeoutS = value
		case "--call-timeout-s":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --call-timeout-s <seconds>")
			}
			value, convErr := strconv.Atoi(strings.TrimSpace(fields[index]))
			if convErr != nil || value < 0 {
				return mcpctl.AddRequest{}, fmt.Errorf("invalid --call-timeout-s value %q", fields[index])
			}
			request.CallTimeoutS = value
		case "--max-concurrency":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --max-concurrency <n>")
			}
			value, convErr := strconv.Atoi(strings.TrimSpace(fields[index]))
			if convErr != nil || value < 0 {
				return mcpctl.AddRequest{}, fmt.Errorf("invalid --max-concurrency value %q", fields[index])
			}
			request.MaxConcurrency = value
		case "--protocol-version":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --protocol-version <version>")
			}
			request.ProtocolVersion = strings.TrimSpace(fields[index])
		case "--protocol-versions":
			index++
			if index >= len(fields) {
				return mcpctl.AddRequest{}, fmt.Errorf("usage: /mcp-add <id> --protocol-versions a,b,c")
			}
			request.ProtocolVersions = splitCSVFields(fields[index])
		default:
			return mcpctl.AddRequest{}, fmt.Errorf("unsupported /mcp-add flag %q", fields[index])
		}
	}

	if strings.TrimSpace(request.Command) == "" {
		return mcpctl.AddRequest{}, fmt.Errorf(mcpAddUsage)
	}
	return request, nil
}

func splitCSVFields(raw string) []string {
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

func parseEnvPair(raw string) (key string, value string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(raw), "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key = strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false
	}
	return key, parts[1], true
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
