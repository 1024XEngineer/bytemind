package model

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

var DefaultTools = []Tool{
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "read_file",
			Description: "读取文件完整内容。用于查看代码、配置、文档等。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "文件路径，相对于工作目录",
					},
				},
				Required: []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "read_file_lines",
			Description: "读取文件的指定行范围。用于查看大文件的局部内容。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "文件路径",
					},
					"start": {
						Type:        "integer",
						Description: "起始行号，从 1 开始",
					},
					"end": {
						Type:        "integer",
						Description: "结束行号",
					},
				},
				Required: []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "list_files",
			Description: "递归列出目录中的所有文件。不包括 node_modules, vendor, .git 等目录。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"dir": {
						Type:        "string",
						Description: "目录路径，默认为根目录",
					},
				},
				Required: []string{},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "glob",
			Description: "按模式搜索文件。支持 * 匹配任意字符，** 匹配任意目录。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"pattern": {
						Type:        "string",
						Description: "文件模式，如 *.go, **/*.ts, src/*.js",
					},
				},
				Required: []string{"pattern"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "grep",
			Description: "在所有文件中搜索文本。返回匹配的行和行号。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"pattern": {
						Type:        "string",
						Description: "搜索关键词或正则表达式",
					},
					"file_type": {
						Type:        "string",
						Description: "文件类型过滤，如 go, js, py",
					},
				},
				Required: []string{"pattern"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "find_functions",
			Description: "查找代码中的函数和方法的定义位置。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"pattern": {
						Type:        "string",
						Description: "函数名",
					},
				},
				Required: []string{"pattern"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "write_file",
			Description: "创建新文件或覆盖已有文件。用于新建代码文件、配置文件等。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "文件路径",
					},
					"content": {
						Type:        "string",
						Description: "文件完整内容",
					},
				},
				Required: []string{"path", "content"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "edit_file",
			Description: "编辑文件的部分内容。用于修改现有代码。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "文件路径",
					},
					"old_content": {
						Type:        "string",
						Description: "要替换的原文",
					},
					"new_content": {
						Type:        "string",
						Description: "替换后的新内容",
					},
				},
				Required: []string{"path", "old_content", "new_content"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "create_directory",
			Description: "创建目录。如果父目录不存在也会一并创建。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "目录路径",
					},
				},
				Required: []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "delete_file",
			Description: "删除文件或空目录。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "要删除的文件或目录路径",
					},
				},
				Required: []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "execute_command",
			Description: "执行终端命令。用于运行构建、测试、脚本等。危险命令会被拦截。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"command": {
						Type:        "string",
						Description: "要执行的命令",
					},
					"timeout": {
						Type:        "integer",
						Description: "超时时间(秒)，默认30秒",
					},
				},
				Required: []string{"command"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "get_file_info",
			Description: "获取文件或目录的元信息。",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"path": {
						Type:        "string",
						Description: "文件或目录路径",
					},
				},
				Required: []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "get_project_structure",
			Description: "获取项目整体结构。返回目录树和主要配置文件。",
			Parameters: Parameters{
				Type:       "object",
				Properties: map[string]Property{},
				Required:   []string{},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "analyze_dependencies",
			Description: "分析项目的依赖关系。查看 package.json, go.mod 等依赖文件。",
			Parameters: Parameters{
				Type:       "object",
				Properties: map[string]Property{},
				Required:   []string{},
			},
		},
	},
}

var readOnlyToolNames = map[string]struct{}{
	"read_file":             {},
	"read_file_lines":       {},
	"list_files":            {},
	"glob":                  {},
	"grep":                  {},
	"find_functions":        {},
	"get_file_info":         {},
	"get_project_structure": {},
	"analyze_dependencies":  {},
}

var ReadOnlyTools = subsetTools(DefaultTools, readOnlyToolNames)

func subsetTools(tools []Tool, allowed map[string]struct{}) []Tool {
	filtered := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if _, ok := allowed[tool.Function.Name]; !ok {
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered
}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}
