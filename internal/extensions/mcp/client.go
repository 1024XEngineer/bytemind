package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	defaultStartupTimeout = 5 * time.Second
	defaultCallTimeout    = 30 * time.Second
)

var defaultProtocolVersions = []string{
	"2026-04-01",
	"2025-03-26",
	"2024-11-05",
}

var defaultEnvWhitelist = []string{
	"PATH",
	"Path",
	"SYSTEMROOT",
	"SystemRoot",
	"WINDIR",
	"COMSPEC",
	"ComSpec",
	"PATHEXT",
	"TEMP",
	"TMP",
	"HOME",
	"USERPROFILE",
}

type ServerConfig struct {
	ID               string
	Name             string
	Version          string
	ProtocolVersion  string
	ProtocolVersions []string
	Command          string
	Args             []string
	Env              map[string]string
	CWD              string
	StartupTimeout   time.Duration
	CallTimeout      time.Duration
}

type ToolDescriptor struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type ServerSnapshot struct {
	ID      string
	Name    string
	Version string
	Tools   []ToolDescriptor
}

type Client interface {
	Discover(ctx context.Context, cfg ServerConfig) (ServerSnapshot, error)
	CallTool(ctx context.Context, cfg ServerConfig, toolName string, raw json.RawMessage) (string, error)
}

type ClientErrorCode string

const (
	ClientErrorInvalidConfig   ClientErrorCode = "invalid_config"
	ClientErrorTransport       ClientErrorCode = "transport_error"
	ClientErrorProtocol        ClientErrorCode = "protocol_error"
	ClientErrorTimeout         ClientErrorCode = "timeout"
	ClientErrorHandshakeFailed ClientErrorCode = "handshake_failed"
	ClientErrorListToolsFailed ClientErrorCode = "tools_list_failed"
	ClientErrorCallFailed      ClientErrorCode = "call_failed"
	ClientErrorPermission      ClientErrorCode = "permission_denied"
	ClientErrorInvalidArgs     ClientErrorCode = "invalid_args"
)

type ClientError struct {
	Code    ClientErrorCode
	Message string
	Err     error
}

func (e *ClientError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *ClientError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newClientError(code ClientErrorCode, message string, err error) error {
	return &ClientError{
		Code:    code,
		Message: strings.TrimSpace(message),
		Err:     err,
	}
}

type StdioClient struct{}

func NewStdioClient() *StdioClient {
	return &StdioClient{}
}

func (c *StdioClient) Discover(ctx context.Context, cfg ServerConfig) (ServerSnapshot, error) {
	cfg = normalizeServerConfig(cfg)
	if err := validateServerConfig(cfg, true); err != nil {
		return ServerSnapshot{}, err
	}

	callCtx, cancel := withTimeoutIfMissing(ctx, cfg.StartupTimeout)
	defer cancel()

	responses, err := c.runWithProtocolFallback(callCtx, cfg, func(protocolVersion string) []rpcRequest {
		return []rpcRequest{
			newRPCRequest(1, "initialize", initializeParams(protocolVersion)),
			newRPCNotification("notifications/initialized", map[string]any{}),
			newRPCRequest(2, "tools/list", map[string]any{}),
		}
	})
	if err != nil {
		return ServerSnapshot{}, err
	}
	if len(responses) != 2 {
		return ServerSnapshot{}, newClientError(ClientErrorProtocol, "mcp server returned incomplete discovery responses", nil)
	}

	initResult := struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}{}
	if err := json.Unmarshal(responses[0].Result, &initResult); err != nil {
		return ServerSnapshot{}, newClientError(ClientErrorProtocol, "failed to decode initialize result", err)
	}

	toolsResult := struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
			Parameters  map[string]any `json:"parameters"`
		} `json:"tools"`
	}{}
	if err := json.Unmarshal(responses[1].Result, &toolsResult); err != nil {
		return ServerSnapshot{}, newClientError(ClientErrorProtocol, "failed to decode tools/list result", err)
	}

	descriptors := make([]ToolDescriptor, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		schema := cloneMap(tool.InputSchema)
		if len(schema) == 0 {
			schema = cloneMap(tool.Parameters)
		}
		descriptors = append(descriptors, ToolDescriptor{
			Name:        strings.TrimSpace(tool.Name),
			Description: strings.TrimSpace(tool.Description),
			InputSchema: schema,
		})
	}

	name := strings.TrimSpace(initResult.ServerInfo.Name)
	if name == "" {
		name = cfg.Name
	}
	version := strings.TrimSpace(initResult.ServerInfo.Version)
	if version == "" {
		version = cfg.Version
	}
	return ServerSnapshot{
		ID:      cfg.ID,
		Name:    name,
		Version: version,
		Tools:   descriptors,
	}, nil
}

