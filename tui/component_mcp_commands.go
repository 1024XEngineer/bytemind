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
	mcpAddUsage = "usage: /mcp-add <id> --cmd <command> [--args a,b] [--env K=V]"
)

func (m *model) runMCPCommand(input string, fields []string) error {
	if m.mcpService == nil {
		return fmt.Errorf("mcp service is unavailable")
	}
	if len(fields) < 2 {
		return fmt.Errorf("usage: /mcp <list|add|remove|enable|disable|test|reload|auth> ...")
	}
	sub := strings.ToLower(strings.TrimSpace(fields[1]))

	switch sub {
	case "list":
		items, err := m.mcpService.List(context.Background())
		if err != nil {
			return err
		}
		m.appendCommandExchange(input, formatMCPStatusText(items))
		m.statusNote = fmt.Sprintf("Listed %d MCP server(s).", len(items))
		return nil
	case "reload":
		if err := m.mcpService.Reload(context.Background()); err != nil {
			return err
		}
		m.appendCommandExchange(input, "MCP runtime reloaded.")
		m.statusNote = "MCP reloaded."
		return nil
	case "test":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /mcp test <id>")
		}
		status, err := m.mcpService.Test(context.Background(), fields[2])
		if err != nil {
			return err
		}
		m.appendCommandExchange(input, formatMCPStatusText([]mcpctl.ServerStatus{status}))
		m.statusNote = "MCP test completed."
		return nil
	case "remove":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /mcp remove <id>")
		}
		if err := m.mcpService.Remove(context.Background(), fields[2]); err != nil {
			return err
		}
		m.appendCommandExchange(input, fmt.Sprintf("Removed MCP server `%s`.", strings.TrimSpace(fields[2])))
		m.statusNote = "MCP server removed."
		return nil
	case "enable":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /mcp enable <id>")
		}
		status, err := m.mcpService.Enable(context.Background(), fields[2], true)
		if err != nil {
			return err
		}
		m.appendCommandExchange(input, formatMCPStatusText([]mcpctl.ServerStatus{status}))
		m.statusNote = "MCP server enabled."
		return nil
	case "disable":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /mcp disable <id>")
		}
		status, err := m.mcpService.Enable(context.Background(), fields[2], false)
		if err != nil {
			return err
		}
		m.appendCommandExchange(input, formatMCPStatusText([]mcpctl.ServerStatus{status}))
		m.statusNote = "MCP server disabled."
		return nil
	case "add":
		request, err := parseMCPAddFields(fields)
		if err != nil {
			return err
		}
		status, err := m.mcpService.Add(context.Background(), request)
		if err != nil {
			return err
		}
		m.appendCommandExchange(input, formatMCPStatusText([]mcpctl.ServerStatus{status}))
		m.statusNote = "MCP server added."
		return nil
	case "auth":
		if len(fields) < 3 {
			return fmt.Errorf("usage: /mcp auth <id>")
		}
		serverID := strings.TrimSpace(fields[2])
		response := strings.Join([]string{
			fmt.Sprintf("Auth guide for `%s`:", serverID),
			"- Configure secrets through environment variables and pass them with `--env KEY=VALUE` when adding/updating the server.",
			"- Do not paste plaintext tokens into chat history.",
			"- Run `/mcp test " + serverID + "` after updating credentials.",
		}, "\n")
		m.appendCommandExchange(input, response)
		m.statusNote = "MCP auth guidance shown."
		return nil
	default:
		return fmt.Errorf("usage: /mcp <list|add|remove|enable|disable|test|reload|auth> ...")
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

func firstNonEmptyStatus(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
