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

type ResolvedTool struct {
	Definition llm.ToolDefinition
	Spec       ToolSpec
	Tool       Tool
}

type Registry struct {
	tools map[string]ResolvedTool
}

func DefaultRegistry() *Registry {
	r := &Registry{tools: map[string]ResolvedTool{}}
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
	if r.tools == nil {
		r.tools = map[string]ResolvedTool{}
	}
	definition := normalizeDefinitionSchema(tool.Definition())
	spec := DefaultToolSpec(definition)
	if provider, ok := tool.(ToolSpecProvider); ok {
		spec = MergeToolSpec(spec, provider.Spec())
	}
	spec = NormalizeToolSpec(spec)
	if err := ValidateToolSpec(spec); err != nil {
		panic(err)
	}
	definition = applySpecToDefinitionSchema(definition, spec)
	r.tools[definition.Function.Name] = ResolvedTool{
		Definition: definition,
		Spec:       spec,
		Tool:       tool,
	}
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
		if modeAllowed(r.tools[name].Spec, mode) {
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
		defs = append(defs, r.tools[name].Definition)
	}
	return defs
}

func (r *Registry) Spec(name string) (ToolSpec, bool) {
	resolved, ok := r.tools[name]
	if !ok {
		return ToolSpec{}, false
	}
	return resolved.Spec, true
}

func (r *Registry) ResolveForMode(mode planpkg.AgentMode, name string) (ResolvedTool, error) {
	resolved, ok := r.tools[name]
	if !ok {
		return ResolvedTool{}, NewToolExecError(ToolErrorInvalidArgs, fmt.Sprintf("unknown tool %q", name), false, nil)
	}
	if !modeAllowed(resolved.Spec, mode) {
		return ResolvedTool{}, NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q is unavailable in %s mode", name, mode), false, nil)
	}
	return resolved, nil
}

func (r *Registry) ResolveForModeWithFilters(mode planpkg.AgentMode, allowlist, denylist []string) []ResolvedTool {
	allowSet := toNameSet(allowlist)
	denySet := toNameSet(denylist)

	names := make([]string, 0, len(r.tools))
	for name, resolved := range r.tools {
		if !modeAllowed(resolved.Spec, mode) {
			continue
		}
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
	sort.Strings(names)

	items := make([]ResolvedTool, 0, len(names))
	for _, name := range names {
		items = append(items, r.tools[name])
	}
	return items
}

func (r *Registry) Execute(ctx context.Context, name, rawArgs string, execCtx *ExecutionContext) (string, error) {
	return r.ExecuteForMode(ctx, planpkg.ModeBuild, name, rawArgs, execCtx)
}

func (r *Registry) ExecuteForMode(ctx context.Context, mode planpkg.AgentMode, name, rawArgs string, execCtx *ExecutionContext) (string, error) {
	resolved, err := r.ResolveForMode(mode, name)
	if err != nil {
		return "", err
	}
	if !toolAllowedByPolicy(name, execCtx) {
		return "", NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q is unavailable by active skill policy", name), false, nil)
	}
	if rawArgs == "" {
		rawArgs = "{}"
	}
	if execCtx != nil {
		execCtx.Mode = mode
	}
	return resolved.Tool.Run(ctx, json.RawMessage(rawArgs), execCtx)
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

func normalizeDefinitionSchema(def llm.ToolDefinition) llm.ToolDefinition {
	if def.Function.Parameters == nil {
		def.Function.Parameters = map[string]any{}
	}
	if _, ok := def.Function.Parameters["type"]; !ok {
		def.Function.Parameters["type"] = "object"
	}
	if _, ok := def.Function.Parameters["properties"]; !ok {
		def.Function.Parameters["properties"] = map[string]any{}
	}
	return def
}

func applySpecToDefinitionSchema(def llm.ToolDefinition, spec ToolSpec) llm.ToolDefinition {
	if !spec.StrictArgs {
		return def
	}
	if _, ok := def.Function.Parameters["additionalProperties"]; !ok {
		def.Function.Parameters["additionalProperties"] = false
	}
	return def
}
