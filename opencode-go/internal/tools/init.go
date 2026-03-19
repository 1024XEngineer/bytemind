package tools

// InitRegistry 初始化工具注册中心，注册所有内置工具
func InitRegistry() *Registry {
	registry := NewRegistry()

	// 注册所有工具
	registry.Register(NewReadTool())
	registry.Register(NewWriteTool())
	registry.Register(NewShellTool())
	registry.Register(NewAskTool())
	registry.Register(NewMkdirTool())
	registry.Register(NewListTool())
	registry.Register(NewExistsTool())
	registry.Register(NewRemoveTool())

	// TODO: 注册更多工具 (edit, grep, glob, todowrite, etc.)

	return registry
}

// DefaultRegistry 默认工具注册中心
var DefaultRegistry = InitRegistry()