func (c *StdioClient) CallTool(ctx context.Context, cfg ServerConfig, toolName string, raw json.RawMessage) (string, error) {
	cfg = normalizeServerConfig(cfg)
	if err := validateServerConfig(cfg, true); err != nil {
		return "", err
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "", newClientError(ClientErrorInvalidArgs, "tool name is required", nil)
	}

	args := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", newClientError(ClientErrorInvalidArgs, "tool arguments must be a JSON object", err)
		}
	}

	callCtx, cancel := withTimeoutIfMissing(ctx, cfg.CallTimeout)
	defer cancel()

	responses, err := c.runWithProtocolFallback(callCtx, cfg, func(protocolVersion string) []rpcRequest {
		return []rpcRequest{
			newRPCRequest(1, "initialize", initializeParams(protocolVersion)),
			newRPCNotification("notifications/initialized", map[string]any{}),
			newRPCRequest(2, "tools/call", map[string]any{
				"name":      toolName,
				"arguments": args,
			}),
		}
	})
	if err != nil {
		return "", err
	}
	if len(responses) != 2 {
		return "", newClientError(ClientErrorProtocol, "mcp server returned incomplete call responses", nil)
	}

	callResult := struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}{}
	if err := json.Unmarshal(responses[1].Result, &callResult); err == nil {
		if callResult.IsError {
			return "", newClientError(ClientErrorCallFailed, fmt.Sprintf("mcp tool %q returned isError", toolName), nil)
		}
		parts := make([]string, 0, len(callResult.Content))
		for _, item := range callResult.Content {
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			parts = append(parts, item.Text)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n"), nil
		}
	}

	compact, err := compactJSON(responses[1].Result)
	if err != nil {
		return "", newClientError(ClientErrorProtocol, "failed to normalize tools/call result", err)
	}
	return compact, nil
}

func (c *StdioClient) runRPC(ctx context.Context, cfg ServerConfig, requests []rpcRequest) ([]rpcResponse, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if strings.TrimSpace(cfg.CWD) != "" {
		cmd.Dir = cfg.CWD
	}
	cmd.Env = mergeCommandEnv(cfg.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, newClientError(ClientErrorTransport, "failed to open mcp stdin pipe", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, newClientError(ClientErrorTransport, "failed to open mcp stdout pipe", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, newClientError(ClientErrorTransport, "failed to start mcp server process", err)
	}
	defer stopCommand(cmd, stdin)

	reader := bufio.NewReader(stdout)
	writer := bufio.NewWriter(stdin)
	responses := make([]rpcResponse, 0, len(requests))

	for _, request := range requests {
		if err := writeRPCRequest(writer, request); err != nil {
			return nil, newClientError(ClientErrorTransport, "failed to write mcp request", err)
		}
		expectedID, hasRequestID, requestIDErr := normalizeRPCResponseID(request.ID)
		if requestIDErr != nil {
			return nil, newClientError(ClientErrorProtocol, "failed to encode mcp request id", requestIDErr)
		}
		if !hasRequestID {
			// Notifications do not have ids and do not produce responses.
			continue
		}
		for {
			response, err := readRPCResponse(reader)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return nil, newClientError(ClientErrorTimeout, "mcp request timed out", err)
				}
				if errors.Is(err, io.EOF) {
					message := "mcp server closed stdout before replying"
					if trimmed := strings.TrimSpace(stderr.String()); trimmed != "" {
						message = fmt.Sprintf("%s: %s", message, trimmed)
					}
					return nil, newClientError(ClientErrorTransport, message, err)
				}
				return nil, newClientError(ClientErrorProtocol, "failed to read mcp response", err)
			}

			actualID, hasID, idErr := normalizeRPCResponseID(response.ID)
			if idErr != nil {
				return nil, newClientError(ClientErrorProtocol, "failed to decode mcp response id", idErr)
			}
			if !hasID {
				// MCP servers may emit notifications before the response.
				if strings.TrimSpace(response.Method) != "" {
					continue
				}
				return nil, newClientError(ClientErrorProtocol, "mcp response missing id", nil)
			}
			if actualID != expectedID {
				return nil, newClientError(ClientErrorProtocol, "mcp response id mismatch", nil)
			}
			if response.Error != nil {
				return nil, mapRPCError(request.Method, response.Error)
			}
			responses = append(responses, response)
			break
		}
	}
	return responses, nil
}

