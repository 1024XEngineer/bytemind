package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"forgecli/internal/config"
	"forgecli/internal/editor"
	"forgecli/internal/executor"
	"forgecli/internal/model"
	"forgecli/internal/policy"
	"forgecli/internal/ui"
	"forgecli/internal/workspace"
)

type Mode string

const (
	ModeAnalyze Mode = "analyze"
	ModeFull    Mode = "full"
)

type Agent struct {
	cfg      config.Config
	terminal ui.Terminal
	logger   *slog.Logger
	client   model.ChatClient
	editor   editor.Service
	runner   executor.Runner
	policy   policy.Policy
	ws       *workspace.Workspace
	history  []model.Message
	mode     Mode
}

type listFilesArgs struct {
	Path  string `json:"path"`
	Limit int    `json:"limit"`
}

type readFileArgs struct {
	Path string `json:"path"`
}

type searchArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type runCommandArgs struct {
	Command string `json:"command"`
}

func New(cfg config.Config, terminal ui.Terminal, logger *slog.Logger, client model.ChatClient) (*Agent, error) {
	if terminal == nil {
		return nil, errors.New("terminal is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if client == nil {
		var err error
		client, err = model.NewChatClient(cfg.Model)
		if err != nil {
			return nil, err
		}
	}

	return &Agent{
		cfg:      cfg,
		terminal: terminal,
		logger:   logger,
		client:   client,
		editor:   editor.Service{},
		runner: executor.Runner{
			Timeout: time.Duration(cfg.CommandTimeoutSeconds) * time.Second,
		},
		policy: policy.New(cfg),
		mode:   ModeAnalyze,
	}, nil
}

func ParseMode(raw string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(ModeAnalyze):
		return ModeAnalyze, nil
	case string(ModeFull):
		return ModeFull, nil
	default:
		return "", fmt.Errorf("unsupported mode: %s", raw)
	}
}

func (a *Agent) SetMode(raw string) error {
	mode, err := ParseMode(raw)
	if err != nil {
		return err
	}
	a.mode = mode
	return nil
}

func (a *Agent) StartSession(repo string) error {
	ws, err := workspace.Open(repo, a.cfg.SearchIgnore, a.cfg.SensitivePatterns)
	if err != nil {
		return err
	}
	a.ws = ws
	a.history = nil
	return nil
}

func (a *Agent) Reset() {
	a.history = nil
}

func (a *Agent) WorkspaceRoot() string {
	if a.ws == nil {
		return ""
	}
	return a.ws.Root
}

func (a *Agent) ModelName() string {
	return a.cfg.Model.Model
}

func (a *Agent) ModeName() string {
	return string(a.mode)
}

func (a *Agent) ToolNames() []string {
	base := []string{"list_files", "read_file", "search"}
	if a.mode == ModeFull {
		base = append(base, "write_file", "run_command")
	}
	return base
}

func (a *Agent) RunTurn(ctx context.Context, userInput string) (string, error) {
	if a.ws == nil {
		return "", errors.New("session has not been started")
	}
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return "", errors.New("prompt is empty")
	}

	a.history = append(a.history, model.Message{Role: "user", Content: userInput})

	if a.mode == ModeAnalyze {
		lower := strings.ToLower(userInput)
		if asksCapabilities(lower) {
			answer := a.capabilitiesFallback()
			a.history = append(a.history, model.Message{Role: "assistant", Content: answer})
			return answer, nil
		}
		if asksProjectSummary(lower) {
			answer := a.projectSummaryFallback()
			a.history = append(a.history, model.Message{Role: "assistant", Content: answer})
			return answer, nil
		}
	}

	maxRounds := a.cfg.Agent.MaxToolRounds
	if maxRounds <= 0 {
		maxRounds = 12
	}
	toolRepeatCount := 0
	lastToolSignature := ""

	for round := 0; round < maxRounds; round++ {
		resp, err := a.client.Complete(ctx, model.ChatRequest{
			Model:       a.cfg.Model.Model,
			Messages:    a.requestMessages(),
			Tools:       a.toolDefinitions(),
			Temperature: a.cfg.Model.Temperature,
		})
		if err != nil {
			if a.hasToolResultInHistory() {
				answer := a.localToolFailureAnswer(err)
				a.history = append(a.history, model.Message{Role: "assistant", Content: answer})
				return answer, nil
			}
			return "", err
		}

		a.history = append(a.history, resp.Message)
		if len(resp.Message.ToolCalls) == 0 {
			return strings.TrimSpace(resp.Message.Content), nil
		}

		blocked := false
		for _, call := range resp.Message.ToolCalls {
			signature := call.Function.Name + "::" + strings.TrimSpace(call.Function.Arguments)
			if signature == lastToolSignature {
				toolRepeatCount++
			} else {
				lastToolSignature = signature
				toolRepeatCount = 1
			}

			var toolMessage model.Message
			if toolRepeatCount >= 3 {
				blocked = true
				toolMessage = model.Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: marshalToolResult(map[string]any{"ok": false, "error": "repeated tool call blocked; provide a final answer instead"})}
			} else {
				toolMessage = a.executeTool(ctx, call)
			}
			a.history = append(a.history, toolMessage)
		}

		if blocked || round == maxRounds-2 {
			return a.finalizeAnswer(ctx)
		}
	}

	return a.finalizeAnswer(ctx)
}

