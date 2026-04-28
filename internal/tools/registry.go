package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	runtimepkg "bytemind/internal/runtime"
	sandboxpkg "bytemind/internal/sandbox"
	"bytemind/internal/session"
)

type ExecutionContext struct {
	Workspace                 string
	WritableRoots             []string
	ApprovalPolicy            string
	ApprovalMode              string
	AwayPolicy                string
	SandboxEnabled            bool
	SystemSandboxMode         string
	SkipShellApproval         bool
	SandboxEscalationApproved bool
	LeaseID                   string
	RunID                     string
	FSRead                    []string
	FSWrite                   []string
	ExecAllowlist             []sandboxpkg.ExecRule
	NetworkAllowlist          []sandboxpkg.NetworkRule
	Lease                     *sandboxpkg.Lease
	LeaseKeyring              map[string][]byte

	Approval    ApprovalHandler
	Session     *session.Session
	TaskManager runtimepkg.TaskManager
	// Extensions is an optional passthrough hook for callers that need extension context.
	// It stays untyped here to keep tools/extension packages decoupled.
	Extensions   any
	Mode         planpkg.AgentMode
	Stdin        io.Reader
	Stdout       io.Writer
	AllowedTools map[string]struct{}
	DeniedTools  map[string]struct{}
}

const (
	approvalModeInteractive = "interactive"
	approvalModeAway        = "away"

	awayPolicyAutoDenyContinue = "auto_deny_continue"
	awayPolicyFailFast         = "fail_fast"
)

func (c *ExecutionContext) isAwayMode() bool {
	if c == nil {
		return false
	}
	return c.approvalMode() == approvalModeAway
}

func (c *ExecutionContext) approvalMode() string {
	if c == nil {
		return approvalModeInteractive
	}
	mode := strings.ToLower(strings.TrimSpace(c.ApprovalMode))
	if mode == "" {
		return approvalModeInteractive
	}
	return mode
}

func (c *ExecutionContext) awayPolicy() string {
	if c == nil {
		return awayPolicyAutoDenyContinue
	}
	policy := strings.ToLower(strings.TrimSpace(c.AwayPolicy))
	if policy == "" {
		return awayPolicyAutoDenyContinue
	}
	return policy
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
	mu    sync.RWMutex
	tools map[string]ResolvedTool
	meta  map[string]RegistrationMeta
}

func DefaultRegistry() *Registry {
	r := &Registry{}
	r.mustRegisterBuiltin(ListFilesTool{})
	r.mustRegisterBuiltin(ReadFileTool{})
	r.mustRegisterBuiltin(SearchTextTool{})
	r.mustRegisterBuiltin(NewWebSearchTool())
	r.mustRegisterBuiltin(NewWebFetchTool())
	r.mustRegisterBuiltin(WriteFileTool{})
	r.mustRegisterBuiltin(ReplaceInFileTool{})
	r.mustRegisterBuiltin(ApplyPatchTool{})
	r.mustRegisterBuiltin(UpdatePlanTool{})
	r.mustRegisterBuiltin(RunShellTool{})
	return r
}

// Add keeps backward compatibility with tests and call sites that used the old API.
func (r *Registry) Add(tool Tool) {
	if err := r.addBuiltin(tool); err != nil {
		panic(err)
	}
}

func (r *Registry) Register(tool Tool, opts RegisterOptions) error {
	if tool == nil {
		return &RegistryError{Code: RegistryErrorInvalidTool, Message: "tool is required", Source: opts.Source}
	}
	meta, resolved, err := buildRegistration(tool, opts)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMapsLocked()
	if existingMeta, exists := r.meta[meta.ToolKey]; exists {
		return &RegistryError{
			Code:             RegistryErrorDuplicateName,
			Message:          fmt.Sprintf("tool %q already registered", meta.ToolKey),
			ToolKey:          meta.ToolKey,
			OriginalToolName: meta.OriginalToolName,
			Source:           meta.Source,
			ExtensionID:      meta.ExtensionID,
			ConflictWith:     existingMeta,
		}
	}
	if conflicts := r.findConflictsByOriginalNameLocked(meta.OriginalToolName, meta.ToolKey); len(conflicts) > 0 {
		blocking := conflicts
		if meta.Source == RegistrationSourceExtension {
			blocking = filterBlockingOriginalNameConflicts(meta, conflicts, opts.AllowOriginalNameShadowBuiltin)
		}
		if len(blocking) == 0 {
			r.tools[meta.ToolKey] = resolved
			r.meta[meta.ToolKey] = meta
			return nil
		}
		return &RegistryError{
			Code:             RegistryErrorDuplicateName,
			Message:          fmt.Sprintf("tool original name %q already registered", meta.OriginalToolName),
			ToolKey:          meta.ToolKey,
			OriginalToolName: meta.OriginalToolName,
			Source:           meta.Source,
			ExtensionID:      meta.ExtensionID,
			ConflictWith:     blocking[0],
		}
	}
	r.tools[meta.ToolKey] = resolved
	r.meta[meta.ToolKey] = meta
	return nil
}

