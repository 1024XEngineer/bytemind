package provider

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	defaultHealthFailThreshold           = 3
	defaultHealthRecoverSuccessThreshold = 2
	defaultHealthRecoverProbeInterval    = 20 * time.Second
	defaultHealthWindowSize              = 5
)

type healthChecker struct {
	mu        sync.RWMutex
	cfg       HealthConfig
	clock     func() time.Time
	checker   func(context.Context, ProviderID) error
	providers map[ProviderID]*healthState
}

type healthState struct {
	providerID    ProviderID
	status        HealthStatus
	outcomes      []bool
	nextProbeAt   time.Time
	lastCheckAt   time.Time
	lastFailureAt time.Time
	lastSuccessAt time.Time
	lastError     string
}

func NewHealthChecker(cfg HealthConfig, checker func(context.Context, ProviderID) error) HealthChecker {
	return &healthChecker{
		cfg:       normalizeHealthConfig(cfg),
		clock:     time.Now,
		checker:   checker,
		providers: make(map[ProviderID]*healthState),
	}
}

func normalizeHealthConfig(cfg HealthConfig) HealthConfig {
	if cfg.FailThreshold <= 0 {
		cfg.FailThreshold = defaultHealthFailThreshold
	}
	if cfg.RecoverSuccessThreshold <= 0 {
		cfg.RecoverSuccessThreshold = defaultHealthRecoverSuccessThreshold
	}
	if cfg.RecoverProbeSec <= 0 {
		cfg.RecoverProbeSec = int(defaultHealthRecoverProbeInterval / time.Second)
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = defaultHealthWindowSize
	}
	return cfg
}

func (h *healthChecker) Check(ctx context.Context, id ProviderID) error {
	if h == nil {
		return nil
	}
	id = normalizeRouteProviderID(id)
	if id == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now := h.now()
	h.mu.Lock()
	state := h.ensureStateLocked(id)
	if state.status == "" {
		state.status = HealthStatusHealthy
	}
	if state.status == HealthStatusUnavailable && now.Before(state.nextProbeAt) {
		snapshot := h.snapshotLocked(state)
		h.mu.Unlock()
		return unavailableHealthError(snapshot)
	}
	if state.status == HealthStatusUnavailable {
		state.status = HealthStatusHalfOpen
	}
	state.lastCheckAt = now
	h.mu.Unlock()
	if h.checker == nil {
		return nil
	}
	err := h.checker(ctx, id)
	if errors.Is(err, context.Canceled) {
		return err
	}
	if err != nil {
		h.RecordFailure(ctx, id, err)
		return err
	}
	h.RecordSuccess(ctx, id)
	return nil
}

func (h *healthChecker) Status(_ context.Context, id ProviderID) HealthSnapshot {
	if h == nil {
		return HealthSnapshot{ProviderID: normalizeRouteProviderID(id), Status: HealthStatusHealthy}
	}
	id = normalizeRouteProviderID(id)
	h.mu.RLock()
	defer h.mu.RUnlock()
	state, ok := h.providers[id]
	if !ok {
		return HealthSnapshot{ProviderID: id, Status: HealthStatusHealthy, WindowSize: h.cfg.WindowSize}
	}
	return h.snapshotLocked(state)
}

