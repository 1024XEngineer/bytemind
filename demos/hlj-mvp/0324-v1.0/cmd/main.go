package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/forgecli/forgecli/pkg/edit"
	"github.com/forgecli/forgecli/pkg/execx"
	"github.com/forgecli/forgecli/pkg/model"
	"github.com/forgecli/forgecli/pkg/policy"
	"github.com/forgecli/forgecli/pkg/session"
	"github.com/forgecli/forgecli/pkg/skills"
	"github.com/forgecli/forgecli/pkg/workspace"
)

var (
	apiKey        string
	modelName     string
	apiBase       string
	autoApprove   bool
	timeout       int
	resetConfig   bool
	workspacePath string
	skillsDir     string
)

func main() {
	parseFlags()

	if len(os.Args) > 1 && os.Args[1] == "--help" {
		printUsage()
		os.Exit(0)
	}

	runREPL()
}

func printUsage() {
	fmt.Println(`ForgeCLI - 终端原生 coding agent

用法:
  forgecli [选项] [工作目录]

选项:
  -w, --workspace <目录>  工作目录 (默认: .)
  --model <模型>         模型名称 (默认: deepseek-chat)
  --api-base <url>      API 地址 (默认: https://api.deepseek.com)
  --timeout <秒>        命令超时 (默认: 30)
  -y, --yes             自动批准
  -i, --init            初始化配置
  -h, --help            显示帮助
`)
}

func parseFlags() {
	workspacePath = "."
	skillsDir = "gstack-main"
	modelName = "deepseek-chat"
	apiBase = "https://api.deepseek.com"
	timeout = 30

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "-w", "--workspace":
			if i+1 < len(os.Args) {
				workspacePath = os.Args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(os.Args) {
				modelName = os.Args[i+1]
				i++
			}
		case "--api-base":
			if i+1 < len(os.Args) {
				apiBase = os.Args[i+1]
				i++
			}
		case "--timeout":
			if i+1 < len(os.Args) {
				fmt.Sscanf(os.Args[i+1], "%d", &timeout)
				i++
			}
		case "-y", "--yes":
			autoApprove = true
		case "-i", "--init", "--reset":
			resetConfig = true
		}
	}
}

