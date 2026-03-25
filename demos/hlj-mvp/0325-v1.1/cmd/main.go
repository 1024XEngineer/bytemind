package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	workspacePath string
	skillsDir     string
	initConfig    bool
	resetConfig   bool
	resumeTarget  string
)

type app struct {
	ws          *workspace.Workspace
	sess        *session.Session
	policy      *policy.Policy
	edit        *edit.Edit
	exec        *execx.Executor
	model       *model.Client
	skills      *skills.Manager
	activeSkill *skills.Skill
	scanner     *bufio.Scanner
	autoApprove bool
}

type turnMode int

const (
	turnModeChat turnMode = iota
	turnModeTask
)

func main() {
	parseFlags()

	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printUsage()
		os.Exit(0)
	}

	runREPL()
}

func printUsage() {
	fmt.Print(`ForgeCLI - 面向团队的终端原生 coding agent runtime

用法:
  forgecli [选项] [工作目录]

选项:
  -w, --workspace <目录>  工作目录 (默认: .)
  --model <模型>         模型名称 (默认: deepseek-chat)
  --api-base <url>       API 地址 (默认: https://api.deepseek.com)
  --timeout <秒>         命令超时 (默认: 30)
  --resume <id|latest>   恢复已保存会话
  -y, --yes              自动批准计划、编辑和命令
  -i, --init             初始化或更新 API 配置
  --reset                清空现有配置
  -h, --help             显示帮助
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
		case "--resume":
			if i+1 < len(os.Args) {
				resumeTarget = os.Args[i+1]
				i++
			}
		case "-y", "--yes":
			autoApprove = true
		case "-i", "--init":
			initConfig = true
		case "--reset":
			resetConfig = true
		case "-h", "--help":
			printUsage()
			os.Exit(0)
		default:
			if !strings.HasPrefix(arg, "-") && workspacePath == "." {
				workspacePath = arg
			}
		}
	}
}

func runREPL() {
	loadConfig()

	ws, err := workspace.New(workspacePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	if !ws.IsTrusted() {
		fmt.Fprintln(os.Stderr, "错误: 工作目录不受信任，至少需要 .git、go.mod、package.json 或 .forgecli 标记。")
		os.Exit(1)
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	sess := session.New(ws.Root)
	isResumed := false
	if resumeTarget != "" {
		resumed, err := loadSession(ws.Root, resumeTarget)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
		sess = resumed
		isResumed = true
	}

	cli := &app{
		ws:          ws,
		sess:        sess,
		policy:      policy.New(ws.Root),
		edit:        edit.New(),
		exec:        execx.New(timeout, ws.Root),
		model:       model.NewClient(apiKey, modelName, apiBase),
		skills:      skills.NewManager(skillsDir),
		scanner:     scanner,
		autoApprove: autoApprove,
	}

	if err := cli.skills.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "警告: skills 加载失败: %v\n", err)
	}

	if !isResumed {
		loadRules(cli.sess, ws.Root)
	}

	cli.printBanner(isResumed)
	cli.saveSession()

	for {
		fmt.Print(aPrompt(cli))
		if !scanner.Scan() {
			fmt.Println()
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if !cli.handleInput(input) {
			break
		}

		cli.saveSession()
	}
}

func (a *app) printBanner(isResumed bool) {
	fmt.Printf("=== ForgeCLI #%s ===\n", a.sess.ID[:8])
	fmt.Printf("工作目录: %s\n", a.ws.Root)
	fmt.Printf("模型: %s\n", modelName)
	if count := len(a.skills.List()); count > 0 {
		fmt.Printf("已加载技能：%d 个\n", count)
	}
	if isResumed {
		fmt.Println("会话: 已恢复")
	}
	if !a.model.IsConfigured() {
		fmt.Println("提示: 未配置 API Key，当前可使用本地读取、编辑和命令功能。")
	}
	fmt.Println("快捷入口：")
	fmt.Println("  查看技能：输入 skills")
	fmt.Println("  查看工具与命令：输入 help")
	fmt.Println("  清除当前技能：输入 /clear-skill")
}

func (a *app) handleInput(input string) bool {
	switch {
	case input == "exit" || input == "quit":
		return false
	case input == "help":
		printHelp()
	case input == "config":
		handleConfig()
	case input == "status":
		a.printStatus()
	case input == "sessions":
		a.printSessions()
	case strings.HasPrefix(input, "resume"):
		a.handleResume(input)
	case input == "skills":
		fmt.Print(a.skills.PrintHelp())
	case strings.HasPrefix(input, "/"):
		a.handleSkillCommand(input)
	case input == "list":
		a.printList("")
	case strings.HasPrefix(input, "glob "):
		a.printGlob(strings.TrimSpace(input[5:]))
	case strings.HasPrefix(input, "grep "):
		a.printGrep(strings.TrimSpace(input[5:]))
	case strings.HasPrefix(input, "read "):
		fmt.Println(a.readFile(strings.TrimSpace(input[5:]), "manual_read"))
	case strings.HasPrefix(input, "edit "):
		a.handleManualEdit(strings.TrimSpace(input[5:]))
	case strings.HasPrefix(input, "!"):
		fmt.Println(a.executeCommand(strings.TrimSpace(input[1:]), "manual_command"))
	default:
		a.handleModelTask(input)
	}

	return true
}

func (a *app) printStatus() {
	fmt.Printf("会话: %s\n", a.sess.ID)
	fmt.Printf("消息数: %d\n", len(a.sess.GetMessages()))
	fmt.Printf("事件数: %d\n", len(a.sess.GetEvents()))
	if a.activeSkill != nil {
		fmt.Printf("当前技能: /%s\n", a.activeSkill.Name)
		fmt.Println("退出技能: /clear-skill")
	} else {
		fmt.Println("当前技能: （无）")
	}
	fmt.Printf("最近更新时间: %s\n", a.sess.UpdatedAt.Format("2006-01-02 15:04:05"))
}

func (a *app) printSessions() {
	summaries, err := session.List(a.ws.Root)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	if len(summaries) == 0 {
		fmt.Println("没有已保存会话。")
		return
	}

	for _, item := range summaries {
		fmt.Printf("%s  更新时间=%s  消息=%d  事件=%d\n",
			item.ID,
			item.UpdatedAt.Format("2006-01-02 15:04:05"),
			item.Messages,
			item.Events,
		)
	}
}

func (a *app) handleResume(input string) {
	fields := strings.Fields(input)
	target := "latest"
	if len(fields) > 1 {
		target = fields[1]
	}

	resumed, err := loadSession(a.ws.Root, target)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	a.sess = resumed
	fmt.Printf("已恢复会话 %s\n", a.sess.ID)
	a.recordAction("resume_session", a.sess.ID, true, true, "session resumed")
}

func (a *app) handleSkillCommand(input string) {
	command, skill, args, ok := parseSkillCommand(input, a.skills)
	if !ok {
		fmt.Printf("未知技能: %s\n", input)
		fmt.Print(a.skills.PrintHelp())
		return
	}

	switch command {
	case "clear":
		a.activeSkill = nil
		fmt.Println("已退出当前技能。")
		return
	case "activate":
		a.activeSkill = skill
		fmt.Printf("已进入技能 /%s。\n", skill.Name)
		if skill.Description != "" {
			fmt.Printf("%s\n", skill.Description)
		}
		fmt.Println("后续输入会默认带上这个技能上下文。")
		fmt.Println("退出技能：输入 /clear-skill")
		return
	case "run":
		previousSkill := a.activeSkill
		a.activeSkill = skill
		fmt.Printf("使用技能 /%s 执行任务。\n", skill.Name)
		a.handleModelTask(args)
		a.activeSkill = previousSkill
		return
	}
}

func (a *app) handleManualEdit(arg string) {
	parts := strings.SplitN(arg, " ", 2)
	if len(parts) < 2 {
		fmt.Println("用法: edit <filename> <完整内容>")
		return
	}

	fmt.Println(a.writeFile(parts[0], parts[1], "manual_edit"))
}

func (a *app) printList(dir string) {
	files, err := a.ws.ListFiles()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	if dir != "" {
		files = filterByPrefix(files, dir)
	}

	for _, file := range files {
		fmt.Println(file)
	}
}

func (a *app) printGlob(pattern string) {
	files, err := a.ws.GlobFiles(pattern)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	for _, file := range files {
		fmt.Println(file)
	}
}

func (a *app) printGrep(pattern string) {
	results, err := a.ws.GrepFiles(pattern)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	for _, result := range results {
		fmt.Println(result)
	}
}

func (a *app) handleModelTask(input string) {
	a.sess.AddUserMessage(input)

	if !a.model.IsConfigured() {
		fmt.Println("未配置 API Key，无法调用模型。")
		return
	}

	if !a.isReviewRequest(input) && detectTurnMode(input) == turnModeChat {
		a.handleChatTurn()
		return
	}

	planInstructions := []string{
		"Before using any tools, write a concise execution plan. " +
			"List files you expect to inspect or edit, any commands you may run, and what user approvals will be required. " +
			"Do not call tools in this step.",
		"Write the plan as short bullets. Do not use code fences.",
	}
	if a.isReadOnlyReviewMode(input) {
		planInstructions = append(planInstructions,
			"This is a read-only code review. Plan to inspect files with read-only tools only.",
			"Do not plan shell commands or workspace changes unless the user explicitly asked for them.",
		)
	}
	planMessages := a.buildModelMessages(planInstructions...)

	planResp, err := a.model.Chat(planMessages, nil)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	plan := strings.TrimSpace(planResp.Content)
	if plan == "" {
		plan = "模型没有返回计划。"
	}
	plan = sanitizePlanOutput(plan)

	fmt.Println("\n计划：")
	fmt.Println(plan)
	a.sess.AddAssistantMessage(plan)

	approved := a.approve("是否执行这个计划?", "")
	a.recordAction("plan", "model_task", approved, approved, truncate(plan, 400))
	if !approved {
		fmt.Println("计划已取消。")
		return
	}

	a.sess.AddUserMessage("Plan approved. Execute carefully, request approvals through the tool responses, and finish with a concise summary.")

	extraSystem := []string{
		"You are executing an approved task inside ForgeCLI.",
		"If the user asks to create, modify, or delete files, you must use tools to perform the change.",
		"Never claim a file was created, modified, deleted, or a command was run unless the corresponding tool succeeded in this conversation.",
		"If the task requires workspace changes, use write_file, edit_file, create_directory, delete_file, or execute_command instead of only describing what you would do.",
		"Do not paste large raw file contents into the final answer unless the user explicitly asks for them.",
	}
	if a.isReadOnlyReviewMode(input) {
		extraSystem = append(extraSystem,
			"This request is a read-only code review.",
			"Use only read-only inspection tools. Do not run shell commands and do not modify workspace files.",
		)
	}
	if a.isReviewRequest(input) {
		extraSystem = append(extraSystem,
			"This is a code review request. Focus on bugs, regressions, risks, and missing tests.",
			"Findings come first. Keep the summary brief and do not dump raw code unless the user explicitly asks for it.",
		)
	}
	messages := a.buildModelMessages(extraSystem...)
	requiresMutation := taskRequiresMutation(input)
	tools := a.selectToolsForTask(input)
	forcedToolRetry := false
	totalToolCalls := 0
	emptyReadOnlyResults := 0
	backgroundThinkingShown := false
	for iter := 0; iter < 10; iter++ {
		resp, err := a.model.Chat(messages, tools)
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}

		if len(resp.ToolCalls) > 0 {
			messages = append(messages, model.Message{
				Role:      "assistant",
				ToolCalls: resp.ToolCalls,
			})

			for _, toolCall := range resp.ToolCalls {
				totalToolCalls++
				visibleTool := shouldDisplayToolTrace(toolCall.Function.Name)
				if visibleTool {
					fmt.Printf("\n[tool] %s\n", toolCall.Function.Name)
				} else if !backgroundThinkingShown {
					fmt.Println("\n思考中...")
					backgroundThinkingShown = true
				}

				result := a.executeTool(toolCall.Function.Name, toolCall.Function.Arguments)
				if visibleTool {
					fmt.Printf("%s\n", renderToolResultPreview(toolCall.Function.Name, toolCall.Function.Arguments, result))
				}
				if isUserRejectedToolResult(toolCall.Function.Name, result) {
					fmt.Println("本轮任务已停止，因为你拒绝了这一步操作。")
					a.sess.AddAssistantMessage("任务已停止：用户拒绝了待审批的工具操作。")
					return
				}
				if isLowSignalReadOnlyResult(toolCall.Function.Name, result) {
					emptyReadOnlyResults++
				} else {
					emptyReadOnlyResults = 0
				}
				messages = append(messages, model.Message{
					Role:       "tool",
					ToolCallID: toolCall.ID,
					Content:    truncate(result, 4000),
				})
			}

			if a.shouldForceToolSummary(input, totalToolCalls, emptyReadOnlyResults) {
				a.finishWithForcedSummary(messages, input)
				return
			}
			continue
		}

		if strings.TrimSpace(resp.Content) != "" {
			if requiresMutation && !forcedToolRetry && shouldRetryForToolUse(resp.Content) {
				forcedToolRetry = true
				messages = append(messages, model.Message{
					Role: "system",
					Content: "The approved task requires workspace changes. " +
						"You have write tools available. Use write_file, edit_file, create_directory, delete_file, or execute_command now. " +
						"Do not say you cannot create or modify files unless a tool actually failed.",
				})
				continue
			}

			finalContent := a.reviewFinalResponse(input, resp.Content)
			fmt.Println(finalContent)
			a.sess.AddAssistantMessage(finalContent)
		}
		return
	}

	fmt.Println("达到最大工具迭代次数，已停止。")
}

func (a *app) handleChatTurn() {
	messages := a.buildModelMessages(
		"Respond directly and naturally.",
		"Do not produce an execution plan unless the user explicitly asks for one.",
		"Do not call tools unless the user is clearly asking you to inspect or modify the workspace.",
	)

	resp, err := a.model.Chat(messages, nil)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	if strings.TrimSpace(resp.Content) == "" {
		fmt.Println("模型没有返回内容。")
		return
	}

	finalContent := a.reviewFinalResponse(a.latestUserRequest(), resp.Content)
	fmt.Println(finalContent)
	a.sess.AddAssistantMessage(finalContent)
}

func (a *app) readFile(path, source string) string {
	resolved, displayPath, err := a.resolvePath(path)
	if err != nil {
		a.recordAction(source, path, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	if err := a.policy.CheckRead(displayPath); err != nil {
		a.recordAction(source, displayPath, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		a.recordAction(source, displayPath, true, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	a.recordAction(source, displayPath, true, true, "read file")
	return string(data)
}

func (a *app) readFileLines(path string, start, end int) string {
	content := a.readFile(path, "read_file_lines")
	if strings.HasPrefix(content, "错误:") {
		return content
	}

	lines := strings.Split(content, "\n")
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start + 99
	}
	if end > len(lines) {
		end = len(lines)
	}

	var result []string
	for i := start - 1; i < end && i < len(lines); i++ {
		result = append(result, fmt.Sprintf("%d: %s", i+1, lines[i]))
	}

	return strings.Join(result, "\n")
}

func (a *app) writeFile(path, content, source string) string {
	resolved, displayPath, err := a.resolvePath(path)
	if err != nil {
		a.recordAction(source, path, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	if err := a.policy.CheckWrite(displayPath); err != nil {
		a.recordAction(source, displayPath, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	preview, err := a.edit.Prepare(resolved, content)
	if err != nil {
		a.recordAction(source, displayPath, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	if preview.OldHash == preview.NewHash {
		a.recordAction(source, displayPath, true, true, "no changes")
		return fmt.Sprintf("未检测到变更: %s", displayPath)
	}

	fmt.Println(preview.Diff)
	approved := a.approve(fmt.Sprintf("是否将改动写入 %s?", displayPath), "")
	if !approved {
		a.recordAction(source, displayPath, false, false, "user rejected")
		return "已取消写入。"
	}

	result, err := a.edit.Edit(resolved, content, preview.OldHash)
	if err != nil {
		a.recordAction(source, displayPath, true, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	details := fmt.Sprintf("old_hash=%s new_hash=%s", shortHash(result.OldHash), shortHash(result.NewHash))
	a.recordAction(source, displayPath, true, true, details)
	return fmt.Sprintf("已更新 %s", displayPath)
}

func (a *app) replaceFile(path, oldContent, newContent, source string) string {
	current := a.readFile(path, source+"_read")
	if strings.HasPrefix(current, "错误:") {
		return current
	}

	if !strings.Contains(current, oldContent) {
		return "错误: 未找到要替换的内容"
	}

	updated := strings.Replace(current, oldContent, newContent, 1)
	return a.writeFile(path, updated, source)
}

func (a *app) createDirectory(path, source string) string {
	_, displayPath, err := a.resolvePath(path)
	if err != nil {
		a.recordAction(source, path, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	if err := a.policy.CheckWrite(displayPath); err != nil {
		a.recordAction(source, displayPath, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	approved := a.approve(fmt.Sprintf("是否创建目录 %s?", displayPath), fmt.Sprintf("mkdir %s", displayPath))
	if !approved {
		a.recordAction(source, displayPath, false, false, "user rejected")
		return "已取消创建目录。"
	}

	if err := a.ws.CreateDirectory(path); err != nil {
		a.recordAction(source, displayPath, true, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	a.recordAction(source, displayPath, true, true, "directory created")
	return fmt.Sprintf("已创建目录 %s", displayPath)
}

func (a *app) deletePath(path, source string) string {
	resolved, displayPath, err := a.resolvePath(path)
	if err != nil {
		a.recordAction(source, path, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	if err := a.policy.CheckDelete(displayPath); err != nil {
		a.recordAction(source, displayPath, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		a.recordAction(source, displayPath, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	preview := fmt.Sprintf("delete %s (dir=%t size=%d)", displayPath, info.IsDir(), info.Size())
	approved := a.approve(fmt.Sprintf("是否删除 %s?", displayPath), preview)
	if !approved {
		a.recordAction(source, displayPath, false, false, "user rejected")
		return "已取消删除。"
	}

	if err := a.ws.Remove(path); err != nil {
		a.recordAction(source, displayPath, true, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	a.recordAction(source, displayPath, true, true, "path deleted")
	return fmt.Sprintf("已删除 %s", displayPath)
}

func (a *app) executeCommand(cmd, source string) string {
	if strings.TrimSpace(cmd) == "" {
		return "错误: command 不能为空"
	}

	if err := a.policy.CheckExec(cmd); err != nil {
		a.recordAction(source, cmd, false, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	preview := fmt.Sprintf("command: %s\nworkdir: %s\napproval: required", cmd, a.ws.Root)
	approved := a.approve("是否执行命令?", preview)
	if !approved {
		a.recordAction(source, cmd, false, false, "user rejected")
		return "已取消执行命令。"
	}

	result, err := a.exec.Run(cmd)
	if err != nil {
		a.recordAction(source, cmd, true, false, err.Error())
		return fmt.Sprintf("错误: %v", err)
	}

	success := !result.TimedOut && result.ExitCode == 0
	details := fmt.Sprintf("exit=%d duration=%s timeout=%t", result.ExitCode, result.Duration.Round(time.Millisecond), result.TimedOut)
	a.recordAction(source, cmd, true, success, details)

	return formatExecResult(result)
}

func (a *app) executeTool(name, args string) string {
	params := map[string]interface{}{}
	if strings.TrimSpace(args) != "" {
		if err := json.Unmarshal([]byte(args), &params); err != nil {
			return fmt.Sprintf("参数解析错误: %v", err)
		}
	}

	switch name {
	case "read_file":
		path, _ := params["path"].(string)
		return a.readFile(path, "read_file")
	case "read_file_lines":
		path, _ := params["path"].(string)
		start, _ := params["start"].(float64)
		end, _ := params["end"].(float64)
		return a.readFileLines(path, int(start), int(end))
	case "list_files":
		dir, _ := params["dir"].(string)
		files, err := a.ws.ListFiles()
		if err != nil {
			return fmt.Sprintf("错误: %v", err)
		}
		if dir != "" {
			files = filterByPrefix(files, dir)
		}
		return strings.Join(files, "\n")
	case "glob":
		pattern, _ := params["pattern"].(string)
		files, err := a.ws.GlobFiles(pattern)
		if err != nil {
			return fmt.Sprintf("错误: %v", err)
		}
		return strings.Join(files, "\n")
	case "grep":
		pattern, _ := params["pattern"].(string)
		results, err := a.ws.GrepFiles(pattern)
		if err != nil {
			return fmt.Sprintf("错误: %v", err)
		}
		return strings.Join(results[:min(len(results), 50)], "\n")
	case "find_functions":
		pattern, _ := params["pattern"].(string)
		results, err := a.ws.GrepFiles(pattern)
		if err != nil {
			return fmt.Sprintf("错误: %v", err)
		}
		var matches []string
		for _, item := range results {
			if strings.Contains(item, "func ") || strings.Contains(item, "def ") || strings.Contains(item, "function ") {
				matches = append(matches, item)
			}
		}
		if len(matches) == 0 {
			return "未找到函数定义"
		}
		return strings.Join(matches, "\n")
	case "write_file":
		path, _ := params["path"].(string)
		content, _ := params["content"].(string)
		return a.writeFile(path, content, "write_file")
	case "edit_file":
		path, _ := params["path"].(string)
		oldContent, _ := params["old_content"].(string)
		newContent, _ := params["new_content"].(string)
		return a.replaceFile(path, oldContent, newContent, "edit_file")
	case "create_directory":
		path, _ := params["path"].(string)
		return a.createDirectory(path, "create_directory")
	case "delete_file":
		path, _ := params["path"].(string)
		return a.deletePath(path, "delete_file")
	case "execute_command":
		cmd, _ := params["command"].(string)
		timeoutSeconds, _ := params["timeout"].(float64)
		previousTimeout := a.exec.Timeout()
		if timeoutSeconds > 0 {
			a.exec.SetTimeout(int(timeoutSeconds))
		}
		result := a.executeCommand(cmd, "execute_command")
		a.exec.SetTimeout(int(previousTimeout.Seconds()))
		return result
	case "get_file_info":
		path, _ := params["path"].(string)
		resolved, displayPath, err := a.resolvePath(path)
		if err != nil {
			return fmt.Sprintf("错误: %v", err)
		}
		if err := a.policy.CheckRead(displayPath); err != nil {
			return fmt.Sprintf("错误: %v", err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return fmt.Sprintf("错误: %v", err)
		}
		return fmt.Sprintf("名称: %s\n大小: %d bytes\n修改时间: %s\n目录: %t",
			info.Name(),
			info.Size(),
			info.ModTime().Format("2006-01-02 15:04:05"),
			info.IsDir(),
		)
	case "get_project_structure":
		files, err := a.ws.ListFiles()
		if err != nil {
			return fmt.Sprintf("错误: %v", err)
		}
		return buildProjectStructure(files)
	case "analyze_dependencies":
		return a.analyzeDependencies()
	default:
		return fmt.Sprintf("未知工具: %s", name)
	}
}

func (a *app) analyzeDependencies() string {
	dependencyFiles := []string{"package.json", "go.mod", "Cargo.toml", "requirements.txt", "pom.xml", "Gemfile", "Pipfile"}
	var blocks []string

	for _, file := range dependencyFiles {
		if !a.ws.FileExists(file) {
			continue
		}

		content, err := a.ws.ReadFile(file)
		if err != nil {
			continue
		}

		lines := strings.Split(content, "\n")
		var important []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.Contains(trimmed, "\"") || strings.HasPrefix(trimmed, "require") || strings.HasPrefix(trimmed, "import") || strings.HasPrefix(trimmed, "module ") {
				important = append(important, trimmed)
			}
		}

		if len(important) == 0 {
			continue
		}

		block := []string{file + ":"}
		block = append(block, important[:min(len(important), 10)]...)
		blocks = append(blocks, strings.Join(block, "\n"))
	}

	if len(blocks) == 0 {
		return "未找到依赖文件"
	}

	return strings.Join(blocks, "\n\n")
}

func (a *app) resolvePath(path string) (string, string, error) {
	resolved, err := a.ws.ResolvePath(path)
	if err != nil {
		return "", "", err
	}

	displayPath, err := filepath.Rel(a.ws.Root, resolved)
	if err != nil || displayPath == "." {
		displayPath = resolved
	}

	return resolved, filepath.Clean(displayPath), nil
}

func (a *app) approve(prompt, preview string) bool {
	if preview != "" {
		fmt.Println(preview)
	}

	if a.autoApprove {
		fmt.Println("自动批准。")
		return true
	}

	fmt.Printf("%s (y/n): ", prompt)
	if !a.scanner.Scan() {
		return false
	}

	answer := strings.ToLower(strings.TrimSpace(a.scanner.Text()))
	return answer == "y" || answer == "yes"
}

func (a *app) recordAction(action, target string, approved, success bool, details string) {
	a.sess.AddEvent(action, target, details, approved, success)
	if err := a.policy.Log(a.sess.ID, action, target, approved, success, details); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 审计日志写入失败: %v\n", err)
	}
}

func (a *app) saveSession() {
	if err := a.sess.Save(a.ws.Root); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 会话保存失败: %v\n", err)
	}
}

func loadConfig() {
	if resetConfig {
		_ = os.Remove(model.GetConfigPath())
		apiKey = ""
	}

	cfg, err := model.LoadConfig()
	if err == nil {
		if cfg.APIKey != "" {
			apiKey = cfg.APIKey
		}
		if cfg.Model != "" {
			modelName = cfg.Model
		}
		if cfg.APIBase != "" {
			apiBase = cfg.APIBase
		}
	}

	if initConfig {
		promptForConfig()
	}
}

func promptForConfig() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Printf("当前模型: %s\n", modelName)
	fmt.Print("API Key: ")
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			apiKey = input
		}
	}

	fmt.Printf("模型名 [%s]: ", modelName)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			modelName = input
		}
	}

	fmt.Printf("API Base [%s]: ", apiBase)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			apiBase = input
		}
	}

	if apiKey == "" {
		fmt.Println("未保存配置: API Key 为空。")
		return
	}

	if err := model.SaveConfig(model.Config{
		APIKey:  apiKey,
		Model:   modelName,
		APIBase: apiBase,
	}); err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Println("配置已保存。")
}

func handleConfig() {
	fmt.Println("\n=== 配置 ===")
	fmt.Printf("API Key: %s\n", maskKey(apiKey))
	fmt.Printf("模型: %s\n", modelName)
	fmt.Printf("API: %s\n", apiBase)
	fmt.Printf("配置文件: %s\n", model.GetConfigPath())
}

func maskKey(key string) string {
	if len(key) <= 8 {
		if key == "" {
			return "(not set)"
		}
		return "***"
	}

	return key[:4] + "***" + key[len(key)-4:]
}

func printHelp() {
	fmt.Print(`可用命令:
  help              显示帮助
  config            查看当前配置
  status            查看当前会话状态
  sessions          列出已保存会话
  resume [id|latest] 恢复已保存会话
  skills            列出可用技能
  /<技能名>         进入技能
  /<技能名> <任务>  使用技能直接执行任务
  /clear-skill      退出当前技能
  list              列出文件
  glob <pattern>    搜索文件
  grep <pattern>    搜索内容
  read <file>       读取文件
  edit <file> <c>   写入完整文件内容
  !<command>        执行命令
  exit              退出
`)
}

func loadRules(sess *session.Session, workspaceRoot string) {
	for _, file := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		path := filepath.Join(workspaceRoot, file)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		sess.AddSystemMessage(fmt.Sprintf("规则文件 %s:\n%s", file, string(data)))
	}
}

func loadSession(workspaceRoot, target string) (*session.Session, error) {
	if target == "" || target == "latest" {
		return session.LoadLatest(workspaceRoot)
	}

	sess := &session.Session{}
	if err := sess.Load(workspaceRoot, strings.TrimSuffix(target, ".json")); err != nil {
		return nil, err
	}

	return sess, nil
}

func (a *app) reviewFinalResponse(userRequest, draft string) string {
	trimmedDraft := strings.TrimSpace(draft)
	if trimmedDraft == "" || !a.model.IsConfigured() {
		return draft
	}

	reviewGuidance := "Review the draft response before it is shown to the user. " +
		"Rewrite only if needed. Keep the answer concise, professional, and accurate. " +
		"Do not claim commands ran or files changed unless the execution facts support it. " +
		"If the user rejected an approval or an action was not executed, say so plainly. " +
		"Do not include large raw code blocks or long file dumps unless the user explicitly asked for them. "
	if requestLooksLikeReview(userRequest) {
		reviewGuidance += "This is a code review response. Present findings first, ordered by severity when possible, and keep any summary brief. "
	}

	reviewMessages := []model.Message{
		{
			Role: "system",
			Content: "You are ForgeCLI's response QA reviewer. " +
				reviewGuidance +
				"Return only the final user-facing response text.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf(
				"User request:\n%s\n\nDraft response:\n%s\n\nRecent execution facts:\n%s",
				strings.TrimSpace(userRequest),
				trimmedDraft,
				a.recentExecutionFacts(8),
			),
		},
	}

	resp, err := a.model.Chat(reviewMessages, nil)
	if err != nil {
		return draft
	}

	reviewed := strings.TrimSpace(resp.Content)
	if reviewed == "" {
		return sanitizeFinalResponse(userRequest, draft)
	}

	return sanitizeFinalResponse(userRequest, reviewed)
}

func (a *app) recentExecutionFacts(limit int) string {
	events := a.sess.GetEvents()
	if len(events) == 0 {
		return "(no execution events)"
	}

	start := 0
	if len(events) > limit {
		start = len(events) - limit
	}

	var lines []string
	for _, event := range events[start:] {
		lines = append(lines, fmt.Sprintf(
			"%s | target=%s | approved=%t | success=%t | details=%s",
			event.Type,
			event.Target,
			event.Approved,
			event.Success,
			strings.TrimSpace(event.Details),
		))
	}

	return strings.Join(lines, "\n")
}

func (a *app) latestUserRequest() string {
	messages := a.sess.GetMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}

	return ""
}

func (a *app) buildModelMessages(extraSystem ...string) []model.Message {
	var messages []model.Message

	if a.activeSkill != nil {
		messages = append(messages, model.Message{
			Role:    "system",
			Content: skillSystemPrompt(a.activeSkill),
		})
	}

	for _, item := range extraSystem {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		messages = append(messages, model.Message{
			Role:    "system",
			Content: trimmed,
		})
	}

	messages = append(messages, toModelMessages(a.sess.GetMessages())...)
	return messages
}

func toModelMessages(messages []session.Message) []model.Message {
	result := make([]model.Message, 0, len(messages))
	for _, item := range messages {
		result = append(result, model.Message{
			Role:    item.Role,
			Content: item.Content,
		})
	}
	return result
}

func filterByPrefix(files []string, dir string) []string {
	normalized := filepath.Clean(strings.TrimSpace(dir))
	if normalized == "" || normalized == "." || normalized == string(os.PathSeparator) {
		return append([]string(nil), files...)
	}

	var filtered []string
	prefix := normalized + string(os.PathSeparator)
	for _, file := range files {
		cleaned := filepath.Clean(file)
		if cleaned == normalized || strings.HasPrefix(cleaned, prefix) {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

func buildProjectStructure(files []string) string {
	rootFiles := []string{}
	dirCounts := map[string]int{}

	for _, file := range files {
		parts := strings.Split(file, string(os.PathSeparator))
		if len(parts) == 1 {
			rootFiles = append(rootFiles, file)
			continue
		}
		dirCounts[parts[0]]++
	}

	sort.Strings(rootFiles)
	dirs := make([]string, 0, len(dirCounts))
	for dir := range dirCounts {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	lines := []string{"项目结构:"}
	lines = append(lines, rootFiles...)
	for _, dir := range dirs {
		lines = append(lines, fmt.Sprintf("%s/ (%d files)", dir, dirCounts[dir]))
	}

	return strings.Join(lines, "\n")
}

func formatExecResult(result *execx.ExecResult) string {
	output := strings.TrimSpace(result.Output)
	if len(output) > 5000 {
		output = output[:5000] + "\n...(output truncated)"
	}
	if output == "" {
		output = "(no output)"
	}

	return fmt.Sprintf("exit=%d duration=%s timeout=%t\n%s",
		result.ExitCode,
		result.Duration.Round(time.Millisecond),
		result.TimedOut,
		output,
	)
}

func shortHash(value string) string {
	if value == "" {
		return "(new)"
	}
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
}

func sanitizePlanOutput(plan string) string {
	cleaned := strings.TrimSpace(stripLargeCodeBlocks(plan, true))
	if cleaned == "" {
		return "1. 检查相关文件\n2. 提炼主要问题\n3. 输出简洁结论"
	}

	lines := compactLines(cleaned)
	if len(lines) > 8 {
		lines = append(lines[:8], "...")
	}

	result := strings.Join(lines, "\n")
	if len(result) > 900 {
		result = truncatePreviewLine(result, 900)
	}

	return strings.TrimSpace(result)
}

func sanitizeFinalResponse(userRequest, response string) string {
	if strings.TrimSpace(response) == "" {
		return response
	}

	if requestWantsRawCode(userRequest) {
		return strings.TrimSpace(response)
	}

	cleaned := stripLargeCodeBlocks(response, true)
	cleaned = stripLargePlainCodeBlocks(cleaned)
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return "已省略大段源码输出。如需查看完整代码，请明确要求贴出代码，或直接使用 `read <file>`。"
	}

	lines := compactLines(cleaned)
	if len(lines) > 40 {
		lines = append(lines[:40], "...")
	}

	result := strings.Join(lines, "\n")
	if len(result) > 2200 {
		result = truncatePreviewLine(result, 2200)
	}

	return strings.TrimSpace(result)
}

func stripLargeCodeBlocks(text string, alwaysReplace bool) string {
	lines := strings.Split(text, "\n")
	var out []string
	inFence := false
	var fenceLines []string

	flushFence := func() {
		if len(fenceLines) == 0 {
			return
		}

		lineCount := len(fenceLines)
		block := strings.Join(fenceLines, "\n")
		if alwaysReplace || lineCount > 8 || len(block) > 400 {
			out = append(out, fmt.Sprintf("[已省略 %d 行代码/原文块]", lineCount))
		} else {
			out = append(out, fenceLines...)
		}
		fenceLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				flushFence()
				inFence = false
			} else {
				inFence = true
			}
			continue
		}

		if inFence {
			fenceLines = append(fenceLines, line)
			continue
		}

		out = append(out, line)
	}

	if inFence {
		flushFence()
	}

	return strings.Join(out, "\n")
}

func stripLargePlainCodeBlocks(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	var block []string

	flushBlock := func() {
		if len(block) == 0 {
			return
		}

		if len(block) >= 4 {
			out = append(out, fmt.Sprintf("[已省略 %d 行代码/原文块]", len(block)))
		} else {
			out = append(out, block...)
		}
		block = nil
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" && len(block) > 0 {
			block = append(block, line)
			continue
		}

		if isCodeLikeLine(line) {
			block = append(block, line)
			continue
		}

		flushBlock()
		out = append(out, line)
	}

	flushBlock()
	return strings.Join(out, "\n")
}

func compactLines(text string) []string {
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	lastBlank := false

	for _, line := range rawLines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmed) == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			lines = append(lines, "")
			continue
		}

		lastBlank = false
		lines = append(lines, trimmed)
	}

	return lines
}

func isCodeLikeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	prefixes := []string{
		"def ", "class ", "func ", "package ", "import ", "from ", "return ", "if ", "elif ", "else:",
		"for ", "while ", "switch ", "case ", "try:", "except", "finally:", "public ", "private ",
		"const ", "var ", "let ", "type ", "#include", "//", "/*", "* ", "*/",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}

	if strings.Contains(trimmed, " = ") || strings.Contains(trimmed, " := ") {
		return true
	}

	if strings.ContainsAny(trimmed, "{}();") {
		return true
	}

	if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
		return true
	}

	return false
}

func requestWantsRawCode(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}

	hints := []string{
		"完整代码", "完整内容", "贴代码", "贴出代码", "源码", "原文", "逐行", "全文",
		"show code", "paste code", "raw code", "full file", "verbatim", "entire file",
	}

	for _, hint := range hints {
		if strings.Contains(normalized, hint) {
			return true
		}
	}

	return false
}

func renderToolResultPreview(toolName, args, result string) string {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return renderEmptyToolResult(toolName, args)
	}

	if strings.HasPrefix(trimmed, "错误:") || isUserRejectedToolResult(toolName, trimmed) {
		return truncate(trimmed, 600)
	}

	params := parseToolArgs(args)

	switch toolName {
	case "read_file":
		path := stringParam(params, "path")
		return fmt.Sprintf("已读取 %s（%d 行，%d 字节，内容已发送给模型）", fallbackText(path, "(unknown file)"), countLines(trimmed), len(result))
	case "read_file_lines":
		path := stringParam(params, "path")
		start := intParam(params, "start")
		end := intParam(params, "end")
		lineCount := countLines(trimmed)
		if start <= 0 {
			start = 1
		}
		if end <= 0 {
			end = start + lineCount - 1
		}
		return fmt.Sprintf("已读取 %s 第 %d-%d 行（%d 行，内容已发送给模型）", fallbackText(path, "(unknown file)"), start, end, lineCount)
	case "list_files", "glob", "grep", "find_functions":
		items := nonEmptyLines(trimmed)
		preview := previewList(items, 5)
		if preview == "" {
			return fmt.Sprintf("已返回 %d 条结果。", len(items))
		}
		return fmt.Sprintf("已返回 %d 条结果，预览：\n%s", len(items), preview)
	case "get_project_structure":
		return fmt.Sprintf("已读取项目结构摘要（%d 行，内容已发送给模型）", countLines(trimmed))
	case "analyze_dependencies":
		return fmt.Sprintf("已分析依赖信息（%d 行，内容已发送给模型）", countLines(trimmed))
	default:
		return truncate(trimmed, 600)
	}
}

func parseToolArgs(args string) map[string]interface{} {
	params := map[string]interface{}{}
	if strings.TrimSpace(args) == "" {
		return params
	}
	_ = json.Unmarshal([]byte(args), &params)
	return params
}

func stringParam(params map[string]interface{}, key string) string {
	value, _ := params[key].(string)
	return strings.TrimSpace(value)
}

func intParam(params map[string]interface{}, key string) int {
	value, _ := params[key].(float64)
	return int(value)
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func countLines(value string) int {
	if value == "" {
		return 0
	}
	return len(strings.Split(value, "\n"))
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(value, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}
	return items
}

func previewList(items []string, limit int) string {
	if len(items) == 0 {
		return ""
	}

	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}

	preview := make([]string, 0, limit)
	for _, item := range items[:limit] {
		preview = append(preview, truncatePreviewLine(item, 140))
	}

	return strings.Join(preview, "\n")
}

func truncatePreviewLine(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}

func renderEmptyToolResult(toolName, args string) string {
	params := parseToolArgs(args)
	switch toolName {
	case "glob":
		return fmt.Sprintf("未找到匹配文件（pattern=%s）。", fallbackText(stringParam(params, "pattern"), "(empty)"))
	case "grep":
		return fmt.Sprintf("未找到匹配内容（pattern=%s）。", fallbackText(stringParam(params, "pattern"), "(empty)"))
	case "list_files":
		dir := stringParam(params, "dir")
		if dir == "" || dir == "." {
			return "当前目录下未找到可见文件。"
		}
		return fmt.Sprintf("目录 %s 下未找到可见文件。", dir)
	case "find_functions":
		return "未找到匹配函数。"
	default:
		return "(empty tool result)"
	}
}

func isLowSignalReadOnlyResult(toolName, result string) bool {
	if isMutatingTool(toolName) {
		return false
	}

	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return true
	}

	switch toolName {
	case "glob", "list_files", "grep":
		return countLines(trimmed) == 0
	case "find_functions":
		return trimmed == "未找到函数定义"
	default:
		return false
	}
}

func (a *app) shouldForceToolSummary(input string, totalToolCalls, emptyReadOnlyResults int) bool {
	if totalToolCalls >= 8 {
		return true
	}

	if a.isReadOnlyReviewMode(input) && emptyReadOnlyResults >= 3 {
		return true
	}

	return false
}

func (a *app) finishWithForcedSummary(messages []model.Message, userRequest string) {
	forcedMessages := append([]model.Message{}, messages...)
	forcedMessages = append(forcedMessages, model.Message{
		Role: "system",
		Content: "Stop calling tools now. Based on the evidence already collected, provide the best concise answer you can. " +
			"If the evidence is insufficient, say what is missing in one short sentence.",
	})

	resp, err := a.model.Chat(forcedMessages, nil)
	if err != nil {
		fmt.Println("工具探索次数过多，已停止。请尝试更具体的请求。")
		a.sess.AddAssistantMessage("工具探索次数过多，未能稳定收敛。")
		return
	}

	content := strings.TrimSpace(resp.Content)
	if content == "" {
		fmt.Println("工具探索次数过多，已停止。请尝试更具体的请求。")
		a.sess.AddAssistantMessage("工具探索次数过多，未能稳定收敛。")
		return
	}

	finalContent := a.reviewFinalResponse(userRequest, content)
	fmt.Println(finalContent)
	a.sess.AddAssistantMessage(finalContent)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseSkillCommand(input string, manager *skills.Manager) (command string, skill *skills.Skill, args string, ok bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(input, "/"))
	if trimmed == "" {
		return "", nil, "", false
	}

	if trimmed == "clear-skill" {
		return "clear", nil, "", true
	}

	parts := strings.SplitN(trimmed, " ", 2)
	skillName := parts[0]
	if !manager.Has(skillName) {
		return "", nil, "", false
	}

	var remainder string
	if len(parts) == 2 {
		remainder = strings.TrimSpace(parts[1])
	}

	if remainder == "" {
		return "activate", manager.Get(skillName), "", true
	}

	return "run", manager.Get(skillName), remainder, true
}

func skillSystemPrompt(skill *skills.Skill) string {
	if skill == nil {
		return ""
	}

	if skill.Name == "review" {
		return "当前技能：/review\n" +
			"这是 ForgeCLI 内的只读代码审查工作流。\n" +
			"请用只读工具检查相关文件，然后报告 bug、回归风险、缺失测试和设计问题。\n" +
			"不要运行 shell 命令，不要修改文件；除非用户明确要求，否则不要推断 git 状态。\n" +
			"结论保持简洁，尽量给出文件位置或行号。"
	}

	description := strings.TrimSpace(skill.Description)
	if description == "" {
		description = "把这个技能作为轻量工作流指引使用。"
	}

	return fmt.Sprintf(
		"当前技能：/%s\n技能摘要：%s\n请把它作为 ForgeCLI 内部的轻量工作流指引。忽略 bash 片段、安装步骤、遥测说明以及当前环境不可用的外部工具引用。",
		skill.Name,
		description,
	)
}

func detectTurnMode(input string) turnMode {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return turnModeChat
	}

	if requestLooksLikeReview(normalized) || looksLikeFileInspectionRequest(normalized) || mentionsLikelyFilePath(normalized) {
		return turnModeTask
	}

	if isGreeting(normalized) || isSmallTalk(normalized) {
		return turnModeChat
	}

	taskHints := []string{
		"帮我", "请帮", "分析", "查看", "搜索", "查找", "读取", "读一下", "总结", "解释代码",
		"修改", "修复", "重构", "生成", "创建", "编译", "构建", "运行", "测试", "执行",
		"项目", "仓库", "代码", "文件", "函数", "报错", "bug", "diff", "命令",
		"analyze", "inspect", "check", "search", "read", "open", "summarize", "explain",
		"modify", "fix", "refactor", "generate", "create", "build", "compile", "run",
		"test", "execute", "project", "repo", "repository", "code", "file", "function",
		"error", "bug", "diff", "command",
	}

	for _, hint := range taskHints {
		if strings.Contains(normalized, hint) {
			return turnModeTask
		}
	}

	if strings.Contains(normalized, "?") || strings.Contains(normalized, "？") {
		return turnModeChat
	}

	if countWords(normalized) <= 4 {
		return turnModeChat
	}

	return turnModeTask
}

func isGreeting(input string) bool {
	greetings := []string{
		"你好", "您好", "嗨", "hello", "hi", "hey", "yo",
	}

	for _, item := range greetings {
		if input == item {
			return true
		}
	}

	return false
}

func isSmallTalk(input string) bool {
	phrases := []string{
		"谢谢", "多谢", "ok", "okay", "好的", "明白", "收到",
		"你是谁", "你会什么", "你能做什么", "你有什么工具", "告诉我你的tool有什么",
	}

	for _, phrase := range phrases {
		if input == phrase {
			return true
		}
	}

	return false
}

func countWords(input string) int {
	fields := strings.Fields(input)
	if len(fields) > 0 {
		return len(fields)
	}

	return len([]rune(input))
}

func taskRequiresMutation(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	hints := []string{
		"创建", "生成", "新建", "写", "修改", "修复", "重构", "删除", "添加", "编写",
		"create", "generate", "write", "modify", "edit", "fix", "refactor", "delete", "add", "implement",
	}

	for _, hint := range hints {
		if strings.Contains(normalized, hint) {
			return true
		}
	}

	return false
}

func requestLooksLikeReview(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}

	hints := []string{
		"review", "code review", "审查", "审阅", "检查代码", "review代码", "review 我的代码", "review我的代码",
	}

	for _, hint := range hints {
		if strings.Contains(normalized, hint) {
			return true
		}
	}

	return false
}

func looksLikeFileInspectionRequest(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}

	verbs := []string{"看", "看看", "看下", "查看", "inspect", "check", "open", "read"}
	hasVerb := false
	for _, verb := range verbs {
		if strings.Contains(normalized, verb) {
			hasVerb = true
			break
		}
	}
	if !hasVerb {
		return false
	}

	extensions := []string{
		".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".java", ".rb", ".rs", ".cpp", ".c",
		".h", ".hpp", ".cs", ".php", ".swift", ".kt", ".md", ".json", ".yaml", ".yml", ".toml",
	}

	for _, ext := range extensions {
		if strings.Contains(normalized, ext) {
			return true
		}
	}

	return false
}

func mentionsLikelyFilePath(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}

	extensions := []string{
		".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".java", ".rb", ".rs", ".cpp", ".c",
		".h", ".hpp", ".cs", ".php", ".swift", ".kt", ".md", ".json", ".yaml", ".yml", ".toml",
	}

	for _, ext := range extensions {
		index := strings.Index(normalized, ext)
		if index <= 0 {
			continue
		}

		prefix := normalized[:index]
		last := prefix[len(prefix)-1]
		if (last >= 'a' && last <= 'z') || (last >= '0' && last <= '9') || last == '_' || last == '-' {
			return true
		}
	}

	return false
}

func requestExplicitlyAllowsCommands(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}

	hints := []string{
		"运行", "执行", "测试", "编译", "构建", "命令", "shell", "终端",
		"run ", "test", "build", "compile", "command", "shell", "terminal", "git ",
	}

	for _, hint := range hints {
		if strings.Contains(normalized, hint) {
			return true
		}
	}

	return false
}

func (a *app) isReviewRequest(input string) bool {
	if requestLooksLikeReview(input) {
		return true
	}

	return a.activeSkill != nil && a.activeSkill.Name == "review"
}

func (a *app) isReadOnlyReviewMode(input string) bool {
	if !a.isReviewRequest(input) {
		return false
	}

	if taskRequiresMutation(input) || requestExplicitlyAllowsCommands(input) {
		return false
	}

	return true
}

func (a *app) selectToolsForTask(input string) []model.Tool {
	if a.isReadOnlyReviewMode(input) {
		return model.ReadOnlyTools
	}

	return model.DefaultTools
}

func shouldRetryForToolUse(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return true
	}

	patterns := []string{
		"无法创建", "不能创建", "没法创建", "无法修改", "不能修改", "没法修改",
		"unable to create", "cannot create", "can't create", "unable to modify", "cannot modify", "can't modify",
		"i can't create", "i cannot create", "i'm unable to create",
	}

	for _, pattern := range patterns {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}

	return false
}

func isUserRejectedToolResult(toolName, result string) bool {
	if !isMutatingTool(toolName) {
		return false
	}

	normalized := strings.TrimSpace(result)
	cancellations := []string{
		"已取消执行命令。",
		"已取消写入。",
		"已取消创建目录。",
		"已取消删除。",
	}

	for _, item := range cancellations {
		if normalized == item {
			return true
		}
	}

	return false
}

func isMutatingTool(toolName string) bool {
	switch toolName {
	case "write_file", "edit_file", "create_directory", "delete_file", "execute_command":
		return true
	default:
		return false
	}
}

func shouldDisplayToolTrace(toolName string) bool {
	if isMutatingTool(toolName) {
		return true
	}

	switch toolName {
	case "read_file", "read_file_lines", "list_files", "glob", "grep", "find_functions", "get_file_info", "get_project_structure", "analyze_dependencies":
		return false
	default:
		return true
	}
}

func aPrompt(a *app) string {
	if a != nil && a.activeSkill != nil {
		return fmt.Sprintf("\n【技能：/%s｜退出：/clear-skill】\n> ", a.activeSkill.Name)
	}
	return "\n> "
}
