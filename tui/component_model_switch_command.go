package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/provider"
)

const (
	modelCommandUsage     = "Usage: /model add|delete"
	modelPickerModeSwitch = "switch"
	modelPickerModeDelete = "delete"
)

type runnerRuntimeUpdater interface {
	UpdateProviderRuntime(config.ProviderRuntimeConfig, config.ProviderConfig, llm.Client)
}

func (m *model) runModelCommand(input string, fields []string) error {
	if m == nil {
		return fmt.Errorf("model command is unavailable")
	}
	if len(fields) != 2 {
		return fmt.Errorf(modelCommandUsage)
	}
	switch strings.ToLower(strings.TrimSpace(fields[1])) {
	case "add":
		return m.openAddModelGuide()
	case "delete":
		return m.openModelDeletePicker()
	default:
		return fmt.Errorf(modelCommandUsage)
	}
}

func (m *model) openAddModelGuide() error {
	configPath, err := config.ResolveWritableConfigPathForWorkspace(m.workspace, m.startupGuide.ConfigPath)
	if err != nil {
		return err
	}

	m.closeModelPicker()
	m.skillsOpen = false
	m.commandOpen = false
	m.sessionsOpen = false
	m.helpOpen = false
	m.closeMentionPalette()

	m.startupGuide.Active = true
	m.startupGuide.Title = "Add model"
	m.startupGuide.Status = "Bytemind will guide you through provider, base_url, model, and API key."
	m.startupGuide.ConfigPath = configPath
	m.startupGuide.CurrentField = startupFieldType
	m.initializeStartupGuide()
	m.input.Reset()
	m.clearPasteTransaction()
	m.clearVirtualPasteParts()
	m.statusNote = "Opened add model guide."
	return nil
}

func (m *model) openModelPicker() error {
	return m.openModelPickerWithMode(modelPickerModeSwitch)
}

func (m *model) openModelDeletePicker() error {
	return m.openModelPickerWithMode(modelPickerModeDelete)
}

func (m *model) openModelPickerWithMode(mode string) error {
	if m == nil || m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	mode = normalizeModelPickerMode(mode)

	var refreshErr error
	targets := m.sortedConfiguredModelCommandTargets()
	if mode == modelPickerModeSwitch {
		refreshErr = m.refreshDiscoveredModels()
		targets = m.sortedModelCommandTargets()
	}
	if len(targets) == 0 {
		if refreshErr != nil {
			return refreshErr
		}
		if mode == modelPickerModeDelete {
			m.statusNote = "No configured models are available to delete."
		} else {
			m.statusNote = "No switchable models available. Use /model add to configure one."
		}
		return nil
	}

	m.modelsOpen = true
	m.modelPickerMode = mode
	m.skillsOpen = false
	m.commandOpen = false
	m.sessionsOpen = false
	m.closeMentionPalette()
	m.commandCursor = 0

	activeProvider, activeModel := activeProviderAndModel(m.cfg)
	for i, target := range targets {
		if strings.EqualFold(strings.TrimSpace(string(target.ProviderID)), activeProvider) &&
			strings.TrimSpace(string(target.ModelID)) == activeModel {
			m.commandCursor = i
			break
		}
	}

	if refreshErr != nil {
		m.statusNote = "Opened models list from configured targets."
		return nil
	}
	if mode == modelPickerModeDelete {
		m.statusNote = "Opened model delete picker."
	} else {
		m.statusNote = "Opened models list."
	}
	return nil
}

