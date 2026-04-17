package provider

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"
)

type RouterConfig struct {
	DefaultProvider ProviderID
	DefaultModel    ModelID
}

type registryRouter struct {
	registry Registry
	health   HealthChecker
	policy   RouterConfig
}

func NewRouter(reg Registry, health HealthChecker, cfg RouterConfig) Router {
	return &registryRouter{
		registry: reg,
		health:   health,
		policy: RouterConfig{
			DefaultProvider: normalizeRouteProviderID(cfg.DefaultProvider),
			DefaultModel:    normalizeRouteModelID(cfg.DefaultModel),
		},
	}
}

func (r *registryRouter) Route(ctx context.Context, requestedModel ModelID, rc RouteContext) (RouteResult, error) {
	if r == nil || r.registry == nil {
		return RouteResult{}, unavailableRouteError("no provider candidates available")
	}
	candidates, err := r.collectCandidates(ctx)
	if err != nil {
		return RouteResult{}, err
	}
	requested := normalizeRouteModelID(requestedModel)
	filtered := filterCandidatesByModel(candidates, requested)
	if len(filtered) == 0 {
		return RouteResult{}, unavailableRouteError("no provider candidates available")
	}
	available, err := filterHealthyCandidates(ctx, r.health, filtered)
	if err != nil {
		return RouteResult{}, err
	}
	if len(available) == 0 {
		return RouteResult{}, unavailableRouteError("no provider candidates available")
	}
	ordered := sortRouteCandidates(available, requested, normalizeRouteContext(rc), r.policy)
	if len(ordered) == 0 {
		return RouteResult{}, unavailableRouteError("no provider candidates available")
	}
	result := RouteResult{Primary: toRouteTarget(ordered[0])}
	if rc.AllowFallback {
		result.Fallbacks = make([]RouteTarget, 0, len(ordered)-1)
		for _, candidate := range ordered[1:] {
			result.Fallbacks = append(result.Fallbacks, toRouteTarget(candidate))
		}
	}
	return result, nil
}

func (r *registryRouter) collectCandidates(ctx context.Context) ([]routeCandidate, error) {
	ids, err := r.registry.List(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]routeCandidate, 0, len(ids))
	warnings := make([]Warning, 0)
	seen := make(map[string]struct{})
	for _, id := range ids {
		client, ok := r.registry.Get(ctx, id)
		if !ok || client == nil {
			continue
		}
		providerID := normalizeRouteProviderID(client.ProviderID())
		if providerID == "" {
			continue
		}
		models, err := client.ListModels(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, err
			}
			warnings = append(warnings, Warning{ProviderID: providerID, Reason: "provider_list_models_failed"})
			continue
		}
		for _, model := range models {
			modelID := normalizeRouteModelID(model.ModelID)
			if modelID == "" {
				continue
			}
			key := string(providerID) + "\x00" + string(modelID)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, routeCandidate{
				ProviderID:   providerID,
				ModelID:      modelID,
				Client:       client,
				HealthStatus: HealthStatusHealthy,
			})
		}
	}
	for _, warning := range warnings {
		log.Printf("provider router warning: provider=%s reason=%s", warning.ProviderID, warning.Reason)
	}
	return candidates, nil
}

func normalizeRouteProviderID(id ProviderID) ProviderID {
	return ProviderID(strings.ToLower(strings.TrimSpace(string(id))))
}

func normalizeRouteModelID(id ModelID) ModelID {
	return ModelID(strings.TrimSpace(string(id)))
}

func normalizeRouteContext(rc RouteContext) RouteContext {
	rc.Scenario = strings.TrimSpace(rc.Scenario)
	rc.Region = strings.TrimSpace(rc.Region)
	if rc.Tags == nil {
		rc.Tags = map[string]string{}
	}
	return rc
}

func unavailableRouteError(message string) *Error {
	return &Error{
		Code:      ErrCodeUnavailable,
		Message:   message,
		Retryable: true,
		Err:       errorsUnavailable,
	}
}

