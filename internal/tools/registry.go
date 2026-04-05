package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
)

type ExecutionContext struct {
	Workspace      string
	ApprovalPolicy string
	Approval       ApprovalHandler
	Session        *session.Session
	Mode           planpkg.AgentMode
	Stdin          io.Reader
	Stdout         io.Writer
	AllowedTools   map[string]struct{}
	DeniedTools    map[string]struct{}
}

type Tool interface {
	Definition() llm.ToolDefinition
	Run(context.Context, json.RawMessage, *ExecutionContext) (string, error)
}

type Registry struct {
	tools map[string]Tool
}

func DefaultRegistry() *Registry {
	r := &Registry{tools: map[string]Tool{}}
	r.Add(ListFilesTool{})
	r.Add(ReadFileTool{})
	r.Add(SearchTextTool{})
	r.Add(NewWebSearchTool())
	r.Add(NewWebFetchTool())
	r.Add(WriteFileTool{})
	r.Add(ReplaceInFileTool{})
	r.Add(ApplyPatchTool{})
	r.Add(UpdatePlanTool{})
	r.Add(RunShellTool{})
	return r
}

func (r *Registry) Add(tool Tool) {
	r.tools[tool.Definition().Function.Name] = tool
}

func (r *Registry) Definitions() []llm.ToolDefinition {
	return r.DefinitionsForMode(planpkg.ModeBuild)
}

func (r *Registry) DefinitionsForMode(mode planpkg.AgentMode) []llm.ToolDefinition {
	return r.DefinitionsForModeWithFilters(mode, nil, nil)
}

func (r *Registry) DefinitionsForModeWithFilters(mode planpkg.AgentMode, allowlist, denylist []string) []llm.ToolDefinition {
	allowSet := toNameSet(allowlist)
	denySet := toNameSet(denylist)

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		if toolAllowedInMode(mode, name) {
			if len(allowSet) > 0 {
				if _, ok := allowSet[name]; !ok {
					continue
				}
			}
			if _, blocked := denySet[name]; blocked {
				continue
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)

	defs := make([]llm.ToolDefinition, 0, len(names))
	for _, name := range names {
		defs = append(defs, r.tools[name].Definition())
	}
	return defs
}

func (r *Registry) Execute(ctx context.Context, name, rawArgs string, execCtx *ExecutionContext) (string, error) {
	return r.ExecuteForMode(ctx, planpkg.ModeBuild, name, rawArgs, execCtx)
}

func (r *Registry) ExecuteForMode(ctx context.Context, mode planpkg.AgentMode, name, rawArgs string, execCtx *ExecutionContext) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	if !toolAllowedInMode(mode, name) {
		return "", fmt.Errorf("tool %q is unavailable in %s mode", name, mode)
	}
	if !toolAllowedByPolicy(name, execCtx) {
		return "", fmt.Errorf("tool %q is unavailable by active skill policy", name)
	}
	if rawArgs == "" {
		rawArgs = "{}"
	}
	if execCtx != nil {
		execCtx.Mode = mode
	}
	return tool.Run(ctx, json.RawMessage(rawArgs), execCtx)
}

func toNameSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result[item] = struct{}{}
	}
	return result
}

func toolAllowedByPolicy(name string, execCtx *ExecutionContext) bool {
	if execCtx == nil {
		return true
	}
	if len(execCtx.AllowedTools) > 0 {
		if _, ok := execCtx.AllowedTools[name]; !ok {
			return false
		}
	}
	if len(execCtx.DeniedTools) > 0 {
		if _, blocked := execCtx.DeniedTools[name]; blocked {
			return false
		}
	}
	return true
}

func toolAllowedInMode(mode planpkg.AgentMode, name string) bool {
	if planpkg.NormalizeMode(string(mode)) != planpkg.ModePlan {
		return true
	}
	switch name {
	case "list_files", "read_file", "search_text", "web_search", "web_fetch", "update_plan", "run_shell":
		return true
	default:
		return false
	}
}
