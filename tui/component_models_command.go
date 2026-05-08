package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/provider"
)

func (m *model) runModelsCommand(input string, fields []string) error {
	if m == nil || m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	return m.openModelPicker()
}

func formatModelsStatus(cfg config.Config, models []provider.ModelInfo, warnings []provider.Warning) string {
	lines := []string{
		fmt.Sprintf("active: %s", activeModelLabel(cfg)),
		fmt.Sprintf("default provider: %s", defaultProviderLabel(cfg)),
		"switch: /model",
	}
	if len(models) == 0 {
		lines = append(lines, "", "No models discovered.")
	} else {
		lines = append(lines, "", "providers:")
		providerGroups := make(map[string][]provider.ModelInfo)
		providerOrder := make([]string, 0)
		for _, model := range models {
			key := strings.TrimSpace(string(model.ProviderID))
			if _, ok := providerGroups[key]; !ok {
				providerOrder = append(providerOrder, key)
			}
			providerGroups[key] = append(providerGroups[key], model)
		}
		sort.Strings(providerOrder)
		activeProvider, activeModel := activeProviderAndModel(cfg)
		for _, providerID := range providerOrder {
			entries := providerGroups[providerID]
			providerLine := fmt.Sprintf("- %s", providerID)
			if providerID == activeProvider {
				providerLine = "* " + providerID
			}
			lines = append(lines, providerLine)
			sort.Slice(entries, func(i, j int) bool { return entries[i].ModelID < entries[j].ModelID })
			for _, entry := range entries {
				metadata := entry.ModelMetadata()
				labels := make([]string, 0, 3)
				if providerID == activeProvider && string(entry.ModelID) == activeModel {
					labels = append(labels, "active")
				}
				if providerID == config.SelectedProviderID(cfg.ProviderRuntime) && string(entry.ModelID) == config.SelectedModelID(cfg.ProviderRuntime) {
					labels = append(labels, "default")
				}
				if metadata.Family != "" {
					labels = append(labels, "family="+metadata.Family)
				}
				meta := ""
				if len(labels) > 0 {
					meta = "  [" + strings.Join(labels, ", ") + "]"
				}
				lines = append(lines, fmt.Sprintf("  - %s%s", entry.ModelID, meta))
			}
		}
	}
	if len(warnings) > 0 {
		lines = append(lines, "", "warnings:")
		for _, warning := range warnings {
			lines = append(lines, fmt.Sprintf("- %s: %s", warning.ProviderID, warning.Reason))
		}
	}
	return strings.Join(lines, "\n")
}

func activeModelLabel(cfg config.Config) string {
	providerID, modelID := activeProviderAndModel(cfg)
	if providerID == "" && modelID == "" {
		return "unknown"
	}
	if providerID == "" {
		return modelID
	}
	if modelID == "" {
		return providerID
	}
	return providerID + "/" + modelID
}

func activeProviderAndModel(cfg config.Config) (string, string) {
	providerID := config.SelectedProviderID(cfg.ProviderRuntime)
	modelID := config.SelectedModelID(cfg.ProviderRuntime)
	if modelID == "" {
		modelID = strings.TrimSpace(cfg.Provider.Model)
	}
	return providerID, modelID
}

func defaultProviderLabel(cfg config.Config) string {
	providerID := config.SelectedProviderID(cfg.ProviderRuntime)
	if providerID == "" {
		return "unknown"
	}
	return providerID
}