func (c *StdioClient) runWithProtocolFallback(
	ctx context.Context,
	cfg ServerConfig,
	buildRequests func(protocolVersion string) []rpcRequest,
) ([]rpcResponse, error) {
	protocolVersions := cloneStringSlice(cfg.ProtocolVersions)
	if len(protocolVersions) == 0 {
		protocolVersions = cloneStringSlice(defaultProtocolVersions)
	}

	for index, protocolVersion := range protocolVersions {
		responses, err := c.runRPC(ctx, cfg, buildRequests(protocolVersion))
		if err == nil {
			return responses, nil
		}
		if isClientErrorCode(err, ClientErrorHandshakeFailed) && index < len(protocolVersions)-1 {
			continue
		}
		if isClientErrorCode(err, ClientErrorHandshakeFailed) && len(protocolVersions) > 1 {
			message := fmt.Sprintf("mcp initialize failed for protocol versions: %s", strings.Join(protocolVersions, ", "))
			return nil, newClientError(ClientErrorHandshakeFailed, message, err)
		}
		return nil, err
	}

	message := fmt.Sprintf("mcp initialize failed for protocol versions: %s", strings.Join(protocolVersions, ", "))
	return nil, newClientError(ClientErrorHandshakeFailed, message, nil)
}

func normalizeServerConfig(cfg ServerConfig) ServerConfig {
	cfg.ID = normalizeID(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Version = strings.TrimSpace(cfg.Version)
	cfg.ProtocolVersion = strings.TrimSpace(cfg.ProtocolVersion)
	cfg.Command = strings.TrimSpace(cfg.Command)
	cfg.CWD = strings.TrimSpace(cfg.CWD)
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	if cfg.StartupTimeout <= 0 {
		cfg.StartupTimeout = defaultStartupTimeout
	}
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = defaultCallTimeout
	}
	if cfg.Args == nil {
		cfg.Args = []string{}
	}
	cfg.ProtocolVersions = normalizeProtocolVersions(cfg.ProtocolVersion, cfg.ProtocolVersions)
	cfg.Env = cloneStringMap(cfg.Env)
	return cfg
}

func validateServerConfig(cfg ServerConfig, requireCommand bool) error {
	if strings.TrimSpace(cfg.ID) == "" {
		return newClientError(ClientErrorInvalidConfig, "mcp server id is required", nil)
	}
	if requireCommand && strings.TrimSpace(cfg.Command) == "" {
		return newClientError(ClientErrorInvalidConfig, "mcp server command is required", nil)
	}
	if strings.TrimSpace(cfg.CWD) != "" {
		if _, err := os.Stat(cfg.CWD); err != nil {
			return newClientError(ClientErrorInvalidConfig, "mcp server cwd is not accessible", err)
		}
	}
	return nil
}

