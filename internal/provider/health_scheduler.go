package provider

import (
	"context"
	"time"
)

type HealthScheduler struct {
	health   HealthChecker
	ids      func(context.Context) ([]ProviderID, error)
	interval time.Duration
}

func NewHealthScheduler(health HealthChecker, ids func(context.Context) ([]ProviderID, error), cfg HealthConfig) *HealthScheduler {
	interval := time.Duration(normalizeHealthConfig(cfg).CheckIntervalSec) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &HealthScheduler{health: health, ids: ids, interval: interval}
}

func (s *HealthScheduler) Tick(ctx context.Context) error {
	if s == nil || s.health == nil || s.ids == nil {
		return nil
	}
	ids, err := s.ids(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.health.Check(ctx, id); err != nil && ctx.Err() != nil {
			return err
		}
	}
	return nil
}