func (m *model) switchModelCommandTarget(input, target string) error {
	if m == nil || m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	providerID, modelID, err := parseModelCommandTarget(target)
	if err != nil {
		return err
	}

	activeProvider, activeModel := activeProviderAndModel(m.cfg)
	if strings.EqualFold(strings.TrimSpace(activeProvider), providerID) && strings.TrimSpace(activeModel) == modelID {
		response := "Model already active: " + providerID + "/" + modelID + "."
		status := "Model already active."
		m.appendCommandExchange(input, response)
		m.statusNote = status
		return nil
	}

	targets := m.modelCommandTargets()
	if !hasModelCommandTarget(targets, providerID, modelID) {
		if err := m.refreshDiscoveredModels(); err != nil {
			return err
		}
		targets = m.modelCommandTargets()
		if !hasModelCommandTarget(targets, providerID, modelID) {
			return fmt.Errorf("unknown model target %q; open /models to refresh the list first", providerID+"/"+modelID)
		}
	}

	runtimeCfg := m.cfg.ProviderRuntime
	if len(runtimeCfg.Providers) == 0 {
		runtimeCfg = config.LegacyProviderRuntimeConfig(m.cfg.Provider)
	}
	runtimeCfg, providerCfg, err := config.SelectProviderRuntimeModel(runtimeCfg, providerID, modelID)
	if err != nil {
		return err
	}
	client, err := provider.NewClientFromRuntime(runtimeCfg, nil)
	if err != nil {
		return err
	}
	runtimeUpdater, ok := any(m.runner).(runnerRuntimeUpdater)
	if !ok {
		return fmt.Errorf("runtime model switching is unavailable in this build")
	}
	runtimeUpdater.UpdateProviderRuntime(runtimeCfg, providerCfg, client)

	m.cfg.ProviderRuntime = runtimeCfg
	m.cfg.Provider = providerCfg
	m.refreshTokenBudget()
	m.syncTokenUsageComponent()

	response := "Switched model to " + providerID + "/" + modelID + "."
	status := "Model switched."
	if path, saveErr := m.persistModelCommandSelection(runtimeCfg); saveErr != nil {
		status = "Model switched, but config save failed."
		response += "\nConfig save failed: " + compact(saveErr.Error(), 120)
	} else if strings.TrimSpace(path) != "" {
		m.startupGuide.ConfigPath = path
		response += "\nSaved to " + compact(path, 72)
		status = "Model switched and saved."
	}

	m.appendCommandExchange(input, response)
	m.statusNote = status
	return nil
}

func (m *model) activateSelectedModelTarget() error {
	targets := m.sortedModelCommandTargets()
	if len(targets) == 0 {
		return nil
	}
	selected := targets[clamp(m.commandCursor, 0, len(targets)-1)]
	target := modelTargetLabel(selected)
	return m.switchModelCommandTarget("/models "+target, target)
}

func (m *model) deleteSelectedModelTarget() error {
	targets := m.sortedConfiguredModelCommandTargets()
	if len(targets) == 0 {
		return nil
	}
	selected := targets[clamp(m.commandCursor, 0, len(targets)-1)]
	target := modelTargetLabel(selected)
	return m.deleteModelCommandTarget("/model delete", target)
}

func (m *model) refreshDiscoveredModels() error {
	if m == nil || m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	models, warnings, err := m.runner.ListModels(context.Background())
	if err != nil {
		return err
	}
	m.setDiscoveredModels(models)
	m.modelWarnings = append([]provider.Warning(nil), warnings...)
	return nil
}

func (m model) modelCommandTargets() []provider.ModelInfo {
	runtimeCfg := m.cfg.ProviderRuntime
	if len(runtimeCfg.Providers) == 0 {
		runtimeCfg = config.LegacyProviderRuntimeConfig(m.cfg.Provider)
	}
	return provider.NewModelRegistry(runtimeCfg, m.discoveredModels).Models()
}

func (m model) configuredModelCommandTargets() []provider.ModelInfo {
	runtimeCfg := m.cfg.ProviderRuntime
	if len(runtimeCfg.Providers) == 0 {
		runtimeCfg = config.LegacyProviderRuntimeConfig(m.cfg.Provider)
	}
	return provider.NewModelRegistry(runtimeCfg, nil).Models()
}

func (m model) sortedModelCommandTargets() []provider.ModelInfo {
	targets := append([]provider.ModelInfo(nil), m.modelCommandTargets()...)
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].ProviderID == targets[j].ProviderID {
			return targets[i].ModelID < targets[j].ModelID
		}
		return targets[i].ProviderID < targets[j].ProviderID
	})
	return targets
}

func (m model) sortedConfiguredModelCommandTargets() []provider.ModelInfo {
	targets := append([]provider.ModelInfo(nil), m.configuredModelCommandTargets()...)
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].ProviderID == targets[j].ProviderID {
			return targets[i].ModelID < targets[j].ModelID
		}
		return targets[i].ProviderID < targets[j].ProviderID
	})
	return targets
}

func (m model) persistModelCommandSelection(runtimeCfg config.ProviderRuntimeConfig) (string, error) {
	configPath, err := config.ResolveWritableConfigPathForWorkspace(m.workspace, m.startupGuide.ConfigPath)
	if err != nil {
		return "", err
	}
	return config.UpsertProviderRuntimeSelection(configPath, runtimeCfg)
}

func parseModelCommandTarget(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf(modelCommandUsage)
	}
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("model target must be provider/model")
	}
	providerID := strings.ToLower(strings.TrimSpace(parts[0]))
	modelID := strings.TrimSpace(parts[1])
	if providerID == "" || modelID == "" {
		return "", "", fmt.Errorf("model target must be provider/model")
	}
	return providerID, modelID, nil
}