func runREPL() {
	ws, err := workspace.New(workspacePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	if !ws.IsTrusted() {
		fmt.Fprintf(os.Stderr, "错误: 不受信任的工作目录\n")
		os.Exit(1)
	}

	absPath, _ := filepath.Abs(workspacePath)

	loadConfig()

	cfg, _ := model.LoadConfig()
	if cfg.APIKey != "" {
		apiKey = cfg.APIKey
	}
	if cfg.Model != "" {
		modelName = cfg.Model
	}
	if cfg.APIBase != "" {
		apiBase = cfg.APIBase
	}

	sess := session.New(absPath)
	policyEngine := policy.New(absPath)
	editEngine := edit.New()
	execEngine := execx.New(timeout, absPath)
	modelClient := model.NewClient(apiKey, modelName, apiBase)

	// 加载 skills
	skillsManager := skills.NewManager(skillsDir)
	skillsManager.Load()

	fmt.Printf("=== ForgeCLI #%s ===\n", sess.ID[:8])
	fmt.Printf("工作目录: %s\n", absPath)
	fmt.Printf("模型: %s\n", modelName)

	if len(skillsManager.List()) > 0 {
		fmt.Printf("Skills: %d loaded\n", len(skillsManager.List()))
	}

	if apiKey == "" {
		fmt.Println("\n提示: 使用 -i 初始化 API Key")
	}

	loadRules(sess, absPath)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "exit" || input == "quit" {
			break
		}
		if input == "" {
			continue
		}

		if input == "help" {
			printHelp()
			continue
		}

		// Skill 命令
		if strings.HasPrefix(input, "/") {
			skillName := strings.TrimSpace(input)
			if skillsManager.Has(skillName) {
				skill := skillsManager.Get(skillName)
				fmt.Printf("\n=== Skill: %s ===\n", skill.Name)
				fmt.Printf("%s\n\n", skill.Description)

				// 提取并显示 skill 内容的主要部分
				lines := strings.Split(skill.Content, "\n")
				inBody := false
				bodyStarted := false
				for i, line := range lines {
					if strings.HasPrefix(line, "---") {
						if !inBody {
							inBody = true
							continue
						}
					}
					if inBody && !bodyStarted {
						if !strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "```") {
							continue
						}
						bodyStarted = true
					}
					if bodyStarted && i > 0 {
						fmt.Println(line)
					}
				}
				continue
			} else {
				fmt.Printf("未知 skill: %s\n", skillName)
				fmt.Print(skillsManager.PrintHelp())
				continue
			}
		}

		if input == "skills" {
			fmt.Print(skillsManager.PrintHelp())
			continue
		}

		if input == "config" {
			handleConfig(modelClient)
			continue
		}

		if input == "status" {
			fmt.Printf("会话: %s\n", sess.ID[:8])
			fmt.Printf("消息数: %d\n", len(sess.GetMessages()))
			continue
		}

		if input == "list" {
			files, _ := ws.ListFiles()
			for _, f := range files {
				fmt.Println(f)
			}
			continue
		}

		if strings.HasPrefix(input, "glob ") {
			pattern := strings.TrimSpace(input[5:])
			files, _ := ws.GlobFiles(pattern)
			for _, f := range files {
				fmt.Println(f)
			}
			continue
		}

		if strings.HasPrefix(input, "grep ") {
			pattern := strings.TrimSpace(input[5:])
			results, _ := ws.GrepFiles(pattern)
			for _, r := range results {
				fmt.Println(r)
			}
			continue
		}

		if strings.HasPrefix(input, "read ") {
			path := strings.TrimSpace(input[5:])
			content, err := ws.ReadFile(path)
			if err != nil {
				fmt.Printf("错误: %v\n", err)
			} else {
				fmt.Println(content)
			}
			continue
		}

		if strings.HasPrefix(input, "edit ") {
			parts := strings.SplitN(input[5:], " ", 2)
			if len(parts) < 2 {
				fmt.Println("用法: edit <filename> <内容>")
				continue
			}
			path := ws.ResolvePath(parts[0])
			content := parts[1]

			if policyEngine.IsSensitiveFile(parts[0]) {
				fmt.Println("错误: 拒绝编辑敏感文件")
				continue
			}

			diff := editEngine.GenerateDiff(path, "", content)
			fmt.Println(diff)

			if !autoApprove {
				fmt.Print("确认写入? (y/n): ")
				if !scanner.Scan() {
					break
				}
				if strings.ToLower(scanner.Text()) != "y" {
					fmt.Println("已取消")
					continue
				}
			}

			_, err := editEngine.Edit(path, content)
			if err != nil {
				fmt.Printf("错误: %v\n", err)
			} else {
				fmt.Println("已写入")
			}
			continue
		}

		if strings.HasPrefix(input, "!") {
			cmdStr := strings.TrimSpace(input[1:])
			if !execEngine.IsDangerous(cmdStr) {
				result, _ := execEngine.Run(cmdStr)
				fmt.Println(result.Output)
			} else {
				fmt.Println("危险命令被拦截")
			}
			continue
		}

		sess.AddUserMessage(input)

		if modelClient.IsConfigured() {
			msgs := toModelMessages(sess.GetMessages())
			maxIterations := 10

			for iter := 0; iter < maxIterations; iter++ {
				resp, err := modelClient.Chat(msgs, model.DefaultTools)
				if err != nil {
					fmt.Printf("错误: %v\n", err)
					break
				}

				// 有工具调用
				if len(resp.ToolCalls) > 0 {
					// 先把 assistant 消息（包含 tool_calls）添加到 msgs
					assistantMsg := model.Message{
						Role:      "assistant",
						ToolCalls: resp.ToolCalls,
					}
					msgs = append(msgs, assistantMsg)

					for _, tc := range resp.ToolCalls {
						fmt.Printf("\n[工具 %d] %s\n", iter+1, tc.Function.Name)
						result := executeTool(tc.Function.Name, tc.Function.Arguments, ws, editEngine, execEngine)
						fmt.Printf("[结果] %s\n", truncate(result, 300))
						fmt.Println()

						// 添加工具结果到消息
						toolMsg := model.Message{
							Role:       "tool",
							ToolCallID: tc.ID,
							Content:    fmt.Sprintf("工具 %s 返回:\n%s", tc.Function.Name, result),
						}
						msgs = append(msgs, toolMsg)
					}
					// 继续循环，让模型基于工具结果生成回复
					continue
				}

				// 没有工具调用，输出回复并结束
				if resp.Content != "" {
					fmt.Println(resp.Content)
					sess.AddAssistantMessage(resp.Content)
				}
				break
			}
		} else {
			fmt.Println("请配置 API Key 或使用本地命令")
		}

		sess.Save(absPath)
	}
}

func loadConfig() {
	if resetConfig {
		os.Remove(model.GetConfigPath())
		apiKey = ""
		fmt.Println("配置已重置")
	}

	cfg, err := model.LoadConfig()
	if err == nil && cfg.APIKey != "" {
		apiKey = cfg.APIKey
		if cfg.Model != "" {
			modelName = cfg.Model
		}
		if cfg.APIBase != "" {
			apiBase = cfg.APIBase
		}
		return
	}

	if apiKey == "" {
		fmt.Println("\n=== 首次使用，请配置 API Key ===")
		fmt.Printf("模型: %s\n", modelName)
		fmt.Printf("API: %s\n", apiBase)
		fmt.Print("请输入 API Key: ")

		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			inputKey := strings.TrimSpace(scanner.Text())
			if inputKey != "" {
				apiKey = inputKey
				model.SaveConfig(model.Config{
					APIKey:  apiKey,
					Model:   modelName,
					APIBase: apiBase,
				})
				fmt.Println("配置已保存")
			}
		}
	}
}

