package provider

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/1024XEngineer/bytemind/internal/config"
)

type providerRegistry struct {
	mu      sync.RWMutex
	clients map[ProviderID]Client
}

func normalizeRegistryProviderID(id string) ProviderID {
	return ProviderID(strings.ToLower(strings.TrimSpace(id)))
}

func NewRegistry(cfg config.ProviderRuntimeConfig) (Registry, error) {
	reg := &providerRegistry{clients: make(map[ProviderID]Client)}
	normalizedProviders := make(map[ProviderID]config.ProviderConfig, len(cfg.Providers))
	ids := make([]string, 0, len(cfg.Providers))
	for id, providerCfg := range cfg.Providers {
		normalizedID := normalizeRegistryProviderID(id)
		if normalizedID == "" {
			return nil, &Error{Code: ErrCodeProviderNotFound, Provider: normalizedID, Message: string(ErrCodeProviderNotFound), Retryable: false, Err: ErrProviderNotFound}
		}
		if _, exists := normalizedProviders[normalizedID]; exists {
			return nil, &Error{Code: ErrCodeDuplicateProvider, Provider: normalizedID, Message: string(ErrCodeDuplicateProvider), Retryable: false, Err: ErrDuplicateProvider}
		}
		normalizedProviders[normalizedID] = providerCfg
		ids = append(ids, string(normalizedID))
	}
	if cfg.DefaultProvider != "" {
		defaultProvider := normalizeRegistryProviderID(cfg.DefaultProvider)
		if _, exists := normalizedProviders[defaultProvider]; !exists {
			return nil, &Error{Code: ErrCodeProviderNotFound, Provider: defaultProvider, Message: string(ErrCodeProviderNotFound), Retryable: false, Err: ErrProviderNotFound}
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		providerCfg := normalizedProviders[ProviderID(id)]
		client, err := NewDomainClientWithID(ProviderID(id), providerCfg)
		if err != nil {
			return nil, err
		}
		if err := reg.Register(context.Background(), client); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func NewRegistryFromProviderConfig(cfg config.ProviderConfig) (Registry, error) {
	return NewRegistry(config.LegacyProviderRuntimeConfig(cfg))
}

func (r *providerRegistry) Register(_ context.Context, client Client) error {
	if client == nil {
		return nil
	}
	id := normalizeRegistryProviderID(string(client.ProviderID()))
	if id == "" {
		return &Error{Code: ErrCodeProviderNotFound, Provider: id, Message: string(ErrCodeProviderNotFound), Retryable: false, Err: ErrProviderNotFound}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.clients[id]; exists {
		return &Error{Code: ErrCodeDuplicateProvider, Provider: id, Message: string(ErrCodeDuplicateProvider), Retryable: false, Err: ErrDuplicateProvider}
	}
	r.clients[id] = client
	return nil
}

func (r *providerRegistry) Get(_ context.Context, id ProviderID) (Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.clients[normalizeRegistryProviderID(string(id))]
	return client, ok
}

func (r *providerRegistry) List(_ context.Context) ([]ProviderID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]ProviderID, 0, len(r.clients))
	for id := range r.clients {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}