func (r *Registry) Unregister(name string) error {
	toolKey := strings.TrimSpace(name)
	if toolKey == "" {
		return &RegistryError{Code: RegistryErrorInvalidToolKey, Message: "tool key is required"}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.tools) == 0 {
		return &RegistryError{Code: RegistryErrorNotFound, Message: fmt.Sprintf("tool %q is not registered", toolKey), ToolKey: toolKey}
	}
	if _, ok := r.tools[toolKey]; !ok {
		return &RegistryError{Code: RegistryErrorNotFound, Message: fmt.Sprintf("tool %q is not registered", toolKey), ToolKey: toolKey}
	}
	delete(r.tools, toolKey)
	delete(r.meta, toolKey)
	return nil
}

func (r *Registry) Get(name string) (ResolvedTool, bool) {
	toolKey := strings.TrimSpace(name)
	if toolKey == "" {
		return ResolvedTool{}, false
	}
	r.mu.RLock()
	resolved, ok := r.tools[toolKey]
	r.mu.RUnlock()
	if !ok {
		return ResolvedTool{}, false
	}
	return cloneResolvedTool(resolved), true
}

func (r *Registry) List() []ResolvedTool {
	r.mu.RLock()
	snapshot := make(map[string]ResolvedTool, len(r.tools))
	for name, resolved := range r.tools {
		snapshot[name] = cloneResolvedTool(resolved)
	}
	r.mu.RUnlock()
	return sortedResolvedTools(snapshot, nil, nil, func(ResolvedTool) bool { return true })
}

func (r *Registry) FindByOriginalName(name string) []RegistrationMeta {
	original := normalizeOriginalToolName(name)
	if original == "" {
		return nil
	}
	r.mu.RLock()
	items := make([]RegistrationMeta, 0, len(r.meta))
	for _, meta := range r.meta {
		if meta.OriginalToolName != original {
			continue
		}
		items = append(items, cloneRegistrationMeta(meta))
	}
	r.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].ToolKey < items[j].ToolKey
	})
	return items
}

func (r *Registry) FindByExtensionID(extensionID string) []RegistrationMeta {
	extensionID = strings.TrimSpace(extensionID)
	if extensionID == "" {
		return nil
	}
	r.mu.RLock()
	items := make([]RegistrationMeta, 0, len(r.meta))
	for _, meta := range r.meta {
		if meta.Source != RegistrationSourceExtension {
			continue
		}
		if meta.ExtensionID != extensionID {
			continue
		}
		items = append(items, cloneRegistrationMeta(meta))
	}
	r.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].ToolKey < items[j].ToolKey
	})
	return items
}

func (r *Registry) Definitions() []llm.ToolDefinition {
	return r.DefinitionsForMode(planpkg.ModeBuild)
}

func (r *Registry) DefinitionsForMode(mode planpkg.AgentMode) []llm.ToolDefinition {
	return r.DefinitionsForModeWithFilters(mode, nil, nil)
}

func (r *Registry) DefinitionsForModeWithFilters(mode planpkg.AgentMode, allowlist, denylist []string) []llm.ToolDefinition {
	items := r.ResolveForModeWithFilters(mode, allowlist, denylist)
	defs := make([]llm.ToolDefinition, 0, len(items))
	for _, item := range items {
		defs = append(defs, cloneToolDefinition(item.Definition))
	}
	return defs
}

func (r *Registry) Spec(name string) (ToolSpec, bool) {
	resolved, ok := r.Get(name)
	if !ok {
		return ToolSpec{}, false
	}
	return cloneToolSpec(resolved.Spec), true
}

