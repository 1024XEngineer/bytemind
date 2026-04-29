package tools

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

type RegistrationSource string

const (
	RegistrationSourceBuiltin   RegistrationSource = "builtin"
	RegistrationSourceExtension RegistrationSource = "extension"
)

type RegisterOptions struct {
	Source       RegistrationSource
	ExtensionID  string
	OriginalName string
	// AllowOriginalNameShadowBuiltin permits extension registrations to reuse an
	// existing builtin original tool name. This is only intended for bridge
	// aliases whose tool key is source-aware and unique.
	AllowOriginalNameShadowBuiltin bool
}

type RegistrationMeta struct {
	ToolKey          string
	StableToolKey    string
	OriginalToolName string
	Source           RegistrationSource
	ExtensionID      string
	ConflictPolicy   string
}

type RegistryErrorCode string

const (
	RegistryErrorDuplicateName  RegistryErrorCode = "duplicate_name"
	RegistryErrorInvalidSource  RegistryErrorCode = "invalid_source"
	RegistryErrorInvalidSchema  RegistryErrorCode = "invalid_schema"
	RegistryErrorInvalidTool    RegistryErrorCode = "invalid_tool"
	RegistryErrorInvalidToolKey RegistryErrorCode = "invalid_tool_key"
	RegistryErrorNotFound       RegistryErrorCode = "not_found"
)

type RegistryError struct {
	Code             RegistryErrorCode
	Message          string
	ToolKey          string
	OriginalToolName string
	Source           RegistrationSource
	ExtensionID      string
	ConflictWith     RegistrationMeta
	Cause            error
}

func (e *RegistryError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if strings.TrimSpace(e.ToolKey) != "" {
		return fmt.Sprintf("registry error for tool %q", e.ToolKey)
	}
	return string(e.Code)
}

func (e *RegistryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func buildRegistration(tool Tool, opts RegisterOptions) (RegistrationMeta, ResolvedTool, error) {
	meta := RegistrationMeta{
		Source:         normalizeRegistrationSource(opts.Source),
		ExtensionID:    strings.TrimSpace(opts.ExtensionID),
		ConflictPolicy: "reject",
	}
	if meta.Source == "" {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSource, Message: "registration source is required", Source: opts.Source}
	}
	if meta.Source == RegistrationSourceBuiltin && meta.ExtensionID != "" {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSource, Message: "builtin registration cannot include extension id", Source: meta.Source, ExtensionID: meta.ExtensionID}
	}
	if meta.Source == RegistrationSourceExtension && meta.ExtensionID == "" {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSource, Message: "extension registration requires extension id", Source: meta.Source}
	}
	definition := cloneToolDefinition(tool.Definition())
	meta.ToolKey = strings.TrimSpace(definition.Function.Name)
	if meta.ToolKey == "" {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidToolKey, Message: "tool key is required", Source: meta.Source, ExtensionID: meta.ExtensionID}
	}
	meta.StableToolKey = meta.ToolKey
	meta.OriginalToolName = normalizeOriginalToolName(opts.OriginalName)
	if meta.OriginalToolName == "" {
		meta.OriginalToolName = normalizeOriginalToolName(meta.ToolKey)
	}
	definition.Function.Name = meta.ToolKey
	if provider, ok := tool.(ToolSpecProvider); ok {
		spec := NormalizeToolSpec(MergeToolSpec(DefaultToolSpec(definition), provider.Spec()))
		if strings.TrimSpace(spec.Name) != "" && spec.Name != meta.ToolKey {
			return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidToolKey, Message: fmt.Sprintf("tool spec name %q must match tool key %q", spec.Name, meta.ToolKey), ToolKey: meta.ToolKey, Source: meta.Source, ExtensionID: meta.ExtensionID}
		}
		if err := ValidateToolSpec(spec); err != nil {
			return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSchema, Message: err.Error(), ToolKey: meta.ToolKey, Source: meta.Source, ExtensionID: meta.ExtensionID, Cause: err}
		}
		return meta, ResolvedTool{Definition: definition, Spec: cloneToolSpec(spec), Tool: tool}, nil
	}
	spec := NormalizeToolSpec(DefaultToolSpec(definition))
	if err := ValidateToolSpec(spec); err != nil {
		return RegistrationMeta{}, ResolvedTool{}, &RegistryError{Code: RegistryErrorInvalidSchema, Message: err.Error(), ToolKey: meta.ToolKey, Source: meta.Source, ExtensionID: meta.ExtensionID, Cause: err}
	}
	return meta, ResolvedTool{Definition: definition, Spec: cloneToolSpec(spec), Tool: tool}, nil
}

func normalizeRegistrationSource(source RegistrationSource) RegistrationSource {
	switch RegistrationSource(strings.TrimSpace(string(source))) {
	case RegistrationSourceBuiltin:
		return RegistrationSourceBuiltin
	case RegistrationSourceExtension:
		return RegistrationSourceExtension
	default:
		return ""
	}
}

func normalizeOriginalToolName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func cloneResolvedTool(resolved ResolvedTool) ResolvedTool {
	return ResolvedTool{Definition: cloneToolDefinition(resolved.Definition), Spec: cloneToolSpec(resolved.Spec), Tool: resolved.Tool}
}

func cloneToolSpec(spec ToolSpec) ToolSpec {
	cloned := spec
	if spec.AllowedModes != nil {
		cloned.AllowedModes = append([]planpkg.AgentMode(nil), spec.AllowedModes...)
	}
	return cloned
}

func cloneToolDefinition(def llm.ToolDefinition) llm.ToolDefinition {
	cloned := def
	cloned.Function = def.Function
	cloned.Function.Parameters = cloneAnyMap(def.Function.Parameters)
	return cloned
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneAny(value)
	}
	return cloned
}

func cloneAnySlice(input []any) []any {
	if input == nil {
		return nil
	}
	cloned := make([]any, len(input))
	for i, value := range input {
		cloned[i] = cloneAny(value)
	}
	return cloned
}

func cloneAny(value any) any {
	cloned := cloneReflectValue(reflect.ValueOf(value))
	if !cloned.IsValid() {
		return nil
	}
	return cloned.Interface()
}

func cloneReflectValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneReflectValue(value.Elem())
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(cloned)
		return wrapped
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.New(value.Type().Elem())
		cloned.Elem().Set(cloneReflectValue(value.Elem()))
		return cloned
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			cloned.SetMapIndex(iter.Key(), cloneReflectValue(iter.Value()))
		}
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return cloned
	case reflect.Struct:
		return value
	default:
		return value
	}
}
