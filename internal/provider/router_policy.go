package provider

import "errors"

type routeCandidate struct {
	ProviderID   ProviderID
	ModelID      ModelID
	Client       Client
	HealthStatus HealthStatus
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
	if isGeminiModel(model) {
		return ProviderGemini
	}
	if isOpenAIModel(model) {
		return ProviderOpenAI
	}
	return ""
}

func routeHealthRank(status HealthStatus) int {
	switch status {
	case "", HealthStatusHealthy:
		return 1
	case HealthStatusDegraded:
		return 2
	case HealthStatusHalfOpen:
		return 3
	case HealthStatusUnavailable:
		return 4
	default:
		return 5
	}
}

func routeRankLatency(id ProviderID) int {
	switch normalizeRouteProviderID(id) {
	case ProviderOpenAI:
		return 1
	case ProviderAnthropic:
		return 2
	case ProviderGemini:
		return 3
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
	case ProviderGemini:
		return 3
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

func isGeminiModel(model ModelID) bool {
	value := string(model)
	return len(value) >= len("gemini") && value[:len("gemini")] == "gemini"
}