func (r *Registry) ResolveForMode(mode planpkg.AgentMode, name string) (ResolvedTool, error) {
	resolved, ok := r.Get(name)
	if !ok {
		return ResolvedTool{}, NewToolExecError(ToolErrorInvalidArgs, fmt.Sprintf("unknown tool %q", name), false, nil)
	}
	if !modeAllowed(resolved.Spec, mode) {
		return ResolvedTool{}, NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q is unavailable in %s mode", name, mode), false, nil)
	}
	return resolved, nil
}

func (r *Registry) ResolveForModeWithFilters(mode planpkg.AgentMode, allowlist, denylist []string) []ResolvedTool {
	r.mu.RLock()
	snapshot := make(map[string]ResolvedTool, len(r.tools))
	for name, resolved := range r.tools {
		snapshot[name] = cloneResolvedTool(resolved)
	}
	r.mu.RUnlock()
	return sortedResolvedTools(snapshot, allowlist, denylist, func(resolved ResolvedTool) bool {
		return modeAllowed(resolved.Spec, mode)
	})
}

func (r *Registry) mustRegisterBuiltin(tool Tool) {
	if err := r.addBuiltin(tool); err != nil {
		panic(err)
	}
}

func (r *Registry) addBuiltin(tool Tool) error {
	if tool == nil {
		return &RegistryError{Code: RegistryErrorInvalidTool, Message: "tool is required", Source: RegistrationSourceBuiltin}
	}
	return r.Register(tool, RegisterOptions{Source: RegistrationSourceBuiltin})
}

func (r *Registry) ensureMapsLocked() {
	if r.tools == nil {
		r.tools = map[string]ResolvedTool{}
	}
	if r.meta == nil {
		r.meta = map[string]RegistrationMeta{}
	}
}

func (r *Registry) findConflictsByOriginalNameLocked(originalName, toolKey string) []RegistrationMeta {
	originalName = normalizeOriginalToolName(originalName)
	toolKey = strings.TrimSpace(toolKey)
	if originalName == "" {
		return nil
	}
	conflicts := make([]RegistrationMeta, 0, len(r.meta))
	for existingKey, existingMeta := range r.meta {
		if existingKey == toolKey {
			continue
		}
		if existingMeta.OriginalToolName != originalName {
			continue
		}
		conflicts = append(conflicts, cloneRegistrationMeta(existingMeta))
	}
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].ToolKey < conflicts[j].ToolKey
	})
	return conflicts
}

func filterBlockingOriginalNameConflicts(meta RegistrationMeta, conflicts []RegistrationMeta, allowShadowBuiltin bool) []RegistrationMeta {
	if len(conflicts) == 0 {
		return nil
	}
	blocking := make([]RegistrationMeta, 0, len(conflicts))
	for _, conflict := range conflicts {
		switch conflict.Source {
		case RegistrationSourceBuiltin:
			if allowShadowBuiltin {
				continue
			}
		case RegistrationSourceExtension:
			// Session-isolated bridges may share original tool names across extensions.
			// Keep same-extension conflicts rejected to avoid ambiguous aliases.
			if meta.ExtensionID != "" && conflict.ExtensionID != "" && conflict.ExtensionID != meta.ExtensionID {
				continue
			}
		}
		blocking = append(blocking, conflict)
	}
	return blocking
}

func sortedResolvedTools(snapshot map[string]ResolvedTool, allowlist, denylist []string, include func(ResolvedTool) bool) []ResolvedTool {
	allowSet := toNameSet(allowlist)
	denySet := toNameSet(denylist)
	names := make([]string, 0, len(snapshot))
	for name, resolved := range snapshot {
		if !include(resolved) {
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
		items = append(items, cloneResolvedTool(snapshot[name]))
	}
	return items
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

func cloneRegistrationMeta(meta RegistrationMeta) RegistrationMeta {
	return RegistrationMeta{
		ToolKey:          meta.ToolKey,
		StableToolKey:    meta.StableToolKey,
		OriginalToolName: meta.OriginalToolName,
		Source:           meta.Source,
		ExtensionID:      meta.ExtensionID,
		ConflictPolicy:   meta.ConflictPolicy,
	}
}
