package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opencode-go/internal/llm"
	"github.com/opencode-go/internal/store"
	"golang.org/x/term"
)

type State int

const (
	StateIdle State = iota
	StateWaitingInput
	StateTaskActive
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateWaitingInput:
		return "waiting_for_input"
	case StateTaskActive:
		return "task_active"
	default:
		return "unknown"
	}
}

const SYSTEM_PROMPT = `You are an expert coding agent.

TOOLS:

1. fs - File system operations
   - read: {"tool":"fs","action":"read","path":"file.txt"}
   - write: {"tool":"fs","action":"write","path":"file.txt","content":"..."}
   - mkdir: {"tool":"fs","action":"mkdir","path":"folder"}
   - list: {"tool":"fs","action":"list","path":"."}
   - exists: {"tool":"fs","action":"exists","path":"file.txt"}
   - remove: {"tool":"fs","action":"remove","path":"file.txt"}

2. shell - Execute commands (ALLOWED: dir, ls, pwd, echo, go, python, node, git)
   - {"tool":"shell","action":"run","command":"ls -la"}

3. ask - Ask user
   - {"tool":"ask","action":"confirm","question":"Continue?"}
   - {"tool":"ask","action":"input","prompt":"Enter filename:"}

RULES:
- Output ONLY one JSON object per response. No arrays. No markdown.
- When done: {"tool":"done","result":"what was done"}
- When answering: {"tool":"answer","content":"your response"}
- If user says hi/hello/thanks/bye, use {"tool":"answer","content":"greeting"}`

const CHAT_PATTERNS = `If user says ONLY greetings like "你好", "hello", "hi", "在吗", "谢谢", "bye", respond with:
{"tool":"answer","content":"your greeting response"}

Do NOT assume any task is in progress unless user explicitly mentions one.`

