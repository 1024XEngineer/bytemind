package provider

import (
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/config"
)

type ModelRegistry struct {
	models []ModelInfo
	index  map[string]ModelInfo
}

func NewModelRegistry(runtimeCfg config.ProviderRuntimeConfig, discovered []ModelInfo) ModelRegistry {
	merged := make([]ModelInfo, 0, len(discovered)+len(runtimeCfg.Providers))
	seen := make(map[string]struct{})
	for _, model := range discovered {
		normalized := normalizeModelInfo(model)
		key := modelRegistryKey(normalized.ProviderID, normalized.ModelID)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, normalized)
	}
	for providerID, providerCfg := range runtimeCfg.Providers {
		modelID := strings.TrimSpace(providerCfg.Model)
		if modelID == "" {
			continue
		}
		info := ModelInfo{
			ProviderID: ProviderID(strings.ToLower(strings.TrimSpace(providerID))),
			ModelID:    ModelID(modelID),
			Metadata: map[string]string{
				"family":          strings.TrimSpace(providerCfg.Family),
				"context_window":  "",
				"max_output_tokens": "",
				"supports_tools":  "true",
				"source":          "config",
			},
		}
		info = normalizeModelInfo(info)
		key := modelRegistryKey(info.ProviderID, info.ModelID)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, info)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].ProviderID == merged[j].ProviderID {
			return merged[i].ModelID < merged[j].ModelID
		}
		return merged[i].ProviderID < merged[j].ProviderID
	})
	index := make(map[string]ModelInfo, len(merged))
	for _, model := range merged {
		index[modelRegistryKey(model.ProviderID, model.ModelID)] = model
	}
	return ModelRegistry{models: merged, index: index}
}

func (r ModelRegistry) Models() []ModelInfo {
	return append([]ModelInfo(nil), r.models...)
}

func (r ModelRegistry) Lookup(providerID ProviderID, modelID ModelID) (ModelInfo, bool) {
	info, ok := r.index[modelRegistryKey(providerID, modelID)]
	return info, ok
}

func (r ModelRegistry) ContextWindow(providerID ProviderID, modelID ModelID) int {
	info, ok := r.Lookup(providerID, modelID)
	if !ok {
		return 0
	}
	return info.ModelMetadata().ContextWindow
}

func normalizeModelInfo(model ModelInfo) ModelInfo {
	model.ProviderID = ProviderID(strings.ToLower(strings.TrimSpace(string(model.ProviderID))))
	model.ModelID = ModelID(strings.TrimSpace(string(model.ModelID)))
	model.DisplayAlias = strings.TrimSpace(model.DisplayAlias)
	if model.Metadata == nil {
		model.Metadata = map[string]string{}
	}
	if strings.TrimSpace(model.Metadata["source"]) == "" {
		model.Metadata["source"] = "provider"
	}
	if strings.TrimSpace(model.Metadata["supports_tools"]) == "" {
		model.Metadata["supports_tools"] = "true"
	}
	return model
}

func modelRegistryKey(providerID ProviderID, modelID ModelID) string {
	provider := strings.ToLower(strings.TrimSpace(string(providerID)))
	model := strings.TrimSpace(string(modelID))
	if provider == "" || model == "" {
		return ""
	}
	return provider + "/" + model
}
