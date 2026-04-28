package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStdioClientDiscoverWithHelperServer(t *testing.T) {
	client := NewStdioClient()
	cfg := helperServerConfig(t, "discover_ok")
	snapshot, err := client.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if snapshot.Name != "helper-mcp" {
		t.Fatalf("unexpected server name: %q", snapshot.Name)
	}
	if snapshot.Version != "1.2.3" {
		t.Fatalf("unexpected server version: %q", snapshot.Version)
	}
	if len(snapshot.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(snapshot.Tools))
	}
	if snapshot.Tools[0].Name != "echo" {
		t.Fatalf("unexpected tool name: %q", snapshot.Tools[0].Name)
	}
}

func TestStdioClientHandshakeFailureWithHelperServer(t *testing.T) {
	client := NewStdioClient()
	cfg := helperServerConfig(t, "handshake_fail")
	_, err := client.Discover(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected handshake error")
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T", err)
	}
	if clientErr.Code != ClientErrorHandshakeFailed {
		t.Fatalf("expected handshake_failed, got %q", clientErr.Code)
	}
}

func TestStdioClientCallToolWithHelperServer(t *testing.T) {
	client := NewStdioClient()
	cfg := helperServerConfig(t, "call_ok")
	output, err := client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if output != "ok-from-helper" {
		t.Fatalf("unexpected call output: %q", output)
	}
}

func TestStdioClientHandlesNotificationBeforeResponse(t *testing.T) {
	client := NewStdioClient()
	cfg := helperServerConfig(t, "notification_before_response")
	output, err := client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("CallTool with notification-before-response failed: %v", err)
	}
	if output != "ok-from-helper" {
		t.Fatalf("unexpected call output: %q", output)
	}
}

func TestStdioClientProtocolFallbackWithHelperServer(t *testing.T) {
	client := NewStdioClient()
	cfg := helperServerConfig(t, "protocol_fallback")
	snapshot, err := client.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover with protocol fallback failed: %v", err)
	}
	if snapshot.Name != "helper-mcp" {
		t.Fatalf("unexpected server name: %q", snapshot.Name)
	}
}

func TestStdioClientDiscoverWithStringResponseID(t *testing.T) {
	client := NewStdioClient()
	cfg := helperServerConfig(t, "response_id_as_string")
	snapshot, err := client.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover with string response id failed: %v", err)
	}
	if len(snapshot.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(snapshot.Tools))
	}
}

func TestStdioClientDiscoverRequiresInitializedNotification(t *testing.T) {
	client := NewStdioClient()
	cfg := helperServerConfig(t, "require_initialized")
	snapshot, err := client.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover with required initialized notification failed: %v", err)
	}
	if len(snapshot.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(snapshot.Tools))
	}
}

