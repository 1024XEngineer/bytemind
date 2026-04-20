package extensions

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"bytemind/internal/llm"
	policypkg "bytemind/internal/policy"
	skillspkg "bytemind/internal/skills"
	toolspkg "bytemind/internal/tools"
)

const (
	stableToolKeySeparator = ":"
	maxStableToolKeyLength = 128
)

type ExtensionTool struct {
	Source      ExtensionKind
	ExtensionID string
	Tool        toolspkg.Tool
}

type BridgeBinding struct {
	Source       ExtensionKind
	ExtensionID  string
	OriginalName string
	StableKey    string
}

// Bridge projects an extension tool to the source-aware tool namespace.
// Invalid inputs are returned unchanged; callers that need strict validation
// should use RegisterBridgedTool.
func Bridge(extensionTool ExtensionTool) toolspkg.Tool {
	bridged, _, err := buildBridgedTool(extensionTool)
	if err != nil {
		return extensionTool.Tool
	}
	return bridged
}

func RegisterBridgedTool(registry *toolspkg.Registry, extensionTool ExtensionTool) (BridgeBinding, error) {
	if registry == nil {
		return BridgeBinding{}, wrapError(ErrCodeInvalidExtension, "tool registry is required", nil)
	}
	bridged, binding, err := buildBridgedTool(extensionTool)
	if err != nil {
		return BridgeBinding{}, err
	}
	if err := registry.Register(bridged, toolspkg.RegisterOptions{
		Source:       toolspkg.RegistrationSourceExtension,
		ExtensionID:  binding.ExtensionID,
		OriginalName: binding.OriginalName,
	}); err != nil {
		return BridgeBinding{}, err
	}
	return binding, nil
}

func StableToolKey(source ExtensionKind, extensionID, toolName string) (string, error) {
	sourceKey := normalizeStableSegment(string(source))
	if sourceKey == "" {
		return "", wrapError(ErrCodeInvalidSource, "extension source is required", nil)
	}
	extensionKey := normalizeStableSegment(extensionID)
	if extensionKey == "" {
		return "", wrapError(ErrCodeInvalidExtension, "extension id is required", nil)
	}
	toolKey := normalizeStableSegment(toolName)
	if toolKey == "" {
		return "", wrapError(ErrCodeInvalidExtension, "tool name is required", nil)
	}
	stable := strings.Join([]string{sourceKey, extensionKey, toolKey}, stableToolKeySeparator)
	return enforceStableToolKeyLength(stable), nil
}

func ResolvePolicyToolSets(policy skillspkg.ToolPolicy, bindings []BridgeBinding) (map[string]struct{}, map[string]struct{}, error) {
	mapped, err := mapPolicyItems(policy, bindings)
	if err != nil {
		return nil, nil, err
	}
	allow, deny := policypkg.ResolveToolSets(mapped)
	return allow, deny, nil
}

func mapPolicyItems(policy skillspkg.ToolPolicy, bindings []BridgeBinding) (skillspkg.ToolPolicy, error) {
	if len(policy.Items) == 0 || len(bindings) == 0 {
		return policy, nil
	}
	aliases, err := buildPolicyAliases(bindings)
	if err != nil {
		return skillspkg.ToolPolicy{}, err
	}
	mappedItems := make([]string, 0, len(policy.Items))
	for _, item := range policy.Items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if stable, ok := aliases[normalizePolicyAlias(item)]; ok {
			mappedItems = append(mappedItems, stable)
			continue
		}
		mappedItems = append(mappedItems, item)
	}
	return skillspkg.ToolPolicy{
		Policy: policy.Policy,
		Items:  mappedItems,
	}, nil
}

func buildPolicyAliases(bindings []BridgeBinding) (map[string]string, error) {
	aliases := make(map[string]string, len(bindings)*2)
	for _, binding := range bindings {
		original := strings.TrimSpace(binding.OriginalName)
		stable := strings.TrimSpace(binding.StableKey)
		if original == "" || stable == "" {
			return nil, wrapError(ErrCodeInvalidExtension, "bridge binding requires original and stable tool names", nil)
		}
		for _, alias := range []string{original, stable} {
			normalizedAlias := normalizePolicyAlias(alias)
			if normalizedAlias == "" {
				continue
			}
			existing, ok := aliases[normalizedAlias]
			if ok && existing != stable {
				return nil, wrapError(ErrCodeConflict, fmt.Sprintf("policy alias %q maps to multiple tools", alias), nil)
			}
			aliases[normalizedAlias] = stable
		}
	}
	return aliases, nil
}

func normalizePolicyAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(alias))
}

func buildBridgedTool(extensionTool ExtensionTool) (toolspkg.Tool, BridgeBinding, error) {
	if extensionTool.Tool == nil {
		return nil, BridgeBinding{}, wrapError(ErrCodeInvalidExtension, "extension tool is required", nil)
	}
	definition := extensionTool.Tool.Definition()
	originalName := strings.TrimSpace(definition.Function.Name)
	if originalName == "" {
		return nil, BridgeBinding{}, wrapError(ErrCodeInvalidExtension, "tool name is required", nil)
	}
	stableKey, err := StableToolKey(extensionTool.Source, extensionTool.ExtensionID, originalName)
	if err != nil {
		return nil, BridgeBinding{}, err
	}
	binding := BridgeBinding{
		Source:       extensionTool.Source,
		ExtensionID:  strings.TrimSpace(extensionTool.ExtensionID),
		OriginalName: originalName,
		StableKey:    stableKey,
	}
	return bridgedTool{
		delegate:  extensionTool.Tool,
		stableKey: stableKey,
	}, binding, nil
}

func normalizeStableSegment(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	var out strings.Builder
	lastUnderscore := false
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if r == '-' || r == '_' {
			out.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			out.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(out.String(), "_-")
}

func enforceStableToolKeyLength(stable string) string {
	stable = strings.TrimSpace(stable)
	if len(stable) <= maxStableToolKeyLength {
		return stable
	}
	sum := sha1.Sum([]byte(stable))
	suffix := hex.EncodeToString(sum[:6])
	cut := maxStableToolKeyLength - len(suffix) - len(stableToolKeySeparator)
	if cut <= 0 {
		if len(suffix) <= maxStableToolKeyLength {
			return suffix
		}
		return suffix[:maxStableToolKeyLength]
	}
	prefix := strings.TrimRight(stable[:cut], stableToolKeySeparator)
	if prefix == "" {
		return suffix
	}
	return prefix + stableToolKeySeparator + suffix
}

type bridgedTool struct {
	delegate  toolspkg.Tool
	stableKey string
}

func (t bridgedTool) Definition() llm.ToolDefinition {
	def := t.delegate.Definition()
	def.Function.Name = t.stableKey
	return def
}

func (t bridgedTool) Run(ctx context.Context, raw json.RawMessage, execCtx *toolspkg.ExecutionContext) (string, error) {
	return t.delegate.Run(ctx, raw, execCtx)
}

func (t bridgedTool) Spec() toolspkg.ToolSpec {
	provider, ok := t.delegate.(toolspkg.ToolSpecProvider)
	if !ok {
		return toolspkg.ToolSpec{Name: t.stableKey}
	}
	spec := provider.Spec()
	spec.Name = t.stableKey
	return spec
}
