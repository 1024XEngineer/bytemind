package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/forgecli/forgecli/pkg/session"
	"github.com/forgecli/forgecli/pkg/skills"
)

func TestDetectTurnModeForChat(t *testing.T) {
	cases := []string{
		"你好",
		"hello",
		"谢谢",
		"你会什么",
		"告诉我你的tool有什么",
	}

	for _, input := range cases {
		if mode := detectTurnMode(input); mode != turnModeChat {
			t.Fatalf("expected chat mode for %q", input)
		}
	}
}

func TestDetectTurnModeForTask(t *testing.T) {
	cases := []string{
		"帮我分析这个项目",
		"请读取 go.mod 并解释依赖",
		"run the tests and fix failures",
		"帮我修改 cmd/main.go",
		"review calculator.py",
		"看看calculator.py",
	}

	for _, input := range cases {
		if mode := detectTurnMode(input); mode != turnModeTask {
			t.Fatalf("expected task mode for %q", input)
		}
	}
}

func TestParseSkillCommand(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "review")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("name: review\ndescription: review code\n"), 0644); err != nil {
		t.Fatal(err)
	}

	manager := skills.NewManager(root)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}

	command, skill, args, ok := parseSkillCommand("/review inspect this branch", manager)
	if !ok {
		t.Fatal("expected skill command to be parsed")
	}
	if command != "run" {
		t.Fatalf("expected run command, got %s", command)
	}
	if skill == nil || skill.Name != "review" {
		t.Fatalf("expected review skill, got %+v", skill)
	}
	if args != "inspect this branch" {
		t.Fatalf("unexpected args: %q", args)
	}

	command, skill, args, ok = parseSkillCommand("/review", manager)
	if !ok {
		t.Fatal("expected activation command")
	}
	if command != "activate" || skill == nil || args != "" {
		t.Fatalf("unexpected activation parse result: command=%s skill=%v args=%q", command, skill, args)
	}

	command, skill, args, ok = parseSkillCommand("/clear-skill", manager)
	if !ok || command != "clear" || skill != nil || args != "" {
		t.Fatalf("unexpected clear parse result: command=%s skill=%v args=%q ok=%t", command, skill, args, ok)
	}
}

func TestTaskRequiresMutation(t *testing.T) {
	if !taskRequiresMutation("帮我创建一个 python 计算器程序") {
		t.Fatal("expected create request to require mutation")
	}
	if taskRequiresMutation("解释一下这个项目结构") {
		t.Fatal("did not expect read-only request to require mutation")
	}
}

func TestShouldRetryForToolUse(t *testing.T) {
	if !shouldRetryForToolUse("我无法创建这个文件。") {
		t.Fatal("expected tool retry for refusal to create")
	}
	if shouldRetryForToolUse("我已经通过 write_file 创建了 calculator.py。") {
		t.Fatal("did not expect retry after successful tool-grounded response")
	}
}

func TestIsUserRejectedToolResult(t *testing.T) {
	if !isUserRejectedToolResult("execute_command", "已取消执行命令。") {
		t.Fatal("expected execute_command rejection to stop task")
	}
	if !isUserRejectedToolResult("write_file", "已取消写入。") {
		t.Fatal("expected write_file rejection to stop task")
	}
	if isUserRejectedToolResult("get_file_info", "已取消执行命令。") {
		t.Fatal("did not expect read-only tool to be treated as rejection")
	}
}

func TestRequestLooksLikeReview(t *testing.T) {
	cases := []string{
		"请 review 我的代码",
		"帮我做 code review",
		"审查这个仓库的主要问题",
	}

	for _, input := range cases {
		if !requestLooksLikeReview(input) {
			t.Fatalf("expected review detection for %q", input)
		}
	}

	if requestLooksLikeReview("帮我创建一个计算器程序") {
		t.Fatal("did not expect non-review request to be detected as review")
	}
}

func TestRequestExplicitlyAllowsCommands(t *testing.T) {
	if !requestExplicitlyAllowsCommands("请 review 代码并运行测试") {
		t.Fatal("expected command-capable review request to allow commands")
	}
	if requestExplicitlyAllowsCommands("请 review 我的代码结构") {
		t.Fatal("did not expect plain review request to allow commands")
	}
}

func TestSelectToolsForReadOnlyReview(t *testing.T) {
	app := &app{}
	tools := app.selectToolsForTask("请 review 我的代码")

	if len(tools) == 0 {
		t.Fatal("expected read-only tools for review")
	}

	for _, tool := range tools {
		if tool.Function.Name == "execute_command" || tool.Function.Name == "write_file" {
			t.Fatalf("did not expect mutating tool %q in read-only review", tool.Function.Name)
		}
	}
}