func (a *Agent) finalizeAnswer(ctx context.Context) (string, error) {
	resp, err := a.client.Complete(ctx, model.ChatRequest{
		Model:       a.cfg.Model.Model,
		Messages:    a.requestMessages(),
		Temperature: a.cfg.Model.Temperature,
	})
	if err == nil {
		content := strings.TrimSpace(resp.Message.Content)
		if content != "" && len(resp.Message.ToolCalls) == 0 {
			a.history = append(a.history, model.Message{Role: "assistant", Content: content})
			return content, nil
		}
	}
	if a.hasSuccessfulToolAction() {
		answer := a.localCompletedActionAnswer(err)
		a.history = append(a.history, model.Message{Role: "assistant", Content: answer})
		return answer, nil
	}
	answer := a.localFallbackAnswer(err)
	a.history = append(a.history, model.Message{Role: "assistant", Content: answer})
	return answer, nil
}

func (a *Agent) localFallbackAnswer(modelErr error) string {
	reason := "the model did not provide a usable final answer"
	if modelErr != nil {
		reason = modelErr.Error()
	}
	files, _ := a.ws.ListFiles(8)
	answer := "I inspected the workspace and the model did not finish cleanly, so here is a local fallback answer."
	if len(files) > 0 {
		answer += "\n\nVisible files:\n- " + strings.Join(files, "\n- ")
	}
	answer += "\n\nAvailable capabilities:\n- analyze mode can list files, read files, and search content\n- full mode can write files, show a diff preview, and run whitelisted verification commands\n- the session keeps in-memory conversation context\n- model settings are configurable through an OpenAI-compatible endpoint"
	answer += "\n\nFallback reason: " + reason
	return answer
}

func (a *Agent) localToolFailureAnswer(modelErr error) string {
	reason := "the model timed out after tool execution"
	if modelErr != nil && strings.TrimSpace(modelErr.Error()) != "" {
		reason = modelErr.Error()
	}
	summaries := a.recentToolSummaries()
	answer := "The requested file or command action already ran, but the final model response failed."
	if len(summaries) > 0 {
		answer += "\n\nCompleted actions:\n- " + strings.Join(summaries, "\n- ")
	}
	answer += "\n\nYou can inspect the generated files in the workspace directly."
	answer += "\n\nFallback reason: " + reason
	return answer
}

func (a *Agent) localCompletedActionAnswer(modelErr error) string {
	summaries := a.recentToolSummaries()
	if len(summaries) == 0 {
		return "操作已完成，你可以直接在工作区查看结果。"
	}
	answer := "操作已完成。"
	answer += "\n\n已执行内容:\n- " + strings.Join(summaries, "\n- ")
	answer += "\n\n你可以直接在工作区查看生成或更新后的文件。"
	if modelErr != nil && strings.TrimSpace(modelErr.Error()) != "" {
		answer += "\n\n补充说明：最终模型总结超时，但不影响已完成的文件或命令操作。"
	}
	return answer
}

func asksCapabilities(input string) bool {
	keywords := []string{"有什么功能", "有哪些功能", "能做什么", "支持什么", "capabilities", "what can", "features", "功能"}
	for _, keyword := range keywords {
		if strings.Contains(input, keyword) {
			return true
		}
	}
	return false
}

func asksProjectSummary(input string) bool {
	keywords := []string{"总结", "概览", "summary", "summarize", "项目结构", "repo", "readme", "介绍"}
	for _, keyword := range keywords {
		if strings.Contains(input, keyword) {
			return true
		}
	}
	return false
}

func (a *Agent) capabilitiesFallback() string {
	items := []string{
		"interactive chat with in-memory session context",
		"analyze mode for listing files, reading files, and searching the repo",
		"full mode for file edits, compact write previews, and whitelisted verification commands",
		"OpenAI-compatible model configuration via base_url, model, and api key",
		"workspace boundary checks, sensitive-file protection, and approval before writes or commands",
	}
	return "This MVP currently supports:\n- " + strings.Join(items, "\n- ")
}