func handleConfig(client *model.Client) {
	fmt.Println("\n=== 配置 ===")
	fmt.Printf("API Key: %s\n", maskKey(apiKey))
	fmt.Printf("模型: %s\n", modelName)
	fmt.Printf("API: %s\n", apiBase)
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "***" + key[len(key)-4:]
}

func printHelp() {
	fmt.Println(`
可用命令:
  help       - 显示帮助
  config     - 查看配置
  status     - 会话状态
  skills     - 列出可用 skills
  /<skill>   - 使用 skill (如 /office-hours)
  list       - 列出文件
  glob <p>   - 搜索文件
  grep <p>   - 搜索内容
  read <f>   - 读取文件
  edit <f> <c> - 编辑文件
  !<cmd>     - 执行命令
  exit       - 退出
`)
}

func loadRules(sess *session.Session, workspace string) {
	rulesFiles := []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"}
	for _, rf := range rulesFiles {
		path := filepath.Join(workspace, rf)
		if data, err := os.ReadFile(path); err == nil {
			sess.AddSystemMessage(fmt.Sprintf("规则文件 %s:\n%s", rf, string(data)))
		}
	}
}

func toModelMessages(msgs []session.Message) []model.Message {
	var result []model.Message
	for _, m := range msgs {
		result = append(result, model.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func executeTool(name, args string, ws *workspace.Workspace, editEngine *edit.Edit, execEngine *execx.Executor) string {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("参数解析错误: %v", err)
	}

	switch name {
	case "read_file":
		path, _ := params["path"].(string)
		if path == "" {
			return "错误: 缺少 path 参数"
		}
		content, err := ws.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("读取错误: %v", err)
		}
		return content

	case "read_file_lines":
		path, _ := params["path"].(string)
		start, _ := params["start"].(float64)
		end, _ := params["end"].(float64)
		if path == "" {
			return "错误: 缺少 path 参数"
		}
		content, err := ws.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("读取错误: %v", err)
		}
		lines := strings.Split(content, "\n")
		startLine := int(start)
		if startLine < 1 {
			startLine = 1
		}
		endLine := int(end)
		if endLine > len(lines) {
			endLine = len(lines)
		}
		if endLine < startLine {
			endLine = startLine + 100
		}
		var result []string
		for i := startLine - 1; i < endLine && i < len(lines); i++ {
			result = append(result, fmt.Sprintf("%d: %s", i+1, lines[i]))
		}
		return strings.Join(result, "\n")

	case "list_files":
		dir, _ := params["dir"].(string)
		files, err := ws.ListFiles()
		if err != nil {
			return fmt.Sprintf("列出文件错误: %v", err)
		}
		if dir != "" {
			var filtered []string
			for _, f := range files {
				if strings.HasPrefix(f, dir) {
					filtered = append(filtered, f)
				}
			}
			files = filtered
		}
		return strings.Join(files, "\n")

	case "glob":
		pattern, _ := params["pattern"].(string)
		if pattern == "" {
			return "错误: 缺少 pattern 参数"
		}
		files, err := ws.GlobFiles(pattern)
		if err != nil {
			return fmt.Sprintf("搜索错误: %v", err)
		}
		return strings.Join(files, "\n")

	case "grep":
		pattern, _ := params["pattern"].(string)
		if pattern == "" {
			return "错误: 缺少 pattern 参数"
		}
		results, err := ws.GrepFiles(pattern)
		if err != nil {
			return fmt.Sprintf("搜索错误: %v", err)
		}
		return strings.Join(results[:min(50, len(results))], "\n")

	case "find_functions":
		pattern, _ := params["pattern"].(string)
		if pattern == "" {
			return "错误: 缺少 pattern 参数"
		}
		results, err := ws.GrepFiles(pattern)
		if err != nil {
			return fmt.Sprintf("搜索错误: %v", err)
		}
		var funcs []string
		for _, r := range results {
			if strings.Contains(r, "func ") || strings.Contains(r, "def ") || strings.Contains(r, "function ") {
				funcs = append(funcs, r)
			}
		}
		if len(funcs) == 0 {
			return "未找到函数定义"
		}
		return strings.Join(funcs, "\n")

	case "write_file":
		path, _ := params["path"].(string)
		content, _ := params["content"].(string)
		if path == "" {
			return "错误: 缺少 path 参数"
		}
		err := ws.WriteFile(path, content)
		if err != nil {
			return fmt.Sprintf("写入错误: %v", err)
		}
		return "文件已写入: " + path

	case "edit_file":
		path, _ := params["path"].(string)
		oldContent, _ := params["old_content"].(string)
		newContent, _ := params["new_content"].(string)
		if path == "" || oldContent == "" {
			return "错误: 缺少 path 或 old_content 参数"
		}
		currentContent, err := ws.ReadFile(path)
		if err != nil {
			return fmt.Sprintf("读取错误: %v", err)
		}
		newFileContent := strings.Replace(currentContent, oldContent, newContent, 1)
		if newFileContent == currentContent {
			return "错误: 未找到要替换的内容"
		}
		err = ws.WriteFile(path, newFileContent)
		if err != nil {
			return fmt.Sprintf("写入错误: %v", err)
		}
		return "文件已修改: " + path

	case "create_directory":
		path, _ := params["path"].(string)
		if path == "" {
			return "错误: 缺少 path 参数"
		}
		fullPath := ws.ResolvePath(path)
		err := os.MkdirAll(fullPath, 0755)
		if err != nil {
			return fmt.Sprintf("创建目录错误: %v", err)
		}
		return "目录已创建: " + path

	case "delete_file":
		path, _ := params["path"].(string)
		if path == "" {
			return "错误: 缺少 path 参数"
		}
		fullPath := ws.ResolvePath(path)
		err := os.Remove(fullPath)
		if err != nil {
			return fmt.Sprintf("删除错误: %v", err)
		}
		return "已删除: " + path

	case "execute_command":
		cmd, _ := params["command"].(string)
		timeout, _ := params["timeout"].(float64)
		if cmd == "" {
			return "错误: 缺少 command 参数"
		}
		if execEngine.IsDangerous(cmd) {
			return "错误: 危险命令被拦截: " + cmd
		}
		if timeout > 0 {
			execEngine.SetTimeout(int(timeout))
		}
		result, err := execEngine.Run(cmd)
		if err != nil {
			return fmt.Sprintf("执行错误: %v", err)
		}
		if result.TimedOut {
			return "命令执行超时: " + cmd
		}
		output := result.Output
		if len(output) > 5000 {
			output = output[:5000] + "\n...(输出已截断)"
		}
		return output

	case "get_file_info":
		path, _ := params["path"].(string)
		if path == "" {
			return "错误: 缺少 path 参数"
		}
		fullPath := ws.ResolvePath(path)
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Sprintf("获取信息错误: %v", err)
		}
		return fmt.Sprintf("名称: %s\n大小: %d 字节\n修改时间: %s\n是目录: %v",
			info.Name(), info.Size(), info.ModTime().Format("2006-01-02 15:04:05"), info.IsDir())

	case "get_project_structure":
		files, err := ws.ListFiles()
		if err != nil {
			return fmt.Sprintf("获取结构错误: %v", err)
		}
		var tree []string
		tree = append(tree, "项目结构:")

		// 按目录分组
		dirs := make(map[string][]string)
		for _, f := range files {
			parts := strings.Split(f, string(os.PathSeparator))
			if len(parts) > 1 {
				dirs[parts[0]] = append(dirs[parts[0]], f)
			} else {
				tree = append(tree, f)
			}
		}
		for dir, files := range dirs {
			tree = append(tree, fmt.Sprintf("📁 %s/", dir))
			for _, f := range files {
				if len(files) <= 10 {
					tree = append(tree, fmt.Sprintf("  ├── %s", f))
				}
			}
			if len(files) > 10 {
				tree = append(tree, fmt.Sprintf("  └── ... (%d files)", len(files)))
			}
		}
		return strings.Join(tree[:min(30, len(tree))], "\n")

	case "analyze_dependencies":
		deps := []string{}

		// 检查各种依赖文件
		depFiles := []string{"package.json", "go.mod", "Cargo.toml", "requirements.txt", "pom.xml", "Gemfile", "Pipfile"}
		for _, df := range depFiles {
			if ws.FileExists(df) {
				content, _ := ws.ReadFile(df)
				lines := strings.Split(content, "\n")
				var important []string
				for _, line := range lines {
					if strings.Contains(line, "\"") || strings.HasPrefix(line, "import") || strings.HasPrefix(line, "require") {
						important = append(important, line)
					}
				}
				if len(important) > 0 {
					deps = append(deps, fmt.Sprintf("📄 %s:", df))
					deps = append(deps, important[:min(10, len(important))]...)
				}
			}
		}
		if len(deps) == 0 {
			return "未找到依赖文件"
		}
		return strings.Join(deps, "\n")

	default:
		return fmt.Sprintf("未知工具: %s", name)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