func hasModelCommandTarget(targets []provider.ModelInfo, providerID, modelID string) bool {
	for _, target := range targets {
		if strings.EqualFold(strings.TrimSpace(string(target.ProviderID)), providerID) && strings.TrimSpace(string(target.ModelID)) == modelID {
			return true
		}
	}
	return false
}

func modelTargetLabel(target provider.ModelInfo) string {
	return strings.TrimSpace(string(target.ProviderID)) + "/" + strings.TrimSpace(string(target.ModelID))
}

func normalizeModelPickerMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), modelPickerModeDelete) {
		return modelPickerModeDelete
	}
	return modelPickerModeSwitch
}

func (m model) modelPickerTargets() []provider.ModelInfo {
	if normalizeModelPickerMode(m.modelPickerMode) == modelPickerModeDelete {
		return m.sortedConfiguredModelCommandTargets()
	}
	return m.sortedModelCommandTargets()
}

func (m *model) deleteModelCommandTarget(input, target string) error {
	if m == nil || m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	providerID, modelID, err := parseModelCommandTarget(target)
	if err != nil {
		return err
	}

	targets := m.configuredModelCommandTargets()
	if !hasModelCommandTarget(targets, providerID, modelID) {
		return fmt.Errorf("unknown configured model target %q", providerID+"/"+modelID)
	}
	if len(targets) <= 1 {
		return fmt.Errorf("cannot delete the last configured model; use /model add to configure another target first")
	}

	runtimeCfg := m.cfg.ProviderRuntime
	if len(runtimeCfg.Providers) == 0 {
		runtimeCfg = config.LegacyProviderRuntimeConfig(m.cfg.Provider)
	}
	runtimeCfg, providerCfg, err := config.DeleteProviderRuntimeProvider(runtimeCfg, providerID)
	if err != nil {
		return err
	}
	client, err := provider.NewClientFromRuntime(runtimeCfg, nil)
	if err != nil {
		return err
	}
	runtimeUpdater, ok := any(m.runner).(runnerRuntimeUpdater)
	if !ok {
		return fmt.Errorf("runtime model deletion is unavailable in this build")
	}
	runtimeUpdater.UpdateProviderRuntime(runtimeCfg, providerCfg, client)

	m.cfg.ProviderRuntime = runtimeCfg
	m.cfg.Provider = providerCfg
	m.removeDiscoveredModelsForProvider(providerID)
	m.refreshTokenBudget()
	m.syncTokenUsageComponent()

	response := "Deleted model " + providerID + "/" + modelID + "."
	response += "\nActive model is now " + activeModelLabel(m.cfg) + "."
	status := "Model deleted."
	if path, saveErr := m.persistModelCommandSelection(runtimeCfg); saveErr != nil {
		status = "Model deleted, but config save failed."
		response += "\nConfig save failed: " + compact(saveErr.Error(), 120)
	} else if strings.TrimSpace(path) != "" {
		m.startupGuide.ConfigPath = path
		response += "\nSaved to " + compact(path, 72)
		status = "Model deleted and saved."
	}

	m.appendCommandExchange(input, response)
	m.statusNote = status
	return nil
}

func (m *model) removeDiscoveredModelsForProvider(providerID string) {
	if m == nil || len(m.discoveredModels) == 0 {
		return
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if providerID == "" {
		return
	}
	filtered := m.discoveredModels[:0]
	for _, info := range m.discoveredModels {
		if strings.EqualFold(strings.TrimSpace(string(info.ProviderID)), providerID) {
			continue
		}
		filtered = append(filtered, info)
	}
	m.discoveredModels = append([]provider.ModelInfo(nil), filtered...)
	if len(m.modelWarnings) > 0 {
		warnings := m.modelWarnings[:0]
		for _, warning := range m.modelWarnings {
			if strings.EqualFold(strings.TrimSpace(string(warning.ProviderID)), providerID) {
				continue
			}
			warnings = append(warnings, warning)
		}
		m.modelWarnings = append([]provider.Warning(nil), warnings...)
	}
}

func (m *model) closeModelPicker() {
	if m == nil {
		return
	}
	m.modelsOpen = false
	m.modelPickerMode = ""
	m.commandCursor = 0
}

func formatModelSelectionStatus(cfg config.Config, targets []provider.ModelInfo) string {
	lines := []string{
		"current: " + activeModelLabel(cfg),
		"add: /model add",
		"delete: /model delete",
		"models: /models",
		"status: /models",
	}
	activeProvider, activeModel := activeProviderAndModel(cfg)
	if activeProvider != "" && activeModel != "" && hasModelCommandTarget(targets, activeProvider, activeModel) {
		lines = append(lines, "available: "+fmt.Sprintf("%d target(s)", len(targets)))
	}
	return strings.Join(lines, "\n")
}
