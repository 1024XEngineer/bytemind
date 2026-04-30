package provider

import "strings"

type AssistantToolCallContentMode string

const (
	AssistantToolCallContentPreserve    AssistantToolCallContentMode = "preserve"
	AssistantToolCallContentEmptyString AssistantToolCallContentMode = "empty_string"
	AssistantToolCallContentNull        AssistantToolCallContentMode = "null"
	AssistantToolCallContentOmit        AssistantToolCallContentMode = "omit"
)

type ReasoningReplayMode string

const (
	ReasoningReplayNone        ReasoningReplayMode = "none"
	ReasoningReplayHiddenField ReasoningReplayMode = "hidden_field"
)

const (
	ModelPolicySourceDefault        = "default"
	ModelPolicySourceFamilyOverride = "family"
	ModelPolicySourceProviderID     = "provider_id"
	ModelPolicySourceBaseURL        = "base_url"
)

type ResolvedModelPolicy struct {
	ProviderID                   ProviderID
	ProviderType                 string
	ModelID                      ModelID
	Family                       string
	AssistantToolCallContentMode AssistantToolCallContentMode
	ReasoningField               string
	ReasoningReplayMode          ReasoningReplayMode
	Source                       string
}

func (p ResolvedModelPolicy) ReplayReasoning() bool {
	return p.ReasoningReplayMode == ReasoningReplayHiddenField && strings.TrimSpace(p.ReasoningField) != ""
}

func ResolveModelPolicy(providerID ProviderID, providerType string, modelID ModelID, family string, baseURL string) ResolvedModelPolicy {
	policy := ResolvedModelPolicy{
		ProviderID:                   normalizePolicyProviderID(providerID),
		ProviderType:                 normalizePolicyProviderType(providerType),
		ModelID:                      ModelID(strings.TrimSpace(string(modelID))),
		Family:                       normalizePolicyFamily(family),
		AssistantToolCallContentMode: AssistantToolCallContentOmit,
		ReasoningReplayMode:          ReasoningReplayNone,
		Source:                       ModelPolicySourceDefault,
	}

	if policy.Family != "" {
		policy.Source = ModelPolicySourceFamilyOverride
		if providerFamilyUsesOpenAIReasoningContent(policy.Family) {
			policy.enableOpenAIReasoningContent()
		}
		return policy
	}

	if detected, ok := detectOpenAIReasoningFamily(string(policy.ProviderID)); ok {
		policy.Family = detected
		policy.Source = ModelPolicySourceProviderID
		policy.enableOpenAIReasoningContent()
		return policy
	}

	if detected, ok := detectOpenAIReasoningFamily(baseURL); ok {
		policy.Family = detected
		policy.Source = ModelPolicySourceBaseURL
		policy.enableOpenAIReasoningContent()
	}

	return policy
}

func (p *ResolvedModelPolicy) enableOpenAIReasoningContent() {
	p.AssistantToolCallContentMode = AssistantToolCallContentEmptyString
	p.ReasoningField = openAIReasoningContentKey
	p.ReasoningReplayMode = ReasoningReplayHiddenField
}

func normalizePolicyProviderID(providerID ProviderID) ProviderID {
	id := ProviderID(strings.ToLower(strings.TrimSpace(string(providerID))))
	if id == "" {
		return ProviderID("unknown")
	}
	return id
}

func normalizePolicyProviderType(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "", "openai_compatible":
		return "openai-compatible"
	default:
		return strings.ToLower(strings.TrimSpace(providerType))
	}
}

func normalizePolicyFamily(family string) string {
	family = strings.ToLower(strings.TrimSpace(family))
	family = strings.ReplaceAll(family, "_", "-")
	switch family {
	case "moonshot-ai":
		return "moonshot"
	case "z-ai", "z.ai", "zhipu-ai":
		return "zai"
	default:
		return family
	}
}

func providerFamilyUsesOpenAIReasoningContent(value string) bool {
	_, ok := detectOpenAIReasoningFamily(value)
	return ok
}

func detectOpenAIReasoningFamily(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", false
	}
	for _, marker := range []struct {
		match  string
		family string
	}{
		{match: "deepseek", family: "deepseek"},
		{match: "kimi", family: "kimi"},
		{match: "moonshot", family: "moonshot"},
		{match: "glm", family: "glm"},
		{match: "zhipu", family: "zai"},
		{match: "zai", family: "zai"},
		{match: "z-ai", family: "zai"},
		{match: "z.ai", family: "zai"},
		{match: "bigmodel", family: "zai"},
	} {
		if strings.Contains(value, marker.match) {
			return marker.family, true
		}
	}
	return "", false
}