func (a *Agent) projectSummaryFallback() string {
	files, _ := a.ws.ListFiles(10)
	answer := "Here is a local project summary fallback."
	if len(files) > 0 {
		answer += "\n\nTop files:\n- " + strings.Join(files, "\n- ")
	}
	answer += "\n\nFrom the code layout, this demo mainly provides chat, file reading/search, optional file edits, and whitelisted command execution."
	return answer
}

func (a *Agent) requestMessages() []model.Message {
	systemPrompt := strings.TrimSpace(a.cfg.Model.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = "You are ForgeCLI, a local coding assistant."
	}
	prompt := []string{systemPrompt, "Workspace root: " + a.ws.Root, "Current mode: " + string(a.mode)}
	if a.mode == ModeAnalyze {
		prompt = append(prompt, "Analyze mode is read-only. Use only list_files, read_file, and search. Do not attempt write_file or run_command.")
	} else {
		prompt = append(prompt, "Full mode can edit files and run a small set of approved verification commands. Use run_command only when file tools are insufficient.")
		prompt = append(prompt, "Allowed run_command prefixes: "+strings.Join(a.allowedCommandPrefixes(), ", "))
	}
	messages := []model.Message{{Role: "system", Content: strings.Join(prompt, "\n")}}
	maxHistory := a.cfg.Agent.MaxHistoryMessages
	if maxHistory <= 0 || len(a.history) <= maxHistory {
		return append(messages, a.history...)
	}
	return append(messages, a.history[len(a.history)-maxHistory:]...)
}