func TestStdioClientCallToolRequiresInitializedNotification(t *testing.T) {
	client := NewStdioClient()
	cfg := helperServerConfig(t, "require_initialized")
	output, err := client.CallTool(context.Background(), cfg, "echo", json.RawMessage(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("CallTool with required initialized notification failed: %v", err)
	}
	if output != "ok-from-helper" {
		t.Fatalf("unexpected call output: %q", output)
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("BYTEMIND_MCP_HELPER") != "1" {
		return
	}
	scenario := strings.TrimSpace(os.Getenv("BYTEMIND_MCP_SCENARIO"))
	if scenario == "eof_with_stderr" {
		_, _ = os.Stderr.WriteString("helper exited early")
		os.Exit(0)
	}
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	initialized := false
	for {
		if scenario == "invalid_json_line" {
			_, _ = os.Stdout.WriteString("not-json\n")
			os.Exit(0)
		}
		if scenario == "sleep" {
			time.Sleep(250 * time.Millisecond)
		}
		request, err := readRPCRequest(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			_ = writeRPCResponse(writer, rpcResponse{
				JSONRPC: "2.0",
				ID:      0,
				Error: &rpcError{
					Code:    -32700,
					Message: "parse error",
				},
			})
			continue
		}
		if request.Method == "notifications/initialized" {
			initialized = true
			continue
		}
		requestID, hasRequestID, requestIDErr := normalizeRPCResponseID(request.ID)
		if requestIDErr != nil {
			_ = writeRPCResponse(writer, rpcResponse{
				JSONRPC: "2.0",
				Error: &rpcError{
					Code:    -32600,
					Message: "invalid request id",
				},
			})
			continue
		}
		if !hasRequestID {
			_ = writeRPCResponse(writer, rpcResponse{
				JSONRPC: "2.0",
				Error: &rpcError{
					Code:    -32600,
					Message: "request id is required for this method",
				},
			})
			continue
		}
		responseID := any(request.ID)
		if scenario == "bad_response_id" {
			if value, err := strconv.Atoi(requestID); err == nil {
				responseID = value + 1
			} else {
				responseID = requestID + "-bad"
			}
		}
		if scenario == "response_id_as_string" {
			responseID = requestID
		}
		if scenario == "non_integer_response_id" {
			responseID = 1.5
		}
		response := rpcResponse{
			JSONRPC: "2.0",
			ID:      responseID,
		}
		if scenario == "notification_before_response" {
			_ = writeRPCResponse(writer, rpcResponse{
				JSONRPC: "2.0",
				Method:  "notifications/progress",
			})
		}
		switch request.Method {
		case "initialize":
			if scenario == "handshake_fail" {
				response.Error = &rpcError{
					Code:    -32000,
					Message: "handshake failed",
				}
			} else if scenario == "protocol_fallback" {
				version := protocolVersionFromRequest(request)
				if version == "2026-04-01" {
					response.Error = &rpcError{
						Code:    -32000,
						Message: "unsupported protocol version",
					}
				} else {
					response.Result = json.RawMessage(`{"serverInfo":{"name":"helper-mcp","version":"1.2.3"}}`)
				}
			} else if scenario == "discover_invalid_initialize_result" {
				response.Result = json.RawMessage(`"oops"`)
			} else if scenario == "discover_empty_server_info" {
				response.Result = json.RawMessage(`{"serverInfo":{"name":"","version":""}}`)
			} else {
				response.Result = json.RawMessage(`{"serverInfo":{"name":"helper-mcp","version":"1.2.3"}}`)
			}
		case "tools/list":
			if scenario == "require_initialized" && !initialized {
				response.Error = &rpcError{
					Code:    -32000,
					Message: "server not initialized",
				}
			} else if scenario == "discover_invalid_tools_result" {
				response.Result = json.RawMessage(`"oops"`)
			} else {
				response.Result = json.RawMessage(`{"tools":[{"name":"echo","description":"echo text","inputSchema":{"type":"object","properties":{"message":{"type":"string"}}}}]}`)
			}
		case "tools/call":
			if scenario == "require_initialized" && !initialized {
				response.Error = &rpcError{
					Code:    -32000,
					Message: "server not initialized",
				}
			} else if scenario == "call_fail" {
				response.Error = &rpcError{
					Code:    -32000,
					Message: "call failed",
				}
			} else if scenario == "call_is_error" {
				response.Result = json.RawMessage(`{"isError":true,"content":[{"type":"text","text":"tool failed"}]}`)
			} else if scenario == "call_compact_fallback" {
				response.Result = json.RawMessage(`{"foo":"bar"}`)
			} else {
				response.Result = json.RawMessage(`{"content":[{"type":"text","text":"ok-from-helper"}]}`)
			}
		default:
			response.Error = &rpcError{
				Code:    -32601,
				Message: "method not found",
			}
		}
		_ = writeRPCResponse(writer, response)
	}
	os.Exit(0)
}

func protocolVersionFromRequest(request rpcRequest) string {
	params, ok := request.Params.(map[string]any)
	if !ok || params == nil {
		return ""
	}
	protocolVersion, _ := params["protocolVersion"].(string)
	return strings.TrimSpace(protocolVersion)
}

func helperServerConfig(t *testing.T, scenario string) ServerConfig {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to resolve test executable: %v", err)
	}
	return ServerConfig{
		ID:      "helper",
		Name:    "helper",
		Command: exe,
		Args:    []string{"-test.run=^TestMCPHelperProcess$"},
		Env: map[string]string{
			"BYTEMIND_MCP_HELPER":   "1",
			"BYTEMIND_MCP_SCENARIO": scenario,
		},
		StartupTimeout: 3 * time.Second,
		CallTimeout:    3 * time.Second,
	}
}