type AgentRequest struct {
	Tool     string `json:"tool"`
	Action   string `json:"action"`
	Path     string `json:"path,omitempty"`
	Content  string `json:"content,omitempty"`
	Command  string `json:"command,omitempty"`
	Question string `json:"question,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	Result   string `json:"result,omitempty"`
}

type ToolResult struct {
	Success bool
	Output  string
	Error   string
}

type CLI struct {
	llm           *llm.Client
	store         *store.Store
	reader        *bufio.Reader
	width         int
	workDir       string
	state         State
	pendingPrompt string
	pendingType   string
}

func Run(ctx context.Context) error {
	workDir, _ := os.Getwd()
	c := &CLI{
		llm:     llm.New(),
		store:   &store.Store{},
		reader:  bufio.NewReader(os.Stdin),
		workDir: workDir,
		state:   StateIdle,
	}
	var err error
	c.store, err = store.New()
	if err != nil {
		return err
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		c.width = w
	}
	if c.width == 0 {
		c.width = 80
	}

	if !c.llm.HasKey() {
		fmt.Println("⚠️  Please set DEEPSEEK_API_KEY")
		return nil
	}

	fmt.Println("🤖 OpenCode Agent")
	fmt.Println("   Working dir:", workDir)
	fmt.Println("   Type 'quit' to exit")
	fmt.Println()

	for {
		fmt.Printf("[state=%s] > ", c.state)
		input, err := c.reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if input == "quit" || input == "exit" {
			break
		}

		if err := c.handleInput(ctx, input); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	return nil
}

func (c *CLI) handleInput(ctx context.Context, input string) error {
	fmt.Printf("\n[DEBUG] state=%s pending_prompt=%q pending_type=%s\n",
		c.state, c.pendingPrompt, c.pendingType)

	switch c.state {
	case StateWaitingInput:
		fmt.Println("[DEBUG] clearing pending ask state")
		answer := input
		c.state = StateIdle
		c.pendingPrompt = ""
		c.pendingType = ""

		fmt.Printf("User input to '%s': %s\n", c.pendingType, answer)
		c.store.AddMessage("user", answer)
		c.store.Save()

		resp := fmt.Sprintf("User answered: %s", answer)
		c.store.AddMessage("assistant", resp)
		c.store.Save()
		fmt.Printf("\n%s\n", resp)
		return nil

	case StateIdle, StateTaskActive:
		return c.runAgent(ctx, input)

	default:
		fmt.Println("[DEBUG] unknown state, resetting to idle")
		c.resetState()
		return c.runAgent(ctx, input)
	}
}

func (c *CLI) resetState() {
	fmt.Println("[DEBUG] resetting state to idle")
	c.state = StateIdle
	c.pendingPrompt = ""
	c.pendingType = ""
}

func (c *CLI) isGreeting(input string) bool {
	greetings := []string{"你好", "hi", "hello", "嗨", "在吗", "在不在", "嗨", "hey", "您好"}
	lower := strings.ToLower(strings.TrimSpace(input))
	for _, g := range greetings {
		if lower == g || lower == g+"。" || lower == g+"!" {
			return true
		}
	}
	return false
}

func (c *CLI) runAgent(ctx context.Context, userInput string) error {
	if c.isGreeting(userInput) {
		c.store.AddMessage("user", userInput)
		greeting := "你好！有什么我可以帮你的吗？"
		c.store.AddMessage("assistant", greeting)
		c.store.Save()
		fmt.Printf("\n%s\n", greeting)
		c.state = StateIdle
		return nil
	}

	c.state = StateTaskActive
	c.store.AddMessage("user", userInput)

	maxSteps := 10
	invalidCount := 0

	for step := 0; step < maxSteps; step++ {
		fmt.Printf("\n[Step %d]\n", step+1)

		messages := []llm.Message{
			{Role: "system", Content: SYSTEM_PROMPT},
			{Role: "system", Content: CHAT_PATTERNS},
		}

		history := c.store.GetHistory()
		start := 0
		if len(history) > 20 {
			start = len(history) - 20
		}
		for _, m := range history[start:] {
			messages = append(messages, llm.Message{Role: m.Role, Content: m.Content})
		}

		resp, err := c.llm.Chat(ctx, messages)
		if err != nil {
			return err
		}

		fmt.Printf("  Raw: %s\n", truncate(resp, 200))

		resp = strings.TrimSpace(resp)
		resp = strings.TrimPrefix(resp, "```json")
		resp = strings.TrimPrefix(resp, "```JSON")
		resp = strings.TrimPrefix(resp, "```")
		resp = strings.TrimSuffix(resp, "```")
		resp = strings.TrimSpace(resp)

		var req AgentRequest
		if err := json.Unmarshal([]byte(resp), &req); err != nil {
			fmt.Printf("  ⚠️  Parse error: %v\n", err)
			invalidCount++
			if invalidCount >= 2 {
				fmt.Println("\n❌ Invalid responses twice. Stopping.")
				c.resetState()
				return nil
			}
			continue
		}

		fmt.Printf("  Tool: %s, Action: %s\n", req.Tool, req.Action)
		req.Action = normalizeAction(req.Action)

		var result ToolResult

		switch req.Tool {
		case "done":
			fmt.Printf("\n✅ %s\n", req.Result)
			c.store.AddMessage("assistant", req.Result)
			c.store.Save()
			c.resetState()
			return nil

		case "answer":
			fmt.Printf("\n%s\n", req.Content)
			c.store.AddMessage("assistant", req.Content)
			c.store.Save()
			c.resetState()
			return nil

		case "fs":
			result = c.execFS(req)

		case "shell":
			if req.Action == "run" {
				result = c.execShell(req)
			} else {
				result = ToolResult{Success: false, Error: "shell only supports 'run'"}
			}

		case "ask":
			if req.Action == "confirm" || req.Action == "input" {
				fmt.Printf("\n❓ %s\n", req.Question+req.Prompt)
				c.pendingPrompt = req.Question + req.Prompt
				c.pendingType = req.Action
				c.state = StateWaitingInput
				return nil
			} else {
				result = ToolResult{Success: false, Error: "ask only supports 'confirm' and 'input'"}
			}

		default:
			result = ToolResult{Success: false, Error: fmt.Sprintf("unknown tool: %s", req.Tool)}
		}

		fmt.Printf("  Result: success=%v output=%s error=%s\n", result.Success, truncate(result.Output, 100), result.Error)

		c.store.AddMessage("assistant", resp)

		var feedback string
		if result.Success {
			feedback = fmt.Sprintf("Success: %s", result.Output)
		} else {
			feedback = fmt.Sprintf("Error: %s", result.Error)
			invalidCount++
		}
		c.store.AddMessage("user", feedback)

		if invalidCount >= 2 {
			fmt.Printf("\n❌ Tool errors twice. Stopping.\n")
			c.resetState()
			return nil
		}
	}

	fmt.Println("\n⚠️  Max steps reached.")
	c.resetState()
	return nil
}