func TestSelectToolsForReviewWithCommands(t *testing.T) {
	app := &app{}
	tools := app.selectToolsForTask("请 review 这个项目并运行测试")

	foundExec := false
	for _, tool := range tools {
		if tool.Function.Name == "execute_command" {
			foundExec = true
			break
		}
	}

	if !foundExec {
		t.Fatal("expected execute_command for review request that explicitly asks to run tests")
	}
}

func TestRenderToolResultPreviewForReadFileLines(t *testing.T) {
	args := `{"path":"calculator.py","start":210,"end":218}`
	result := "210: break\n211:\n212: except Exception as e:\n213: print('x')"

	preview := renderToolResultPreview("read_file_lines", args, result)
	if !strings.Contains(preview, "calculator.py") {
		t.Fatalf("expected path in preview, got %q", preview)
	}
	if strings.Contains(preview, "except Exception as e") {
		t.Fatalf("did not expect raw code in preview, got %q", preview)
	}
	if !strings.Contains(preview, "内容已发送给模型") {
		t.Fatalf("expected compact summary, got %q", preview)
	}
}

func TestRenderToolResultPreviewForGrep(t *testing.T) {
	result := "cmd/main.go:10: detectTurnMode\ncmd/main.go:20: handleModelTask\npkg/edit/edit.go:35: Prepare"
	preview := renderToolResultPreview("grep", `{"pattern":"handle"}`, result)

	if !strings.Contains(preview, "已返回 3 条结果") {
		t.Fatalf("expected result count in preview, got %q", preview)
	}
	if !strings.Contains(preview, "cmd/main.go:10: detectTurnMode") {
		t.Fatalf("expected compact preview lines, got %q", preview)
	}
}

func TestRenderToolResultPreviewForGrepTruncatesLongLines(t *testing.T) {
	longLine := "calculator.exe:1865: " + strings.Repeat("abcdef", 80)
	preview := renderToolResultPreview("grep", `{"pattern":"abc"}`, longLine)

	if strings.Contains(preview, strings.Repeat("abcdef", 20)) {
		t.Fatalf("expected long grep preview to be truncated, got %q", preview)
	}
	if !strings.Contains(preview, "...") {
		t.Fatalf("expected truncated preview indicator, got %q", preview)
	}
}

func TestRenderEmptyToolResultForGlob(t *testing.T) {
	preview := renderToolResultPreview("glob", `{"pattern":"**/*.py"}`, "")
	if !strings.Contains(preview, "未找到匹配文件") {
		t.Fatalf("expected friendly empty glob message, got %q", preview)
	}
}

func TestSanitizePlanOutputRemovesCodeBlocks(t *testing.T) {
	plan := "Inspect calculator.py\n\n```python\ndef add(a, b):\n    return a + b\n```\n\nReport findings"
	sanitized := sanitizePlanOutput(plan)

	if strings.Contains(sanitized, "def add") {
		t.Fatalf("did not expect raw code in plan, got %q", sanitized)
	}
	if !strings.Contains(sanitized, "[已省略") {
		t.Fatalf("expected code block placeholder, got %q", sanitized)
	}
}

func TestSanitizeFinalResponseRemovesLargeCodeBlocks(t *testing.T) {
	response := "我检查了 calculator.py。\n\n```python\ndef add(a, b):\n    return a + b\n\ndef sub(a, b):\n    return a - b\n\ndef mul(a, b):\n    return a * b\n\ndef div(a, b):\n    return a / b\n```"
	sanitized := sanitizeFinalResponse("看看calculator.py", response)

	if strings.Contains(sanitized, "def add") {
		t.Fatalf("did not expect raw code in sanitized response, got %q", sanitized)
	}
	if !strings.Contains(sanitized, "[已省略") {
		t.Fatalf("expected placeholder after code removal, got %q", sanitized)
	}
}

func TestSanitizeFinalResponseRemovesShortPlainCodeBlocks(t *testing.T) {
	response := "我检查了 calculator.py。\ndef add(a, b):\n    return a + b\n\ndef sub(a, b):\n    return a - b\n主要问题是缺少测试。"
	sanitized := sanitizeFinalResponse("看看calculator.py", response)

	if strings.Contains(sanitized, "def add") {
		t.Fatalf("did not expect plain code block in sanitized response, got %q", sanitized)
	}
	if !strings.Contains(sanitized, "[已省略") {
		t.Fatalf("expected placeholder for plain code block, got %q", sanitized)
	}
}