func (a *Agent) toolDefinitions() []model.ToolDefinition {
	definitions := []model.ToolDefinition{
		{Type: "function", Function: model.FunctionDefinition{Name: "list_files", Description: "List files in the workspace or in a subdirectory.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}}}}},
		{Type: "function", Function: model.FunctionDefinition{Name: "read_file", Description: "Read a text file from the workspace.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}}},
		{Type: "function", Function: model.FunctionDefinition{Name: "search", Description: "Search text in workspace files.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}}, "required": []string{"query"}}}},
	}
	if a.mode == ModeFull {
		definitions = append(definitions,
			model.ToolDefinition{Type: "function", Function: model.FunctionDefinition{Name: "write_file", Description: "Write the full updated content of a file in the workspace.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"path", "content"}}}},
			model.ToolDefinition{Type: "function", Function: model.FunctionDefinition{Name: "run_command", Description: "Run an approved verification command.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}, "required": []string{"command"}}}},
		)
	}
	return definitions
}

func (a *Agent) executeTool(ctx context.Context, call model.ToolCall) model.Message {
	var result string
	switch call.Function.Name {
	case "list_files":
		result = a.handleListFiles(call.Function.Arguments)
	case "read_file":
		result = a.handleReadFile(call.Function.Arguments)
	case "search":
		result = a.handleSearch(call.Function.Arguments)
	case "write_file":
		result = a.handleWriteFile(call.Function.Arguments)
	case "run_command":
		result = a.handleRunCommand(ctx, call.Function.Arguments)
	default:
		result = marshalToolResult(map[string]any{"ok": false, "error": fmt.Sprintf("unknown tool: %s", call.Function.Name)})
	}
	return model.Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result}
}

func (a *Agent) handleListFiles(raw string) string {
	var args listFilesArgs
	if err := decodeToolArgs(raw, &args); err != nil {
		return toolError(err)
	}
	limit := clamp(args.Limit, 1, a.cfg.Agent.ListFilesLimit)
	if limit == 0 {
		limit = 200
	}
	files, err := a.ws.ListFiles(limit * 4)
	if err != nil {
		return toolError(err)
	}
	prefix := normalizePrefix(args.Path)
	filtered := make([]string, 0, min(limit, len(files)))
	for _, file := range files {
		if prefix == "" || strings.HasPrefix(file, prefix) {
			filtered = append(filtered, file)
		}
		if len(filtered) >= limit {
			break
		}
	}
	return marshalToolResult(map[string]any{"ok": true, "path": prefix, "count": len(filtered), "files": filtered})
}

func (a *Agent) handleReadFile(raw string) string {
	var args readFileArgs
	if err := decodeToolArgs(raw, &args); err != nil {
		return toolError(err)
	}
	result, err := a.ws.ReadFile(args.Path)
	if err != nil {
		return toolError(err)
	}
	if result.Sensitive {
		return toolError(fmt.Errorf("refusing to read sensitive file: %s", args.Path))
	}
	maxBytes := a.cfg.Agent.ReadFileMaxBytes
	if maxBytes <= 0 {
		maxBytes = 20000
	}
	content := result.Content
	truncated := false
	if len(content) > maxBytes {
		content = content[:maxBytes]
		truncated = true
	}
	return marshalToolResult(map[string]any{"ok": true, "path": result.Path, "sensitive": result.Sensitive, "truncated": truncated, "total_bytes": len(result.Content), "content": string(content)})
}

func (a *Agent) handleSearch(raw string) string {
	var args searchArgs
	if err := decodeToolArgs(raw, &args); err != nil {
		return toolError(err)
	}
	limit := clamp(args.Limit, 1, a.cfg.Agent.SearchLimit)
	if limit == 0 {
		limit = 20
	}
	hits, err := a.ws.Search(args.Query, limit)
	if err != nil {
		return toolError(err)
	}
	return marshalToolResult(map[string]any{"ok": true, "query": args.Query, "count": len(hits), "hits": hits})
}

func (a *Agent) handleWriteFile(raw string) string {
	if a.mode != ModeFull {
		return toolError(errors.New("write_file is unavailable in analyze mode"))
	}
	var args writeFileArgs
	if err := decodeToolArgs(raw, &args); err != nil {
		return toolError(err)
	}
	if strings.TrimSpace(args.Path) == "" {
		return toolError(errors.New("path is required"))
	}
	if err := a.policy.EnsureEditablePath(args.Path); err != nil {
		return toolError(err)
	}
	absPath, err := resolveWritePath(a.ws.Root, args.Path)
	if err != nil {
		return toolError(err)
	}
	oldContent, hash, existed, err := readExistingFile(absPath)
	if err != nil {
		return toolError(err)
	}
	preview := renderWritePreview(filepath.ToSlash(args.Path), oldContent, []byte(args.Content), existed, a.editor)
	a.terminal.Printf("\n%s\n\n", preview)
	approved, err := a.terminal.PromptYesNo(fmt.Sprintf("Apply write to %s?", filepath.ToSlash(args.Path)))
	if err != nil {
		return toolError(err)
	}
	if !approved {
		return marshalToolResult(map[string]any{"ok": false, "path": filepath.ToSlash(args.Path), "approved": false, "message": "user denied file write"})
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return toolError(err)
	}
	if existed {
		if err := a.editor.Apply(absPath, hash, []byte(args.Content)); err != nil {
			return toolError(err)
		}
	} else {
		if err := os.WriteFile(absPath, []byte(args.Content), 0o644); err != nil {
			return toolError(err)
		}
	}
	lineCount := 0
	if args.Content != "" {
		lineCount = strings.Count(args.Content, "\n") + 1
	}
	return marshalToolResult(map[string]any{"ok": true, "path": filepath.ToSlash(args.Path), "approved": true, "created": !existed, "bytes": len(args.Content), "line_count": lineCount})
}

func (a *Agent) handleRunCommand(ctx context.Context, raw string) string {
	if a.mode != ModeFull {
		return toolError(errors.New("run_command is unavailable in analyze mode"))
	}
	var args runCommandArgs
	if err := decodeToolArgs(raw, &args); err != nil {
		return toolError(err)
	}
	if shortcut, ok := a.handleCommandShortcut(args.Command); ok {
		return shortcut
	}
	if !a.commandAllowed(args.Command) {
		return toolError(fmt.Errorf("command is not allowed; allowed prefixes: %s", strings.Join(a.allowedCommandPrefixes(), ", ")))
	}
	if err := a.policy.ValidateCommand(args.Command); err != nil {
		return toolError(err)
	}
	approved, err := a.terminal.PromptYesNo(fmt.Sprintf("Run command %q?", args.Command))
	if err != nil {
		return toolError(err)
	}
	if !approved {
		return marshalToolResult(map[string]any{"ok": false, "approved": false, "command": args.Command, "message": "user denied command execution"})
	}
	result, err := a.runner.Run(ctx, a.ws.Root, args.Command)
	if err != nil {
		return marshalToolResult(map[string]any{"ok": false, "approved": true, "command": args.Command, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr, "timed_out": result.TimedOut, "error": err.Error()})
	}
	return marshalToolResult(map[string]any{"ok": true, "approved": true, "command": args.Command, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr, "timed_out": result.TimedOut, "duration": result.Duration.String()})
}

func (a *Agent) handleCommandShortcut(command string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(command))
	switch normalized {
	case "pwd":
		return marshalToolResult(map[string]any{"ok": true, "approved": true, "command": command, "virtual": true, "stdout": a.ws.Root, "message": "Returned workspace root without spawning a subprocess."}), true
	case "cd", "cd .":
		return marshalToolResult(map[string]any{"ok": true, "approved": true, "command": command, "virtual": true, "stdout": a.ws.Root, "message": "The agent already runs inside the workspace root."}), true
	case "dir", "dir .", "ls", "ls .", "ls -l", "ls -la", "get-childitem":
		files, err := a.ws.ListFiles(200)
		if err != nil {
			return toolError(err), true
		}
		return marshalToolResult(map[string]any{"ok": true, "approved": true, "command": command, "virtual": true, "count": len(files), "stdout": strings.Join(files, "\n"), "message": "Listed workspace files without spawning a subprocess."}), true
	default:
		return "", false
	}
}

func (a *Agent) allowedCommandPrefixes() []string {
	return []string{"go test", "go build", "git status"}
}

func (a *Agent) commandAllowed(command string) bool {
	normalized := strings.ToLower(strings.TrimSpace(command))
	for _, prefix := range a.allowedCommandPrefixes() {
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if normalized == prefix || strings.HasPrefix(normalized, prefix+" ") {
			return true
		}
	}
	return false
}

func (a *Agent) hasToolResultInHistory() bool {
	for _, message := range a.history {
		if message.Role == "tool" {
			return true
		}
	}
	return false
}

func (a *Agent) hasSuccessfulToolAction() bool {
	for i := len(a.history) - 1; i >= 0; i-- {
		message := a.history[i]
		if message.Role != "tool" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(message.Content), &payload); err != nil {
			continue
		}
		ok, _ := payload["ok"].(bool)
		approved, hasApproved := payload["approved"].(bool)
		if ok && (!hasApproved || approved) {
			return true
		}
	}
	return false
}

func (a *Agent) recentToolSummaries() []string {
	items := make([]string, 0, 4)
	for i := len(a.history) - 1; i >= 0 && len(items) < 4; i-- {
		message := a.history[i]
		if message.Role != "tool" {
			continue
		}
		if summary := summarizeToolMessage(message); summary != "" {
			items = append([]string{summary}, items...)
		}
	}
	return items
}

func summarizeToolMessage(message model.Message) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(message.Content), &payload); err != nil {
		return ""
	}
	if path, ok := payload["path"].(string); ok {
		if approved, ok := payload["approved"].(bool); ok && approved {
			if created, ok := payload["created"].(bool); ok && created {
				return "created file " + path
			}
			return "updated file " + path
		}
	}
	if command, ok := payload["command"].(string); ok {
		if approved, ok := payload["approved"].(bool); ok && approved {
			return "ran command " + command
		}
	}
	return ""
}

func renderWritePreview(path string, oldContent, newContent []byte, existed bool, service editor.Service) string {
	lineCount := 0
	if len(newContent) > 0 {
		lineCount = strings.Count(string(newContent), "\n") + 1
	}
	if !existed {
		return fmt.Sprintf("New file preview for %s omitted from terminal. File size: %d bytes, about %d lines.", path, len(newContent), lineCount)
	}
	diffPreview := service.Preview(oldContent, newContent)
	lines := strings.Split(diffPreview, "\n")
	const maxLines = 80
	if len(lines) <= maxLines {
		return fmt.Sprintf("Diff preview for %s:\n%s", path, diffPreview)
	}
	return fmt.Sprintf("Diff preview for %s (truncated, %d lines shown of %d):\n%s\n...", path, maxLines, len(lines), strings.Join(lines[:maxLines], "\n"))
}

func decodeToolArgs(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

func marshalToolResult(value map[string]any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return `{"ok":false,"error":"failed to encode tool result"}`
	}
	return string(data)
}

func toolError(err error) string {
	return marshalToolResult(map[string]any{"ok": false, "error": err.Error()})
}

func clamp(value, minValue, maxValue int) int {
	if value == 0 {
		return 0
	}
	if value < minValue {
		return minValue
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizePrefix(path string) string {
	trimmed := strings.Trim(filepath.ToSlash(strings.TrimSpace(path)), "/")
	if trimmed == "" {
		return ""
	}
	return trimmed + "/"
}

func resolveWritePath(root, relativePath string) (string, error) {
	candidate := relativePath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, filepath.FromSlash(relativePath))
	}
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes workspace: %s", relativePath)
	}
	return candidate, nil
}

func readExistingFile(path string) ([]byte, string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", false, nil
		}
		return nil, "", false, err
	}
	return data, hashBytes(data), true, nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