func withTimeoutIfMissing(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	if _, has := ctx.Deadline(); has {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func mergeCommandEnv(extra map[string]string) []string {
	base := map[string]string{}
	for _, key := range defaultEnvWhitelist {
		if key = strings.TrimSpace(key); key == "" {
			continue
		}
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		base[key] = value
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		base[key] = value
	}
	env := make([]string, 0, len(base))
	for key, value := range base {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}

func stopCommand(cmd *exec.Cmd, stdin io.WriteCloser) {
	if stdin != nil {
		_ = stdin.Close()
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(200 * time.Millisecond):
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	<-done
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newRPCRequest(id int, method string, params any) rpcRequest {
	return rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

func newRPCNotification(method string, params any) rpcRequest {
	return rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
}

func initializeParams(protocolVersion string) map[string]any {
	return map[string]any{
		"protocolVersion": strings.TrimSpace(protocolVersion),
		"clientInfo": map[string]any{
			"name":    "bytemind",
			"version": "dev",
		},
	}
}

func isClientErrorCode(err error, code ClientErrorCode) bool {
	if err == nil {
		return false
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		return false
	}
	return clientErr.Code == code
}

func normalizeProtocolVersions(primary string, extras []string) []string {
	versions := make([]string, 0, 1+len(extras))
	if strings.TrimSpace(primary) != "" {
		versions = append(versions, primary)
	}
	versions = append(versions, extras...)

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(versions))
	for _, version := range versions {
		version = strings.TrimSpace(version)
		if version == "" {
			continue
		}
		if _, ok := seen[version]; ok {
			continue
		}
		seen[version] = struct{}{}
		normalized = append(normalized, version)
	}
	if len(normalized) > 0 {
		return normalized
	}
	return cloneStringSlice(defaultProtocolVersions)
}

func normalizeRPCResponseID(id any) (string, bool, error) {
	switch value := id.(type) {
	case nil:
		return "", false, nil
	case int:
		return strconv.Itoa(value), true, nil
	case int8:
		return strconv.Itoa(int(value)), true, nil
	case int16:
		return strconv.Itoa(int(value)), true, nil
	case int32:
		return strconv.Itoa(int(value)), true, nil
	case int64:
		return strconv.FormatInt(value, 10), true, nil
	case uint:
		return strconv.FormatUint(uint64(value), 10), true, nil
	case uint8:
		return strconv.FormatUint(uint64(value), 10), true, nil
	case uint16:
		return strconv.FormatUint(uint64(value), 10), true, nil
	case uint32:
		return strconv.FormatUint(uint64(value), 10), true, nil
	case uint64:
		return strconv.FormatUint(value, 10), true, nil
	case json.Number:
		if integer, err := value.Int64(); err == nil {
			return strconv.FormatInt(integer, 10), true, nil
		}
		if _, err := value.Float64(); err == nil {
			return "", true, fmt.Errorf("response id must be an integer, got %q", value.String())
		}
		return "", true, fmt.Errorf("invalid numeric response id %q", value.String())
	case float64:
		if value != float64(int64(value)) {
			return "", true, fmt.Errorf("response id must be an integer, got %v", value)
		}
		return strconv.FormatInt(int64(value), 10), true, nil
	case string:
		return strings.TrimSpace(value), true, nil
	default:
		return "", true, fmt.Errorf("unsupported response id type %T", id)
	}
}

func writeRPCRequest(writer *bufio.Writer, request rpcRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return writeFramedJSON(writer, data)
}

func readRPCResponse(reader *bufio.Reader) (rpcResponse, error) {
	payload, err := readFramedJSON(reader)
	if err != nil {
		return rpcResponse{}, err
	}
	var response rpcResponse
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return rpcResponse{}, err
	}
	return response, nil
}

func writeRPCResponse(writer *bufio.Writer, response rpcResponse) error {
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return writeFramedJSON(writer, data)
}

func readRPCRequest(reader *bufio.Reader) (rpcRequest, error) {
	payload, err := readFramedJSON(reader)
	if err != nil {
		return rpcRequest{}, err
	}
	var request rpcRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return rpcRequest{}, err
	}
	return request, nil
}

func writeFramedJSON(writer *bufio.Writer, payload []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := writer.WriteString(header); err != nil {
		return err
	}
	if _, err := writer.Write(payload); err != nil {
		return err
	}
	return writer.Flush()
}

func readFramedJSON(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid mcp frame header line %q", line)
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if key != "content-length" {
			continue
		}
		length, err := strconv.Atoi(value)
		if err != nil || length < 0 {
			return nil, fmt.Errorf("invalid content-length %q", value)
		}
		contentLength = length
	}
	if contentLength < 0 {
		return nil, errors.New("missing content-length header")
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func mapRPCError(method string, rpcErr *rpcError) error {
	if rpcErr == nil {
		return nil
	}
	message := strings.TrimSpace(rpcErr.Message)
	if message == "" {
		message = "mcp server returned an unknown error"
	}
	switch strings.TrimSpace(method) {
	case "initialize":
		return newClientError(ClientErrorHandshakeFailed, message, nil)
	case "tools/list":
		return newClientError(ClientErrorListToolsFailed, message, nil)
	case "tools/call":
		switch rpcErr.Code {
		case -32602:
			return newClientError(ClientErrorInvalidArgs, message, nil)
		case -32001:
			return newClientError(ClientErrorPermission, message, nil)
		default:
			return newClientError(ClientErrorCallFailed, message, nil)
		}
	default:
		return newClientError(ClientErrorProtocol, message, nil)
	}
}

func compactJSON(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return "", err
	}
	return out.String(), nil
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneStringSlice(input []string) []string {
	if input == nil {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func normalizeID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	raw = replacer.Replace(raw)
	raw = strings.Trim(raw, "-_")
	if raw == "" {
		return ""
	}
	return raw
}
