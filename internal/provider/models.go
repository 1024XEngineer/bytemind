package provider

import (
	"context"
	"sort"
	"sync"
)

const listModelsWarningReason = "provider_list_models_failed"
const listModelsConcurrency = 4

func ListModels(ctx context.Context, reg Registry) ([]ModelInfo, []Warning, error) {
	ids, err := reg.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	models := make([]ModelInfo, 0)
	warnings := make([]Warning, 0)
	seen := make(map[string]struct{})
	var mu sync.Mutex
	sem := make(chan struct{}, listModelsConcurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			client, ok := reg.Get(ctx, id)
			if ctx.Err() != nil {
				return
			}
			if !ok {
				mu.Lock()
				warnings = append(warnings, Warning{ProviderID: id, Reason: string(ErrCodeProviderNotFound)})
				mu.Unlock()
				return
			}
			providerModels, err := client.ListModels(ctx)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				mu.Lock()
				warnings = append(warnings, Warning{ProviderID: id, Reason: listModelsWarningReason})
				mu.Unlock()
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, model := range providerModels {
				providerID := id
				key := string(providerID) + "\x00" + string(model.ModelID)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				model.ProviderID = providerID
				models = append(models, model)
			}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].ProviderID == models[j].ProviderID {
			return models[i].ModelID < models[j].ModelID
		}
		return models[i].ProviderID < models[j].ProviderID
	})
	sort.Slice(warnings, func(i, j int) bool { return warnings[i].ProviderID < warnings[j].ProviderID })
	return models, warnings, nil
}