func TestSanitizeFinalResponseAllowsRawCodeWhenExplicitlyRequested(t *testing.T) {
	response := "```python\ndef add(a, b):\n    return a + b\n```"
	sanitized := sanitizeFinalResponse("把完整代码贴出来", response)

	if !strings.Contains(sanitized, "def add") {
		t.Fatalf("expected raw code to remain when explicitly requested, got %q", sanitized)
	}
}

func TestLooksLikeFileInspectionRequest(t *testing.T) {
	if !looksLikeFileInspectionRequest("看看calculator.py") {
		t.Fatal("expected file inspection request to be detected")
	}
	if looksLikeFileInspectionRequest("看看这个思路") {
		t.Fatal("did not expect generic request without file extension to be treated as file inspection")
	}
}

func TestMentionsLikelyFilePath(t *testing.T) {
	if !mentionsLikelyFilePath("calculator.py") {
		t.Fatal("expected file path mention to be detected")
	}
	if !mentionsLikelyFilePath("请帮我看看 cmd/main.go") {
		t.Fatal("expected nested file path mention to be detected")
	}
	if mentionsLikelyFilePath("python 是什么") {
		t.Fatal("did not expect plain language mention to be treated as file path")
	}
}

func TestFilterByPrefixRootReturnsAllFiles(t *testing.T) {
	files := []string{"calculator.py", filepath.Join("cmd", "main.go")}
	filtered := filterByPrefix(files, ".")
	if len(filtered) != len(files) {
		t.Fatalf("expected root prefix filter to keep all files, got %v", filtered)
	}
}

func TestShouldForceToolSummary(t *testing.T) {
	app := &app{activeSkill: &skills.Skill{Name: "review"}}
	if !app.shouldForceToolSummary("review calculator.py", 8, 0) {
		t.Fatal("expected many tool calls to force summary")
	}
	if !app.shouldForceToolSummary("review calculator.py", 3, 3) {
		t.Fatal("expected repeated empty read-only results to force summary")
	}
	if app.shouldForceToolSummary("帮我创建文件", 2, 0) {
		t.Fatal("did not expect normal task to force summary early")
	}
}

func TestShouldDisplayToolTrace(t *testing.T) {
	if shouldDisplayToolTrace("read_file_lines") {
		t.Fatal("did not expect background read tool to be shown")
	}
	if shouldDisplayToolTrace("glob") {
		t.Fatal("did not expect background search tool to be shown")
	}
	if !shouldDisplayToolTrace("execute_command") {
		t.Fatal("expected mutating tool to remain visible")
	}
}

func TestPromptShowsActiveSkillAndExitHint(t *testing.T) {
	app := &app{activeSkill: &skills.Skill{Name: "review"}}
	prompt := aPrompt(app)

	if !strings.Contains(prompt, "技能：/review") {
		t.Fatalf("expected active skill in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "/clear-skill") {
		t.Fatalf("expected clear skill hint in prompt, got %q", prompt)
	}
}

func TestPromptWithoutSkillIsPlain(t *testing.T) {
	prompt := aPrompt(&app{})
	if prompt != "\n> " {
		t.Fatalf("expected plain prompt without skill, got %q", prompt)
	}
}

func TestSkillSystemPromptForReviewIsSanitized(t *testing.T) {
	prompt := skillSystemPrompt(&skills.Skill{
		Name:        "review",
		Description: "review code",
		Content:     "```bash\ngit branch --show-current\n```",
	})

	if strings.Contains(prompt, "git branch --show-current") {
		t.Fatalf("did not expect raw skill bash content in prompt: %q", prompt)
	}
	if !strings.Contains(prompt, "只读代码审查") {
		t.Fatalf("expected sanitized review prompt, got %q", prompt)
	}
}

func TestRecentExecutionFactsAndLatestUserRequest(t *testing.T) {
	sess := session.New("repo")
	sess.AddUserMessage("帮我检查输出是否规范")
	sess.AddEvent("write_file", "reply.txt", "ok", true, true)
	sess.AddEvent("execute_command", "python --version", "user rejected", false, false)

	app := &app{sess: sess}

	if got := app.latestUserRequest(); got != "帮我检查输出是否规范" {
		t.Fatalf("unexpected latest user request: %q", got)
	}

	facts := app.recentExecutionFacts(5)
	if !strings.Contains(facts, "write_file") || !strings.Contains(facts, "execute_command") {
		t.Fatalf("expected execution facts to include recent events, got %q", facts)
	}
}