func normalizeAction(action string) string {
	action = strings.ToLower(action)
	aliases := map[string]string{
		"create_dir":  "mkdir",
		"mkdirs":      "mkdir",
		"write_file":  "write",
		"read_file":   "read",
		"file_exists": "exists",
		"ls":          "list",
		"dir":         "list",
		"delete":      "remove",
		"delete_file": "remove",
		"remove_file": "remove",
		"clarify":     "input",
		"input":       "input",
		"approve":     "confirm",
		"yes_no":      "confirm",
		"exec":        "run",
		"execute":     "run",
		"command":     "run",
	}
	if newAction, ok := aliases[action]; ok {
		return newAction
	}
	return action
}

func (c *CLI) execFS(req AgentRequest) ToolResult {
	path := c.sanitizePath(req.Path)

	switch req.Action {
	case "read":
		if path == "" {
			return ToolResult{Success: false, Error: "path required"}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}
		}
		return ToolResult{Success: true, Output: string(data)}

	case "write":
		if path == "" || req.Content == "" {
			return ToolResult{Success: false, Error: "path and content required"}
		}
		dir := filepath.Dir(path)
		if dir != "" && dir != "." {
			os.MkdirAll(dir, 0755)
		}
		if err := os.WriteFile(path, []byte(req.Content), 0644); err != nil {
			return ToolResult{Success: false, Error: err.Error()}
		}
		return ToolResult{Success: true, Output: fmt.Sprintf("Written %d bytes to %s", len(req.Content), path)}

	case "mkdir":
		if path == "" {
			return ToolResult{Success: false, Error: "path required"}
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return ToolResult{Success: false, Error: err.Error()}
		}
		return ToolResult{Success: true, Output: fmt.Sprintf("Created directory: %s", path)}

	case "list":
		target := path
		if target == "" {
			target = c.workDir
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}
		}
		var out strings.Builder
		for _, e := range entries {
			mark := "[FILE]"
			if e.IsDir() {
				mark = "[DIR] "
			}
			out.WriteString(fmt.Sprintf("  %s %s\n", mark, e.Name()))
		}
		return ToolResult{Success: true, Output: out.String()}

	case "exists":
		if path == "" {
			return ToolResult{Success: false, Error: "path required"}
		}
		_, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ToolResult{Success: true, Output: fmt.Sprintf("Does not exist: %s", path)}
			}
			return ToolResult{Success: false, Error: err.Error()}
		}
		return ToolResult{Success: true, Output: fmt.Sprintf("Exists: %s", path)}

	case "remove":
		if path == "" {
			return ToolResult{Success: false, Error: "path required"}
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ToolResult{Success: true, Output: fmt.Sprintf("Already removed: %s", path)}
			}
			return ToolResult{Success: false, Error: err.Error()}
		}
		if info.IsDir() {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil {
			return ToolResult{Success: false, Error: err.Error()}
		}
		return ToolResult{Success: true, Output: fmt.Sprintf("Removed: %s", path)}

	default:
		return ToolResult{Success: false, Error: fmt.Sprintf("unknown fs action: %s", req.Action)}
	}
}

func (c *CLI) execShell(req AgentRequest) ToolResult {
	if req.Command == "" {
		return ToolResult{Success: false, Error: "command required"}
	}

	parts := strings.Fields(req.Command)
	if len(parts) == 0 {
		return ToolResult{Success: false, Error: "empty command"}
	}

	cmd := exec.Command("cmd", "/C", req.Command)
	cmd.Dir = c.workDir
	out, err := cmd.CombinedOutput()

	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("%s\n%s", string(out), err.Error())}
	}
	return ToolResult{Success: true, Output: string(out)}
}

func (c *CLI) sanitizePath(p string) string {
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if len(p) >= 2 && p[1] == ':' {
		return p
	}
	if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "/") {
		return filepath.Join(c.workDir, p[2:])
	}
	return p
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
