package agent

import "strings"

func toolAuditMetadata(toolName string, extra map[string]string) map[string]string {
	toolName = strings.TrimSpace(toolName)
	metadata := map[string]string{
		"tool_name": toolName,
	}
	if serverID, mcpToolName, ok := parseMCPStableToolKey(toolName); ok {
		metadata["stable_tool_key"] = toolName
		metadata["extension_source"] = "mcp"
		metadata["mcp_server_id"] = serverID
		metadata["mcp_tool_name"] = mcpToolName
		metadata["extension_id"] = "mcp." + serverID
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		metadata[key] = value
	}
	return metadata
}

func parseMCPStableToolKey(toolName string) (serverID string, mcpToolName string, ok bool) {
	parts := strings.Split(strings.TrimSpace(toolName), ":")
	if len(parts) != 3 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) != "mcp" {
		return "", "", false
	}
	serverID = strings.TrimSpace(parts[1])
	mcpToolName = strings.TrimSpace(parts[2])
	if serverID == "" || mcpToolName == "" {
		return "", "", false
	}
	return serverID, mcpToolName, true
}