func toRouteTarget(candidate routeCandidate) RouteTarget {
	return RouteTarget{
		ProviderID: candidate.ProviderID,
		ModelID:    candidate.ModelID,
		Client:     candidate.Client,
	}
}

func filterCandidatesByModel(candidates []routeCandidate, requested ModelID) []routeCandidate {
	if requested == "" {
		return append([]routeCandidate(nil), candidates...)
	}
	filtered := make([]routeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ModelID == requested {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func filterHealthyCandidates(ctx context.Context, health HealthChecker, candidates []routeCandidate) ([]routeCandidate, error) {
	if health == nil {
		return append([]routeCandidate(nil), candidates...), nil
	}
	filtered := make([]routeCandidate, 0, len(candidates))
	snapshots := make(map[ProviderID]HealthSnapshot, len(candidates))
	checked := make(map[ProviderID]error, len(candidates))
	for _, candidate := range candidates {
		snapshot, ok := snapshots[candidate.ProviderID]
		if !ok {
			snapshot = health.Status(ctx, candidate.ProviderID)
			snapshots[candidate.ProviderID] = snapshot
		}
		if snapshot.Status == HealthStatusUnavailable {
			continue
		}
		err, ok := checked[candidate.ProviderID]
		if !ok {
			err = health.Check(ctx, candidate.ProviderID)
			if errors.Is(err, context.Canceled) {
				return nil, err
			}
			checked[candidate.ProviderID] = err
		}
		if err == nil || snapshot.Status == HealthStatusHalfOpen {
			candidate.HealthStatus = snapshot.Status
			if candidate.HealthStatus == "" {
				candidate.HealthStatus = HealthStatusHealthy
			}
			filtered = append(filtered, candidate)
		}
	}
	return filtered, nil
}

func sortRouteCandidates(candidates []routeCandidate, requested ModelID, rc RouteContext, cfg RouterConfig) []routeCandidate {
	ordered := append([]routeCandidate(nil), candidates...)
	defaultProvider := normalizeRouteProviderID(cfg.DefaultProvider)
	defaultModel := normalizeRouteModelID(cfg.DefaultModel)
	preferredProvider := preferredRouteProvider(requested, rc)
	preferLatency := rc.PreferLatency
	preferLowCost := rc.PreferLowCost
	preference := make(map[ProviderID]int, len(ordered))
	for _, candidate := range ordered {
		if _, ok := preference[candidate.ProviderID]; ok {
			continue
		}
		preference[candidate.ProviderID] = routeHealthRank(candidate.HealthStatus)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]
		leftHealth, rightHealth := routeHealthRank(left.HealthStatus), routeHealthRank(right.HealthStatus)
		if leftHealth != rightHealth {
			return leftHealth < rightHealth
		}
		if left.ProviderID == preferredProvider && right.ProviderID != preferredProvider {
			return true
		}
		if right.ProviderID == preferredProvider && left.ProviderID != preferredProvider {
			return false
		}
		if left.ProviderID == defaultProvider && right.ProviderID != defaultProvider {
			return true
		}
		if right.ProviderID == defaultProvider && left.ProviderID != defaultProvider {
			return false
		}
		if left.ModelID == requested && right.ModelID != requested {
			return true
		}
		if right.ModelID == requested && left.ModelID != requested {
			return false
		}
		if left.ModelID == defaultModel && right.ModelID != defaultModel {
			return true
		}
		if right.ModelID == defaultModel && left.ModelID != defaultModel {
			return false
		}
		leftLatency, rightLatency := routeRankLatency(left.ProviderID), routeRankLatency(right.ProviderID)
		leftCost, rightCost := routeRankCost(left.ProviderID), routeRankCost(right.ProviderID)
		if preferLatency && leftLatency != rightLatency {
			return leftLatency < rightLatency
		}
		if preferLowCost && leftCost != rightCost {
			return leftCost < rightCost
		}
		if left.ProviderID != right.ProviderID {
			return left.ProviderID < right.ProviderID
		}
		return left.ModelID < right.ModelID
	})
	return ordered
}
