package provider

import "errors"

type routeCandidate struct {
	ProviderID ProviderID
	ModelID    ModelID
	Client     Client
}

var errorsUnavailable = errors.New(string(ErrCodeUnavailable))

func preferredRouteProvider(requested ModelID, rc RouteContext) ProviderID {
	if id := normalizeRouteProviderID(ProviderID(rc.Tags["provider"])); id != "" {
		return id
	}
	model := normalizeRouteModelID(requested)
	if model == "" {
		return ""
	}
	if isAnthropicModel(model) {
		return ProviderAnthropic
	}
	if isOpenAIModel(model) {
		return ProviderOpenAI
	}
	return ""
}

func routeRankLatency(id ProviderID) int {
	switch normalizeRouteProviderID(id) {
	case ProviderOpenAI:
		return 1
	case ProviderAnthropic:
		return 2
	default:
		return 10
	}
}

func routeRankCost(id ProviderID) int {
	switch normalizeRouteProviderID(id) {
	case ProviderAnthropic:
		return 1
	case ProviderOpenAI:
		return 2
	default:
		return 10
	}
}

func isAnthropicModel(model ModelID) bool {
	value := string(model)
	return len(value) >= len("claude") && value[:len("claude")] == "claude"
}

func isOpenAIModel(model ModelID) bool {
	value := string(model)
	if len(value) >= len("gpt") && value[:len("gpt")] == "gpt" {
		return true
	}
	return len(value) >= len("o1") && value[:len("o1")] == "o1"
}