func (h *healthChecker) RecordSuccess(_ context.Context, id ProviderID) {
	if h == nil {
		return
	}
	id = normalizeRouteProviderID(id)
	if id == "" {
		return
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.ensureStateLocked(id)
	state.lastSuccessAt = now
	state.lastError = ""
	state.outcomes = appendOutcome(state.outcomes, true, h.cfg.WindowSize)
	successes := trailingSuccesses(state.outcomes)
	failures := trailingFailures(state.outcomes)
	switch state.status {
	case HealthStatusHalfOpen:
		if successes >= h.cfg.RecoverSuccessThreshold {
			state.status = HealthStatusHealthy
			state.nextProbeAt = time.Time{}
		} else {
			state.status = HealthStatusHalfOpen
		}
	case HealthStatusUnavailable:
		if successes >= h.cfg.RecoverSuccessThreshold {
			state.status = HealthStatusHealthy
			state.nextProbeAt = time.Time{}
		}
	default:
		if failures > 0 {
			state.status = HealthStatusDegraded
		} else {
			state.status = HealthStatusHealthy
		}
	}
}

func (h *healthChecker) RecordFailure(_ context.Context, id ProviderID, err error) {
	if h == nil {
		return
	}
	id = normalizeRouteProviderID(id)
	if id == "" {
		return
	}
	if !countsTowardAvailability(err) {
		return
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.ensureStateLocked(id)
	state.lastFailureAt = now
	state.lastError = strings.TrimSpace(errorMessage(err))
	state.outcomes = appendOutcome(state.outcomes, false, h.cfg.WindowSize)
	failures := trailingFailures(state.outcomes)
	switch state.status {
	case HealthStatusHalfOpen:
		state.status = HealthStatusUnavailable
		state.nextProbeAt = now.Add(time.Duration(h.cfg.RecoverProbeSec) * time.Second)
	case HealthStatusUnavailable:
		state.nextProbeAt = now.Add(time.Duration(h.cfg.RecoverProbeSec) * time.Second)
	default:
		if failures >= h.cfg.FailThreshold {
			state.status = HealthStatusUnavailable
			state.nextProbeAt = now.Add(time.Duration(h.cfg.RecoverProbeSec) * time.Second)
		} else {
			state.status = HealthStatusDegraded
		}
	}
}

func (h *healthChecker) now() time.Time {
	if h.clock != nil {
		return h.clock()
	}
	return time.Now()
}

func (h *healthChecker) ensureStateLocked(id ProviderID) *healthState {
	state, ok := h.providers[id]
	if ok {
		return state
	}
	state = &healthState{providerID: id, status: HealthStatusHealthy}
	h.providers[id] = state
	return state
}

func (h *healthChecker) snapshotLocked(state *healthState) HealthSnapshot {
	return HealthSnapshot{
		ProviderID:       state.providerID,
		Status:           state.status,
		FailureCount:     trailingFailures(state.outcomes),
		SuccessCount:     trailingSuccesses(state.outcomes),
		WindowSize:       h.cfg.WindowSize,
		NextProbeAt:      state.nextProbeAt,
		LastCheckAt:      state.lastCheckAt,
		LastFailureAt:    state.lastFailureAt,
		LastSuccessAt:    state.lastSuccessAt,
		LastErrorMessage: state.lastError,
	}
}

func appendOutcome(outcomes []bool, success bool, windowSize int) []bool {
	outcomes = append(outcomes, success)
	if len(outcomes) > windowSize {
		outcomes = append([]bool(nil), outcomes[len(outcomes)-windowSize:]...)
	}
	return outcomes
}

func trailingFailures(outcomes []bool) int {
	count := 0
	for i := len(outcomes) - 1; i >= 0; i-- {
		if outcomes[i] {
			break
		}
		count++
	}
	return count
}

func trailingSuccesses(outcomes []bool) int {
	count := 0
	for i := len(outcomes) - 1; i >= 0; i-- {
		if !outcomes[i] {
			break
		}
		count++
	}
	return count
}

func countsTowardAvailability(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var providerErr *Error
	if errors.As(err, &providerErr) {
		switch providerErr.Code {
		case ErrCodeUnauthorized, ErrCodeBadRequest, ErrCodeProviderNotFound, ErrCodeDuplicateProvider:
			return false
		}
	}
	return true
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	var providerErr *Error
	if errors.As(err, &providerErr) {
		if strings.TrimSpace(providerErr.Detail) != "" {
			return providerErr.Detail
		}
	}
	return err.Error()
}

func unavailableHealthError(snapshot HealthSnapshot) error {
	return &Error{Code: ErrCodeUnavailable, Provider: snapshot.ProviderID, Message: "provider unavailable", Retryable: true, Detail: snapshot.LastErrorMessage, Err: errorsUnavailable}
}
